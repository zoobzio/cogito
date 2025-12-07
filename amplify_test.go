package cogito

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zoobzio/zyn"
)

// mockAmplifyProvider implements Provider interface for testing Amplify.
type mockAmplifyProvider struct {
	callCount         int
	completionResults []bool // sequence of completion decisions
	completionIndex   int
}

func (m *mockAmplifyProvider) Call(ctx context.Context, messages []zyn.Message, temperature float32) (*zyn.ProviderResponse, error) {
	m.callCount++

	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	lastMessage := messages[len(messages)-1]

	// Transform synapse call (refinement)
	if strings.Contains(lastMessage.Content, "Transform:") {
		return &zyn.ProviderResponse{
			Content: fmt.Sprintf(`{"output": "Refined content iteration %d: improved clarity and removed redundancy.", "confidence": 0.92, "changes": ["Improved clarity"], "reasoning": ["Applied refinement"]}`, m.callCount),
			Usage: zyn.TokenUsage{
				Prompt:     15,
				Completion: 25,
				Total:      40,
			},
		}, nil
	}

	// Binary synapse call (completion check)
	completed := false
	if m.completionIndex < len(m.completionResults) {
		completed = m.completionResults[m.completionIndex]
		m.completionIndex++
	}

	return &zyn.ProviderResponse{
		Content: fmt.Sprintf(`{"decision": %t, "confidence": 0.90, "reasoning": ["Completion check result"]}`, completed),
		Usage: zyn.TokenUsage{
			Prompt:     10,
			Completion: 20,
			Total:      30,
		},
	}, nil
}

func (m *mockAmplifyProvider) Name() string {
	return "mock-amplify"
}

func TestAmplifyCompletesFirstIteration(t *testing.T) {
	provider := &mockAmplifyProvider{
		completionResults: []bool{true}, // completes on first check
	}
	SetProvider(provider)
	defer SetProvider(nil)

	amplify := NewAmplify(
		"refined_output",
		"draft",
		"Improve clarity",
		"Is the content clear and concise?",
		3,
	)

	thought := newTestThought("test amplify completes first")
	thought.SetContent(context.Background(), "draft", "Initial rough draft content", "initial")

	result, err := amplify.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify result
	output, err := amplify.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if output.Iterations != 1 {
		t.Errorf("expected 1 iteration, got %d", output.Iterations)
	}

	if !output.Completed {
		t.Error("expected completed to be true")
	}

	if output.Content == "" {
		t.Error("expected non-empty content")
	}
}

func TestAmplifyMultipleIterations(t *testing.T) {
	provider := &mockAmplifyProvider{
		completionResults: []bool{false, false, true}, // completes on third check
	}
	SetProvider(provider)
	defer SetProvider(nil)

	amplify := NewAmplify(
		"refined_output",
		"draft",
		"Improve clarity",
		"Is the content clear and concise?",
		5,
	)

	thought := newTestThought("test amplify multiple iterations")
	thought.SetContent(context.Background(), "draft", "Initial rough draft content", "initial")

	result, err := amplify.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output, err := amplify.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if output.Iterations != 3 {
		t.Errorf("expected 3 iterations, got %d", output.Iterations)
	}

	if !output.Completed {
		t.Error("expected completed to be true")
	}

	// Verify provider was called 6 times (3 transforms + 3 binary checks)
	if provider.callCount != 6 {
		t.Errorf("expected 6 provider calls, got %d", provider.callCount)
	}
}

func TestAmplifyMaxIterationsReached(t *testing.T) {
	provider := &mockAmplifyProvider{
		completionResults: []bool{false, false, false}, // never completes
	}
	SetProvider(provider)
	defer SetProvider(nil)

	amplify := NewAmplify(
		"refined_output",
		"draft",
		"Improve clarity",
		"Is the content clear and concise?",
		3, // max 3 iterations
	)

	thought := newTestThought("test amplify max iterations")
	thought.SetContent(context.Background(), "draft", "Initial rough draft content", "initial")

	result, err := amplify.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output, err := amplify.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if output.Iterations != 3 {
		t.Errorf("expected 3 iterations (max), got %d", output.Iterations)
	}

	if output.Completed {
		t.Error("expected completed to be false when max iterations reached")
	}

	// Still has content (last refinement)
	if output.Content == "" {
		t.Error("expected non-empty content even when not completed")
	}
}

func TestAmplifySourceKeyNotFound(t *testing.T) {
	provider := &mockAmplifyProvider{
		completionResults: []bool{true},
	}
	SetProvider(provider)
	defer SetProvider(nil)

	amplify := NewAmplify(
		"refined_output",
		"nonexistent_key",
		"Improve clarity",
		"Is the content clear?",
		3,
	)

	thought := newTestThought("test amplify source not found")
	thought.SetContent(context.Background(), "other_key", "Some content", "initial")

	_, err := amplify.Process(context.Background(), thought)
	if err == nil {
		t.Error("expected error when source key not found")
	}

	if !strings.Contains(err.Error(), "source key") {
		t.Errorf("expected error about source key, got: %v", err)
	}
}

func TestAmplifyBuilderMethods(t *testing.T) {
	provider := &mockAmplifyProvider{
		completionResults: []bool{true},
	}
	SetProvider(provider)
	defer SetProvider(nil)

	amplify := NewAmplify(
		"refined_output",
		"draft",
		"Improve clarity",
		"Is the content clear?",
		3,
	).
		WithTemperature(0.5).
		WithRefinementTemperature(0.7).
		WithCompletionTemperature(0.1).
		WithMaxIterations(5)

	thought := newTestThought("test amplify builder methods")
	thought.SetContent(context.Background(), "draft", "Initial content", "initial")

	result, err := amplify.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output, err := amplify.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if !output.Completed {
		t.Error("expected completed to be true")
	}
}

func TestAmplifyWithMaxIterationsSetToZero(t *testing.T) {
	provider := &mockAmplifyProvider{
		completionResults: []bool{true},
	}
	SetProvider(provider)
	defer SetProvider(nil)

	// maxIterations < 1 should default to 1
	amplify := NewAmplify(
		"refined_output",
		"draft",
		"Improve clarity",
		"Is the content clear?",
		0, // invalid, should become 1
	)

	thought := newTestThought("test amplify zero iterations")
	thought.SetContent(context.Background(), "draft", "Initial content", "initial")

	result, err := amplify.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output, err := amplify.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	// Should have at least 1 iteration
	if output.Iterations < 1 {
		t.Errorf("expected at least 1 iteration, got %d", output.Iterations)
	}
}

func TestAmplifyName(t *testing.T) {
	amplify := NewAmplify("my_refinement", "source", "prompt", "criteria", 3)

	if amplify.Name() != "my_refinement" {
		t.Errorf("expected name 'my_refinement', got %q", amplify.Name())
	}
}

func TestAmplifyClose(t *testing.T) {
	amplify := NewAmplify("my_refinement", "source", "prompt", "criteria", 3)

	err := amplify.Close()
	if err != nil {
		t.Errorf("unexpected error on close: %v", err)
	}
}

func TestAmplifyReasoningPreserved(t *testing.T) {
	provider := &mockAmplifyProvider{
		completionResults: []bool{true},
	}
	SetProvider(provider)
	defer SetProvider(nil)

	amplify := NewAmplify(
		"refined_output",
		"draft",
		"Improve clarity",
		"Is the content clear?",
		3,
	)

	thought := newTestThought("test amplify reasoning")
	thought.SetContent(context.Background(), "draft", "Initial content", "initial")

	result, err := amplify.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output, err := amplify.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	// Reasoning should be captured from final completion check
	if len(output.Reasoning) == 0 {
		t.Error("expected reasoning to be preserved")
	}
}
