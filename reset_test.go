package cogito

import (
	"context"
	"testing"
)

func TestResetBasic(t *testing.T) {
	thought := newTestThought("test reset")

	// Add some messages
	thought.Session.Append("system", "Original system prompt")
	thought.Session.Append("user", "Hello")
	thought.Session.Append("assistant", "Hi there!")

	if thought.Session.Len() != 3 {
		t.Fatalf("expected 3 messages, got %d", thought.Session.Len())
	}

	reset := NewReset("session_reset")

	result, err := reset.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Session should be empty
	if result.Session.Len() != 0 {
		t.Errorf("expected 0 messages after reset, got %d", result.Session.Len())
	}
}

func TestResetWithSystemMessage(t *testing.T) {
	thought := newTestThought("test reset with system")

	// Add some messages
	thought.Session.Append("user", "Hello")
	thought.Session.Append("assistant", "Hi!")

	reset := NewReset("session_reset").
		WithSystemMessage("You are a helpful coding assistant.")

	result, err := reset.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have exactly 1 message (the new system message)
	if result.Session.Len() != 1 {
		t.Errorf("expected 1 message after reset, got %d", result.Session.Len())
	}

	msg, err := result.Session.At(0)
	if err != nil {
		t.Fatalf("failed to get message: %v", err)
	}

	if msg.Role != "system" {
		t.Errorf("expected system role, got %q", msg.Role)
	}

	if msg.Content != "You are a helpful coding assistant." {
		t.Errorf("expected system message content, got %q", msg.Content)
	}
}

func TestResetWithPreserveNote(t *testing.T) {
	thought := newTestThought("test reset preserve note")

	// Add a note that we want to preserve as context
	thought.SetContent(context.Background(), "conversation_summary", "User was asking about API authentication.", "compress")

	// Add some messages
	thought.Session.Append("user", "How do I authenticate?")
	thought.Session.Append("assistant", "Use OAuth2...")

	reset := NewReset("session_reset").
		WithPreserveNote("conversation_summary")

	result, err := reset.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 1 message with note content
	if result.Session.Len() != 1 {
		t.Errorf("expected 1 message after reset, got %d", result.Session.Len())
	}

	msg, err := result.Session.At(0)
	if err != nil {
		t.Fatalf("failed to get message: %v", err)
	}

	if msg.Role != "system" {
		t.Errorf("expected system role, got %q", msg.Role)
	}

	if msg.Content != "User was asking about API authentication." {
		t.Errorf("expected note content as system message, got %q", msg.Content)
	}
}

func TestResetWithPreserveNoteMissing(t *testing.T) {
	thought := newTestThought("test reset preserve note missing")

	// No note set, but configure to preserve one
	thought.Session.Append("user", "Hello")

	reset := NewReset("session_reset").
		WithPreserveNote("nonexistent_note").
		WithSystemMessage("Fallback message")

	result, err := reset.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should fall back to system message
	if result.Session.Len() != 1 {
		t.Errorf("expected 1 message after reset, got %d", result.Session.Len())
	}

	msg, err := result.Session.At(0)
	if err != nil {
		t.Fatalf("failed to get message: %v", err)
	}

	if msg.Content != "Fallback message" {
		t.Errorf("expected fallback message, got %q", msg.Content)
	}
}

func TestResetWithPreserveNoteMissingNoFallback(t *testing.T) {
	thought := newTestThought("test reset preserve note missing no fallback")

	// No note set, no fallback
	thought.Session.Append("user", "Hello")

	reset := NewReset("session_reset").
		WithPreserveNote("nonexistent_note")

	result, err := reset.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Session should be completely empty
	if result.Session.Len() != 0 {
		t.Errorf("expected 0 messages after reset, got %d", result.Session.Len())
	}
}

func TestResetEmptySession(t *testing.T) {
	thought := newTestThought("test reset empty")
	// No messages in session

	reset := NewReset("session_reset").
		WithSystemMessage("Fresh start")

	result, err := reset.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have the system message
	if result.Session.Len() != 1 {
		t.Errorf("expected 1 message after reset, got %d", result.Session.Len())
	}
}

func TestResetChainable(t *testing.T) {
	reset := NewReset("test_reset")

	if reset.Identity().Name() != "test_reset" {
		t.Errorf("expected name 'test_reset', got %q", reset.Identity().Name())
	}

	if err := reset.Close(); err != nil {
		t.Errorf("unexpected error from Close(): %v", err)
	}
}

func TestResetPreserveNoteOverridesSystemMessage(t *testing.T) {
	thought := newTestThought("test reset priority")

	// Set both a note and a system message
	thought.SetContent(context.Background(), "context", "Note content wins", "test")
	thought.Session.Append("user", "Hello")

	reset := NewReset("session_reset").
		WithSystemMessage("System message loses").
		WithPreserveNote("context")

	result, err := reset.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg, err := result.Session.At(0)
	if err != nil {
		t.Fatalf("failed to get message: %v", err)
	}

	// Note content should win
	if msg.Content != "Note content wins" {
		t.Errorf("expected note content to override system message, got %q", msg.Content)
	}
}
