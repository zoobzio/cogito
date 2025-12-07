package cogito

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// Embedder generates vector embeddings from text.
type Embedder interface {
	// Embed generates a vector embedding for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// Dimensions returns the vector dimensions produced by this embedder.
	Dimensions() int
}

// ErrNoEmbedder is returned when no embedder is configured.
var ErrNoEmbedder = fmt.Errorf("no embedder configured")

// Global embedder state.
var (
	globalEmbedder   Embedder
	globalEmbedderMu sync.RWMutex
)

// SetEmbedder sets the global embedder instance.
func SetEmbedder(e Embedder) {
	globalEmbedderMu.Lock()
	defer globalEmbedderMu.Unlock()
	globalEmbedder = e
}

// GetEmbedder returns the global embedder instance.
func GetEmbedder() Embedder {
	globalEmbedderMu.RLock()
	defer globalEmbedderMu.RUnlock()
	return globalEmbedder
}

// embedderKey is the context key for embedder.
type embedderKey struct{}

// WithEmbedder returns a context with the given embedder.
func WithEmbedder(ctx context.Context, e Embedder) context.Context {
	return context.WithValue(ctx, embedderKey{}, e)
}

// EmbedderFromContext retrieves an embedder from context.
func EmbedderFromContext(ctx context.Context) (Embedder, bool) {
	e, ok := ctx.Value(embedderKey{}).(Embedder)
	return e, ok
}

// ResolveEmbedder finds an embedder using the resolution hierarchy:
// 1. Explicit embedder parameter (if non-nil)
// 2. Context embedder
// 3. Global embedder.
func ResolveEmbedder(ctx context.Context, explicit Embedder) (Embedder, error) {
	if explicit != nil {
		return explicit, nil
	}
	if e, ok := EmbedderFromContext(ctx); ok {
		return e, nil
	}
	if e := GetEmbedder(); e != nil {
		return e, nil
	}
	return nil, ErrNoEmbedder
}

// OpenAIEmbedder implements Embedder using OpenAI's embedding API.
type OpenAIEmbedder struct {
	apiKey     string
	model      string
	dimensions int
	baseURL    string
	client     *http.Client
}

// OpenAI embedding models and their dimensions.
const (
	ModelTextEmbeddingAda002  = "text-embedding-ada-002"
	ModelTextEmbedding3Small  = "text-embedding-3-small"
	ModelTextEmbedding3Large  = "text-embedding-3-large"
	DimensionsAda002          = 1536
	DimensionsTextEmbedding3S = 1536
	DimensionsTextEmbedding3L = 3072
)

// OpenAIEmbedderOption configures an OpenAIEmbedder.
type OpenAIEmbedderOption func(*OpenAIEmbedder)

// WithEmbeddingModel sets the embedding model.
func WithEmbeddingModel(model string, dimensions int) OpenAIEmbedderOption {
	return func(e *OpenAIEmbedder) {
		e.model = model
		e.dimensions = dimensions
	}
}

// WithEmbedderBaseURL sets a custom base URL (for proxies or compatible APIs).
func WithEmbedderBaseURL(url string) OpenAIEmbedderOption {
	return func(e *OpenAIEmbedder) {
		e.baseURL = url
	}
}

// WithEmbedderHTTPClient sets a custom HTTP client.
func WithEmbedderHTTPClient(client *http.Client) OpenAIEmbedderOption {
	return func(e *OpenAIEmbedder) {
		e.client = client
	}
}

// NewOpenAIEmbedder creates an OpenAI embedder with the given API key.
func NewOpenAIEmbedder(apiKey string, opts ...OpenAIEmbedderOption) *OpenAIEmbedder {
	e := &OpenAIEmbedder{
		apiKey:     apiKey,
		model:      ModelTextEmbeddingAda002,
		dimensions: DimensionsAda002,
		baseURL:    "https://api.openai.com/v1",
		client:     http.DefaultClient,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

type embeddingRequest struct {
	Input string `json:"input"`
	Model string `json:"model"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Embed generates an embedding for the given text.
func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := embeddingRequest{
		Input: text,
		Model: e.model,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var embResp embeddingResponse
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if embResp.Error != nil {
		return nil, fmt.Errorf("OpenAI API error: %s", embResp.Error.Message)
	}

	if len(embResp.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	return embResp.Data[0].Embedding, nil
}

// Dimensions returns the vector dimensions for this embedder.
func (e *OpenAIEmbedder) Dimensions() int {
	return e.dimensions
}

var _ Embedder = (*OpenAIEmbedder)(nil)
