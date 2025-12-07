package cogito

import (
	"context"
	"fmt"
	"testing"
)

func TestTruncateBasic(t *testing.T) {
	thought := newTestThought("test truncate")

	// Add 10 messages
	thought.Session.Append("system", "You are a helpful assistant.")
	for i := 1; i <= 9; i++ {
		if i%2 == 1 {
			thought.Session.Append("user", fmt.Sprintf("User message %d", i))
		} else {
			thought.Session.Append("assistant", fmt.Sprintf("Assistant message %d", i))
		}
	}

	if thought.Session.Len() != 10 {
		t.Fatalf("expected 10 messages, got %d", thought.Session.Len())
	}

	truncate := NewTruncate("session_truncate").
		WithKeepFirst(1).
		WithKeepLast(3)

	result, err := truncate.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 4 messages (1 first + 3 last)
	if result.Session.Len() != 4 {
		t.Errorf("expected 4 messages after truncation, got %d", result.Session.Len())
	}

	// First message should be system prompt
	first, err := result.Session.At(0)
	if err != nil {
		t.Fatalf("failed to get first message: %v", err)
	}
	if first.Role != "system" {
		t.Errorf("expected first message to be system, got %q", first.Role)
	}

	// Last message should be message 9
	last, err := result.Session.At(3)
	if err != nil {
		t.Fatalf("failed to get last message: %v", err)
	}
	if last.Content != "User message 9" {
		t.Errorf("expected last message to be 'User message 9', got %q", last.Content)
	}
}

func TestTruncateWithThreshold(t *testing.T) {
	thought := newTestThought("test truncate threshold")

	// Add 5 messages
	for i := 0; i < 5; i++ {
		thought.Session.Append("user", fmt.Sprintf("Message %d", i))
	}

	// Threshold of 10, but only 5 messages
	truncate := NewTruncate("session_truncate").
		WithKeepFirst(1).
		WithKeepLast(2).
		WithThreshold(10)

	result, err := truncate.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should pass through unchanged
	if result.Session.Len() != 5 {
		t.Errorf("expected 5 messages (unchanged), got %d", result.Session.Len())
	}
}

func TestTruncateNothingToRemove(t *testing.T) {
	thought := newTestThought("test truncate nothing")

	// Add 3 messages
	thought.Session.Append("system", "System prompt")
	thought.Session.Append("user", "Hello")
	thought.Session.Append("assistant", "Hi")

	// Keep first 1 and last 3 = 4, but only have 3
	truncate := NewTruncate("session_truncate").
		WithKeepFirst(1).
		WithKeepLast(3)

	result, err := truncate.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should pass through unchanged
	if result.Session.Len() != 3 {
		t.Errorf("expected 3 messages (unchanged), got %d", result.Session.Len())
	}
}

func TestTruncateKeepOnlyFirst(t *testing.T) {
	thought := newTestThought("test truncate first only")

	// Add 10 messages
	thought.Session.Append("system", "System prompt")
	for i := 1; i < 10; i++ {
		thought.Session.Append("user", fmt.Sprintf("Message %d", i))
	}

	truncate := NewTruncate("session_truncate").
		WithKeepFirst(2).
		WithKeepLast(0)

	result, err := truncate.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 2 messages
	if result.Session.Len() != 2 {
		t.Errorf("expected 2 messages, got %d", result.Session.Len())
	}
}

func TestTruncateKeepOnlyLast(t *testing.T) {
	thought := newTestThought("test truncate last only")

	// Add 10 messages
	for i := 0; i < 10; i++ {
		thought.Session.Append("user", fmt.Sprintf("Message %d", i))
	}

	truncate := NewTruncate("session_truncate").
		WithKeepFirst(0).
		WithKeepLast(3)

	result, err := truncate.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 3 messages
	if result.Session.Len() != 3 {
		t.Errorf("expected 3 messages, got %d", result.Session.Len())
	}

	// First remaining should be Message 7
	first, err := result.Session.At(0)
	if err != nil {
		t.Fatalf("failed to get first message: %v", err)
	}
	if first.Content != "Message 7" {
		t.Errorf("expected first message 'Message 7', got %q", first.Content)
	}
}

func TestTruncateChainable(t *testing.T) {
	truncate := NewTruncate("test_truncate")

	if truncate.Name() != "test_truncate" {
		t.Errorf("expected name 'test_truncate', got %q", truncate.Name())
	}

	if err := truncate.Close(); err != nil {
		t.Errorf("unexpected error from Close(): %v", err)
	}
}

func TestTruncateDefaults(t *testing.T) {
	thought := newTestThought("test truncate defaults")

	// Add 20 messages
	thought.Session.Append("system", "System prompt")
	for i := 1; i < 20; i++ {
		thought.Session.Append("user", fmt.Sprintf("Message %d", i))
	}

	// Use defaults: keepFirst=1, keepLast=10
	truncate := NewTruncate("session_truncate")

	result, err := truncate.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 11 messages (1 + 10)
	if result.Session.Len() != 11 {
		t.Errorf("expected 11 messages with defaults, got %d", result.Session.Len())
	}
}
