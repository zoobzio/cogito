package cogito

import (
	"context"
	"fmt"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
	"github.com/zoobzio/zyn"
)

// sentimentConfig implements stepConfig for sentiment analysis steps.
type sentimentConfig struct {
	outputKey                string
	summaryKey               string  // Custom key for summary note (if empty, uses outputKey+"_summary")
	useIntrospection         bool    // Whether to use Transform synapse for semantic summary
	reasoningTemperature     float32 // Temperature for Sentiment synapse (0 = use step default)
	introspectionTemperature float32 // Temperature for Transform synapse (0 = use creative default)
}

// buildSentimentIntrospectionInput formats sentiment result and original notes
// into input for the Transform synapse to generate semantic summary.
func buildSentimentIntrospectionInput(sentResponse zyn.SentimentResponse, originalNotes []Note) zyn.TransformInput {
	// Format the sentiment result
	sentText := fmt.Sprintf(
		"Sentiment: %s (confidence: %.2f)\n",
		sentResponse.Overall,
		sentResponse.Confidence,
	)

	sentText += fmt.Sprintf("Scores: positive=%.2f, negative=%.2f, neutral=%.2f\n",
		sentResponse.Scores.Positive,
		sentResponse.Scores.Negative,
		sentResponse.Scores.Neutral,
	)

	if len(sentResponse.Emotions) > 0 {
		sentText += fmt.Sprintf("Emotions: %v\n", sentResponse.Emotions)
	}

	if len(sentResponse.Aspects) > 0 {
		sentText += "Aspect sentiments:\n"
		for aspect, sentiment := range sentResponse.Aspects {
			sentText += fmt.Sprintf("  %s: %s\n", aspect, sentiment)
		}
	}

	sentText += "Reasoning:\n"
	for i, reason := range sentResponse.Reasoning {
		sentText += fmt.Sprintf("  %d. %s\n", i+1, reason)
	}

	// Include original context
	originalContext := RenderNotesToContext(originalNotes)

	return zyn.TransformInput{
		Text:    sentText,
		Context: originalContext,
		Style:   "Synthesize this sentiment analysis into rich semantic context for the next reasoning step. Focus on emotional tone implications, what it suggests about user state or satisfaction, and actionable insights. Be concise but comprehensive.",
	}
}

// build creates the pipz pipeline for a Sentiment step.
// It gathers unpublished Notes, calls zyn Sentiment synapse,
// and optionally calls Transform synapse for semantic summary.
func (c *sentimentConfig) build(ctx context.Context, provider Provider, temperature float32) (pipz.Chainable[*Thought], error) {
	// Create zyn sentiment synapse (reasoning phase)
	sentimentSynapse, err := zyn.NewSentiment("overall emotional tone", provider)
	if err != nil {
		return nil, fmt.Errorf("failed to create sentiment synapse: %w", err)
	}

	// Create zyn transform synapse (introspection phase, if enabled)
	var transformSynapse *zyn.TransformSynapse
	if c.useIntrospection {
		transformSynapse, err = zyn.Transform(
			"Synthesize sentiment analysis into context for next reasoning step",
			provider,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create transform synapse: %w", err)
		}
	}

	// Return a pipz processor that handles Thought → zyn(s) → Thought
	return pipz.Apply(pipz.Name("sentiment"), func(ctx context.Context, t *Thought) (*Thought, error) {
		// Get unpublished notes (new context since last LLM call)
		unpublished := t.GetUnpublishedNotes()

		// Render to context string
		context := RenderNotesToContext(unpublished)

		// Determine reasoning temperature (use config if set, otherwise step default)
		reasoningTemp := temperature
		if c.reasoningTemperature != 0 {
			reasoningTemp = c.reasoningTemperature
		}

		// PHASE 1: REASONING - Sentiment analysis
		sentResponse, err := sentimentSynapse.FireWithInput(ctx, t.Session, zyn.SentimentInput{
			Text:        context,
			Temperature: reasoningTemp,
		})
		if err != nil {
			return t, fmt.Errorf("sentiment synapse execution failed: %w", err)
		}

		// Write sentiment result as Note with metadata
		metadata := map[string]string{
			"confidence":      fmt.Sprintf("%.2f", sentResponse.Confidence),
			"score_positive":  fmt.Sprintf("%.2f", sentResponse.Scores.Positive),
			"score_negative":  fmt.Sprintf("%.2f", sentResponse.Scores.Negative),
			"score_neutral":   fmt.Sprintf("%.2f", sentResponse.Scores.Neutral),
		}

		// Add emotions if available
		if len(sentResponse.Emotions) > 0 {
			for i, emotion := range sentResponse.Emotions {
				metadata[fmt.Sprintf("emotion_%d", i)] = emotion
			}
		}

		// Add aspect sentiments if available
		if len(sentResponse.Aspects) > 0 {
			for aspect, sentiment := range sentResponse.Aspects {
				metadata[fmt.Sprintf("aspect_%s", aspect)] = sentiment
			}
		}

		// Add reasoning if available
		if len(sentResponse.Reasoning) > 0 {
			for i, reason := range sentResponse.Reasoning {
				metadata[fmt.Sprintf("reasoning_%d", i)] = reason
			}
		}

		t.SetNote(c.outputKey, sentResponse.Overall, "sentiment", metadata)

		// PHASE 2: INTROSPECTION - Semantic summary (optional)
		if c.useIntrospection && transformSynapse != nil {
			// Emit introspection started event
			capitan.Emit(ctx, IntrospectionStarted,
				FieldTraceID.Field(t.TraceID),
				FieldStepType.Field("sentiment"),
			)

			introspectionInput := buildSentimentIntrospectionInput(sentResponse, unpublished)

			// Determine introspection temperature (use config if set, otherwise creative default)
			introspectionTemp := zyn.DefaultTemperatureCreative
			if c.introspectionTemperature != 0 {
				introspectionTemp = c.introspectionTemperature
			}
			introspectionInput.Temperature = introspectionTemp

			summary, err := transformSynapse.FireWithInput(ctx, t.Session, introspectionInput)
			if err != nil {
				return t, fmt.Errorf("transform synapse execution failed: %w", err)
			}

			// Determine summary key (use config if set, otherwise default suffix)
			summaryKey := c.summaryKey
			if summaryKey == "" {
				summaryKey = c.outputKey + "_summary"
			}

			// Write semantic summary for next steps
			t.SetContent(summaryKey, summary, "sentiment-introspection")

			// Emit introspection completed event
			capitan.Emit(ctx, IntrospectionCompleted,
				FieldTraceID.Field(t.TraceID),
				FieldStepType.Field("sentiment"),
				FieldContextSize.Field(len(summary)),
			)
		}

		// Mark all notes as published after successful execution
		t.MarkNotesPublished()

		return t, nil
	}), nil
}

// stepType returns the semantic type of this step.
func (c *sentimentConfig) stepType() string {
	return "sentiment"
}

// defaultTemperature returns the default temperature for sentiment analysis.
func (c *sentimentConfig) defaultTemperature() float32 {
	return zyn.DefaultTemperatureAnalytical
}

// Implement builder interface methods

func (c *sentimentConfig) withIntrospection(enabled bool) stepConfig {
	newCfg := *c
	newCfg.useIntrospection = enabled
	return &newCfg
}

func (c *sentimentConfig) withSummaryKey(key string) stepConfig {
	newCfg := *c
	newCfg.summaryKey = key
	return &newCfg
}

func (c *sentimentConfig) withReasoningTemperature(temp float32) stepConfig {
	newCfg := *c
	newCfg.reasoningTemperature = temp
	return &newCfg
}

func (c *sentimentConfig) withIntrospectionTemperature(temp float32) stepConfig {
	newCfg := *c
	newCfg.introspectionTemperature = temp
	return &newCfg
}

// Sentiment creates a new sentiment analysis step with introspection enabled by default.
// It analyzes the emotional tone of unpublished Notes and writes the result to a Note.
//
// The step uses two zyn synapses:
//  1. Sentiment synapse: Analyzes emotional tone (positive/negative/neutral/mixed)
//  2. Transform synapse: Synthesizes a semantic summary for context accumulation
//
// The step automatically includes all Notes added since the last LLM call as context,
// enabling automatic context accumulation across the reasoning chain.
//
// Output Notes:
//   - {key}: Overall sentiment ("positive", "negative", "neutral", "mixed"), with metadata:
//     - confidence: Analysis confidence (0.0-1.0)
//     - score_positive/negative/neutral: Sentiment scores (0.0-1.0 each)
//     - emotion_N: Detected emotions (joy, anger, fear, etc.)
//     - aspect_X: Sentiment for specific aspects if analyzed
//     - reasoning_N: Step-by-step reasoning
//   - {key}_summary: Semantic summary for next steps (if introspection enabled)
//
// Parameters:
//   - key: The note key to write results to (also used as step name)
//
// Example:
//
//	step := cogito.Sentiment("user_sentiment")
//	thought := cogito.New("process feedback")
//	thought.SetContent("feedback", "This product is terrible and doesn't work!", "initial")
//	result, _ := step.Process(ctx, thought)
//
//	// Access overall sentiment
//	sentiment, _ := result.GetContent("user_sentiment")  // "negative"
//
//	// Access scores
//	negScore, _ := result.GetMetadata("user_sentiment", "score_negative")  // "0.87"
//	posScore, _ := result.GetMetadata("user_sentiment", "score_positive")  // "0.03"
//
//	// Access emotions
//	emotion, _ := result.GetMetadata("user_sentiment", "emotion_0")  // "anger"
//
//	// Next step sees semantic summary
//	summary, _ := result.GetContent("user_sentiment_summary")
//	// "Strong negative sentiment indicating user frustration..."
//
// Use WithoutIntrospection() to disable the Transform synapse and only get the sentiment.
// Use WithSummaryKey(), WithReasoningTemperature(), WithIntrospectionTemperature() to customize.
func Sentiment(key string) *Step {
	return newStep(key, &sentimentConfig{
		outputKey:        key,
		useIntrospection: true, // Default: use two-synapse pattern
	})
}
