package cogito

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/zoobzio/zyn"
)

// mockPrioritizeProvider implements Provider interface for testing Prioritize.
type mockPrioritizeProvider struct {
	callCount int
}

func (m *mockPrioritizeProvider) Call(ctx context.Context, messages []zyn.Message, temperature float32) (*zyn.ProviderResponse, error) {
	m.callCount++

	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	// Check last message to determine which synapse is calling
	lastMessage := messages[len(messages)-1]

	// Transform synapse call - check first since it's more specific
	if strings.Contains(lastMessage.Content, "Transform:") {
		return &zyn.ProviderResponse{
			Content: `{"output": "Critical outage takes highest priority requiring immediate response, followed by authentication issues affecting active users", "confidence": 0.89, "changes": ["Synthesized ranking context"], "reasoning": ["Combined priority with context"]}`,
			Usage: zyn.TokenUsage{
				Prompt:     15,
				Completion: 25,
				Total:      40,
			},
		}, nil
	}

	// Ranking synapse call (contains "Items:" or ranking-related content)
	if strings.Contains(lastMessage.Content, "Items:") || strings.Contains(lastMessage.Content, "Rank") {
		return &zyn.ProviderResponse{
			Content: `{"ranked": ["Critical outage in production", "Login bug affecting users", "Minor UI glitch"], "confidence": 0.92, "reasoning": ["Critical outage has highest business impact", "Login issues prevent user access", "UI glitch is cosmetic"]}`,
			Usage: zyn.TokenUsage{
				Prompt:     10,
				Completion: 20,
				Total:      30,
			},
		}, nil
	}

	// Default fallback - return simple ranking
	return &zyn.ProviderResponse{
		Content: `{"ranked": ["Item 1", "Item 2"], "confidence": 0.7, "reasoning": ["Default ranking"]}`,
		Usage: zyn.TokenUsage{
			Prompt:     10,
			Completion: 10,
			Total:      20,
		},
	}, nil
}

func (m *mockPrioritizeProvider) Name() string {
	return "mock"
}

func TestPrioritizeBasic(t *testing.T) {
	provider := &mockPrioritizeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	items := []string{
		"Critical outage in production",
		"Login bug affecting users",
		"Minor UI glitch",
	}
	step := NewPrioritize("priority", "urgency and business impact", items)

	thought := newTestThought("test ranking")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Use Scan to get typed response
	resp, err := step.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if len(resp.Ranked) != 3 {
		t.Errorf("expected 3 ranked items, got %d", len(resp.Ranked))
	}

	if resp.Ranked[0] != "Critical outage in production" {
		t.Errorf("expected first item 'Critical outage in production', got %q", resp.Ranked[0])
	}

	if resp.Confidence != 0.92 {
		t.Errorf("expected confidence 0.92, got %f", resp.Confidence)
	}

	if len(resp.Reasoning) == 0 {
		t.Error("expected reasoning to be present")
	}
}

func TestPrioritizeDefaultNoIntrospection(t *testing.T) {
	provider := &mockPrioritizeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	items := []string{"Item A", "Item B"}
	step := NewPrioritize("priority", "urgency", items)

	thought := newTestThought("test default no introspection")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify ranking exists
	_, err = result.GetContent("priority")
	if err != nil {
		t.Fatalf("priority note not found: %v", err)
	}

	// Verify summary note does NOT exist (introspection disabled by default)
	_, err = result.GetContent("priority_summary")
	if err == nil {
		t.Error("expected summary note to not exist by default")
	}

	// Verify mockProvider was called only once (Ranking only)
	if provider.callCount != 1 {
		t.Errorf("expected 1 provider call, got %d", provider.callCount)
	}
}

func TestPrioritizeWithIntrospection(t *testing.T) {
	provider := &mockPrioritizeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	items := []string{"Item A", "Item B", "Item C"}
	step := NewPrioritize("priority", "urgency", items).
		WithIntrospection()

	thought := newTestThought("test with introspection")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify ranking exists
	_, err = result.GetContent("priority")
	if err != nil {
		t.Fatalf("priority note not found: %v", err)
	}

	// Verify summary note exists (introspection enabled)
	summary, err := result.GetContent("priority_summary")
	if err != nil {
		t.Fatalf("summary note not found: %v", err)
	}

	if summary == "" {
		t.Error("expected non-empty summary")
	}

	// Verify it contains semantic context
	if !strings.Contains(summary, "Critical outage") {
		t.Errorf("expected summary to contain semantic context, got: %q", summary)
	}

	// Verify mockProvider was called twice (Ranking + Transform)
	if provider.callCount != 2 {
		t.Errorf("expected 2 provider calls, got %d", provider.callCount)
	}
}

func TestPrioritizeWithSummaryKey(t *testing.T) {
	provider := &mockPrioritizeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	items := []string{"Item A", "Item B"}
	step := NewPrioritize("priority", "urgency", items).
		WithIntrospection().
		WithSummaryKey("context")

	thought := newTestThought("test custom summary key")

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
	_, err = result.GetContent("priority_summary")
	if err == nil {
		t.Error("expected default summary key to not exist")
	}
}

func TestNewPrioritizeFrom(t *testing.T) {
	provider := &mockPrioritizeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	// First, set up items in a note as JSON array
	thought := newTestThought("test rank from note")
	items := []string{
		"Critical outage in production",
		"Login bug affecting users",
		"Minor UI glitch",
	}
	itemsJSON, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("failed to marshal items: %v", err)
	}
	thought.SetContent(context.Background(), "items_list", string(itemsJSON), "prep")

	// Now rank from that note
	step := NewPrioritizeFrom("priority", "urgency and impact", "items_list")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Use Scan to get typed response
	resp, err := step.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if len(resp.Ranked) != 3 {
		t.Errorf("expected 3 ranked items, got %d", len(resp.Ranked))
	}

	if resp.Ranked[0] != "Critical outage in production" {
		t.Errorf("expected first item 'Critical outage in production', got %q", resp.Ranked[0])
	}
}

func TestPrioritizeFromMissingNote(t *testing.T) {
	provider := &mockPrioritizeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	thought := newTestThought("test rank from missing note")

	// Try to rank from note that doesn't exist
	step := NewPrioritizeFrom("priority", "urgency", "nonexistent_key")

	_, err := step.Process(context.Background(), thought)
	if err == nil {
		t.Fatal("expected error when items note not found")
	}

	if !strings.Contains(err.Error(), "nonexistent_key") {
		t.Errorf("expected error to mention missing key, got: %v", err)
	}
}

func TestPrioritizeFromInvalidJSON(t *testing.T) {
	provider := &mockPrioritizeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	thought := newTestThought("test rank from invalid json")
	thought.SetContent(context.Background(), "items_list", "not valid json", "prep")

	step := NewPrioritizeFrom("priority", "urgency", "items_list")

	_, err := step.Process(context.Background(), thought)
	if err == nil {
		t.Fatal("expected error when JSON is invalid")
	}

	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("expected parse error, got: %v", err)
	}
}
