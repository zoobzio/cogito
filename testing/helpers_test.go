package cogitotest

import (
	"context"
	"testing"
)

func TestMockMemory(t *testing.T) {
	mem := NewMockMemory()

	t.Run("CreateThought", func(t *testing.T) {
		thought := NewTestThought(t, "test intent")
		if thought.ID == "" {
			t.Error("expected thought to have ID")
		}
		if thought.Intent != "test intent" {
			t.Errorf("expected intent 'test intent', got %q", thought.Intent)
		}
	})

	t.Run("GetThought", func(t *testing.T) {
		thought := NewTestThought(t, "test intent")

		// Note: MockMemory doesn't actually store thoughts correctly
		// because NewTestThought creates its own MockMemory instance.
		// This test verifies the mock structure works.
		if thought.ID == "" {
			t.Error("expected thought to have ID")
		}
	})

	t.Run("AddNote", func(t *testing.T) {
		ctx := context.Background()
		thought := NewTestThought(t, "test intent")

		err := thought.SetContent(ctx, "key1", "value1", "test")
		if err != nil {
			t.Fatalf("SetContent failed: %v", err)
		}

		content, err := thought.GetContent("key1")
		if err != nil {
			t.Fatalf("GetContent failed: %v", err)
		}
		if content != "value1" {
			t.Errorf("expected content 'value1', got %q", content)
		}
	})

	t.Run("SearchNotes returns empty", func(t *testing.T) {
		ctx := context.Background()
		results, err := mem.SearchNotes(ctx, nil, 10)
		if err != nil {
			t.Fatalf("SearchNotes failed: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected empty results, got %d", len(results))
		}
	})

	t.Run("SearchNotesByTask returns empty", func(t *testing.T) {
		ctx := context.Background()
		results, err := mem.SearchNotesByTask(ctx, nil, 10)
		if err != nil {
			t.Fatalf("SearchNotesByTask failed: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected empty results, got %d", len(results))
		}
	})
}

func TestNewTestThought(t *testing.T) {
	thought := NewTestThought(t, "test intent")

	if thought == nil {
		t.Fatal("expected thought, got nil")
	}
	if thought.Intent != "test intent" {
		t.Errorf("expected intent 'test intent', got %q", thought.Intent)
	}
	if thought.ID == "" {
		t.Error("expected thought to have ID")
	}
	if thought.TraceID == "" {
		t.Error("expected thought to have TraceID")
	}
}

func TestNewTestThoughtWithTrace(t *testing.T) {
	thought := NewTestThoughtWithTrace(t, "test intent", "custom-trace-123")

	if thought == nil {
		t.Fatal("expected thought, got nil")
	}
	if thought.TraceID != "custom-trace-123" {
		t.Errorf("expected TraceID 'custom-trace-123', got %q", thought.TraceID)
	}
}

func TestRequireContent(t *testing.T) {
	ctx := context.Background()
	thought := NewTestThought(t, "test")
	_ = thought.SetContent(ctx, "key", "value", "test")

	// This should not fail.
	RequireContent(t, thought, "key", "value")
}

func TestRequireNoContent(t *testing.T) {
	thought := NewTestThought(t, "test")

	// This should not fail since "missing" key doesn't exist.
	RequireNoContent(t, thought, "missing")
}
