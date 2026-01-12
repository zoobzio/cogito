package cogito

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/zoobzio/capitan"
)

// mockSearchMemory extends mockMemory with search capabilities for testing.
type mockSearchMemory struct {
	*mockMemory
	searchResults      []NoteWithThought
	searchByTaskResult []*Thought
}

func newMockSearchMemory() *mockSearchMemory {
	return &mockSearchMemory{
		mockMemory: newMockMemory(),
	}
}

func (m *mockSearchMemory) SearchNotes(_ context.Context, _ Vector, _ int) ([]NoteWithThought, error) {
	return m.searchResults, nil
}

func (m *mockSearchMemory) SearchNotesByTask(_ context.Context, _ Vector, _ int) ([]*Thought, error) {
	return m.searchByTaskResult, nil
}

func TestSeekProcess(t *testing.T) {
	t.Run("returns empty summary when no results", func(t *testing.T) {
		mem := newMockSearchMemory()
		ctx := context.Background()
		thought, _ := New(ctx, mem, "test seek")

		embedder := &mockEmbedder{
			embedding:  []float32{0.1, 0.2, 0.3},
			dimensions: 3,
		}
		thought.SetEmbedder(embedder)

		provider := &mockTestProvider{}

		seek := NewSeek("context", "find authentication notes").
			WithEmbedder(embedder).
			WithProvider(provider)

		result, err := seek.Process(ctx, thought)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		content, err := result.GetContent("context")
		if err != nil {
			t.Fatalf("failed to get content: %v", err)
		}

		if content != "No relevant historical notes found." {
			t.Errorf("expected empty result message, got: %s", content)
		}
	})

	t.Run("synthesizes results when found", func(t *testing.T) {
		mem := newMockSearchMemory()
		ctx := context.Background()
		thought, _ := New(ctx, mem, "test seek")

		// Set up mock search results
		mem.searchResults = []NoteWithThought{
			{
				Note: Note{
					Key:     "auth_decision",
					Content: "We decided to use JWT tokens",
					Source:  "decide",
				},
				Thought: &Thought{
					Intent: "implement authentication",
				},
			},
		}

		embedder := &mockEmbedder{
			embedding:  []float32{0.1, 0.2, 0.3},
			dimensions: 3,
		}

		provider := &mockTestProvider{}

		seek := NewSeek("context", "authentication approach").
			WithEmbedder(embedder).
			WithProvider(provider)

		result, err := seek.Process(ctx, thought)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify result was stored
		_, err = result.GetContent("context")
		if err != nil {
			t.Fatalf("failed to get content: %v", err)
		}

		// Verify Scan returns result
		scanResult := seek.Scan()
		if scanResult == nil {
			t.Fatal("expected scan result")
		}
		if scanResult.Query != "authentication approach" {
			t.Errorf("expected query in result, got: %s", scanResult.Query)
		}
		if len(scanResult.Notes) != 1 {
			t.Errorf("expected 1 note, got: %d", len(scanResult.Notes))
		}
	})

	t.Run("uses custom summary key", func(t *testing.T) {
		mem := newMockSearchMemory()
		ctx := context.Background()
		thought, _ := New(ctx, mem, "test seek")

		embedder := &mockEmbedder{
			embedding:  []float32{0.1, 0.2, 0.3},
			dimensions: 3,
		}

		seek := NewSeek("search", "query").
			WithSummaryKey("custom_key").
			WithEmbedder(embedder)

		result, err := seek.Process(ctx, thought)
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
		thought, _ := New(ctx, mem, "test seek")

		// Ensure no global embedder
		SetEmbedder(nil)

		seek := NewSeek("context", "query")

		_, err := seek.Process(ctx, thought)
		if err == nil {
			t.Fatal("expected error without embedder")
		}
	})
}

func TestSeekSignals(t *testing.T) {
	type stepStartData struct {
		query string
		limit int
	}

	var mu sync.Mutex
	var started *stepStartData
	var resultCount int
	var resultsCaptured, completed bool

	startListener := capitan.Hook(StepStarted, func(_ context.Context, e *capitan.Event) {
		if stepType, ok := FieldStepType.From(e); ok && stepType == "seek" {
			query, _ := FieldSearchQuery.From(e)
			limit, _ := FieldSearchLimit.From(e)
			mu.Lock()
			started = &stepStartData{query, limit}
			mu.Unlock()
		}
	})
	defer startListener.Close()

	resultsListener := capitan.Hook(SeekResultsFound, func(_ context.Context, e *capitan.Event) {
		count, _ := FieldResultCount.From(e)
		mu.Lock()
		resultCount = count
		resultsCaptured = true
		mu.Unlock()
	})
	defer resultsListener.Close()

	completedListener := capitan.Hook(StepCompleted, func(_ context.Context, e *capitan.Event) {
		if stepType, ok := FieldStepType.From(e); ok && stepType == "seek" {
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

	seek := NewSeek("test_seek", "test query").
		WithEmbedder(embedder).
		WithLimit(5).
		WithTemperature(0.5)

	_, err := seek.Process(ctx, thought)
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
	if started.limit != 5 {
		t.Errorf("expected search_limit 5, got %d", started.limit)
	}

	if !resultsCaptured {
		t.Fatal("expected SeekResultsFound event")
	}
	if resultCount != 0 {
		t.Errorf("expected result_count 0, got %d", resultCount)
	}

	if !completed {
		t.Fatal("expected StepCompleted event")
	}
}

func TestSeekBuilders(t *testing.T) {
	seek := NewSeek("key", "query").
		WithLimit(20).
		WithSummaryKey("summary").
		WithTemperature(0.7)

	if seek.limit != 20 {
		t.Errorf("expected limit 20, got %d", seek.limit)
	}
	if seek.summaryKey != "summary" {
		t.Errorf("expected summaryKey 'summary', got %s", seek.summaryKey)
	}
	if seek.temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %f", seek.temperature)
	}
	if seek.Identity().Name() != "key" {
		t.Errorf("expected name 'key', got %s", seek.Identity().Name())
	}
	if seek.Close() != nil {
		t.Error("expected Close to return nil")
	}
}
