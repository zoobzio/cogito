package cogito

import (
	"context"
	"time"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
)

// Reset is a session management primitive that implements pipz.Chainable[*Thought].
// It clears the session entirely, optionally injecting a starting message.
type Reset struct {
	key            string
	systemMessage  string // Optional system message for the new session
	preserveNoteAs string // Optional: copy a note's content as the system message
}

// NewReset creates a new session reset primitive.
//
// The primitive clears all messages from the session. Optionally, a system
// message can be injected into the fresh session. No LLM call is made.
//
// Example:
//
//	reset := cogito.NewReset("session_reset").
//	    WithSystemMessage("You are a helpful assistant.")
//	result, _ := reset.Process(ctx, thought)
//
//	// Or preserve context from a note:
//	reset := cogito.NewReset("session_reset").
//	    WithPreserveNote("session_compress")  // Use compression summary as new context
func NewReset(key string) *Reset {
	return &Reset{
		key: key,
	}
}

// Process implements pipz.Chainable[*Thought].
func (r *Reset) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	previousCount := t.Session.Len()

	// Emit step started
	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(r.key),
		FieldStepType.Field("reset"),
		FieldNoteCount.Field(previousCount),
	)

	// Clear session
	t.Session.Clear()

	// Determine system message
	systemMessage := r.systemMessage

	// If preserveNoteAs is set, use that note's content as system message
	if r.preserveNoteAs != "" {
		if content, err := t.GetContent(r.preserveNoteAs); err == nil {
			systemMessage = content
		}
		// If note doesn't exist, fall back to explicit systemMessage (may be empty)
	}

	// Inject system message if provided
	if systemMessage != "" {
		t.Session.Append("system", systemMessage)
	}

	// Emit step completed
	duration := time.Since(start)
	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(r.key),
		FieldStepType.Field("reset"),
		FieldStepDuration.Field(duration),
		FieldNoteCount.Field(previousCount),
	)

	return t, nil
}

// Name implements pipz.Chainable[*Thought].
func (r *Reset) Name() pipz.Name {
	return pipz.Name(r.key)
}

// Close implements pipz.Chainable[*Thought].
func (r *Reset) Close() error {
	return nil
}

// Builder methods

// WithSystemMessage sets a system message to inject after clearing.
func (r *Reset) WithSystemMessage(msg string) *Reset {
	r.systemMessage = msg
	return r
}

// WithPreserveNote sets a note key whose content will be used as the system message.
// This is useful for chaining with Compress—use the summary as the new context.
// If the note doesn't exist, falls back to WithSystemMessage value.
func (r *Reset) WithPreserveNote(noteKey string) *Reset {
	r.preserveNoteAs = noteKey
	return r
}
