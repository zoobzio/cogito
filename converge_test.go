package cogito

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zoobzio/pipz"
	"github.com/zoobzio/zyn"
)

// mockConvergeProvider implements Provider interface for testing Converge.
type mockConvergeProvider struct {
	callCount int
}

func (m *mockConvergeProvider) Call(ctx context.Context, messages []zyn.Message, temperature float32) (*zyn.ProviderResponse, error) {
	m.callCount++

	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	// Transform synapse call (synthesis)
	return &zyn.ProviderResponse{
		Content: `{"output": "Unified synthesis: Technical analysis shows performance issues, business analysis recommends prioritization, risk analysis identifies potential downtime.", "confidence": 0.95, "changes": ["Combined perspectives"], "reasoning": ["Synthesized all branch outputs"]}`,
		Usage: zyn.TokenUsage{
			Prompt:     50,
			Completion: 100,
			Total:      150,
		},
	}, nil
}

func (m *mockConvergeProvider) Name() string {
	return "mock-converge"
}

// analysisProcessor simulates a specific analysis type.
type analysisProcessor struct {
	name       string
	output     string
	delay      time.Duration
	shouldFail bool
}

func (a *analysisProcessor) Process(ctx context.Context, t *Thought) (*Thought, error) {
	if a.delay > 0 {
		time.Sleep(a.delay)
	}

	if a.shouldFail {
		return t, fmt.Errorf("%s failed", a.name)
	}

	t.SetContent(ctx, a.name+"_result", a.output, a.name)
	return t, nil
}

func (a *analysisProcessor) Name() pipz.Name {
	return pipz.Name(a.name)
}

func (a *analysisProcessor) Close() error {
	return nil
}

func TestConvergeBasic(t *testing.T) {
	provider := &mockConvergeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	technical := &analysisProcessor{name: "technical", output: "CPU usage high"}
	business := &analysisProcessor{name: "business", output: "Priority: high"}
	risk := &analysisProcessor{name: "risk", output: "Downtime risk: medium"}

	converge := NewConverge(
		"unified_analysis",
		"Synthesize these perspectives into a unified recommendation",
		technical,
		business,
		risk,
	)

	thought := newTestThought("test converge basic")
	thought.SetContent(context.Background(), "ticket", "Performance degradation reported", "initial")

	result, err := converge.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify synthesis result
	synthesis, err := converge.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if synthesis == "" {
		t.Error("expected non-empty synthesis")
	}

	// Verify notes from branches were merged
	// Each branch should have added a note
	allNotes := result.AllNotes()

	// Look for branch outputs (they should be tagged with branch name)
	foundTechnical := false
	foundBusiness := false
	foundRisk := false

	for _, note := range allNotes {
		if note.Key == "technical_result" {
			foundTechnical = true
		}
		if note.Key == "business_result" {
			foundBusiness = true
		}
		if note.Key == "risk_result" {
			foundRisk = true
		}
	}

	if !foundTechnical {
		t.Error("expected technical_result note")
	}
	if !foundBusiness {
		t.Error("expected business_result note")
	}
	if !foundRisk {
		t.Error("expected risk_result note")
	}
}

func TestConvergeParallelExecution(t *testing.T) {
	provider := &mockConvergeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	// Each processor has a delay to verify they run in parallel
	fast := &analysisProcessor{name: "fast", output: "Fast result", delay: 10 * time.Millisecond}
	medium := &analysisProcessor{name: "medium", output: "Medium result", delay: 20 * time.Millisecond}
	slow := &analysisProcessor{name: "slow", output: "Slow result", delay: 30 * time.Millisecond}

	converge := NewConverge(
		"parallel_test",
		"Synthesize results",
		fast,
		medium,
		slow,
	)

	thought := newTestThought("test converge parallel")
	thought.SetContent(context.Background(), "input", "Test input", "initial")

	start := time.Now()
	result, err := converge.Process(context.Background(), thought)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// If running sequentially, would take ~60ms (10+20+30)
	// If running in parallel, should take ~30ms (slowest)
	// Allow some buffer for test environment variance
	if duration > 100*time.Millisecond {
		t.Errorf("expected parallel execution (< 100ms), took %v", duration)
	}

	// Verify all results are present
	_, err = converge.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
}

func TestConvergePartialFailure(t *testing.T) {
	provider := &mockConvergeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	success1 := &analysisProcessor{name: "success1", output: "Success 1"}
	failing := &analysisProcessor{name: "failing", output: "", shouldFail: true}
	success2 := &analysisProcessor{name: "success2", output: "Success 2"}

	converge := NewConverge(
		"partial_failure_test",
		"Synthesize available results",
		success1,
		failing,
		success2,
	)

	thought := newTestThought("test converge partial failure")
	thought.SetContent(context.Background(), "input", "Test input", "initial")

	result, err := converge.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("expected partial success, got error: %v", err)
	}

	// Verify synthesis still happened with successful branches
	synthesis, err := converge.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if synthesis == "" {
		t.Error("expected non-empty synthesis even with partial failure")
	}

	// Successful branch notes should exist
	_, err = result.GetContent("success1_result")
	if err != nil {
		t.Error("expected success1_result note")
	}

	_, err = result.GetContent("success2_result")
	if err != nil {
		t.Error("expected success2_result note")
	}
}

func TestConvergeAllBranchesFail(t *testing.T) {
	provider := &mockConvergeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	failing1 := &analysisProcessor{name: "failing1", output: "", shouldFail: true}
	failing2 := &analysisProcessor{name: "failing2", output: "", shouldFail: true}

	converge := NewConverge(
		"all_fail_test",
		"Synthesize results",
		failing1,
		failing2,
	)

	thought := newTestThought("test converge all fail")
	thought.SetContent(context.Background(), "input", "Test input", "initial")

	_, err := converge.Process(context.Background(), thought)
	if err == nil {
		t.Error("expected error when all branches fail")
	}

	if !strings.Contains(err.Error(), "all branches failed") {
		t.Errorf("expected 'all branches failed' error, got: %v", err)
	}
}

func TestConvergeNoProcessors(t *testing.T) {
	provider := &mockConvergeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	converge := NewConverge(
		"empty_test",
		"Synthesize nothing",
	)

	thought := newTestThought("test converge empty")
	thought.SetContent(context.Background(), "input", "Test input", "initial")

	result, err := converge.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should pass through unchanged
	if result != thought {
		t.Error("expected thought to pass through unchanged")
	}
}

func TestConvergeBuilderMethods(t *testing.T) {
	provider := &mockConvergeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	processor := &analysisProcessor{name: "test", output: "Test output"}
	converge := NewConverge("test_converge", "Synthesize").
		WithTemperature(0.5).
		WithSynthesisTemperature(0.8).
		AddProcessor(processor)

	thought := newTestThought("test converge builder")
	thought.SetContent(context.Background(), "input", "Test input", "initial")

	result, err := converge.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	synthesis, err := converge.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if synthesis == "" {
		t.Error("expected non-empty synthesis")
	}
}

func TestConvergeProcessorManagement(t *testing.T) {
	provider := &mockConvergeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	p1 := &analysisProcessor{name: "p1", output: "P1 output"}
	p2 := &analysisProcessor{name: "p2", output: "P2 output"}
	p3 := &analysisProcessor{name: "p3", output: "P3 output"}

	converge := NewConverge("test_converge", "Synthesize", p1, p2)

	// Test Processors() returns copy
	processors := converge.Processors()
	if len(processors) != 2 {
		t.Errorf("expected 2 processors, got %d", len(processors))
	}

	// Test AddProcessor
	converge.AddProcessor(p3)
	processors = converge.Processors()
	if len(processors) != 3 {
		t.Errorf("expected 3 processors after add, got %d", len(processors))
	}

	// Test RemoveProcessor
	converge.RemoveProcessor("p2")
	processors = converge.Processors()
	if len(processors) != 2 {
		t.Errorf("expected 2 processors after remove, got %d", len(processors))
	}

	// Verify p2 was removed
	for _, p := range processors {
		if p.Name() == "p2" {
			t.Error("p2 should have been removed")
		}
	}

	// Test ClearProcessors
	converge.ClearProcessors()
	processors = converge.Processors()
	if len(processors) != 0 {
		t.Errorf("expected 0 processors after clear, got %d", len(processors))
	}
}

func TestConvergeName(t *testing.T) {
	converge := NewConverge("my_synthesis", "Synthesize")

	if converge.Name() != "my_synthesis" {
		t.Errorf("expected name 'my_synthesis', got %q", converge.Name())
	}
}

func TestConvergeClose(t *testing.T) {
	processor := &analysisProcessor{name: "test", output: "Test output"}
	converge := NewConverge("test_converge", "Synthesize", processor)

	err := converge.Close()
	if err != nil {
		t.Errorf("unexpected error on close: %v", err)
	}
}

func TestConvergeThoughtIsolation(t *testing.T) {
	provider := &mockConvergeProvider{}
	SetProvider(provider)
	defer SetProvider(nil)

	// Create processors that modify thought differently
	p1 := &analysisProcessor{name: "branch1", output: "Branch 1 specific output"}
	p2 := &analysisProcessor{name: "branch2", output: "Branch 2 specific output"}

	converge := NewConverge("isolation_test", "Synthesize", p1, p2)

	thought := newTestThought("test converge isolation")
	thought.SetContent(context.Background(), "shared", "This is shared context", "initial")

	originalNoteCount := len(thought.AllNotes())

	result, err := converge.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Result should have more notes than original (branch outputs + synthesis)
	if len(result.AllNotes()) <= originalNoteCount {
		t.Error("expected result to have additional notes from branches")
	}

	// Both branch outputs should be in result
	_, err = result.GetContent("branch1_result")
	if err != nil {
		t.Error("expected branch1_result")
	}

	_, err = result.GetContent("branch2_result")
	if err != nil {
		t.Error("expected branch2_result")
	}
}
