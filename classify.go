package cogito

import (
	"context"
	"fmt"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
	"github.com/zoobzio/zyn"
)

// classifyConfig implements stepConfig for multi-class classification steps.
type classifyConfig struct {
	question                 string   // The classification question
	categories               []string // Valid categories
	outputKey                string
	summaryKey               string  // Custom key for summary note (if empty, uses outputKey+"_summary")
	useIntrospection         bool    // Whether to use Transform synapse for semantic summary
	reasoningTemperature     float32 // Temperature for Classification synapse (0 = use step default)
	introspectionTemperature float32 // Temperature for Transform synapse (0 = use creative default)
}

// buildClassifyIntrospectionInput formats classification result and original notes
// into input for the Transform synapse to generate semantic summary.
func buildClassifyIntrospectionInput(classResponse zyn.ClassificationResponse, originalNotes []Note) zyn.TransformInput {
	// Format the classification result
	classText := fmt.Sprintf(
		"Classification: %s (confidence: %.2f)\n",
		classResponse.Primary,
		classResponse.Confidence,
	)

	if classResponse.Secondary != "" {
		classText += fmt.Sprintf("Secondary: %s\n", classResponse.Secondary)
	}

	classText += "Reasoning:\n"
	for i, reason := range classResponse.Reasoning {
		classText += fmt.Sprintf("  %d. %s\n", i+1, reason)
	}

	// Include original context
	originalContext := RenderNotesToContext(originalNotes)

	return zyn.TransformInput{
		Text:    classText,
		Context: originalContext,
		Style:   "Synthesize this classification into rich semantic context for the next reasoning step. Focus on implications of the category choice, what it means for downstream actions, and actionable insights. Be concise but comprehensive.",
	}
}

// build creates the pipz pipeline for a Classify step.
// It gathers unpublished Notes, calls zyn Classification synapse,
// and optionally calls Transform synapse for semantic summary.
func (c *classifyConfig) build(ctx context.Context, provider Provider, temperature float32) (pipz.Chainable[*Thought], error) {
	// Create zyn classification synapse (reasoning phase)
	classificationSynapse, err := zyn.Classification(c.question, c.categories, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to create classification synapse: %w", err)
	}

	// Create zyn transform synapse (introspection phase, if enabled)
	var transformSynapse *zyn.TransformSynapse
	if c.useIntrospection {
		transformSynapse, err = zyn.Transform(
			"Synthesize classification into context for next reasoning step",
			provider,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create transform synapse: %w", err)
		}
	}

	// Return a pipz processor that handles Thought → zyn(s) → Thought
	return pipz.Apply(pipz.Name("classify"), func(ctx context.Context, t *Thought) (*Thought, error) {
		// Get unpublished notes (new context since last LLM call)
		unpublished := t.GetUnpublishedNotes()

		// Render to context string
		context := RenderNotesToContext(unpublished)

		// Determine reasoning temperature (use config if set, otherwise step default)
		reasoningTemp := temperature
		if c.reasoningTemperature != 0 {
			reasoningTemp = c.reasoningTemperature
		}

		// PHASE 1: REASONING - Classification
		classResponse, err := classificationSynapse.FireWithInput(ctx, t.Session, zyn.ClassificationInput{
			Subject:     c.question,
			Context:     context,
			Temperature: reasoningTemp,
		})
		if err != nil {
			return t, fmt.Errorf("classification synapse execution failed: %w", err)
		}

		// Write classification result as Note with metadata
		metadata := map[string]string{
			"confidence": fmt.Sprintf("%.2f", classResponse.Confidence),
		}

		if classResponse.Secondary != "" {
			metadata["secondary"] = classResponse.Secondary
		}

		// Add reasoning if available
		if len(classResponse.Reasoning) > 0 {
			for i, reason := range classResponse.Reasoning {
				metadata[fmt.Sprintf("reasoning_%d", i)] = reason
			}
		}

		t.SetNote(c.outputKey, classResponse.Primary, "classify", metadata)

		// PHASE 2: INTROSPECTION - Semantic summary (optional)
		if c.useIntrospection && transformSynapse != nil {
			// Emit introspection started event
			capitan.Emit(ctx, IntrospectionStarted,
				FieldTraceID.Field(t.TraceID),
				FieldStepType.Field("classify"),
			)

			introspectionInput := buildClassifyIntrospectionInput(classResponse, unpublished)

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
			t.SetContent(summaryKey, summary, "classify-introspection")

			// Emit introspection completed event
			capitan.Emit(ctx, IntrospectionCompleted,
				FieldTraceID.Field(t.TraceID),
				FieldStepType.Field("classify"),
				FieldContextSize.Field(len(summary)),
			)
		}

		// Mark all notes as published after successful execution
		t.MarkNotesPublished()

		return t, nil
	}), nil
}

// stepType returns the semantic type of this step.
func (c *classifyConfig) stepType() string {
	return "classify"
}

// defaultTemperature returns the default temperature for classification.
func (c *classifyConfig) defaultTemperature() float32 {
	return zyn.DefaultTemperatureCreative
}

// Implement builder interface methods

func (c *classifyConfig) withIntrospection(enabled bool) stepConfig {
	newCfg := *c
	newCfg.useIntrospection = enabled
	return &newCfg
}

func (c *classifyConfig) withSummaryKey(key string) stepConfig {
	newCfg := *c
	newCfg.summaryKey = key
	return &newCfg
}

func (c *classifyConfig) withReasoningTemperature(temp float32) stepConfig {
	newCfg := *c
	newCfg.reasoningTemperature = temp
	return &newCfg
}

func (c *classifyConfig) withIntrospectionTemperature(temp float32) stepConfig {
	newCfg := *c
	newCfg.introspectionTemperature = temp
	return &newCfg
}

// Classify creates a new multi-class classification step with introspection enabled by default.
// It asks the LLM to categorize the input into one of the provided categories and writes
// the result to a Note.
//
// The step uses two zyn synapses:
//  1. Classification synapse: Categorizes input into best matching category
//  2. Transform synapse: Synthesizes a semantic summary for context accumulation
//
// The step automatically includes all Notes added since the last LLM call as context,
// enabling automatic context accumulation across the reasoning chain.
//
// Output Notes:
//   - {key}: Primary category as content, with metadata:
//     - confidence: Classification confidence (0.0-1.0)
//     - secondary: Optional second-best category
//     - reasoning_N: Step-by-step reasoning
//   - {key}_summary: Semantic summary for next steps (if introspection enabled)
//
// Parameters:
//   - key: The note key to write results to (also used as step name)
//   - question: The classification question (e.g., "What type of ticket is this?")
//   - categories: Valid categories to choose from (e.g., ["bug", "feature", "question"])
//
// Example:
//
//	step := cogito.Classify("ticket_type",
//	    "What type of ticket is this?",
//	    []string{"bug", "feature", "question", "documentation"},
//	)
//	thought := cogito.New("process ticket")
//	thought.SetContent("ticket_text", "Login button doesn't work", "initial")
//	result, _ := step.Process(ctx, thought)
//
//	// Access primary category
//	category, _ := result.GetContent("ticket_type")  // "bug"
//
//	// Access metadata
//	confidence, _ := result.GetMetadata("ticket_type", "confidence")  // "0.92"
//	secondary, _ := result.GetMetadata("ticket_type", "secondary")    // "feature"
//
//	// Next step sees semantic summary
//	summary, _ := result.GetContent("ticket_type_summary")
//	// "Bug report: authentication failure requiring immediate fix..."
//
// Use WithoutIntrospection() to disable the Transform synapse and only get the classification.
// Use WithSummaryKey(), WithReasoningTemperature(), WithIntrospectionTemperature() to customize.
func Classify(key, question string, categories []string) *Step {
	return newStep(key, &classifyConfig{
		question:         question,
		categories:       categories,
		outputKey:        key,
		useIntrospection: true, // Default: use two-synapse pattern
	})
}
