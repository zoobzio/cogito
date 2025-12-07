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

// Checkpoint is a memory primitive that creates a persistent snapshot of the current thought.
// It creates a new Thought with the current one as its parent, copies all notes,
// and returns the new Thought. The original Thought is preserved unchanged in the database.
type Checkpoint struct {
	key string
}

// NewCheckpoint creates a new checkpoint primitive.
//
// The primitive creates a new Thought that:
//   - Has the current Thought as its parent (ParentID)
//   - Inherits the same TaskID
//   - Copies all notes from the current Thought
//   - Has a fresh Session
//   - Has publishedCount reset to 0 (all notes will be sent to LLM on next primitive)
//
// Example:
//
//	pipeline := pipz.Chain(
//	    cogito.NewAnalyze[Data]("extract", "Extract data"),
//	    cogito.NewCheckpoint("before_decision"),  // Snapshot here
//	    cogito.NewDecide("risky_choice", "Should we proceed?"),
//	)
func NewCheckpoint(key string) *Checkpoint {
	return &Checkpoint{
		key: key,
	}
}

// Process implements pipz.Chainable[*Thought].
func (c *Checkpoint) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	// Emit step started
	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(c.key),
		FieldStepType.Field("checkpoint"),
		FieldNoteCount.Field(len(t.AllNotes())),
	)

	// Create new thought with current as parent
	parentID := t.ID
	newThought := &Thought{
		Intent:         t.Intent,
		TraceID:        uuid.New().String(),
		ParentID:       &parentID,
		TaskID:         t.TaskID,
		Session:        zyn.NewSession(),
		memory:         t.memory,
		notes:          make([]Note, 0),
		publishedCount: 0,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Persist the new thought
	persisted, err := t.memory.CreateThought(ctx, newThought)
	if err != nil {
		capitan.Error(ctx, StepFailed,
			FieldTraceID.Field(t.TraceID),
			FieldStepName.Field(c.key),
			FieldStepType.Field("checkpoint"),
			FieldStepDuration.Field(time.Since(start)),
			FieldError.Field(err),
		)
		return t, fmt.Errorf("checkpoint: failed to create thought: %w", err)
	}
	newThought.ID = persisted.ID

	// Copy notes to new thought (persisting each)
	for _, note := range t.AllNotes() {
		newNote := Note{
			ThoughtID: newThought.ID,
			Key:       note.Key,
			Content:   note.Content,
			Metadata:  copyMetadata(note.Metadata),
			Source:    note.Source,
			Created:   note.Created,
		}
		if err := newThought.AddNote(ctx, newNote); err != nil {
			capitan.Error(ctx, StepFailed,
				FieldTraceID.Field(t.TraceID),
				FieldStepName.Field(c.key),
				FieldStepType.Field("checkpoint"),
				FieldStepDuration.Field(time.Since(start)),
				FieldError.Field(err),
			)
			return t, fmt.Errorf("checkpoint: failed to copy note: %w", err)
		}
	}

	// Session is NOT copied - it represents runtime execution state with the LLM.
	// The new thought starts with a fresh session. The calling code or subsequent
	// primitives can establish a new LLM conversation as needed.
	// Notes contain the semantic state; session is ephemeral.

	// Emit checkpoint created
	capitan.Emit(ctx, ThoughtCreated,
		FieldIntent.Field(newThought.Intent),
		FieldTraceID.Field(newThought.TraceID),
	)

	// Emit step completed
	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(newThought.TraceID),
		FieldStepName.Field(c.key),
		FieldStepType.Field("checkpoint"),
		FieldStepDuration.Field(time.Since(start)),
		FieldNoteCount.Field(len(newThought.AllNotes())),
	)

	return newThought, nil
}

// Name implements pipz.Chainable[*Thought].
func (c *Checkpoint) Name() pipz.Name {
	return pipz.Name(c.key)
}

// Close implements pipz.Chainable[*Thought].
func (c *Checkpoint) Close() error {
	return nil
}

// copyMetadata creates a deep copy of note metadata.
func copyMetadata(m map[string]string) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	copied := make(map[string]string, len(m))
	for k, v := range m {
		copied[k] = v
	}
	return copied
}
