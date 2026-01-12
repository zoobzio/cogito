package cogito

import (
	"context"
	"testing"
)

func TestReflect_Name(t *testing.T) {
	reflect := NewReflect("my_reflect")
	if reflect.Identity().Name() != "my_reflect" {
		t.Errorf("expected name 'my_reflect', got %q", reflect.Identity().Name())
	}
}

func TestReflect_WithPrompt(t *testing.T) {
	reflect := NewReflect("test").WithPrompt("custom prompt")
	if reflect.prompt != "custom prompt" {
		t.Errorf("expected prompt 'custom prompt', got %q", reflect.prompt)
	}
}

func TestReflect_WithUnpublishedOnly(t *testing.T) {
	reflect := NewReflect("test").WithUnpublishedOnly()
	if !reflect.unpublishedOnly {
		t.Error("expected unpublishedOnly to be true")
	}
}

func TestReflect_FailsOnEmptyNotes(t *testing.T) {
	ctx := context.Background()
	thought := newTestThought("empty")

	reflect := NewReflect("reflection")
	_, err := reflect.Process(ctx, thought)
	if err == nil {
		t.Error("reflect should fail when thought has no notes")
	}
}

func TestReflect_FailsOnEmptyUnpublished(t *testing.T) {
	ctx := context.Background()
	thought := newTestThought("test")
	thought.SetContent(ctx, "key1", "value1", "test")
	thought.MarkNotesPublished()

	reflect := NewReflect("reflection").WithUnpublishedOnly()
	_, err := reflect.Process(ctx, thought)
	if err == nil {
		t.Error("reflect should fail when no unpublished notes")
	}
}
