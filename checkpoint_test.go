package cogito

import (
	"context"
	"testing"
)

func TestCheckpoint_CreatesNewThought(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test intent")

	// Add some notes
	thought.SetContent(ctx, "key1", "value1", "test")
	thought.SetContent(ctx, "key2", "value2", "test")

	originalID := thought.ID
	originalNoteCount := len(thought.AllNotes())

	// Checkpoint
	checkpoint := NewCheckpoint("save_point")
	newThought, err := checkpoint.Process(ctx, thought)
	if err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}

	// New thought should have different ID
	if newThought.ID == originalID {
		t.Error("checkpoint should create new thought with different ID")
	}

	// New thought should have original as parent
	if newThought.ParentID == nil || *newThought.ParentID != originalID {
		t.Error("checkpoint should set ParentID to original thought")
	}

	// New thought should have same notes
	if len(newThought.AllNotes()) != originalNoteCount {
		t.Errorf("expected %d notes, got %d", originalNoteCount, len(newThought.AllNotes()))
	}

	// Notes should have same content
	content, _ := newThought.GetContent("key1")
	if content != "value1" {
		t.Errorf("expected note content 'value1', got %q", content)
	}

	// New thought should have same intent
	if newThought.Intent != thought.Intent {
		t.Errorf("expected intent %q, got %q", thought.Intent, newThought.Intent)
	}

	// New thought should have different TraceID
	if newThought.TraceID == thought.TraceID {
		t.Error("checkpoint should create new TraceID")
	}
}

func TestCheckpoint_PreservesTaskID(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	taskID := "task-123"
	thought, _ := NewForTask(ctx, mem, "test intent", taskID)

	checkpoint := NewCheckpoint("save_point")
	newThought, err := checkpoint.Process(ctx, thought)
	if err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}

	if newThought.TaskID == nil || *newThought.TaskID != taskID {
		t.Error("checkpoint should preserve TaskID")
	}
}

func TestCheckpoint_ResetsPublishedCount(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test intent")

	thought.SetContent(ctx, "key1", "value1", "test")
	thought.MarkNotesPublished()

	if thought.PublishedCount() != 1 {
		t.Fatalf("expected published count 1, got %d", thought.PublishedCount())
	}

	checkpoint := NewCheckpoint("save_point")
	newThought, err := checkpoint.Process(ctx, thought)
	if err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}

	// Published count should be reset - all notes unpublished
	if newThought.PublishedCount() != 0 {
		t.Errorf("expected published count 0, got %d", newThought.PublishedCount())
	}

	// But notes should still exist
	unpublished := newThought.GetUnpublishedNotes()
	if len(unpublished) != 1 {
		t.Errorf("expected 1 unpublished note, got %d", len(unpublished))
	}
}

func TestCheckpoint_CopiesMetadata(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test intent")

	thought.SetNote(ctx, "key1", "value1", "test", map[string]string{
		"confidence": "0.95",
		"source":     "llm",
	})

	checkpoint := NewCheckpoint("save_point")
	newThought, err := checkpoint.Process(ctx, thought)
	if err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}

	note, ok := newThought.GetNote("key1")
	if !ok {
		t.Fatal("note not found")
	}

	if note.Metadata["confidence"] != "0.95" {
		t.Errorf("expected confidence '0.95', got %q", note.Metadata["confidence"])
	}

	// Verify it's a copy, not a reference
	originalNote, _ := thought.GetNote("key1")
	originalNote.Metadata["confidence"] = "changed"

	note, _ = newThought.GetNote("key1")
	if note.Metadata["confidence"] != "0.95" {
		t.Error("metadata should be copied, not referenced")
	}
}

func TestCheckpoint_PersistsToMemory(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test intent")
	thought.SetContent(ctx, "key1", "value1", "test")

	checkpoint := NewCheckpoint("save_point")
	newThought, err := checkpoint.Process(ctx, thought)
	if err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}

	// Verify thought was persisted
	loaded, err := mem.GetThought(ctx, newThought.ID)
	if err != nil {
		t.Fatalf("failed to load checkpointed thought: %v", err)
	}

	if loaded.ID != newThought.ID {
		t.Error("loaded thought should match checkpointed thought")
	}
}

func TestCheckpoint_Name(t *testing.T) {
	checkpoint := NewCheckpoint("my_checkpoint")
	if checkpoint.Name() != "my_checkpoint" {
		t.Errorf("expected name 'my_checkpoint', got %q", checkpoint.Name())
	}
}
