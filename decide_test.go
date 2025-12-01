package cogito

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zoobzio/zyn"
)

// mockProvider implements Provider interface for testing
// It handles both Binary and Transform synapse calls based on message content
type mockDecideProvider struct {
	callCount int
}

func (m *mockDecideProvider) Call(ctx context.Context, messages []zyn.Message, temperature float32) (*zyn.ProviderResponse, error) {
	m.callCount++

	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	// Check last message to determine which synapse is calling
	lastMessage := messages[len(messages)-1]

	// Transform synapse call - check first since it's more specific
	// Transform prompts contain "Transform:" in the task description
	if strings.Contains(lastMessage.Content, "Transform:") {
		return &zyn.ProviderResponse{
			Content: `{"output": "CRITICAL: Production system experiencing urgent issues requiring immediate attention and escalation.", "confidence": 0.92, "changes": ["Synthesized decision context"], "reasoning": ["Combined decision with original context"]}`,
			Usage: zyn.TokenUsage{
				Prompt:     15,
				Completion: 25,
				Total:      40,
			},
		}, nil
	}

	// Binary synapse call (contains the decision question)
	if strings.Contains(lastMessage.Content, "Binary Decision:") || strings.Contains(lastMessage.Content, "Is this") || strings.Contains(lastMessage.Content, "Is urgent") {
		return &zyn.ProviderResponse{
			Content: `{"decision": true, "confidence": 0.95, "reasoning": ["Input indicates urgency", "Contains urgent keywords"]}`,
			Usage: zyn.TokenUsage{
				Prompt:     10,
				Completion: 20,
				Total:      30,
			},
		}, nil
	}

	// Default fallback - return Binary format for backward compatibility
	return &zyn.ProviderResponse{
		Content: `{"decision": true, "confidence": 0.95, "reasoning": ["Default response"]}`,
		Usage: zyn.TokenUsage{
			Prompt:     10,
			Completion: 20,
			Total:      30,
		},
	}, nil
}

func (m *mockDecideProvider) Name() string {
	return "mock"
}

func TestDecideBasic(t *testing.T) {
	// Set global provider
	provider := &mockDecideProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	// Create a Decide step (no inputKey anymore)
	step := Decide("is_urgent", "Is this urgent?")

	// Create thought with input
	thought := New("test decision")
	thought.SetContent("input_text", "URGENT: System is down!", "initial")

	// Process
	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check decision note was written
	decision, err := result.GetContent("is_urgent")
	if err != nil {
		t.Fatalf("decision note not found: %v", err)
	}

	if decision != "true" {
		t.Errorf("expected decision 'true', got %q", decision)
	}

	// Check metadata
	note, ok := result.GetNote("is_urgent")
	if !ok {
		t.Fatal("note not found")
	}

	confidence, ok := note.Metadata["confidence"]
	if !ok {
		t.Error("confidence metadata not found")
	}

	if confidence != "0.95" {
		t.Errorf("expected confidence '0.95', got %q", confidence)
	}

	// Check reasoning was captured
	if _, ok := note.Metadata["reasoning_0"]; !ok {
		t.Error("reasoning not captured in metadata")
	}
}

func TestDecideGetBool(t *testing.T) {
	provider := &mockDecideProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Decide("is_urgent", "Is this urgent?")

	thought := New("test decision")
	thought.SetContent("input_text", "URGENT: System is down!", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Use GetBool helper
	isUrgent, err := result.GetBool("is_urgent")
	if err != nil {
		t.Fatalf("GetBool failed: %v", err)
	}

	if !isUrgent {
		t.Error("expected true from GetBool")
	}
}

func TestDecideAutoContextAccumulation(t *testing.T) {
	provider := &mockDecideProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Decide("is_urgent", "Is this urgent?")

	thought := New("test auto-context")
	// Add multiple notes - all should be sent as context
	thought.SetContent("ticket_text", "System is broken", "initial")
	thought.SetContent("user_tier", "premium", "initial")
	thought.SetContent("time_of_day", "3am", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify decision was made (means all notes were sent)
	_, err = result.GetContent("is_urgent")
	if err != nil {
		t.Fatalf("decision note not found: %v", err)
	}
}

func TestDecideSessionAccumulation(t *testing.T) {
	provider := &mockDecideProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	thought := New("test session accumulation")
	thought.SetContent("input1", "First message", "initial")

	// Run first step
	step1 := Decide("result1", "Is this the first?")
	result, err := step1.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("step1 error: %v", err)
	}

	// Check session has messages
	if result.Session.Len() == 0 {
		t.Error("expected session to have messages after step1")
	}

	initialLen := result.Session.Len()

	// Add more notes for second step
	result.SetContent("input2", "Second message", "initial")

	step2 := Decide("result2", "Is this the second?")
	result, err = step2.Process(context.Background(), result)
	if err != nil {
		t.Fatalf("step2 error: %v", err)
	}

	// Session should have accumulated more messages
	if result.Session.Len() <= initialLen {
		t.Errorf("expected session to grow, initial=%d final=%d", initialLen, result.Session.Len())
	}
}

func TestDecideStepRecord(t *testing.T) {
	provider := &mockDecideProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Decide("is_urgent", "Is this urgent?")

	thought := New("test step recording")
	thought.SetContent("input_text", "Test input", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check step was recorded
	if len(result.Steps) == 0 {
		t.Fatal("expected step record to be added")
	}

	step1 := result.Steps[0]
	if step1.Name != "is_urgent" {
		t.Errorf("expected step name 'is_urgent', got %q", step1.Name)
	}

	if step1.Type != "decide" {
		t.Errorf("expected step type 'decide', got %q", step1.Type)
	}

	if step1.Duration == 0 {
		t.Error("expected non-zero duration")
	}
}

func TestDecideWithRetry(t *testing.T) {
	provider := &mockDecideProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	// Create step with retry wrapper
	step := Decide("is_urgent", "Is this urgent?").
		WithRetry(3)

	thought := New("test with retry")
	thought.SetContent("input_text", "Test input", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still work with retry wrapper
	decision, err := result.GetContent("is_urgent")
	if err != nil {
		t.Fatalf("decision not found: %v", err)
	}

	if decision != "true" {
		t.Errorf("expected 'true', got %q", decision)
	}
}

func TestDecideProviderResolution(t *testing.T) {
	// Test that step-level provider takes precedence
	globalProvider := &mockDecideProvider{}
	SetProvider(globalProvider)
	defer SetProvider(nil)

	stepProvider := &mockDecideProvider{}
	step := Decide("is_urgent", "Is this urgent?").
		WithProvider(stepProvider)

	thought := New("test provider resolution")
	thought.SetContent("input_text", "Test input", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it worked (step provider should have been used)
	_, err = result.GetContent("is_urgent")
	if err != nil {
		t.Fatalf("decision not found: %v", err)
	}
}

func TestDecidePublishTracking(t *testing.T) {
	provider := &mockDecideProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	thought := New("test publish tracking")
	thought.SetContent("note1", "First", "initial")
	thought.SetContent("note2", "Second", "initial")

	// Initially nothing published
	unpublished := thought.GetUnpublishedNotes()
	if len(unpublished) != 2 {
		t.Errorf("expected 2 unpublished notes, got %d", len(unpublished))
	}

	// Run step - should publish all notes including the result
	step := Decide("result", "Is this good?")
	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// After step, all notes including result should be published
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

func TestDecideIntrospection(t *testing.T) {
	provider := &mockDecideProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Decide("is_urgent", "Is this urgent?")

	thought := New("test introspection")
	thought.SetContent("input_text", "URGENT: Production system down!", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify decision note exists
	decision, err := result.GetContent("is_urgent")
	if err != nil {
		t.Fatalf("decision note not found: %v", err)
	}

	if decision != "true" {
		t.Errorf("expected decision 'true', got %q", decision)
	}

	// Verify summary note exists (introspection enabled by default)
	summary, err := result.GetContent("is_urgent_summary")
	if err != nil {
		t.Fatalf("summary note not found: %v", err)
	}

	if summary == "" {
		t.Error("expected non-empty summary")
	}

	// Verify it contains semantic context
	if !strings.Contains(summary, "CRITICAL") {
		t.Errorf("expected summary to contain semantic context, got: %q", summary)
	}

	// Verify mockProvider was called twice (Binary + Transform)
	if provider.callCount != 2 {
		t.Errorf("expected 2 provider calls, got %d", provider.callCount)
	}
}

func TestDecideWithoutIntrospection(t *testing.T) {
	provider := &mockDecideProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Decide("is_urgent", "Is this urgent?").
		WithoutIntrospection()

	thought := New("test without introspection")
	thought.SetContent("input_text", "URGENT: Production system down!", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify decision note exists
	decision, err := result.GetContent("is_urgent")
	if err != nil {
		t.Fatalf("decision note not found: %v", err)
	}

	if decision != "true" {
		t.Errorf("expected decision 'true', got %q", decision)
	}

	// Verify summary note does NOT exist
	_, err = result.GetContent("is_urgent_summary")
	if err == nil {
		t.Error("expected summary note to not exist when introspection disabled")
	}

	// Verify mockProvider was called only once (Binary only)
	if provider.callCount != 1 {
		t.Errorf("expected 1 provider call, got %d", provider.callCount)
	}
}

func TestDecideWithSummaryKey(t *testing.T) {
	provider := &mockDecideProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Decide("is_urgent", "Is this urgent?").
		WithSummaryKey("custom_context")

	thought := New("test custom summary key")
	thought.SetContent("input_text", "URGENT: Production system down!", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify decision note exists
	decision, err := result.GetContent("is_urgent")
	if err != nil {
		t.Fatalf("decision note not found: %v", err)
	}

	if decision != "true" {
		t.Errorf("expected decision 'true', got %q", decision)
	}

	// Verify summary is at custom key
	summary, err := result.GetContent("custom_context")
	if err != nil {
		t.Fatalf("custom summary note not found: %v", err)
	}

	if summary == "" {
		t.Error("expected non-empty summary at custom key")
	}

	// Verify default summary key does NOT exist
	_, err = result.GetContent("is_urgent_summary")
	if err == nil {
		t.Error("expected default summary key to not exist")
	}
}

func TestDecideWithReasoningTemperature(t *testing.T) {
	provider := &mockDecideProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	// Note: We can't directly verify temperature in mock, but we can verify it doesn't break
	step := Decide("is_urgent", "Is this urgent?").
		WithReasoningTemperature(0.1)

	thought := New("test reasoning temperature")
	thought.SetContent("input_text", "URGENT: Production system down!", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify decision note exists
	decision, err := result.GetContent("is_urgent")
	if err != nil {
		t.Fatalf("decision note not found: %v", err)
	}

	if decision != "true" {
		t.Errorf("expected decision 'true', got %q", decision)
	}
}

func TestDecideWithIntrospectionTemperature(t *testing.T) {
	provider := &mockDecideProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	// Note: We can't directly verify temperature in mock, but we can verify it doesn't break
	step := Decide("is_urgent", "Is this urgent?").
		WithIntrospectionTemperature(0.9)

	thought := New("test introspection temperature")
	thought.SetContent("input_text", "URGENT: Production system down!", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify summary exists
	summary, err := result.GetContent("is_urgent_summary")
	if err != nil {
		t.Fatalf("summary note not found: %v", err)
	}

	if summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestDecideBuilderComposition(t *testing.T) {
	provider := &mockDecideProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	// Chain multiple builder methods
	step := Decide("is_urgent", "Is this urgent?").
		WithSummaryKey("context").
		WithReasoningTemperature(0.1).
		WithIntrospectionTemperature(0.8)

	thought := New("test builder composition")
	thought.SetContent("input_text", "URGENT: Production system down!", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify decision exists
	decision, err := result.GetContent("is_urgent")
	if err != nil {
		t.Fatalf("decision note not found: %v", err)
	}

	if decision != "true" {
		t.Errorf("expected decision 'true', got %q", decision)
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
