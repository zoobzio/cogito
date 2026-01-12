package cogito

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/zoobzio/capitan"
)

func TestSurveyProcess(t *testing.T) {
	t.Run("returns empty summary when no results", func(t *testing.T) {
		mem := newMockSearchMemory()
		ctx := context.Background()
		thought, _ := New(ctx, mem, "test survey")

		embedder := &mockEmbedder{
			embedding:  []float32{0.1, 0.2, 0.3},
			dimensions: 3,
		}
		thought.SetEmbedder(embedder)

		provider := &mockTestProvider{}

		survey := NewSurvey("context", "find related tasks").
			WithEmbedder(embedder).
			WithProvider(provider)

		result, err := survey.Process(ctx, thought)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, err := result.GetContent("context")
		if err != nil {
			t.Fatalf("failed to get content: %v", err)
		}

		if content != "No related tasks found." {
			t.Errorf("expected empty result message, got: %s", content)
		}
	})

	t.Run("synthesizes results when found", func(t *testing.T) {
		mem := newMockSearchMemory()
		ctx := context.Background()
		thought, _ := New(ctx, mem, "test survey")

		// Set up mock search results
		taskID := "task-123"
		relatedThought := &Thought{
			Intent: "previous auth work",
			TaskID: &taskID,
		}
		relatedThought.AddNoteWithoutPersist(Note{
			Key:     "decision",
			Content: "Used OAuth2",
			Source:  "decide",
		})
		mem.searchByTaskResult = []*Thought{relatedThought}

		embedder := &mockEmbedder{
			embedding:  []float32{0.1, 0.2, 0.3},
			dimensions: 3,
		}

		provider := &mockTestProvider{}

		survey := NewSurvey("context", "authentication patterns").
			WithEmbedder(embedder).
			WithProvider(provider)

		result, err := survey.Process(ctx, thought)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify result was stored
		_, err = result.GetContent("context")
		if err != nil {
			t.Fatalf("failed to get content: %v", err)
		}

		// Verify Scan returns result
		scanResult := survey.Scan()
		if scanResult == nil {
			t.Fatal("expected scan result")
		}
		if scanResult.Query != "authentication patterns" {
			t.Errorf("expected query in result, got: %s", scanResult.Query)
		}
		if len(scanResult.Thoughts) != 1 {
			t.Errorf("expected 1 thought, got: %d", len(scanResult.Thoughts))
		}
	})

	t.Run("uses custom summary key", func(t *testing.T) {
		mem := newMockSearchMemory()
		ctx := context.Background()
		thought, _ := New(ctx, mem, "test survey")

		embedder := &mockEmbedder{
			embedding:  []float32{0.1, 0.2, 0.3},
			dimensions: 3,
		}

		survey := NewSurvey("search", "query").
			WithSummaryKey("custom_key").
			WithEmbedder(embedder)

		result, err := survey.Process(ctx, thought)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = result.GetContent("custom_key")
		if err != nil {
			t.Fatalf("expected content at custom_key: %v", err)
		}
	})

	t.Run("fails without embedder", func(t *testing.T) {
		mem := newMockSearchMemory()
		ctx := context.Background()
		thought, _ := New(ctx, mem, "test survey")

		// Ensure no global embedder
		SetEmbedder(nil)

		survey := NewSurvey("context", "query")

		_, err := survey.Process(ctx, thought)
		if err == nil {
			t.Fatal("expected error without embedder")
		}
	})
}

func TestSurveySignals(t *testing.T) {
	type stepStartData struct {
		query string
		limit int
	}

	var mu sync.Mutex
	var started *stepStartData
	var resultCount int
	var resultsCaptured, completed bool

	startListener := capitan.Hook(StepStarted, func(_ context.Context, e *capitan.Event) {
		if stepType, ok := FieldStepType.From(e); ok && stepType == "survey" {
			query, _ := FieldSearchQuery.From(e)
			limit, _ := FieldSearchLimit.From(e)
			mu.Lock()
			started = &stepStartData{query, limit}
			mu.Unlock()
		}
	})
	defer startListener.Close()

	resultsListener := capitan.Hook(SurveyResultsFound, func(_ context.Context, e *capitan.Event) {
		count, _ := FieldResultCount.From(e)
		mu.Lock()
		resultCount = count
		resultsCaptured = true
		mu.Unlock()
	})
	defer resultsListener.Close()

	completedListener := capitan.Hook(StepCompleted, func(_ context.Context, e *capitan.Event) {
		if stepType, ok := FieldStepType.From(e); ok && stepType == "survey" {
			mu.Lock()
			completed = true
			mu.Unlock()
		}
	})
	defer completedListener.Close()

	mem := newMockSearchMemory()
	ctx := context.Background()
	thought, _ := New(ctx, mem, "test signals")

	embedder := &mockEmbedder{
		embedding:  []float32{0.1, 0.2, 0.3},
		dimensions: 3,
	}

	survey := NewSurvey("test_survey", "test query").
		WithEmbedder(embedder).
		WithLimit(3).
		WithTemperature(0.5)

	_, err := survey.Process(ctx, thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for events.
	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		gotAll := started != nil && resultsCaptured && completed
		mu.Unlock()
		if gotAll || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	if started == nil {
		t.Fatal("expected StepStarted event")
	}
	if started.query != "test query" {
		t.Errorf("expected search_query 'test query', got %q", started.query)
	}
	if started.limit != 3 {
		t.Errorf("expected search_limit 3, got %d", started.limit)
	}

	if !resultsCaptured {
		t.Fatal("expected SurveyResultsFound event")
	}
	if resultCount != 0 {
		t.Errorf("expected result_count 0, got %d", resultCount)
	}

	if !completed {
		t.Fatal("expected StepCompleted event")
	}
}

func TestSurveyBuilders(t *testing.T) {
	survey := NewSurvey("key", "query").
		WithLimit(10).
		WithSummaryKey("summary").
		WithTemperature(0.8)

	if survey.limit != 10 {
		t.Errorf("expected limit 10, got %d", survey.limit)
	}
	if survey.summaryKey != "summary" {
		t.Errorf("expected summaryKey 'summary', got %s", survey.summaryKey)
	}
	if survey.temperature != 0.8 {
		t.Errorf("expected temperature 0.8, got %f", survey.temperature)
	}
	if survey.Identity().Name() != "key" {
		t.Errorf("expected name 'key', got %s", survey.Identity().Name())
	}
	if survey.Close() != nil {
		t.Error("expected Close to return nil")
	}
}

func TestTruncateContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		maxLen   int
		expected string
	}{
		{
			name:     "short content unchanged",
			content:  "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "exact length unchanged",
			content:  "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "long content truncated",
			content:  "hello world",
			maxLen:   8,
			expected: "hello...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateContent(tt.content, tt.maxLen)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
