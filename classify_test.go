package cogito

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zoobzio/zyn"
)

// mockClassifyProvider implements Provider interface for testing Classify
type mockClassifyProvider struct {
	callCount int
}

func (m *mockClassifyProvider) Call(ctx context.Context, messages []zyn.Message, temperature float32) (*zyn.ProviderResponse, error) {
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

func (m *mockClassifyProvider) Name() string {
	return "mock"
}

func TestClassifyBasic(t *testing.T) {
	provider := &mockClassifyProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Classify("ticket_type", "What type of ticket is this?", []string{"bug", "feature", "question", "documentation"})

	thought := New("test classification")
	thought.SetContent("ticket_text", "Login button doesn't work", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check category note was written
	category, err := result.GetContent("ticket_type")
	if err != nil {
		t.Fatalf("ticket_type note not found: %v", err)
	}

	if category != "bug" {
		t.Errorf("expected category 'bug', got %q", category)
	}

	// Check metadata
	note, ok := result.GetNote("ticket_type")
	if !ok {
		t.Fatal("note not found")
	}

	confidence, ok := note.Metadata["confidence"]
	if !ok {
		t.Error("confidence metadata not found")
	}

	if confidence != "0.87" {
		t.Errorf("expected confidence '0.87', got %q", confidence)
	}

	secondary, ok := note.Metadata["secondary"]
	if !ok {
		t.Error("secondary metadata not found")
	}

	if secondary != "feature" {
		t.Errorf("expected secondary 'feature', got %q", secondary)
	}

	// Check reasoning was captured
	if _, ok := note.Metadata["reasoning_0"]; !ok {
		t.Error("reasoning not captured in metadata")
	}
}

func TestClassifyIntrospection(t *testing.T) {
	provider := &mockClassifyProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Classify("ticket_type", "What type of ticket is this?", []string{"bug", "feature"})

	thought := New("test introspection")
	thought.SetContent("ticket_text", "Login broken", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify category note exists
	category, err := result.GetContent("ticket_type")
	if err != nil {
		t.Fatalf("ticket_type note not found: %v", err)
	}

	if category != "bug" {
		t.Errorf("expected category 'bug', got %q", category)
	}

	// Verify summary note exists (introspection enabled by default)
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

func TestClassifyWithoutIntrospection(t *testing.T) {
	provider := &mockClassifyProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Classify("ticket_type", "What type of ticket is this?", []string{"bug", "feature"}).
		WithoutIntrospection()

	thought := New("test without introspection")
	thought.SetContent("ticket_text", "Login broken", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify classification exists
	category, err := result.GetContent("ticket_type")
	if err != nil {
		t.Fatalf("ticket_type note not found: %v", err)
	}

	if category != "bug" {
		t.Errorf("expected category 'bug', got %q", category)
	}

	// Verify summary note does NOT exist
	_, err = result.GetContent("ticket_type_summary")
	if err == nil {
		t.Error("expected summary note to not exist when introspection disabled")
	}

	// Verify mockProvider was called only once (Classification only)
	if provider.callCount != 1 {
		t.Errorf("expected 1 provider call, got %d", provider.callCount)
	}
}

func TestClassifyWithSummaryKey(t *testing.T) {
	provider := &mockClassifyProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Classify("ticket_type", "What type of ticket is this?", []string{"bug", "feature"}).
		WithSummaryKey("context")

	thought := New("test custom summary key")
	thought.SetContent("ticket_text", "Login broken", "initial")

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

func TestClassifyAutoContextAccumulation(t *testing.T) {
	provider := &mockClassifyProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Classify("ticket_type", "What type of ticket is this?", []string{"bug", "feature"})

	thought := New("test auto-context")
	// Add multiple notes - all should be sent as context
	thought.SetContent("ticket_text", "System is broken", "initial")
	thought.SetContent("user_tier", "premium", "initial")
	thought.SetContent("time_of_day", "3am", "initial")

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

func TestClassifyPublishTracking(t *testing.T) {
	provider := &mockClassifyProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Classify("result", "What is this?", []string{"a", "b"})

	thought := New("test publish tracking")
	thought.SetContent("note1", "First", "initial")
	thought.SetContent("note2", "Second", "initial")

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
	result.SetContent("note3", "Third", "custom")

	unpublished = result.GetUnpublishedNotes()
	if len(unpublished) != 1 {
		t.Errorf("expected 1 unpublished note (note3), got %d", len(unpublished))
	}

	if unpublished[0].Key != "note3" {
		t.Errorf("expected unpublished note key 'note3', got %q", unpublished[0].Key)
	}
}

func TestClassifyBuilderComposition(t *testing.T) {
	provider := &mockClassifyProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	// Chain multiple builder methods
	step := Classify("ticket_type", "What type of ticket is this?", []string{"bug", "feature"}).
		WithSummaryKey("context").
		WithReasoningTemperature(0.3).
		WithIntrospectionTemperature(0.8)

	thought := New("test builder composition")
	thought.SetContent("ticket_text", "Login broken", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify classification exists
	category, err := result.GetContent("ticket_type")
	if err != nil {
		t.Fatalf("ticket_type note not found: %v", err)
	}

	if category != "bug" {
		t.Errorf("expected category 'bug', got %q", category)
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

func TestClassifyStepRecord(t *testing.T) {
	provider := &mockClassifyProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Classify("ticket_type", "What type of ticket is this?", []string{"bug", "feature"})

	thought := New("test step recording")
	thought.SetContent("ticket_text", "Test input", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check step was recorded
	if len(result.Steps) == 0 {
		t.Fatal("expected step record to be added")
	}

	step1 := result.Steps[0]
	if step1.Name != "ticket_type" {
		t.Errorf("expected step name 'ticket_type', got %q", step1.Name)
	}

	if step1.Type != "classify" {
		t.Errorf("expected step type 'classify', got %q", step1.Type)
	}

	if step1.Duration == 0 {
		t.Error("expected non-zero duration")
	}
}
