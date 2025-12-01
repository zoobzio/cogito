package cogito

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/zoobzio/zyn"
)

// mockRankProvider implements Provider interface for testing Rank
type mockRankProvider struct {
	callCount int
}

func (m *mockRankProvider) Call(ctx context.Context, messages []zyn.Message, temperature float32) (*zyn.ProviderResponse, error) {
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

func (m *mockRankProvider) Name() string {
	return "mock"
}

func TestRankBasic(t *testing.T) {
	provider := &mockRankProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	items := []string{
		"Critical outage in production",
		"Login bug affecting users",
		"Minor UI glitch",
	}
	step := RankItems("priority", "urgency and business impact", items)

	thought := New("test ranking")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check ranked list
	ranked, err := result.GetContent("priority")
	if err != nil {
		t.Fatalf("priority note not found: %v", err)
	}

	// Should be newline-separated ranked items
	rankedItems := strings.Split(ranked, "\n")
	if len(rankedItems) != 3 {
		t.Errorf("expected 3 ranked items, got %d", len(rankedItems))
	}

	if rankedItems[0] != "Critical outage in production" {
		t.Errorf("expected first item 'Critical outage in production', got %q", rankedItems[0])
	}

	// Check metadata
	note, ok := result.GetNote("priority")
	if !ok {
		t.Fatal("note not found")
	}

	confidence, ok := note.Metadata["confidence"]
	if !ok {
		t.Error("confidence metadata not found")
	}

	if confidence != "0.92" {
		t.Errorf("expected confidence '0.92', got %q", confidence)
	}

	// Check reasoning was captured
	if _, ok := note.Metadata["reasoning_0"]; !ok {
		t.Error("reasoning not captured in metadata")
	}
}

func TestRankIntrospection(t *testing.T) {
	provider := &mockRankProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	items := []string{"Item A", "Item B", "Item C"}
	step := RankItems("priority", "urgency", items)

	thought := New("test introspection")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify ranking exists
	_, err = result.GetContent("priority")
	if err != nil {
		t.Fatalf("priority note not found: %v", err)
	}

	// Verify summary note exists (introspection enabled by default)
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

func TestRankWithoutIntrospection(t *testing.T) {
	provider := &mockRankProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	items := []string{"Item A", "Item B"}
	step := RankItems("priority", "urgency", items).
		WithoutIntrospection()

	thought := New("test without introspection")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify ranking exists
	_, err = result.GetContent("priority")
	if err != nil {
		t.Fatalf("priority note not found: %v", err)
	}

	// Verify summary note does NOT exist
	_, err = result.GetContent("priority_summary")
	if err == nil {
		t.Error("expected summary note to not exist when introspection disabled")
	}

	// Verify mockProvider was called only once (Ranking only)
	if provider.callCount != 1 {
		t.Errorf("expected 1 provider call, got %d", provider.callCount)
	}
}

func TestRankWithSummaryKey(t *testing.T) {
	provider := &mockRankProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	items := []string{"Item A", "Item B"}
	step := RankItems("priority", "urgency", items).
		WithSummaryKey("context")

	thought := New("test custom summary key")

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

func TestRankStepRecord(t *testing.T) {
	provider := &mockRankProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	items := []string{"A", "B", "C"}
	step := RankItems("priority", "urgency", items)

	thought := New("test step recording")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check step was recorded
	if len(result.Steps) == 0 {
		t.Fatal("expected step record to be added")
	}

	step1 := result.Steps[0]
	if step1.Name != "priority" {
		t.Errorf("expected step name 'prioritize', got %q", step1.Name)
	}

	if step1.Type != "rank" {
		t.Errorf("expected step type 'rank', got %q", step1.Type)
	}

	if step1.Duration == 0 {
		t.Error("expected non-zero duration")
	}
}

func TestRankFrom(t *testing.T) {
	provider := &mockRankProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	// First, set up items in a note as JSON array
	thought := New("test rank from note")
	items := []string{
		"Critical outage in production",
		"Login bug affecting users",
		"Minor UI glitch",
	}
	itemsJSON, _ := json.Marshal(items)
	thought.SetContent("items_list", string(itemsJSON), "prep")

	// Now rank from that note
	step := RankFrom("priority", "urgency and impact", "items_list")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check ranked list
	ranked, err := result.GetContent("priority")
	if err != nil {
		t.Fatalf("priority note not found: %v", err)
	}

	// Should be newline-separated ranked items
	rankedItems := strings.Split(ranked, "\n")
	if len(rankedItems) != 3 {
		t.Errorf("expected 3 ranked items, got %d", len(rankedItems))
	}

	if rankedItems[0] != "Critical outage in production" {
		t.Errorf("expected first item 'Critical outage in production', got %q", rankedItems[0])
	}
}

func TestRankFromMissingNote(t *testing.T) {
	provider := &mockRankProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	thought := New("test rank from missing note")

	// Try to rank from note that doesn't exist
	step := RankFrom("priority", "urgency", "nonexistent_key")

	_, err := step.Process(context.Background(), thought)
	if err == nil {
		t.Fatal("expected error when items note not found")
	}

	if !strings.Contains(err.Error(), "nonexistent_key") {
		t.Errorf("expected error to mention missing key, got: %v", err)
	}
}

func TestRankFromInvalidJSON(t *testing.T) {
	provider := &mockRankProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	thought := New("test rank from invalid json")
	thought.SetContent("items_list", "not valid json", "prep")

	step := RankFrom("priority", "urgency", "items_list")

	_, err := step.Process(context.Background(), thought)
	if err == nil {
		t.Fatal("expected error when JSON is invalid")
	}

	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("expected parse error, got: %v", err)
	}
}
