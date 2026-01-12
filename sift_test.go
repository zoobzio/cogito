package cogito

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zoobzio/pipz"
	"github.com/zoobzio/zyn"
)

// mockSiftProvider implements Provider interface for testing Sift.
type mockSiftProvider struct {
	callCount     int
	decisionValue bool
}

func (m *mockSiftProvider) Call(ctx context.Context, messages []zyn.Message, temperature float32) (*zyn.ProviderResponse, error) {
	m.callCount++

	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	lastMessage := messages[len(messages)-1]

	// Transform synapse call
	if strings.Contains(lastMessage.Content, "Transform:") {
		return &zyn.ProviderResponse{
			Content: `{"output": "Gate decision synthesized: escalation required based on urgency indicators.", "confidence": 0.92, "changes": ["Synthesized gate context"], "reasoning": ["Combined decision with original context"]}`,
			Usage: zyn.TokenUsage{
				Prompt:     15,
				Completion: 25,
				Total:      40,
			},
		}, nil
	}

	// Binary synapse call
	return &zyn.ProviderResponse{
		Content: fmt.Sprintf(`{"decision": %t, "confidence": 0.95, "reasoning": ["Based on context analysis"]}`, m.decisionValue),
		Usage: zyn.TokenUsage{
			Prompt:     10,
			Completion: 20,
			Total:      30,
		},
	}, nil
}

func (m *mockSiftProvider) Name() string {
	return "mock-sift"
}

// mockProcessor tracks whether it was executed.
type mockProcessor struct {
	identity pipz.Identity
	executed bool
	name     string
}

func newMockProcessor(name string) *mockProcessor {
	return &mockProcessor{
		identity: pipz.NewIdentity(name, "Mock processor for "+name),
		name:     name,
	}
}

func (m *mockProcessor) Process(ctx context.Context, t *Thought) (*Thought, error) {
	m.executed = true
	t.SetContent(ctx, "processor_output", "processor was executed", m.name)
	return t, nil
}

func (m *mockProcessor) Identity() pipz.Identity {
	return m.identity
}

func (m *mockProcessor) Schema() pipz.Node {
	return pipz.Node{Identity: m.identity, Type: "mock-processor"}
}

func (m *mockProcessor) Close() error {
	return nil
}

func TestSiftGateOpens(t *testing.T) {
	provider := &mockSiftProvider{decisionValue: true}
	SetProvider(provider)
	defer SetProvider(nil)

	processor := newMockProcessor("test-processor")
	sift := NewSift("escalation_gate", "Does this require escalation?", processor)

	thought := newTestThought("test sift gate opens")
	thought.SetContent(context.Background(), "ticket", "URGENT: Production is down!", "initial")

	result, err := sift.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify gate decision was recorded
	resp, err := sift.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if !resp.Decision {
		t.Error("expected gate to open (decision true)")
	}

	// Verify processor was executed
	if !processor.executed {
		t.Error("expected processor to be executed when gate opens")
	}

	// Verify processor output exists
	_, err = result.GetContent("processor_output")
	if err != nil {
		t.Error("expected processor output note to exist")
	}
}

func TestSiftGateClosed(t *testing.T) {
	provider := &mockSiftProvider{decisionValue: false}
	SetProvider(provider)
	defer SetProvider(nil)

	processor := newMockProcessor("test-processor")
	sift := NewSift("escalation_gate", "Does this require escalation?", processor)

	thought := newTestThought("test sift gate closed")
	thought.SetContent(context.Background(), "ticket", "Minor: CSS alignment issue", "initial")

	result, err := sift.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify gate decision was recorded
	resp, err := sift.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if resp.Decision {
		t.Error("expected gate to stay closed (decision false)")
	}

	// Verify processor was NOT executed
	if processor.executed {
		t.Error("expected processor to NOT be executed when gate is closed")
	}

	// Verify processor output does NOT exist
	_, err = result.GetContent("processor_output")
	if err == nil {
		t.Error("expected processor output note to NOT exist")
	}
}

func TestSiftWithIntrospection(t *testing.T) {
	provider := &mockSiftProvider{decisionValue: true}
	SetProvider(provider)
	defer SetProvider(nil)

	processor := newMockProcessor("test-processor")
	sift := NewSift("escalation_gate", "Does this require escalation?", processor).
		WithIntrospection()

	thought := newTestThought("test sift with introspection")
	thought.SetContent(context.Background(), "ticket", "URGENT: Production is down!", "initial")

	result, err := sift.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify summary note exists
	summary, err := result.GetContent("escalation_gate_summary")
	if err != nil {
		t.Fatalf("summary note not found: %v", err)
	}

	if summary == "" {
		t.Error("expected non-empty summary")
	}

	// Verify provider was called twice (Binary + Transform)
	if provider.callCount != 2 {
		t.Errorf("expected 2 provider calls, got %d", provider.callCount)
	}
}

func TestSiftNoIntrospectionByDefault(t *testing.T) {
	provider := &mockSiftProvider{decisionValue: true}
	SetProvider(provider)
	defer SetProvider(nil)

	processor := newMockProcessor("test-processor")
	sift := NewSift("escalation_gate", "Does this require escalation?", processor)

	thought := newTestThought("test sift no introspection")
	thought.SetContent(context.Background(), "ticket", "URGENT: Production is down!", "initial")

	result, err := sift.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify summary note does NOT exist
	_, err = result.GetContent("escalation_gate_summary")
	if err == nil {
		t.Error("expected summary note to NOT exist by default")
	}

	// Verify provider was called once (Binary only)
	if provider.callCount != 1 {
		t.Errorf("expected 1 provider call, got %d", provider.callCount)
	}
}

func TestSiftWithCustomSummaryKey(t *testing.T) {
	provider := &mockSiftProvider{decisionValue: true}
	SetProvider(provider)
	defer SetProvider(nil)

	processor := newMockProcessor("test-processor")
	sift := NewSift("escalation_gate", "Does this require escalation?", processor).
		WithIntrospection().
		WithSummaryKey("custom_gate_context")

	thought := newTestThought("test sift custom summary key")
	thought.SetContent(context.Background(), "ticket", "URGENT: Production is down!", "initial")

	result, err := sift.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify custom summary key exists
	_, err = result.GetContent("custom_gate_context")
	if err != nil {
		t.Error("expected custom summary key to exist")
	}

	// Verify default summary key does NOT exist
	_, err = result.GetContent("escalation_gate_summary")
	if err == nil {
		t.Error("expected default summary key to NOT exist")
	}
}

func TestSiftBuilderMethods(t *testing.T) {
	provider := &mockSiftProvider{decisionValue: true}
	SetProvider(provider)
	defer SetProvider(nil)

	processor := newMockProcessor("test-processor")
	sift := NewSift("escalation_gate", "Does this require escalation?", processor).
		WithIntrospection().
		WithSummaryKey("context").
		WithTemperature(0.5).
		WithReasoningTemperature(0.1).
		WithIntrospectionTemperature(0.8)

	thought := newTestThought("test sift builder methods")
	thought.SetContent(context.Background(), "ticket", "URGENT: Production is down!", "initial")

	result, err := sift.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify custom summary key
	_, err = result.GetContent("context")
	if err != nil {
		t.Error("expected custom summary key to exist")
	}

	// Verify provider was called twice
	if provider.callCount != 2 {
		t.Errorf("expected 2 provider calls, got %d", provider.callCount)
	}
}

func TestSiftSetProcessor(t *testing.T) {
	provider := &mockSiftProvider{decisionValue: true}
	SetProvider(provider)
	defer SetProvider(nil)

	originalProcessor := newMockProcessor("original-processor")
	newProcessor := newMockProcessor("new-processor")

	sift := NewSift("escalation_gate", "Does this require escalation?", originalProcessor)
	sift.SetProcessor(newProcessor)

	thought := newTestThought("test sift set processor")
	thought.SetContent(context.Background(), "ticket", "URGENT: Production is down!", "initial")

	_, err := sift.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify new processor was executed, not original
	if originalProcessor.executed {
		t.Error("expected original processor to NOT be executed")
	}

	if !newProcessor.executed {
		t.Error("expected new processor to be executed")
	}
}

func TestSiftName(t *testing.T) {
	processor := newMockProcessor("test-processor")
	sift := NewSift("my_gate", "Some question?", processor)

	if sift.Identity().Name() != "my_gate" {
		t.Errorf("expected name 'my_gate', got %q", sift.Identity().Name())
	}
}

func TestSiftClose(t *testing.T) {
	processor := newMockProcessor("test-processor")
	sift := NewSift("my_gate", "Some question?", processor)

	// Close should propagate to processor (which returns nil)
	err := sift.Close()
	if err != nil {
		t.Errorf("unexpected error on close: %v", err)
	}
}
