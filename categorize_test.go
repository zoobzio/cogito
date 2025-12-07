package cogito

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zoobzio/zyn"
)

// mockCategorizeProvider implements Provider interface for testing Categorize.
type mockCategorizeProvider struct {
	callCount int
}

func (m *mockCategorizeProvider) Call(ctx context.Context, messages []zyn.Message, temperature float32) (*zyn.ProviderResponse, error) {
	m.callCount++

	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	// Check last message to determine which synapse is calling
	lastMessage := messages[len(messages)-1]

	// Transform synapse call
	if strings.Contains(lastMessage.Content, "Transform:") {
		return &zyn.ProviderResponse{
			Content: `{"output": "Bug report: authentication failure requiring immediate fix based on user description", "confidence": 0.91, "changes": ["Synthesized classification context"], "reasoning": ["Combined category with original context"]}`,
			Usage: zyn.TokenUsage{
				Prompt:     15,
				Completion: 25,
				Total:      40,
			},
		}, nil
	}

	// Classification synapse call (check for Task prefix which zyn uses)
	if strings.Contains(lastMessage.Content, "Task:") && !strings.Contains(lastMessage.Content, "Transform") {
		return &zyn.ProviderResponse{
			Content: `{"primary": "bug", "secondary": "feature", "confidence": 0.87, "reasoning": ["Login functionality is broken", "Affects core user flow"]}`,
			Usage: zyn.TokenUsage{
				Prompt:     10,
				Completion: 20,
				Total:      30,
			},
		}, nil
	}

	// Default fallback
	return &zyn.ProviderResponse{
		Content: `{"primary": "question", "secondary": "", "confidence": 0.6, "reasoning": ["Unable to determine specific category"]}`,
		Usage: zyn.TokenUsage{
			Prompt:     10,
			Completion: 10,
			Total:      20,
		},
	}, nil
}

func (m *mockCategorizeProvider) Name() string {
	return "mock"
}

func TestCategorizeBasic(t *testing.T) {
	provider := &mockCategorizeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := NewCategorize("ticket_type", "What type of ticket is this?", []string{"bug", "feature", "question", "documentation"})

	thought := newTestThought("test classification")
	thought.SetContent(context.Background(), "ticket_text", "Login button doesn't work", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Use Scan to get typed response
	resp, err := step.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if resp.Primary != "bug" {
		t.Errorf("expected primary 'bug', got %q", resp.Primary)
	}

	if resp.Confidence != 0.87 {
		t.Errorf("expected confidence 0.87, got %f", resp.Confidence)
	}

	if resp.Secondary != "feature" {
		t.Errorf("expected secondary 'feature', got %q", resp.Secondary)
	}

	if len(resp.Reasoning) == 0 {
		t.Error("expected reasoning to be present")
	}
}

func TestCategorizeDefaultNoIntrospection(t *testing.T) {
	provider := &mockCategorizeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := NewCategorize("ticket_type", "What type of ticket is this?", []string{"bug", "feature"})

	thought := newTestThought("test default no introspection")
	thought.SetContent(context.Background(), "ticket_text", "Login broken", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Use Scan to verify classification
	resp, err := step.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if resp.Primary != "bug" {
		t.Errorf("expected primary 'bug', got %q", resp.Primary)
	}

	// Verify summary note does NOT exist (introspection disabled by default)
	_, err = result.GetContent("ticket_type_summary")
	if err == nil {
		t.Error("expected summary note to not exist by default")
	}

	// Verify mockProvider was called only once (Classification only)
	if provider.callCount != 1 {
		t.Errorf("expected 1 provider call, got %d", provider.callCount)
	}
}

func TestCategorizeWithIntrospection(t *testing.T) {
	provider := &mockCategorizeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := NewCategorize("ticket_type", "What type of ticket is this?", []string{"bug", "feature"}).
		WithIntrospection()

	thought := newTestThought("test with introspection")
	thought.SetContent(context.Background(), "ticket_text", "Login broken", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Use Scan to verify classification
	resp, err := step.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if resp.Primary != "bug" {
		t.Errorf("expected primary 'bug', got %q", resp.Primary)
	}

	// Verify summary note exists (introspection enabled)
	summary, err := result.GetContent("ticket_type_summary")
	if err != nil {
		t.Fatalf("summary note not found: %v", err)
	}

	if summary == "" {
		t.Error("expected non-empty summary")
	}

	// Verify it contains semantic context
	if !strings.Contains(summary, "Bug") {
		t.Errorf("expected summary to contain semantic context, got: %q", summary)
	}

	// Verify mockProvider was called twice (Classification + Transform)
	if provider.callCount != 2 {
		t.Errorf("expected 2 provider calls, got %d", provider.callCount)
	}
}

func TestCategorizeWithSummaryKey(t *testing.T) {
	provider := &mockCategorizeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := NewCategorize("ticket_type", "What type of ticket is this?", []string{"bug", "feature"}).
		WithIntrospection().
		WithSummaryKey("context")

	thought := newTestThought("test custom summary key")
	thought.SetContent(context.Background(), "ticket_text", "Login broken", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify summary is at custom key
	summary, err := result.GetContent("context")
	if err != nil {
		t.Fatalf("custom summary note not found: %v", err)
	}

	if summary == "" {
		t.Error("expected non-empty summary at custom key")
	}

	// Verify default summary key does NOT exist
	_, err = result.GetContent("ticket_type_summary")
	if err == nil {
		t.Error("expected default summary key to not exist")
	}
}

func TestCategorizeAutoContextAccumulation(t *testing.T) {
	provider := &mockCategorizeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := NewCategorize("ticket_type", "What type of ticket is this?", []string{"bug", "feature"})

	thought := newTestThought("test auto-context")
	// Add multiple notes - all should be sent as context
	thought.SetContent(context.Background(), "ticket_text", "System is broken", "initial")
	thought.SetContent(context.Background(), "user_tier", "premium", "initial")
	thought.SetContent(context.Background(), "time_of_day", "3am", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify classification was made (means all notes were sent)
	_, err = result.GetContent("ticket_type")
	if err != nil {
		t.Fatalf("ticket_type note not found: %v", err)
	}
}

func TestCategorizePublishTracking(t *testing.T) {
	provider := &mockCategorizeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := NewCategorize("result", "What is this?", []string{"a", "b"})

	thought := newTestThought("test publish tracking")
	thought.SetContent(context.Background(), "note1", "First", "initial")
	thought.SetContent(context.Background(), "note2", "Second", "initial")

	// Initially nothing published
	unpublished := thought.GetUnpublishedNotes()
	if len(unpublished) != 2 {
		t.Errorf("expected 2 unpublished notes, got %d", len(unpublished))
	}

	// Run step - should publish all notes including the result
	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// After step, all notes should be published
	unpublished = result.GetUnpublishedNotes()
	if len(unpublished) != 0 {
		t.Errorf("expected 0 unpublished notes after step, got %d", len(unpublished))
	}

	// Add another note - this should be unpublished
	result.SetContent(context.Background(), "note3", "Third", "custom")

	unpublished = result.GetUnpublishedNotes()
	if len(unpublished) != 1 {
		t.Errorf("expected 1 unpublished note (note3), got %d", len(unpublished))
	}

	if unpublished[0].Key != "note3" {
		t.Errorf("expected unpublished note key 'note3', got %q", unpublished[0].Key)
	}
}

func TestCategorizeBuilderComposition(t *testing.T) {
	provider := &mockCategorizeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	// Chain multiple builder methods
	step := NewCategorize("ticket_type", "What type of ticket is this?", []string{"bug", "feature"}).
		WithIntrospection().
		WithSummaryKey("context").
		WithReasoningTemperature(0.3).
		WithIntrospectionTemperature(0.8)

	thought := newTestThought("test builder composition")
	thought.SetContent(context.Background(), "ticket_text", "Login broken", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Use Scan to verify classification
	resp, err := step.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if resp.Primary != "bug" {
		t.Errorf("expected primary 'bug', got %q", resp.Primary)
	}

	// Verify custom summary key
	summary, err := result.GetContent("context")
	if err != nil {
		t.Fatalf("custom summary note not found: %v", err)
	}

	if summary == "" {
		t.Error("expected non-empty summary at custom key")
	}

	// Verify 2 calls made
	if provider.callCount != 2 {
		t.Errorf("expected 2 provider calls, got %d", provider.callCount)
	}
}
