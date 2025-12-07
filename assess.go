package cogito

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
	"github.com/zoobzio/zyn"
)

// Assess is a sentiment assessment primitive that implements pipz.Chainable[*Thought].
// It assesses the emotional tone of input and stores the full response for typed retrieval.
type Assess struct {
	key                      string
	summaryKey               string
	useIntrospection         bool
	reasoningTemperature     float32
	introspectionTemperature float32
	provider                 Provider
	temperature              float32
}

// NewAssess creates a new sentiment assessment primitive with introspection enabled by default.
//
// The primitive uses two zyn synapses:
//  1. Sentiment synapse: Assesses emotional tone (positive/negative/neutral/mixed)
//  2. Transform synapse: Synthesizes a semantic summary for context accumulation
//
// Output Notes:
//   - {key}: JSON-serialized zyn.SentimentResponse
//   - {key}_summary: Semantic summary for next steps (if introspection enabled)
//
// Example:
//
//	step := cogito.NewAssess("user_mood")
//	result, _ := step.Process(ctx, thought)
//	resp, _ := step.Scan(result)
//	fmt.Println(resp.Overall, resp.Confidence, resp.Scores)
func NewAssess(key string) *Assess {
	return &Assess{
		key:              key,
		useIntrospection: DefaultIntrospection,
		temperature:      DefaultReasoningTemperature,
	}
}

// Process implements pipz.Chainable[*Thought].
func (s *Assess) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	// Resolve provider
	provider, err := ResolveProvider(ctx, s.provider)
	if err != nil {
		return t, fmt.Errorf("assess: %w", err)
	}

	// Create zyn sentiment synapse
	sentimentSynapse, err := zyn.NewSentiment("overall emotional tone", provider)
	if err != nil {
		return t, fmt.Errorf("assess: failed to create sentiment synapse: %w", err)
	}

	// Get unpublished notes
	unpublished := t.GetUnpublishedNotes()
	context := RenderNotesToContext(unpublished)

	// Emit step started
	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(s.key),
		FieldStepType.Field("assess"),
		FieldUnpublishedCount.Field(len(unpublished)),
		FieldTemperature.Field(s.temperature),
	)

	// Determine reasoning temperature
	reasoningTemp := s.temperature
	if s.reasoningTemperature != 0 {
		reasoningTemp = s.reasoningTemperature
	}

	// PHASE 1: REASONING - Sentiment analysis
	sentResponse, err := sentimentSynapse.FireWithInput(ctx, t.Session, zyn.SentimentInput{
		Text:        context,
		Temperature: reasoningTemp,
	})
	if err != nil {
		s.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("assess: sentiment synapse execution failed: %w", err)
	}

	// Store full response as JSON
	respJSON, err := json.Marshal(sentResponse)
	if err != nil {
		s.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("assess: failed to marshal response: %w", err)
	}
	if err := t.SetContent(ctx, s.key, string(respJSON), "assess"); err != nil {
		s.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("assess: failed to persist note: %w", err)
	}

	// PHASE 2: INTROSPECTION - Semantic summary (optional)
	if s.useIntrospection {
		if err := s.runIntrospection(ctx, t, sentResponse, unpublished, provider); err != nil {
			s.emitFailed(ctx, t, start, err)
			return t, err
		}
	}

	// Mark notes as published
	t.MarkNotesPublished()

	// Emit step completed
	duration := time.Since(start)
	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(s.key),
		FieldStepType.Field("assess"),
		FieldStepDuration.Field(duration),
		FieldNoteCount.Field(len(t.AllNotes())),
	)

	return t, nil
}

// runIntrospection executes the transform synapse for semantic summary.
func (s *Assess) runIntrospection(ctx context.Context, t *Thought, resp zyn.SentimentResponse, originalNotes []Note, provider Provider) error {
	transformSynapse, err := zyn.Transform(
		"Synthesize sentiment analysis into context for next reasoning step",
		provider,
	)
	if err != nil {
		return fmt.Errorf("assess: failed to create transform synapse: %w", err)
	}

	introspectionInput := s.buildIntrospectionInput(resp, originalNotes)

	// Determine introspection temperature
	introspectionTemp := DefaultIntrospectionTemperature
	if s.introspectionTemperature != 0 {
		introspectionTemp = s.introspectionTemperature
	}
	introspectionInput.Temperature = introspectionTemp

	summary, err := transformSynapse.FireWithInput(ctx, t.Session, introspectionInput)
	if err != nil {
		return fmt.Errorf("assess: transform synapse execution failed: %w", err)
	}

	// Determine summary key
	summaryKey := s.summaryKey
	if summaryKey == "" {
		summaryKey = s.key + "_summary"
	}
	if err := t.SetContent(ctx, summaryKey, summary, "assess-introspection"); err != nil {
		return fmt.Errorf("assess: failed to persist introspection note: %w", err)
	}

	capitan.Emit(ctx, IntrospectionCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepType.Field("assess"),
		FieldContextSize.Field(len(summary)),
	)

	return nil
}

// buildIntrospectionInput formats sentiment for the transform synapse.
func (s *Assess) buildIntrospectionInput(resp zyn.SentimentResponse, originalNotes []Note) zyn.TransformInput {
	sentText := fmt.Sprintf(
		"Sentiment: %s (confidence: %.2f)\n",
		resp.Overall,
		resp.Confidence,
	)

	sentText += fmt.Sprintf("Scores: positive=%.2f, negative=%.2f, neutral=%.2f\n",
		resp.Scores.Positive,
		resp.Scores.Negative,
		resp.Scores.Neutral,
	)

	if len(resp.Emotions) > 0 {
		sentText += fmt.Sprintf("Emotions: %v\n", resp.Emotions)
	}

	if len(resp.Aspects) > 0 {
		sentText += "Aspect sentiments:\n"
		for aspect, sentiment := range resp.Aspects {
			sentText += fmt.Sprintf("  %s: %s\n", aspect, sentiment)
		}
	}

	sentText += "Reasoning:\n"
	for i, reason := range resp.Reasoning {
		sentText += fmt.Sprintf("  %d. %s\n", i+1, reason)
	}

	return zyn.TransformInput{
		Text:    sentText,
		Context: RenderNotesToContext(originalNotes),
		Style:   "Synthesize this sentiment analysis into rich semantic context for the next reasoning step. Focus on emotional tone implications, what it suggests about user state or satisfaction, and actionable insights. Be concise but comprehensive.",
	}
}

// emitFailed emits a step failed event.
func (s *Assess) emitFailed(ctx context.Context, t *Thought, start time.Time, err error) {
	capitan.Error(ctx, StepFailed,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(s.key),
		FieldStepType.Field("assess"),
		FieldStepDuration.Field(time.Since(start)),
		FieldError.Field(err),
	)
}

// Name implements pipz.Chainable[*Thought].
func (s *Assess) Name() pipz.Name {
	return pipz.Name(s.key)
}

// Close implements pipz.Chainable[*Thought].
func (s *Assess) Close() error {
	return nil
}

// Scan retrieves the typed sentiment response from a thought.
func (s *Assess) Scan(t *Thought) (*zyn.SentimentResponse, error) {
	content, err := t.GetContent(s.key)
	if err != nil {
		return nil, fmt.Errorf("assess scan: %w", err)
	}
	var resp zyn.SentimentResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return nil, fmt.Errorf("assess scan: failed to unmarshal response: %w", err)
	}
	return &resp, nil
}

// Builder methods

// WithProvider sets the provider for this step.
func (s *Assess) WithProvider(p Provider) *Assess {
	s.provider = p
	return s
}

// WithTemperature sets the default temperature for this step.
func (s *Assess) WithTemperature(temp float32) *Assess {
	s.temperature = temp
	return s
}

// WithIntrospection enables the introspection phase.
func (s *Assess) WithIntrospection() *Assess {
	s.useIntrospection = true
	return s
}

// WithSummaryKey sets a custom key for the introspection summary note.
func (s *Assess) WithSummaryKey(key string) *Assess {
	s.summaryKey = key
	return s
}

// WithReasoningTemperature sets the temperature for the reasoning phase.
func (s *Assess) WithReasoningTemperature(temp float32) *Assess {
	s.reasoningTemperature = temp
	return s
}

// WithIntrospectionTemperature sets the temperature for the introspection phase.
func (s *Assess) WithIntrospectionTemperature(temp float32) *Assess {
	s.introspectionTemperature = temp
	return s
}
