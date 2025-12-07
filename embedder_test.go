package cogito

import (
	"context"
	"errors"
	"testing"
)

// mockEmbedder implements Embedder for testing.
type mockEmbedder struct {
	embedding  []float32
	dimensions int
	err        error
}

func (m *mockEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.embedding, nil
}

func (m *mockEmbedder) Dimensions() int {
	return m.dimensions
}

func TestEmbedderResolution(t *testing.T) {
	// Clear any global state
	SetEmbedder(nil)

	t.Run("explicit embedder takes precedence", func(t *testing.T) {
		explicit := &mockEmbedder{dimensions: 100}
		global := &mockEmbedder{dimensions: 200}
		SetEmbedder(global)
		defer SetEmbedder(nil)

		resolved, err := ResolveEmbedder(context.Background(), explicit)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resolved.Dimensions() != 100 {
			t.Errorf("expected explicit embedder, got dimensions %d", resolved.Dimensions())
		}
	})

	t.Run("context embedder second priority", func(t *testing.T) {
		ctxEmbedder := &mockEmbedder{dimensions: 150}
		global := &mockEmbedder{dimensions: 200}
		SetEmbedder(global)
		defer SetEmbedder(nil)

		ctx := WithEmbedder(context.Background(), ctxEmbedder)
		resolved, err := ResolveEmbedder(ctx, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resolved.Dimensions() != 150 {
			t.Errorf("expected context embedder, got dimensions %d", resolved.Dimensions())
		}
	})

	t.Run("global embedder fallback", func(t *testing.T) {
		global := &mockEmbedder{dimensions: 200}
		SetEmbedder(global)
		defer SetEmbedder(nil)

		resolved, err := ResolveEmbedder(context.Background(), nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resolved.Dimensions() != 200 {
			t.Errorf("expected global embedder, got dimensions %d", resolved.Dimensions())
		}
	})

	t.Run("no embedder returns error", func(t *testing.T) {
		SetEmbedder(nil)
		_, err := ResolveEmbedder(context.Background(), nil)
		if !errors.Is(err, ErrNoEmbedder) {
			t.Errorf("expected ErrNoEmbedder, got %v", err)
		}
	})
}

func TestEmbedderFromContext(t *testing.T) {
	embedder := &mockEmbedder{dimensions: 100}
	ctx := WithEmbedder(context.Background(), embedder)

	retrieved, ok := EmbedderFromContext(ctx)
	if !ok {
		t.Fatal("expected embedder in context")
	}
	if retrieved.Dimensions() != 100 {
		t.Errorf("wrong embedder retrieved: got dimensions %d", retrieved.Dimensions())
	}

	// Empty context
	_, ok = EmbedderFromContext(context.Background())
	if ok {
		t.Error("expected no embedder in empty context")
	}
}

func TestOpenAIEmbedderConfiguration(t *testing.T) {
	t.Run("default configuration", func(t *testing.T) {
		e := NewOpenAIEmbedder("test-key")
		if e.dimensions != DimensionsAda002 {
			t.Errorf("expected dimensions %d, got %d", DimensionsAda002, e.dimensions)
		}
		if e.model != ModelTextEmbeddingAda002 {
			t.Errorf("expected model %s, got %s", ModelTextEmbeddingAda002, e.model)
		}
	})

	t.Run("custom model", func(t *testing.T) {
		e := NewOpenAIEmbedder("test-key",
			WithEmbeddingModel(ModelTextEmbedding3Large, DimensionsTextEmbedding3L))
		if e.dimensions != DimensionsTextEmbedding3L {
			t.Errorf("expected dimensions %d, got %d", DimensionsTextEmbedding3L, e.dimensions)
		}
		if e.model != ModelTextEmbedding3Large {
			t.Errorf("expected model %s, got %s", ModelTextEmbedding3Large, e.model)
		}
	})

	t.Run("custom base URL", func(t *testing.T) {
		e := NewOpenAIEmbedder("test-key",
			WithEmbedderBaseURL("https://custom.api.com"))
		if e.baseURL != "https://custom.api.com" {
			t.Errorf("expected custom URL, got %s", e.baseURL)
		}
	})

	t.Run("dimensions method", func(t *testing.T) {
		e := NewOpenAIEmbedder("test-key")
		if e.Dimensions() != DimensionsAda002 {
			t.Errorf("expected %d, got %d", DimensionsAda002, e.Dimensions())
		}
	})
}
