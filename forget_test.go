package cogito

import (
	"context"
	"testing"
)

func TestForget_CreatesNewThought(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test intent")
	thought.SetContent(ctx, "key1", "value1", "test")

	originalID := thought.ID

	forget := NewForget("clean")
	newThought, err := forget.Process(ctx, thought)
	if err != nil {
		t.Fatalf("forget failed: %v", err)
	}

	if newThought.ID == originalID {
		t.Error("forget should create new thought with different ID")
	}

	if newThought.ParentID == nil || *newThought.ParentID != originalID {
		t.Error("forget should set ParentID to original thought")
	}
}

func TestForget_CopiesAllNotesByDefault(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")
	thought.SetContent(ctx, "key1", "value1", "test")
	thought.SetContent(ctx, "key2", "value2", "test")
	thought.SetContent(ctx, "key3", "value3", "test")

	forget := NewForget("clean")
	newThought, err := forget.Process(ctx, thought)
	if err != nil {
		t.Fatalf("forget failed: %v", err)
	}

	if len(newThought.AllNotes()) != 3 {
		t.Errorf("expected 3 notes, got %d", len(newThought.AllNotes()))
	}
}

func TestForget_WithDropKeys(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")
	thought.SetContent(ctx, "keep1", "value1", "test")
	thought.SetContent(ctx, "drop1", "value2", "test")
	thought.SetContent(ctx, "keep2", "value3", "test")
	thought.SetContent(ctx, "drop2", "value4", "test")

	forget := NewForget("clean").WithDropKeys("drop1", "drop2")
	newThought, err := forget.Process(ctx, thought)
	if err != nil {
		t.Fatalf("forget failed: %v", err)
	}

	if len(newThought.AllNotes()) != 2 {
		t.Errorf("expected 2 notes, got %d", len(newThought.AllNotes()))
	}

	// Verify kept notes
	_, ok := newThought.GetNote("keep1")
	if !ok {
		t.Error("should have keep1")
	}
	_, ok = newThought.GetNote("keep2")
	if !ok {
		t.Error("should have keep2")
	}

	// Verify dropped notes
	_, ok = newThought.GetNote("drop1")
	if ok {
		t.Error("should not have drop1")
	}
	_, ok = newThought.GetNote("drop2")
	if ok {
		t.Error("should not have drop2")
	}
}

func TestForget_WithKeepKeys(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")
	thought.SetContent(ctx, "essential", "value1", "test")
	thought.SetContent(ctx, "extra1", "value2", "test")
	thought.SetContent(ctx, "extra2", "value3", "test")
	thought.SetContent(ctx, "critical", "value4", "test")

	forget := NewForget("minimal").WithKeepKeys("essential", "critical")
	newThought, err := forget.Process(ctx, thought)
	if err != nil {
		t.Fatalf("forget failed: %v", err)
	}

	if len(newThought.AllNotes()) != 2 {
		t.Errorf("expected 2 notes, got %d", len(newThought.AllNotes()))
	}

	// Verify kept notes
	_, ok := newThought.GetNote("essential")
	if !ok {
		t.Error("should have essential")
	}
	_, ok = newThought.GetNote("critical")
	if !ok {
		t.Error("should have critical")
	}

	// Verify filtered notes
	_, ok = newThought.GetNote("extra1")
	if ok {
		t.Error("should not have extra1")
	}
}

func TestForget_KeepKeysTakesPrecedence(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")
	thought.SetContent(ctx, "key1", "value1", "test")
	thought.SetContent(ctx, "key2", "value2", "test")
	thought.SetContent(ctx, "key3", "value3", "test")

	// Both drop and keep specified - keep should take precedence
	forget := NewForget("test").
		WithDropKeys("key1").
		WithKeepKeys("key1", "key2")

	newThought, err := forget.Process(ctx, thought)
	if err != nil {
		t.Fatalf("forget failed: %v", err)
	}

	// Should only have key1 and key2 (keepKeys wins)
	if len(newThought.AllNotes()) != 2 {
		t.Errorf("expected 2 notes, got %d", len(newThought.AllNotes()))
	}

	_, ok := newThought.GetNote("key1")
	if !ok {
		t.Error("should have key1 (keepKeys takes precedence)")
	}
}

func TestForget_PreservesTaskID(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	taskID := "task-123"
	thought, _ := NewForTask(ctx, mem, "test", taskID)
	thought.SetContent(ctx, "key1", "value1", "test")

	forget := NewForget("clean")
	newThought, err := forget.Process(ctx, thought)
	if err != nil {
		t.Fatalf("forget failed: %v", err)
	}

	if newThought.TaskID == nil || *newThought.TaskID != taskID {
		t.Error("forget should preserve TaskID")
	}
}

func TestForget_FreshSession(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")
	thought.SetContent(ctx, "key1", "value1", "test")

	// Original session starts empty but exists
	originalMsgCount := len(thought.Session.Messages())

	forget := NewForget("clean")
	newThought, err := forget.Process(ctx, thought)
	if err != nil {
		t.Fatalf("forget failed: %v", err)
	}

	// New thought should have fresh empty session
	if len(newThought.Session.Messages()) != originalMsgCount {
		t.Error("forget should create fresh session")
	}

	// Sessions should be different instances
	if newThought.Session == thought.Session {
		t.Error("forget should create new session instance")
	}
}

func TestForget_ResetsPublishedCount(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")
	thought.SetContent(ctx, "key1", "value1", "test")
	thought.MarkNotesPublished()

	forget := NewForget("clean")
	newThought, err := forget.Process(ctx, thought)
	if err != nil {
		t.Fatalf("forget failed: %v", err)
	}

	if newThought.PublishedCount() != 0 {
		t.Errorf("expected published count 0, got %d", newThought.PublishedCount())
	}

	// All notes should be unpublished
	unpublished := newThought.GetUnpublishedNotes()
	if len(unpublished) != 1 {
		t.Errorf("expected 1 unpublished note, got %d", len(unpublished))
	}
}

func TestForget_CopiesMetadata(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "test")
	thought.SetNote(ctx, "key1", "value1", "test", map[string]string{
		"confidence": "0.95",
	})

	forget := NewForget("clean")
	newThought, err := forget.Process(ctx, thought)
	if err != nil {
		t.Fatalf("forget failed: %v", err)
	}

	note, ok := newThought.GetNote("key1")
	if !ok {
		t.Fatal("note not found")
	}

	if note.Metadata["confidence"] != "0.95" {
		t.Errorf("expected confidence '0.95', got %q", note.Metadata["confidence"])
	}
}

func TestForget_Name(t *testing.T) {
	forget := NewForget("my_forget")
	if forget.Name() != "my_forget" {
		t.Errorf("expected name 'my_forget', got %q", forget.Name())
	}
}
