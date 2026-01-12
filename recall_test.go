package cogito

import (
	"context"
	"testing"
)

func TestRecall_Name(t *testing.T) {
	recall := NewRecall("my_recall", "some-id")
	if recall.Identity().Name() != "my_recall" {
		t.Errorf("expected name 'my_recall', got %q", recall.Identity().Name())
	}
}

func TestRecall_WithPrompt(t *testing.T) {
	recall := NewRecall("test", "id").WithPrompt("custom prompt")
	if recall.prompt != "custom prompt" {
		t.Errorf("expected prompt 'custom prompt', got %q", recall.prompt)
	}
}

func TestRecall_FailsOnInvalidThoughtID(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()
	thought, _ := New(ctx, mem, "current")

	recall := NewRecall("context", "nonexistent-id")
	_, err := recall.Process(ctx, thought)
	if err == nil {
		t.Error("recall should fail for nonexistent thought ID")
	}
}

func TestRecall_FailsOnEmptyNotes(t *testing.T) {
	ctx := context.Background()
	mem := newMockMemory()

	// Create target thought with no notes
	target, _ := New(ctx, mem, "empty target")
	targetID := target.ID

	// Create current thought
	current, _ := New(ctx, mem, "current")

	recall := NewRecall("context", targetID)
	_, err := recall.Process(ctx, current)
	if err == nil {
		t.Error("recall should fail when target has no notes")
	}
}
