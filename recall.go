package cogito

import (
	"context"
	"fmt"
	"time"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
	"github.com/zoobzio/zyn"
)

// Recall is a memory primitive that loads another Thought and summarizes its notes
// into the current Thought. This enables cross-thought knowledge transfer without.
// copying all notes - just a distilled summary.
type Recall struct {
	key       string
	thoughtID string
	prompt    string
	provider  Provider
}

// NewRecall creates a new recall primitive.
//
// The primitive:
//  1. Loads the target Thought by ID
//  2. Renders its notes to text
//  3. Summarizes via LLM transform synapse
//  4. Stores the summary as a note on the current Thought
//
// Output Notes:
//   - {key}: LLM-generated summary of the target Thought's notes
//
// Example:
//
//	// Reference a previous conversation
//	recall := cogito.NewRecall("prior_context", previousThoughtID).
//	    WithPrompt("Summarize the key decisions and their rationale")
//	result, _ := recall.Process(ctx, thought)
func NewRecall(key, thoughtID string) *Recall {
	return &Recall{
		key:       key,
		thoughtID: thoughtID,
		prompt:    "Summarize the key information, decisions, and outcomes",
	}
}

// Process implements pipz.Chainable[*Thought].
func (r *Recall) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	// Resolve provider
	provider, err := ResolveProvider(ctx, r.provider)
	if err != nil {
		return t, fmt.Errorf("recall: %w", err)
	}

	// Emit step started
	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(r.key),
		FieldStepType.Field("recall"),
	)

	// Load target thought
	targetThought, err := t.memory.GetThought(ctx, r.thoughtID)
	if err != nil {
		r.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("recall: failed to load thought %s: %w", r.thoughtID, err)
	}

	// Render target notes to text
	targetNotes := targetThought.AllNotes()
	if len(targetNotes) == 0 {
		r.emitFailed(ctx, t, start, fmt.Errorf("target thought has no notes"))
		return t, fmt.Errorf("recall: target thought %s has no notes", r.thoughtID)
	}
	noteContext := RenderNotesToContext(targetNotes)

	// Create transform synapse for summarization
	transformSynapse, err := zyn.Transform(r.prompt, provider)
	if err != nil {
		r.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("recall: failed to create transform synapse: %w", err)
	}

	// Summarize via LLM
	summary, err := transformSynapse.FireWithInput(ctx, t.Session, zyn.TransformInput{
		Text:  noteContext,
		Style: r.prompt,
	})
	if err != nil {
		r.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("recall: transform synapse execution failed: %w", err)
	}

	// Store summary as note on current thought
	if err := t.SetNote(ctx, r.key, summary, "recall", map[string]string{
		"source_thought_id": r.thoughtID,
		"source_note_count": fmt.Sprintf("%d", len(targetNotes)),
	}); err != nil {
		r.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("recall: failed to persist summary: %w", err)
	}

	// Emit step completed
	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(r.key),
		FieldStepType.Field("recall"),
		FieldStepDuration.Field(time.Since(start)),
		FieldContentSize.Field(len(summary)),
	)

	return t, nil
}

// emitFailed emits a step failed event.
func (r *Recall) emitFailed(ctx context.Context, t *Thought, start time.Time, err error) {
	capitan.Error(ctx, StepFailed,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(r.key),
		FieldStepType.Field("recall"),
		FieldStepDuration.Field(time.Since(start)),
		FieldError.Field(err),
	)
}

// Name implements pipz.Chainable[*Thought].
func (r *Recall) Name() pipz.Name {
	return pipz.Name(r.key)
}

// Close implements pipz.Chainable[*Thought].
func (r *Recall) Close() error {
	return nil
}

// Builder methods

// WithPrompt sets a custom summarization prompt.
func (r *Recall) WithPrompt(prompt string) *Recall {
	r.prompt = prompt
	return r
}

// WithProvider sets the provider for the LLM call.
func (r *Recall) WithProvider(p Provider) *Recall {
	r.provider = p
	return r
}
