package cogito

import (
	"context"
	"testing"
)

func TestRestore_LoadsAndForksThought(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()

	// Create original thought with notes
	original, _ := New(ctx, mem, "original intent")
	original.SetContent(ctx, "key1", "value1", "test")
	original.SetContent(ctx, "key2", "value2", "test")
	originalID := original.ID

	// Create a second thought (simulating pipeline continuation)
	current, _ := New(ctx, mem, "current intent")
	current.SetContent(ctx, "key3", "value3", "test")

	// Restore to original
	restore := NewRestore("restore_point", originalID)
	restored, err := restore.Process(ctx, current)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	// Restored thought should have different ID from both original and current
	if restored.ID == originalID {
		t.Error("restored thought should have different ID from original")
	}
	if restored.ID == current.ID {
		t.Error("restored thought should have different ID from current")
	}

	// Restored thought should have original as parent
	if restored.ParentID == nil || *restored.ParentID != originalID {
		t.Error("restored thought should have original as parent")
	}

	// Restored thought should have original's notes, not current's
	if len(restored.AllNotes()) != 2 {
		t.Errorf("expected 2 notes, got %d", len(restored.AllNotes()))
	}

	content, err := restored.GetContent("key1")
	if err != nil || content != "value1" {
		t.Error("restored thought should have original's notes")
	}

	_, err = restored.GetContent("key3")
	if err == nil {
		t.Error("restored thought should not have current's notes")
	}

	// Restored thought should have original's intent
	if restored.Intent != "original intent" {
		t.Errorf("expected intent 'original intent', got %q", restored.Intent)
	}
}

func TestRestore_PreservesTaskID(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	taskID := "task-123"

	original, _ := NewForTask(ctx, mem, "original intent", taskID)
	originalID := original.ID

	current, _ := New(ctx, mem, "current intent")

	restore := NewRestore("restore_point", originalID)
	restored, err := restore.Process(ctx, current)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	if restored.TaskID == nil || *restored.TaskID != taskID {
		t.Error("restore should preserve TaskID from target thought")
	}
}

func TestRestore_ResetsPublishedCount(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()

	original, _ := New(ctx, mem, "original intent")
	original.SetContent(ctx, "key1", "value1", "test")
	original.MarkNotesPublished()
	originalID := original.ID

	current, _ := New(ctx, mem, "current intent")

	restore := NewRestore("restore_point", originalID)
	restored, err := restore.Process(ctx, current)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	// Published count should be 0 - all notes unpublished for fresh LLM context
	if restored.PublishedCount() != 0 {
		t.Errorf("expected published count 0, got %d", restored.PublishedCount())
	}

	unpublished := restored.GetUnpublishedNotes()
	if len(unpublished) != 1 {
		t.Errorf("expected 1 unpublished note, got %d", len(unpublished))
	}
}

func TestRestore_FailsOnInvalidThoughtID(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()

	current, _ := New(ctx, mem, "current intent")

	restore := NewRestore("restore_point", "nonexistent-id")
	_, err := restore.Process(ctx, current)
	if err == nil {
		t.Error("restore should fail for nonexistent thought ID")
	}
}

func TestRestore_CopiesMetadata(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()

	original, _ := New(ctx, mem, "original intent")
	original.SetNote(ctx, "key1", "value1", "test", map[string]string{
		"confidence": "0.95",
	})
	originalID := original.ID

	current, _ := New(ctx, mem, "current intent")

	restore := NewRestore("restore_point", originalID)
	restored, err := restore.Process(ctx, current)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	note, ok := restored.GetNote("key1")
	if !ok {
		t.Fatal("note not found")
	}

	if note.Metadata["confidence"] != "0.95" {
		t.Errorf("expected confidence '0.95', got %q", note.Metadata["confidence"])
	}
}

func TestRestore_PersistsToMemory(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()

	original, _ := New(ctx, mem, "original intent")
	original.SetContent(ctx, "key1", "value1", "test")
	originalID := original.ID

	current, _ := New(ctx, mem, "current intent")

	restore := NewRestore("restore_point", originalID)
	restored, err := restore.Process(ctx, current)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	// Verify thought was persisted
	loaded, err := mem.GetThought(ctx, restored.ID)
	if err != nil {
		t.Fatalf("failed to load restored thought: %v", err)
	}

	if loaded.ID != restored.ID {
		t.Error("loaded thought should match restored thought")
	}
}

func TestRestore_Name(t *testing.T) {
	restore := NewRestore("my_restore", "some-id")
	if restore.Name() != "my_restore" {
		t.Errorf("expected name 'my_restore', got %q", restore.Name())
	}
}

func TestCheckpointAndRestore_Integration(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()

	// Create initial thought
	thought, _ := New(ctx, mem, "integration test")
	thought.SetContent(ctx, "step1", "completed", "test")

	// Checkpoint - creates new thought, returns it
	// The checkpoint captures the state at this point (with step1)
	checkpoint := NewCheckpoint("before_risky_operation")
	thought, err := checkpoint.Process(ctx, thought)
	if err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}

	// Save the checkpoint ID for reference (we'll actually restore to parent)
	_ = thought.ID // checkpointID - unused in this test but shows the pattern

	// Simulate some work that we might want to undo
	// This adds to the current thought (the checkpoint)
	thought.SetContent(ctx, "risky_step", "failed", "test")

	// Now checkpoint again before restore to capture the "failed" state
	// (In a real scenario, we'd restore to the earlier checkpoint)
	// For this test, we restore to the parent of the checkpoint (original thought)
	// which has step1 but not risky_step

	// Actually, let's restore to the checkpoint's parent to get back to step1 only
	parentID := *thought.ParentID

	restore := NewRestore("retry", parentID)
	thought, err = restore.Process(ctx, thought)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	// Should have step1 but not risky_step
	_, err = thought.GetContent("step1")
	if err != nil {
		t.Error("should have step1 after restore")
	}

	_, err = thought.GetContent("risky_step")
	if err == nil {
		t.Error("should not have risky_step after restore")
	}

	// Should be able to continue from here
	thought.SetContent(ctx, "alternative_step", "success", "test")
	content, _ := thought.GetContent("alternative_step")
	if content != "success" {
		t.Error("should be able to continue after restore")
	}

	// Verify lineage: new thought -> parent (original) -> nil
	if thought.ParentID == nil || *thought.ParentID != parentID {
		t.Error("restored thought should have original as parent")
	}
}
