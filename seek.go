package cogito

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
	"github.com/zoobzio/zyn"
)

// Seek performs semantic search over historical notes.
// It embeds a natural language query and finds the most relevant notes,
// then synthesizes the results into context for the current thought.
type Seek struct {
	key         string
	query       string
	limit       int
	summaryKey  string
	temperature float32
	embedder    Embedder
	provider    Provider
	result      *SeekResult
}

// SeekResult contains the outcome of a semantic search.
type SeekResult struct {
	Query   string            // The search query
	Notes   []NoteWithThought // Matching notes with their thoughts
	Summary string            // Synthesized summary of results
}

// NewSeek creates a new semantic search primitive.
// The query is embedded and used to find semantically similar notes.
func NewSeek(key, query string) *Seek {
	return &Seek{
		key:         key,
		query:       query,
		limit:       10,
		temperature: DefaultReasoningTemperature,
	}
}

// WithLimit sets the maximum number of notes to retrieve.
func (s *Seek) WithLimit(limit int) *Seek {
	s.limit = limit
	return s
}

// WithSummaryKey sets the key for storing the synthesized summary.
func (s *Seek) WithSummaryKey(key string) *Seek {
	s.summaryKey = key
	return s
}

// WithTemperature sets the temperature for the synthesis step.
func (s *Seek) WithTemperature(temp float32) *Seek {
	s.temperature = temp
	return s
}

// WithEmbedder sets a specific embedder for this search.
func (s *Seek) WithEmbedder(e Embedder) *Seek {
	s.embedder = e
	return s
}

// WithProvider sets a specific provider for synthesis.
func (s *Seek) WithProvider(p Provider) *Seek {
	s.provider = p
	return s
}

// Process executes the semantic search and synthesis.
func (s *Seek) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(s.key),
		FieldStepType.Field("seek"),
		FieldSearchQuery.Field(s.query),
		FieldSearchLimit.Field(s.limit),
		FieldTemperature.Field(s.temperature),
	)

	// Resolve embedder
	embedder, err := ResolveEmbedder(ctx, s.embedder)
	if err != nil {
		s.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("seek: %w", err)
	}

	// Embed the query
	queryEmbedding, err := embedder.Embed(ctx, s.query)
	if err != nil {
		s.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("seek: failed to embed query: %w", err)
	}

	// Search for similar notes
	results, err := t.memory.SearchNotes(ctx, queryEmbedding, s.limit)
	if err != nil {
		s.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("seek: search failed: %w", err)
	}

	capitan.Emit(ctx, SeekResultsFound,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(s.key),
		FieldSearchQuery.Field(s.query),
		FieldResultCount.Field(len(results)),
	)

	// Build context from results
	var contextBuilder strings.Builder
	for i, nwt := range results {
		contextBuilder.WriteString(fmt.Sprintf("--- Result %d ---\n", i+1))
		contextBuilder.WriteString(fmt.Sprintf("Thought: %s\n", nwt.Thought.Intent))
		contextBuilder.WriteString(fmt.Sprintf("Note Key: %s\n", nwt.Note.Key))
		contextBuilder.WriteString(fmt.Sprintf("Content: %s\n", nwt.Note.Content))
		contextBuilder.WriteString("\n")
	}

	// Synthesize results if we have any
	var summary string
	if len(results) > 0 {
		provider, err := ResolveProvider(ctx, s.provider)
		if err != nil {
			s.emitFailed(ctx, t, start, err)
			return t, fmt.Errorf("seek: %w", err)
		}

		transformSynapse, err := zyn.Transform(
			"Synthesize search results into relevant context",
			provider,
		)
		if err != nil {
			s.emitFailed(ctx, t, start, err)
			return t, fmt.Errorf("seek: failed to create transform synapse: %w", err)
		}

		summary, err = transformSynapse.FireWithInput(ctx, t.Session, zyn.TransformInput{
			Text:        contextBuilder.String(),
			Context:     fmt.Sprintf("Query: %s\n\nSynthesize the relevant information from these search results.", s.query),
			Style:       "concise, factual summary highlighting key insights relevant to the query",
			Temperature: s.temperature,
		})
		if err != nil {
			s.emitFailed(ctx, t, start, err)
			return t, fmt.Errorf("seek: synthesis failed: %w", err)
		}
	} else {
		summary = "No relevant historical notes found."
	}

	// Store the result
	seekResult := SeekResult{
		Query:   s.query,
		Notes:   results,
		Summary: summary,
	}
	s.result = &seekResult

	// Determine summary key
	summaryKey := s.summaryKey
	if summaryKey == "" {
		summaryKey = s.key
	}

	if err := t.SetContent(ctx, summaryKey, summary, "seek"); err != nil {
		s.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("seek: failed to store result: %w", err)
	}

	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(s.key),
		FieldStepType.Field("seek"),
		FieldStepDuration.Field(time.Since(start)),
		FieldNoteCount.Field(len(results)),
	)

	return t, nil
}

// Scan returns the typed result of the search.
func (s *Seek) Scan() *SeekResult {
	return s.result
}

// Name implements pipz.Chainable[*Thought].
func (s *Seek) Name() pipz.Name {
	return pipz.Name(s.key)
}

// Close implements pipz.Chainable[*Thought].
func (s *Seek) Close() error {
	return nil
}

func (s *Seek) emitFailed(ctx context.Context, t *Thought, start time.Time, err error) {
	capitan.Error(ctx, StepFailed,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(s.key),
		FieldStepType.Field("seek"),
		FieldStepDuration.Field(time.Since(start)),
		FieldError.Field(err),
	)
}

var _ pipz.Chainable[*Thought] = (*Seek)(nil)
