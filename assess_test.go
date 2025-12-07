package cogito

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zoobzio/zyn"
)

// mockAssessProvider implements Provider interface for testing Assess.
type mockAssessProvider struct {
	callCount int
}

func (m *mockAssessProvider) Call(ctx context.Context, messages []zyn.Message, temperature float32) (*zyn.ProviderResponse, error) {
	m.callCount++

	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	// Check last message to determine which synapse is calling
	lastMessage := messages[len(messages)-1]

	// Transform synapse call - check first since it's more specific
	if strings.Contains(lastMessage.Content, "Transform:") {
		return &zyn.ProviderResponse{
			Content: `{"output": "Strong negative sentiment indicating user frustration and dissatisfaction requiring immediate attention", "confidence": 0.88, "changes": ["Synthesized sentiment context"], "reasoning": ["Combined emotional analysis with context"]}`,
			Usage: zyn.TokenUsage{
				Prompt:     15,
				Completion: 25,
				Total:      40,
			},
		}, nil
	}

	// Sentiment synapse call
	if strings.Contains(lastMessage.Content, "Sentiment") || strings.Contains(lastMessage.Content, "emotional") {
		return &zyn.ProviderResponse{
			Content: `{"overall": "negative", "confidence": 0.91, "scores": {"positive": 0.05, "negative": 0.85, "neutral": 0.10}, "aspects": {}, "emotions": ["anger", "frustration"], "reasoning": ["Text contains negative language", "Expresses dissatisfaction", "Indicates product failure"]}`,
			Usage: zyn.TokenUsage{
				Prompt:     10,
				Completion: 20,
				Total:      30,
			},
		}, nil
	}

	// Default fallback - return neutral sentiment
	return &zyn.ProviderResponse{
		Content: `{"overall": "neutral", "confidence": 0.6, "scores": {"positive": 0.33, "negative": 0.33, "neutral": 0.34}, "aspects": {}, "emotions": [], "reasoning": ["No strong emotional indicators"]}`,
		Usage: zyn.TokenUsage{
			Prompt:     10,
			Completion: 10,
			Total:      20,
		},
	}, nil
}

func (m *mockAssessProvider) Name() string {
	return "mock"
}

func TestAssessBasic(t *testing.T) {
	provider := &mockAssessProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := NewAssess("user_sentiment")

	thought := newTestThought("test sentiment")
	thought.SetContent(context.Background(), "feedback", "This product is terrible and doesn't work!", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Use Scan to get typed response
	resp, err := step.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if resp.Overall != "negative" {
		t.Errorf("expected overall 'negative', got %q", resp.Overall)
	}

	if resp.Confidence != 0.91 {
		t.Errorf("expected confidence 0.91, got %f", resp.Confidence)
	}

	if resp.Scores.Negative != 0.85 {
		t.Errorf("expected negative score 0.85, got %f", resp.Scores.Negative)
	}

	if len(resp.Emotions) == 0 || resp.Emotions[0] != "anger" {
		t.Errorf("expected first emotion 'anger', got %v", resp.Emotions)
	}

	if len(resp.Reasoning) == 0 {
		t.Error("expected reasoning to be present")
	}
}

func TestAssessDefaultNoIntrospection(t *testing.T) {
	provider := &mockAssessProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := NewAssess("user_sentiment")

	thought := newTestThought("test default no introspection")
	thought.SetContent(context.Background(), "feedback", "Bad product", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify sentiment exists
	_, err = result.GetContent("user_sentiment")
	if err != nil {
		t.Fatalf("user_sentiment note not found: %v", err)
	}

	// Verify summary note does NOT exist (introspection disabled by default)
	_, err = result.GetContent("user_sentiment_summary")
	if err == nil {
		t.Error("expected summary note to not exist by default")
	}

	// Verify mockProvider was called only once (Sentiment only)
	if provider.callCount != 1 {
		t.Errorf("expected 1 provider call, got %d", provider.callCount)
	}
}

func TestAssessWithIntrospection(t *testing.T) {
	provider := &mockAssessProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := NewAssess("user_sentiment").
		WithIntrospection()

	thought := newTestThought("test with introspection")
	thought.SetContent(context.Background(), "feedback", "Bad experience", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify sentiment exists
	_, err = result.GetContent("user_sentiment")
	if err != nil {
		t.Fatalf("user_sentiment note not found: %v", err)
	}

	// Verify summary note exists (introspection enabled)
	summary, err := result.GetContent("user_sentiment_summary")
	if err != nil {
		t.Fatalf("summary note not found: %v", err)
	}

	if summary == "" {
		t.Error("expected non-empty summary")
	}

	// Verify it contains semantic context
	if !strings.Contains(summary, "negative sentiment") {
		t.Errorf("expected summary to contain semantic context, got: %q", summary)
	}

	// Verify mockProvider was called twice (Sentiment + Transform)
	if provider.callCount != 2 {
		t.Errorf("expected 2 provider calls, got %d", provider.callCount)
	}
}

func TestAssessWithSummaryKey(t *testing.T) {
	provider := &mockAssessProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := NewAssess("user_sentiment").
		WithIntrospection().
		WithSummaryKey("context")

	thought := newTestThought("test custom summary key")
	thought.SetContent(context.Background(), "feedback", "Bad", "initial")

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
	_, err = result.GetContent("user_sentiment_summary")
	if err == nil {
		t.Error("expected default summary key to not exist")
	}
}
