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

// Survey performs broad semantic search across tasks.
// Unlike Seek which returns individual notes, Survey groups results by task
// and returns the most recent thought per task, providing broader context.
// across multiple reasoning chains.
type Survey struct {
	identity    pipz.Identity
	key         string
	query       string
	limit       int
	summaryKey  string
	temperature float32
	embedder    Embedder
	provider    Provider
	result      *SurveyResult
}

// SurveyResult contains the outcome of a task-grouped semantic search.
type SurveyResult struct {
	Query    string     // The search query
	Thoughts []*Thought // Most recent thought per matching task
	Summary  string     // Synthesized summary of results
}

// NewSurvey creates a new task-grouped semantic search primitive.
// The query is embedded and used to find tasks with semantically similar notes,
// returning the most recent thought for each matching task.
func NewSurvey(key, query string) *Survey {
	return &Survey{
		identity:    pipz.NewIdentity(key, "Task-grouped search primitive"),
		key:         key,
		query:       query,
		limit:       5,
		temperature: DefaultReasoningTemperature,
	}
}

// WithLimit sets the maximum number of tasks to retrieve.
func (s *Survey) WithLimit(limit int) *Survey {
	s.limit = limit
	return s
}

// WithSummaryKey sets the key for storing the synthesized summary.
func (s *Survey) WithSummaryKey(key string) *Survey {
	s.summaryKey = key
	return s
}

// WithTemperature sets the temperature for the synthesis step.
func (s *Survey) WithTemperature(temp float32) *Survey {
	s.temperature = temp
	return s
}

// WithEmbedder sets a specific embedder for this search.
func (s *Survey) WithEmbedder(e Embedder) *Survey {
	s.embedder = e
	return s
}

// WithProvider sets a specific provider for synthesis.
func (s *Survey) WithProvider(p Provider) *Survey {
	s.provider = p
	return s
}

// Process executes the task-grouped semantic search and synthesis.
func (s *Survey) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(s.key),
		FieldStepType.Field("survey"),
		FieldSearchQuery.Field(s.query),
		FieldSearchLimit.Field(s.limit),
		FieldTemperature.Field(s.temperature),
	)

	// Resolve embedder
	embedder, err := ResolveEmbedder(ctx, s.embedder)
	if err != nil {
		s.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("survey: %w", err)
	}

	// Embed the query
	queryEmbedding, err := embedder.Embed(ctx, s.query)
	if err != nil {
		s.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("survey: failed to embed query: %w", err)
	}

	// Search for thoughts by task
	thoughts, err := t.memory.SearchNotesByTask(ctx, queryEmbedding, s.limit)
	if err != nil {
		s.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("survey: search failed: %w", err)
	}

	capitan.Emit(ctx, SurveyResultsFound,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(s.key),
		FieldSearchQuery.Field(s.query),
		FieldResultCount.Field(len(thoughts)),
	)

	// Build context from results
	var contextBuilder strings.Builder
	for i, thought := range thoughts {
		contextBuilder.WriteString(fmt.Sprintf("--- Task %d ---\n", i+1))
		contextBuilder.WriteString(fmt.Sprintf("Intent: %s\n", thought.Intent))
		contextBuilder.WriteString(fmt.Sprintf("Created: %s\n", thought.CreatedAt.Format(time.RFC3339)))
		contextBuilder.WriteString("Notes:\n")
		for _, note := range thought.AllNotes() {
			contextBuilder.WriteString(fmt.Sprintf("  - %s: %s\n", note.Key, truncateContent(note.Content, 200)))
		}
		contextBuilder.WriteString("\n")
	}

	// Synthesize results if we have any
	var summary string
	if len(thoughts) > 0 {
		provider, err := ResolveProvider(ctx, s.provider)
		if err != nil {
			s.emitFailed(ctx, t, start, err)
			return t, fmt.Errorf("survey: %w", err)
		}

		transformSynapse, err := zyn.Transform(
			"Synthesize task summaries into relevant context",
			provider,
		)
		if err != nil {
			s.emitFailed(ctx, t, start, err)
			return t, fmt.Errorf("survey: failed to create transform synapse: %w", err)
		}

		summary, err = transformSynapse.FireWithInput(ctx, t.Session, zyn.TransformInput{
			Text:        contextBuilder.String(),
			Context:     fmt.Sprintf("Query: %s\n\nSynthesize insights from these related tasks, highlighting patterns and key learnings.", s.query),
			Style:       "analytical summary identifying common themes and notable differences across tasks",
			Temperature: s.temperature,
		})
		if err != nil {
			s.emitFailed(ctx, t, start, err)
			return t, fmt.Errorf("survey: synthesis failed: %w", err)
		}
	} else {
		summary = "No related tasks found."
	}

	// Store the result
	surveyResult := SurveyResult{
		Query:    s.query,
		Thoughts: thoughts,
		Summary:  summary,
	}
	s.result = &surveyResult

	// Determine summary key
	summaryKey := s.summaryKey
	if summaryKey == "" {
		summaryKey = s.key
	}

	if err := t.SetContent(ctx, summaryKey, summary, "survey"); err != nil {
		s.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("survey: failed to store result: %w", err)
	}

	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(s.key),
		FieldStepType.Field("survey"),
		FieldStepDuration.Field(time.Since(start)),
		FieldNoteCount.Field(len(thoughts)),
	)

	return t, nil
}

// Scan returns the typed result of the survey.
func (s *Survey) Scan() *SurveyResult {
	return s.result
}

// Identity implements pipz.Chainable[*Thought].
func (s *Survey) Identity() pipz.Identity {
	return s.identity
}

// Schema implements pipz.Chainable[*Thought].
func (s *Survey) Schema() pipz.Node {
	return pipz.Node{Identity: s.identity, Type: "survey"}
}

// Close implements pipz.Chainable[*Thought].
func (s *Survey) Close() error {
	return nil
}

func (s *Survey) emitFailed(ctx context.Context, t *Thought, start time.Time, err error) {
	capitan.Error(ctx, StepFailed,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(s.key),
		FieldStepType.Field("survey"),
		FieldStepDuration.Field(time.Since(start)),
		FieldError.Field(err),
	)
}

// truncateContent truncates content to maxLen characters with ellipsis.
func truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen-3] + "..."
}

var _ pipz.Chainable[*Thought] = (*Survey)(nil)
