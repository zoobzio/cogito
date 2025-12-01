package cogito

import (
	"context"
	"fmt"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
	"github.com/zoobzio/zyn"
)

// decideConfig implements stepConfig for binary decision steps.
type decideConfig struct {
	question                 string
	outputKey                string
	summaryKey               string  // Custom key for summary note (if empty, uses outputKey+"_summary")
	useIntrospection         bool    // Whether to use Transform synapse for semantic summary
	reasoningTemperature     float32 // Temperature for Binary synapse (0 = use step default)
	introspectionTemperature float32 // Temperature for Transform synapse (0 = use creative default)
}

// buildIntrospectionInput formats a binary decision and original notes
// into input for the Transform synapse to generate semantic summary.
func buildIntrospectionInput(decision zyn.BinaryResponse, originalNotes []Note) zyn.TransformInput {
	// Format the decision details
	decisionText := fmt.Sprintf(
		"Decision: %v (confidence: %.2f)\nReasoning:\n",
		decision.Decision,
		decision.Confidence,
	)

	for i, reason := range decision.Reasoning {
		decisionText += fmt.Sprintf("  %d. %s\n", i+1, reason)
	}

	// Include original context
	originalContext := RenderNotesToContext(originalNotes)

	return zyn.TransformInput{
		Text: decisionText,
		Context: originalContext,
		Style: "Synthesize this decision into rich semantic context for the next reasoning step. Focus on implications, actionable insights, and what future steps need to know. Be concise but comprehensive.",
	}
}

// build creates the pipz pipeline for a Decide step.
// It gathers unpublished Notes, calls zyn Binary synapse for decision,
// and optionally calls Transform synapse for semantic summary.
func (c *decideConfig) build(ctx context.Context, provider Provider, temperature float32) (pipz.Chainable[*Thought], error) {
	// Create zyn binary synapse (reasoning phase)
	binarySynapse, err := zyn.Binary(c.question, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to create binary synapse: %w", err)
	}

	// Create zyn transform synapse (introspection phase, if enabled)
	var transformSynapse *zyn.TransformSynapse
	if c.useIntrospection {
		transformSynapse, err = zyn.Transform(
			"Synthesize decision into context for next reasoning step",
			provider,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create transform synapse: %w", err)
		}
	}

	// Return a pipz processor that handles Thought → zyn(s) → Thought
	return pipz.Apply(pipz.Name("decide"), func(ctx context.Context, t *Thought) (*Thought, error) {
		// Get unpublished notes (new context since last LLM call)
		unpublished := t.GetUnpublishedNotes()

		// Render to context string
		context := RenderNotesToContext(unpublished)

		// Determine reasoning temperature (use config if set, otherwise step default)
		reasoningTemp := temperature
		if c.reasoningTemperature != 0 {
			reasoningTemp = c.reasoningTemperature
		}

		// PHASE 1: REASONING - Binary decision
		binaryResponse, err := binarySynapse.FireWithInput(ctx, t.Session, zyn.BinaryInput{
			Subject:     c.question,
			Context:     context,
			Temperature: reasoningTemp,
		})
		if err != nil {
			return t, fmt.Errorf("binary synapse execution failed: %w", err)
		}

		// Write structured decision result as Note with metadata
		decision := "false"
		if binaryResponse.Decision {
			decision = "true"
		}

		metadata := map[string]string{
			"confidence": fmt.Sprintf("%.2f", binaryResponse.Confidence),
		}

		// Add reasoning if available
		if len(binaryResponse.Reasoning) > 0 {
			for i, reason := range binaryResponse.Reasoning {
				metadata[fmt.Sprintf("reasoning_%d", i)] = reason
			}
		}

		t.SetNote(c.outputKey, decision, "decide", metadata)

		// PHASE 2: INTROSPECTION - Semantic summary (optional)
		if c.useIntrospection && transformSynapse != nil {
			// Emit introspection started event
			capitan.Emit(ctx, IntrospectionStarted,
				FieldTraceID.Field(t.TraceID),
				FieldStepType.Field("decide"),
			)

			introspectionInput := buildIntrospectionInput(binaryResponse, unpublished)

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
			t.SetContent(summaryKey, summary, "decide-introspection")

			// Emit introspection completed event
			capitan.Emit(ctx, IntrospectionCompleted,
				FieldTraceID.Field(t.TraceID),
				FieldStepType.Field("decide"),
				FieldContextSize.Field(len(summary)),
			)
		}

		// Mark all notes as published after successful execution
		t.MarkNotesPublished()

		return t, nil
	}), nil
}

// stepType returns the semantic type of this step.
func (c *decideConfig) stepType() string {
	return "decide"
}

// defaultTemperature returns the default temperature for binary decisions.
func (c *decideConfig) defaultTemperature() float32 {
	return zyn.DefaultTemperatureDeterministic
}

// Decide creates a new binary decision step with introspection enabled by default.
// It asks the LLM a yes/no question based on all unpublished Notes and writes the result to new Notes.
//
// The step uses two zyn synapses:
//  1. Binary synapse: Makes the decision and provides reasoning
//  2. Transform synapse: Synthesizes a semantic summary for context accumulation
//
// The step automatically includes all Notes added since the last LLM call as context,
// enabling automatic context accumulation across the reasoning chain.
//
// Output Notes:
//   - {key}: "true" or "false" with metadata (confidence, reasoning)
//   - {key}_summary: Semantic summary for next steps (if introspection enabled)
//
// Parameters:
//   - key: The note key to write results to (also used as step name)
//   - question: The yes/no question to ask (e.g., "Is this urgent?")
//
// Example:
//
//	step := cogito.Decide("is_urgent", "Is this ticket urgent?")
//	thought := cogito.New("classify ticket")
//	thought.SetContent("ticket_text", "URGENT: Production is down!", "initial")
//	thought.SetContent("user_tier", "premium", "initial")
//	result, _ := step.Process(ctx, thought)
//
//	// Access decision programmatically
//	isUrgent, _ := result.GetBool("is_urgent")
//
//	// Next step sees semantic summary
//	summary, _ := result.GetContent("is_urgent_summary")
//	// "CRITICAL: Premium user experiencing production database outage..."
//
// Use WithoutIntrospection() to disable the Transform synapse and only get the decision.
func Decide(key, question string) *Step {
	return newStep(key, &decideConfig{
		question:         question,
		outputKey:        key,
		useIntrospection: true, // Default: use two-synapse pattern
	})
}
