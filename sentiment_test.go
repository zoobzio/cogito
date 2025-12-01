package cogito

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zoobzio/zyn"
)

// mockSentimentProvider implements Provider interface for testing Sentiment
type mockSentimentProvider struct {
	callCount int
}

func (m *mockSentimentProvider) Call(ctx context.Context, messages []zyn.Message, temperature float32) (*zyn.ProviderResponse, error) {
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

func (m *mockSentimentProvider) Name() string {
	return "mock"
}

func TestSentimentBasic(t *testing.T) {
	provider := &mockSentimentProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Sentiment("user_sentiment")

	thought := New("test sentiment")
	thought.SetContent("feedback", "This product is terrible and doesn't work!", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check overall sentiment
	sentiment, err := result.GetContent("user_sentiment")
	if err != nil {
		t.Fatalf("user_sentiment note not found: %v", err)
	}

	if sentiment != "negative" {
		t.Errorf("expected sentiment 'negative', got %q", sentiment)
	}

	// Check metadata
	note, ok := result.GetNote("user_sentiment")
	if !ok {
		t.Fatal("note not found")
	}

	confidence, ok := note.Metadata["confidence"]
	if !ok {
		t.Error("confidence metadata not found")
	}

	if confidence != "0.91" {
		t.Errorf("expected confidence '0.91', got %q", confidence)
	}

	// Check sentiment scores
	negScore, ok := note.Metadata["score_negative"]
	if !ok {
		t.Error("score_negative metadata not found")
	}

	if negScore != "0.85" {
		t.Errorf("expected score_negative '0.85', got %q", negScore)
	}

	// Check emotions
	emotion0, ok := note.Metadata["emotion_0"]
	if !ok {
		t.Error("emotion_0 metadata not found")
	}

	if emotion0 != "anger" {
		t.Errorf("expected emotion_0 'anger', got %q", emotion0)
	}

	// Check reasoning was captured
	if _, ok := note.Metadata["reasoning_0"]; !ok {
		t.Error("reasoning not captured in metadata")
	}
}

func TestSentimentIntrospection(t *testing.T) {
	provider := &mockSentimentProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Sentiment("user_sentiment")

	thought := New("test introspection")
	thought.SetContent("feedback", "Bad experience", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify sentiment exists
	_, err = result.GetContent("user_sentiment")
	if err != nil {
		t.Fatalf("user_sentiment note not found: %v", err)
	}

	// Verify summary note exists (introspection enabled by default)
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

func TestSentimentWithoutIntrospection(t *testing.T) {
	provider := &mockSentimentProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Sentiment("user_sentiment").
		WithoutIntrospection()

	thought := New("test without introspection")
	thought.SetContent("feedback", "Bad product", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify sentiment exists
	sentiment, err := result.GetContent("user_sentiment")
	if err != nil {
		t.Fatalf("user_sentiment note not found: %v", err)
	}

	if sentiment != "negative" {
		t.Errorf("expected sentiment 'negative', got %q", sentiment)
	}

	// Verify summary note does NOT exist
	_, err = result.GetContent("user_sentiment_summary")
	if err == nil {
		t.Error("expected summary note to not exist when introspection disabled")
	}

	// Verify mockProvider was called only once (Sentiment only)
	if provider.callCount != 1 {
		t.Errorf("expected 1 provider call, got %d", provider.callCount)
	}
}

func TestSentimentWithSummaryKey(t *testing.T) {
	provider := &mockSentimentProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Sentiment("user_sentiment").
		WithSummaryKey("context")

	thought := New("test custom summary key")
	thought.SetContent("feedback", "Bad", "initial")

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

func TestSentimentStepRecord(t *testing.T) {
	provider := &mockSentimentProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	step := Sentiment("user_sentiment")

	thought := New("test step recording")
	thought.SetContent("feedback", "Test input", "initial")

	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check step was recorded
	if len(result.Steps) == 0 {
		t.Fatal("expected step record to be added")
	}

	step1 := result.Steps[0]
	if step1.Name != "user_sentiment" {
		t.Errorf("expected step name 'analyze-feedback', got %q", step1.Name)
	}

	if step1.Type != "sentiment" {
		t.Errorf("expected step type 'sentiment', got %q", step1.Type)
	}

	if step1.Duration == 0 {
		t.Error("expected non-zero duration")
	}
}
