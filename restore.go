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

// Restore is a memory primitive that loads a previous thought and creates a new branch from it.
// It loads the specified Thought by ID, creates a new Thought with the loaded one as parent,
// copies all notes, and returns the new Thought. This effectively "restores" to a previous.
// checkpoint while preserving the full audit trail.
type Restore struct {
	key       string
	thoughtID string
}

// NewRestore creates a new restore primitive.
//
// The primitive loads a Thought by ID and creates a new Thought that:
//   - Has the loaded Thought as its parent (ParentID)
//   - Inherits the same TaskID
//   - Copies all notes from the loaded Thought
//   - Has a fresh Session
//   - Has publishedCount reset to 0 (all notes will be sent to LLM on next primitive)
//
// The current Thought in the pipeline is abandoned (but preserved in the database).
//
// Example:
//
//	// Store checkpoint ID somewhere accessible
//	var checkpointID string
//
//	pipeline := pipz.Chain(
//	    cogito.NewCheckpoint("save_point"),
//	    // ... more steps that might fail ...
//	)
//
//	// On failure, restore from checkpoint
//	restore := cogito.NewRestore("retry", checkpointID)
//	thought, _ = restore.Process(ctx, thought)
func NewRestore(key, thoughtID string) *Restore {
	return &Restore{
		key:       key,
		thoughtID: thoughtID,
	}
}

// Process implements pipz.Chainable[*Thought].
func (r *Restore) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	// Emit step started
	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(r.key),
		FieldStepType.Field("restore"),
	)

	// Load the target thought
	memory := t.memory
	targetThought, err := memory.GetThought(ctx, r.thoughtID)
	if err != nil {
		capitan.Error(ctx, StepFailed,
			FieldTraceID.Field(t.TraceID),
			FieldStepName.Field(r.key),
			FieldStepType.Field("restore"),
			FieldStepDuration.Field(time.Since(start)),
			FieldError.Field(err),
		)
		return t, fmt.Errorf("restore: failed to load thought %s: %w", r.thoughtID, err)
	}

	// Create new thought with target as parent
	parentID := targetThought.ID
	newThought := &Thought{
		Intent:         targetThought.Intent,
		TraceID:        uuid.New().String(),
		ParentID:       &parentID,
		TaskID:         targetThought.TaskID,
		Session:        zyn.NewSession(),
		memory:         memory,
		notes:          make([]Note, 0),
		publishedCount: 0,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Persist the new thought
	persisted, err := memory.CreateThought(ctx, newThought)
	if err != nil {
		capitan.Error(ctx, StepFailed,
			FieldTraceID.Field(t.TraceID),
			FieldStepName.Field(r.key),
			FieldStepType.Field("restore"),
			FieldStepDuration.Field(time.Since(start)),
			FieldError.Field(err),
		)
		return t, fmt.Errorf("restore: failed to create thought: %w", err)
	}
	newThought.ID = persisted.ID

	// Copy notes from target thought (persisting each)
	for _, note := range targetThought.AllNotes() {
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
				FieldStepName.Field(r.key),
				FieldStepType.Field("restore"),
				FieldStepDuration.Field(time.Since(start)),
				FieldError.Field(err),
			)
			return t, fmt.Errorf("restore: failed to copy note: %w", err)
		}
	}

	// Session is NOT copied - it represents runtime execution state with the LLM.
	// The restored thought starts with a fresh session. Notes carry the semantic
	// state; the LLM conversation begins anew.

	// Emit thought created
	capitan.Emit(ctx, ThoughtCreated,
		FieldIntent.Field(newThought.Intent),
		FieldTraceID.Field(newThought.TraceID),
	)

	// Emit step completed
	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(newThought.TraceID),
		FieldStepName.Field(r.key),
		FieldStepType.Field("restore"),
		FieldStepDuration.Field(time.Since(start)),
		FieldNoteCount.Field(len(newThought.AllNotes())),
	)

	return newThought, nil
}

// Name implements pipz.Chainable[*Thought].
func (r *Restore) Name() pipz.Name {
	return pipz.Name(r.key)
}

// Close implements pipz.Chainable[*Thought].
func (r *Restore) Close() error {
	return nil
}
