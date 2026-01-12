package cogito

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zoobzio/zyn"
)

// mockCompressProvider implements Provider interface for testing Compress.
type mockCompressProvider struct {
	callCount int
	summary   string
}

func (m *mockCompressProvider) Call(ctx context.Context, messages []zyn.Message, temperature float32) (*zyn.ProviderResponse, error) {
	m.callCount++

	// Return a mock summary
	summary := m.summary
	if summary == "" {
		summary = "This is a compressed summary of the conversation."
	}

	return &zyn.ProviderResponse{
		Content: fmt.Sprintf(`{"output": "%s", "confidence": 0.95, "changes": ["Summarized conversation"], "reasoning": ["Extracted key points"]}`, summary),
		Usage: zyn.TokenUsage{
			Prompt:     50,
			Completion: 30,
			Total:      80,
		},
	}, nil
}

func (m *mockCompressProvider) Name() string {
	return "mock-compress"
}

func TestCompressBasic(t *testing.T) {
	provider := &mockCompressProvider{summary: "User asked about billing. Assistant provided invoice details."}
	SetProvider(provider)
	defer SetProvider(nil)

	thought := newTestThought("test compress")
	// Simulate a conversation
	thought.Session.Append("user", "What is my current balance?")
	thought.Session.Append("assistant", "Your current balance is $150.")
	thought.Session.Append("user", "Can you send me an invoice?")
	thought.Session.Append("assistant", "I have sent the invoice to your email.")

	if thought.Session.Len() != 4 {
		t.Fatalf("expected 4 messages, got %d", thought.Session.Len())
	}

	compress := NewCompress("session_summary")

	result, err := compress.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify session was compressed
	if result.Session.Len() != 1 {
		t.Errorf("expected 1 message after compression, got %d", result.Session.Len())
	}

	// Verify the new message is a system message with summary
	msg, err := result.Session.At(0)
	if err != nil {
		t.Fatalf("failed to get message: %v", err)
	}

	if msg.Role != "system" {
		t.Errorf("expected system message, got %q", msg.Role)
	}

	if !strings.Contains(msg.Content, "billing") {
		t.Errorf("expected summary content in message, got %q", msg.Content)
	}

	// Verify summary note was created
	summary, err := result.GetContent("session_summary")
	if err != nil {
		t.Fatalf("summary note not found: %v", err)
	}

	if !strings.Contains(summary, "billing") {
		t.Errorf("expected summary to contain conversation content, got %q", summary)
	}

	// Verify provider was called
	if provider.callCount != 1 {
		t.Errorf("expected 1 provider call, got %d", provider.callCount)
	}
}

func TestCompressWithThreshold(t *testing.T) {
	provider := &mockCompressProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	thought := newTestThought("test compress threshold")
	thought.Session.Append("user", "Hello")
	thought.Session.Append("assistant", "Hi there!")

	// Threshold of 5, but only 2 messages
	compress := NewCompress("session_summary").WithThreshold(5)

	result, err := compress.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should pass through unchanged
	if result.Session.Len() != 2 {
		t.Errorf("expected 2 messages (unchanged), got %d", result.Session.Len())
	}

	// Provider should NOT be called
	if provider.callCount != 0 {
		t.Errorf("expected 0 provider calls, got %d", provider.callCount)
	}
}

func TestCompressWithThresholdMet(t *testing.T) {
	provider := &mockCompressProvider{summary: "Conversation summary"}
	SetProvider(provider)
	defer SetProvider(nil)

	thought := newTestThought("test compress threshold met")
	for i := 0; i < 5; i++ {
		thought.Session.Append("user", fmt.Sprintf("Message %d", i))
	}

	// Threshold of 5, exactly 5 messages
	compress := NewCompress("session_summary").WithThreshold(5)

	result, err := compress.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be compressed
	if result.Session.Len() != 1 {
		t.Errorf("expected 1 message after compression, got %d", result.Session.Len())
	}

	// Provider should be called
	if provider.callCount != 1 {
		t.Errorf("expected 1 provider call, got %d", provider.callCount)
	}
}

func TestCompressEmptySession(t *testing.T) {
	provider := &mockCompressProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	thought := newTestThought("test compress empty")
	// No messages in session

	compress := NewCompress("session_summary")

	result, err := compress.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should pass through unchanged
	if result.Session.Len() != 0 {
		t.Errorf("expected 0 messages, got %d", result.Session.Len())
	}

	// Provider should NOT be called
	if provider.callCount != 0 {
		t.Errorf("expected 0 provider calls, got %d", provider.callCount)
	}
}

func TestCompressWithSummaryKey(t *testing.T) {
	provider := &mockCompressProvider{summary: "Custom key summary"}
	SetProvider(provider)
	defer SetProvider(nil)

	thought := newTestThought("test compress summary key")
	thought.Session.Append("user", "Test message")

	compress := NewCompress("compress_step").WithSummaryKey("conversation_context")

	result, err := compress.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify summary is at custom key
	summary, err := result.GetContent("conversation_context")
	if err != nil {
		t.Fatalf("custom summary note not found: %v", err)
	}

	if summary == "" {
		t.Error("expected non-empty summary")
	}

	// Default key should not exist
	_, err = result.GetContent("compress_step")
	if err == nil {
		t.Error("expected default key to not exist")
	}
}

func TestCompressChainable(t *testing.T) {
	compress := NewCompress("test_compress")

	if compress.Identity().Name() != "test_compress" {
		t.Errorf("expected name 'test_compress', got %q", compress.Identity().Name())
	}

	if err := compress.Close(); err != nil {
		t.Errorf("unexpected error from Close(): %v", err)
	}
}
