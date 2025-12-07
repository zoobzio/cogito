package cogito

import (
	"context"
	"fmt"
	"time"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
	"github.com/zoobzio/zyn"
)

// Reflect is a memory primitive that summarizes the current Thought's notes into
// a single consolidated note. This enables self-compression for long reasoning chains,.
// allowing the agent to "step back" and consolidate its accumulated context.
type Reflect struct {
	key             string
	prompt          string
	unpublishedOnly bool
	provider        Provider
}

// NewReflect creates a new reflect primitive.
//
// The primitive:
//  1. Gathers notes from the current Thought
//  2. Renders them to text
//  3. Summarizes via LLM transform synapse
//  4. Stores the summary as a new note
//
// Output Notes:
//   - {key}: LLM-generated summary/reflection of the Thought's notes
//
// Example:
//
//	// Consolidate reasoning after many steps
//	reflect := cogito.NewReflect("consolidated_context").
//	    WithPrompt("Synthesize all findings into key insights and next steps")
//	result, _ := reflect.Process(ctx, thought)
func NewReflect(key string) *Reflect {
	return &Reflect{
		key:    key,
		prompt: "Synthesize the accumulated context into key insights, decisions made, and important findings",
	}
}

// Process implements pipz.Chainable[*Thought].
func (r *Reflect) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	// Resolve provider
	provider, err := ResolveProvider(ctx, r.provider)
	if err != nil {
		return t, fmt.Errorf("reflect: %w", err)
	}

	// Gather notes
	var notes []Note
	if r.unpublishedOnly {
		notes = t.GetUnpublishedNotes()
	} else {
		notes = t.AllNotes()
	}

	// Emit step started
	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(r.key),
		FieldStepType.Field("reflect"),
		FieldNoteCount.Field(len(notes)),
	)

	if len(notes) == 0 {
		r.emitFailed(ctx, t, start, fmt.Errorf("no notes to reflect on"))
		return t, fmt.Errorf("reflect: no notes to reflect on")
	}

	noteContext := RenderNotesToContext(notes)

	// Create transform synapse for reflection
	transformSynapse, err := zyn.Transform(r.prompt, provider)
	if err != nil {
		r.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("reflect: failed to create transform synapse: %w", err)
	}

	// Generate reflection via LLM
	reflection, err := transformSynapse.FireWithInput(ctx, t.Session, zyn.TransformInput{
		Text:  noteContext,
		Style: r.prompt,
	})
	if err != nil {
		r.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("reflect: transform synapse execution failed: %w", err)
	}

	// Store reflection as note
	if err := t.SetNote(ctx, r.key, reflection, "reflect", map[string]string{
		"source_note_count": fmt.Sprintf("%d", len(notes)),
		"unpublished_only":  fmt.Sprintf("%t", r.unpublishedOnly),
	}); err != nil {
		r.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("reflect: failed to persist reflection: %w", err)
	}

	// Emit step completed
	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(r.key),
		FieldStepType.Field("reflect"),
		FieldStepDuration.Field(time.Since(start)),
		FieldContentSize.Field(len(reflection)),
	)

	return t, nil
}

// emitFailed emits a step failed event.
func (r *Reflect) emitFailed(ctx context.Context, t *Thought, start time.Time, err error) {
	capitan.Error(ctx, StepFailed,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(r.key),
		FieldStepType.Field("reflect"),
		FieldStepDuration.Field(time.Since(start)),
		FieldError.Field(err),
	)
}

// Name implements pipz.Chainable[*Thought].
func (r *Reflect) Name() pipz.Name {
	return pipz.Name(r.key)
}

// Close implements pipz.Chainable[*Thought].
func (r *Reflect) Close() error {
	return nil
}

// Builder methods

// WithPrompt sets a custom reflection prompt.
func (r *Reflect) WithPrompt(prompt string) *Reflect {
	r.prompt = prompt
	return r
}

// WithUnpublishedOnly limits reflection to unpublished notes only.
// This is useful for reflecting on recent work without re-processing
// context that has already been sent to the LLM.
func (r *Reflect) WithUnpublishedOnly() *Reflect {
	r.unpublishedOnly = true
	return r
}

// WithProvider sets the provider for the LLM call.
func (r *Reflect) WithProvider(p Provider) *Reflect {
	r.provider = p
	return r
}
