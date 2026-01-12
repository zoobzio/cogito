package cogito

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
	"github.com/zoobzio/zyn"
)

// Forget is a memory primitive that creates a new Thought with filtered notes.
// Unlike Checkpoint which copies all notes, Forget selectively drops or keeps
// notes based on key patterns. The new Thought starts with a fresh session.
//
// This is useful for pruning failed attempts, irrelevant context, or sensitive
// information before continuing a reasoning chain.
type Forget struct {
	key      string
	dropKeys map[string]bool // Keys to exclude
	keepKeys map[string]bool // Keys to include (if set, only these are kept)
	identity pipz.Identity
}

// NewForget creates a new forget primitive.
//
// The primitive creates a new Thought that:
//   - Has the current Thought as its parent (ParentID)
//   - Inherits the same TaskID
//   - Copies only notes that pass the filter
//   - Has a fresh Session (no LLM state carried over)
//   - Has publishedCount at 0 (all notes unpublished)
//
// By default, all notes are copied. Use WithDropKeys or WithKeepKeys to filter.
//
// Example:
//
//	// Drop failed attempt notes
//	forget := cogito.NewForget("clean_slate").
//	    WithDropKeys("failed_attempt", "bad_path", "error_details")
//
//	// Or keep only essential context
//	forget := cogito.NewForget("minimal_context").
//	    WithKeepKeys("user_input", "final_decision")
func NewForget(key string) *Forget {
	return &Forget{
		key:      key,
		dropKeys: make(map[string]bool),
		keepKeys: make(map[string]bool),
		identity: pipz.NewIdentity(key, "Note filtering primitive"),
	}
}

// Process implements pipz.Chainable[*Thought].
func (f *Forget) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	// Emit step started
	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(f.key),
		FieldStepType.Field("forget"),
		FieldNoteCount.Field(len(t.AllNotes())),
	)

	// Create new thought with current as parent
	parentID := t.ID
	newThought := &Thought{
		Intent:         t.Intent,
		TraceID:        uuid.New().String(),
		ParentID:       &parentID,
		TaskID:         t.TaskID,
		Session:        zyn.NewSession(), // Fresh session - no LLM state carried over
		memory:         t.memory,
		notes:          make([]Note, 0),
		publishedCount: 0,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Persist the new thought
	persisted, err := t.memory.CreateThought(ctx, newThought)
	if err != nil {
		f.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("forget: failed to create thought: %w", err)
	}
	newThought.ID = persisted.ID

	// Copy filtered notes
	copiedCount := 0
	droppedCount := 0
	for _, note := range t.AllNotes() {
		if !f.shouldKeep(note.Key) {
			droppedCount++
			continue
		}

		newNote := Note{
			ThoughtID: newThought.ID,
			Key:       note.Key,
			Content:   note.Content,
			Metadata:  copyMetadata(note.Metadata),
			Source:    note.Source,
			Created:   note.Created,
		}
		if err := newThought.AddNote(ctx, newNote); err != nil {
			f.emitFailed(ctx, t, start, err)
			return t, fmt.Errorf("forget: failed to copy note: %w", err)
		}
		copiedCount++
	}

	// Emit thought created
	capitan.Emit(ctx, ThoughtCreated,
		FieldIntent.Field(newThought.Intent),
		FieldTraceID.Field(newThought.TraceID),
	)

	// Emit step completed
	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(newThought.TraceID),
		FieldStepName.Field(f.key),
		FieldStepType.Field("forget"),
		FieldStepDuration.Field(time.Since(start)),
		FieldNoteCount.Field(copiedCount),
	)

	return newThought, nil
}

// shouldKeep determines if a note with the given key should be kept.
func (f *Forget) shouldKeep(key string) bool {
	// If keepKeys is set, only keep notes in that set
	if len(f.keepKeys) > 0 {
		return f.keepKeys[key]
	}

	// Otherwise, keep unless explicitly dropped
	return !f.dropKeys[key]
}

// emitFailed emits a step failed event.
func (f *Forget) emitFailed(ctx context.Context, t *Thought, start time.Time, err error) {
	capitan.Error(ctx, StepFailed,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(f.key),
		FieldStepType.Field("forget"),
		FieldStepDuration.Field(time.Since(start)),
		FieldError.Field(err),
	)
}

// Identity implements pipz.Chainable[*Thought].
func (f *Forget) Identity() pipz.Identity {
	return f.identity
}

// Schema implements pipz.Chainable[*Thought].
func (f *Forget) Schema() pipz.Node {
	return pipz.Node{Identity: f.identity, Type: "forget"}
}

// Close implements pipz.Chainable[*Thought].
func (f *Forget) Close() error {
	return nil
}

// Builder methods

// WithDropKeys specifies note keys to exclude from the new Thought.
// Notes with these keys will not be copied.
func (f *Forget) WithDropKeys(keys ...string) *Forget {
	for _, key := range keys {
		f.dropKeys[key] = true
	}
	return f
}

// WithKeepKeys specifies note keys to include in the new Thought.
// Only notes with these keys will be copied. This takes precedence over WithDropKeys.
func (f *Forget) WithKeepKeys(keys ...string) *Forget {
	for _, key := range keys {
		f.keepKeys[key] = true
	}
	return f
}
