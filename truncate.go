package cogito

import (
	"context"
	"fmt"
	"time"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
)

// Truncate is a session management primitive that implements pipz.Chainable[*Thought].
// It removes messages from the session without LLM involvement, using a sliding window approach.
type Truncate struct {
	identity  pipz.Identity
	key       string
	keepFirst int // Number of messages to keep from the start (e.g., system prompt)
	keepLast  int // Number of recent messages to keep
	threshold int // Minimum message count to trigger truncation (0 = always)
}

// NewTruncate creates a new session truncation primitive.
//
// The primitive removes messages from the middle of the session, preserving
// the first N messages (typically system prompts) and the last M messages.
// (recent context). No LLM call is made.
//
// Example:
//
//	truncate := cogito.NewTruncate("session_truncate").
//	    WithKeepFirst(1).   // Preserve system prompt
//	    WithKeepLast(10).   // Keep last 10 messages
//	    WithThreshold(20)   // Only truncate if >= 20 messages
//	result, _ := truncate.Process(ctx, thought)
func NewTruncate(key string) *Truncate {
	return &Truncate{
		identity:  pipz.NewIdentity(key, "Session truncation primitive"),
		key:       key,
		keepFirst: 1,  // Default: keep system prompt
		keepLast:  10, // Default: keep last 10 messages
		threshold: 0,
	}
}

// Process implements pipz.Chainable[*Thought].
func (tr *Truncate) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	messageCount := t.Session.Len()

	// Check threshold
	if tr.threshold > 0 && messageCount < tr.threshold {
		// Below threshold, pass through unchanged
		return t, nil
	}

	// Check if truncation would remove anything
	if messageCount <= tr.keepFirst+tr.keepLast {
		// Nothing to truncate
		return t, nil
	}

	// Emit step started
	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(tr.key),
		FieldStepType.Field("truncate"),
		FieldNoteCount.Field(messageCount),
	)

	// Truncate session
	err := t.Session.Truncate(tr.keepFirst, tr.keepLast)
	if err != nil {
		tr.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("truncate: %w", err)
	}

	newCount := t.Session.Len()
	removed := messageCount - newCount

	// Emit step completed
	duration := time.Since(start)
	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(tr.key),
		FieldStepType.Field("truncate"),
		FieldStepDuration.Field(duration),
		FieldNoteCount.Field(removed),
	)

	return t, nil
}

// emitFailed emits a step failed event.
func (tr *Truncate) emitFailed(ctx context.Context, t *Thought, start time.Time, err error) {
	capitan.Error(ctx, StepFailed,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(tr.key),
		FieldStepType.Field("truncate"),
		FieldStepDuration.Field(time.Since(start)),
		FieldError.Field(err),
	)
}

// Identity implements pipz.Chainable[*Thought].
func (tr *Truncate) Identity() pipz.Identity {
	return tr.identity
}

// Schema implements pipz.Chainable[*Thought].
func (tr *Truncate) Schema() pipz.Node {
	return pipz.Node{Identity: tr.identity, Type: "truncate"}
}

// Close implements pipz.Chainable[*Thought].
func (tr *Truncate) Close() error {
	return nil
}

// Builder methods

// WithKeepFirst sets the number of messages to preserve from the start.
// Typically used to preserve system prompts.
func (tr *Truncate) WithKeepFirst(n int) *Truncate {
	tr.keepFirst = n
	return tr
}

// WithKeepLast sets the number of recent messages to preserve.
func (tr *Truncate) WithKeepLast(n int) *Truncate {
	tr.keepLast = n
	return tr
}

// WithThreshold sets the minimum message count to trigger truncation.
// If the session has fewer messages, the primitive passes through unchanged.
func (tr *Truncate) WithThreshold(n int) *Truncate {
	tr.threshold = n
	return tr
}
