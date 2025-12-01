package cogito

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/zoobzio/zyn"
)

// Test extraction type
type TicketData struct {
	Severity  string `json:"severity"`
	Component string `json:"component"`
	UserTier  string `json:"user_tier"`
}

func (t TicketData) Validate() error {
	if t.Severity == "" {
		return fmt.Errorf("severity required")
	}
	return nil
}

// mockAnalyzeProvider implements Provider interface for testing Analyze
type mockAnalyzeProvider struct {
	callCount int
}

func (m *mockAnalyzeProvider) Call(ctx context.Context, messages []zyn.Message, temperature float32) (*zyn.ProviderResponse, error) {
	m.callCount++

	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	// Check last message to determine which synapse is calling
	lastMessage := messages[len(messages)-1]

	// Transform synapse call - check first since it's more specific
	if strings.Contains(lastMessage.Content, "Transform:") {
		return &zyn.ProviderResponse{
			Content: `{"output": "High-severity authentication issue affecting premium user requiring immediate escalation", "confidence": 0.92, "changes": ["Synthesized extraction context"], "reasoning": ["Combined fields with context"]}`,
			Usage: zyn.TokenUsage{
				Prompt:     15,
				Completion: 25,
				Total:      40,
			},
		}, nil
	}

	// Extract synapse call (Task starts with "Extract ")
	if strings.Contains(lastMessage.Content, "Task: Extract ") {
		return &zyn.ProviderResponse{
			Content: `{"severity": "high", "component": "authentication", "user_tier": "premium"}`,
			Usage: zyn.TokenUsage{
				Prompt:     10,
				Completion: 20,
				Total:      30,
			},
		}, nil
	}

	// Default fallback - return valid extraction for any unmatched case
	return &zyn.ProviderResponse{
		Content: `{"severity": "medium", "component": "unknown", "user_tier": "standard"}`,
		Usage: zyn.TokenUsage{
			Prompt:     10,
			Completion: 10,
			Total:      20,
		},
	}, nil
}

func (m *mockAnalyzeProvider) Name() string {
	return "mock"
}

func TestAnalyzeBasic(t *testing.T) {
	provider := &mockAnalyzeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Analyze[TicketData]("ticket_data", "ticket metadata")

	thought := New("test extraction")
	thought.SetContent("ticket_text", "URGENT: Login broken for premium user", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check main note exists
	jsonContent, err := result.GetContent("ticket_data")
	if err != nil {
		t.Fatalf("ticket_data note not found: %v", err)
	}

	if jsonContent == "" {
		t.Error("expected non-empty content")
	}

	// Parse JSON to verify structure
	var ticketData TicketData
	if err := json.Unmarshal([]byte(jsonContent), &ticketData); err != nil {
		t.Fatalf("failed to parse extracted JSON: %v", err)
	}

	// Verify extracted fields
	if ticketData.Severity != "high" {
		t.Errorf("expected severity 'high', got %q", ticketData.Severity)
	}

	if ticketData.Component != "authentication" {
		t.Errorf("expected component 'authentication', got %q", ticketData.Component)
	}

	if ticketData.UserTier != "premium" {
		t.Errorf("expected user_tier 'premium', got %q", ticketData.UserTier)
	}
}

func TestAnalyzeIntrospection(t *testing.T) {
	provider := &mockAnalyzeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Analyze[TicketData]("ticket_data", "ticket metadata")

	thought := New("test introspection")
	thought.SetContent("ticket_text", "URGENT: Login broken for premium user", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify extraction exists
	_, err = result.GetContent("ticket_data")
	if err != nil {
		t.Fatalf("ticket_data note not found: %v", err)
	}

	// Verify summary note exists (introspection enabled by default)
	summary, err := result.GetContent("ticket_data_summary")
	if err != nil {
		t.Fatalf("summary note not found: %v", err)
	}

	if summary == "" {
		t.Error("expected non-empty summary")
	}

	// Verify it contains semantic context
	if !strings.Contains(summary, "High-severity") {
		t.Errorf("expected summary to contain semantic context, got: %q", summary)
	}

	// Verify mockProvider was called twice (Extract + Transform)
	if provider.callCount != 2 {
		t.Errorf("expected 2 provider calls, got %d", provider.callCount)
	}
}

func TestAnalyzeWithoutIntrospection(t *testing.T) {
	provider := &mockAnalyzeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Analyze[TicketData]("ticket_data", "ticket metadata").
		WithoutIntrospection()

	thought := New("test without introspection")
	thought.SetContent("ticket_text", "URGENT: System down", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify extraction exists
	_, err = result.GetContent("ticket_data")
	if err != nil {
		t.Fatalf("ticket_data note not found: %v", err)
	}

	// Verify summary note does NOT exist
	_, err = result.GetContent("ticket_data_summary")
	if err == nil {
		t.Error("expected summary note to not exist when introspection disabled")
	}

	// Verify mockProvider was called only once (Extract only)
	if provider.callCount != 1 {
		t.Errorf("expected 1 provider call, got %d", provider.callCount)
	}
}

func TestAnalyzeWithSummaryKey(t *testing.T) {
	provider := &mockAnalyzeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Analyze[TicketData]("ticket_data", "ticket metadata").
		WithSummaryKey("context")

	thought := New("test custom summary key")
	thought.SetContent("ticket_text", "URGENT: System down", "initial")

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
	_, err = result.GetContent("ticket_data_summary")
	if err == nil {
		t.Error("expected default summary key to not exist")
	}
}

func TestAnalyzeAutoContextAccumulation(t *testing.T) {
	provider := &mockAnalyzeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Analyze[TicketData]("ticket_data", "ticket metadata")

	thought := New("test auto-context")
	// Add multiple notes - all should be sent as context
	thought.SetContent("ticket_text", "System is broken", "initial")
	thought.SetContent("user_tier", "premium", "initial")
	thought.SetContent("time_of_day", "3am", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify extraction was made (means all notes were sent)
	_, err = result.GetContent("ticket_data")
	if err != nil {
		t.Fatalf("ticket_data note not found: %v", err)
	}
}

func TestAnalyzePublishTracking(t *testing.T) {
	provider := &mockAnalyzeProvider{}
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
	step := Analyze[TicketData]("result", "ticket metadata")
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

func TestAnalyzeBuilderComposition(t *testing.T) {
	provider := &mockAnalyzeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	// Chain multiple builder methods
	step := Analyze[TicketData]("ticket_data", "ticket metadata").
		WithSummaryKey("context").
		WithReasoningTemperature(0.1).
		WithIntrospectionTemperature(0.8)

	thought := New("test builder composition")
	thought.SetContent("ticket_text", "URGENT: System down", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify extraction exists
	_, err = result.GetContent("ticket_data")
	if err != nil {
		t.Fatalf("ticket_data note not found: %v", err)
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

func TestAnalyzeStepRecord(t *testing.T) {
	provider := &mockAnalyzeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Analyze[TicketData]("ticket_data", "ticket metadata")

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
	if step1.Name != "ticket_data" {
		t.Errorf("expected step name 'parse-ticket', got %q", step1.Name)
	}

	if step1.Type != "analyze" {
		t.Errorf("expected step type 'analyze', got %q", step1.Type)
	}

	if step1.Duration == 0 {
		t.Error("expected non-zero duration")
	}
}
