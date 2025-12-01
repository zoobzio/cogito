package cogito

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
	"github.com/zoobzio/zyn"
)

// analyzeConfig implements stepConfig for structured data extraction steps.
// It is generic over T, the type of data to extract (must implement zyn.Validator).
type analyzeConfig[T zyn.Validator] struct {
	what                     string  // Description of what to extract
	outputKey                string
	summaryKey               string  // Custom key for summary note (if empty, uses outputKey+"_summary")
	useIntrospection         bool    // Whether to use Transform synapse for semantic summary
	reasoningTemperature     float32 // Temperature for Extract synapse (0 = use step default)
	introspectionTemperature float32 // Temperature for Transform synapse (0 = use creative default)
}

// buildAnalyzeIntrospectionInput formats extracted data and original notes
// into input for the Transform synapse to generate semantic summary.
func buildAnalyzeIntrospectionInput[T zyn.Validator](extracted T, originalNotes []Note) zyn.TransformInput {
	// Serialize extracted data to JSON for display
	extractedJSON, err := json.MarshalIndent(extracted, "", "  ")
	if err != nil {
		extractedJSON = []byte(fmt.Sprintf("%+v", extracted))
	}

	// Format the extracted data
	extractedText := fmt.Sprintf("Extracted data:\n%s", string(extractedJSON))

	// Include original context
	originalContext := RenderNotesToContext(originalNotes)

	return zyn.TransformInput{
		Text:    extractedText,
		Context: originalContext,
		Style:   "Synthesize this extracted data into rich semantic context for the next reasoning step. Focus on implications, relationships between fields, and actionable insights. Be concise but comprehensive.",
	}
}

// build creates the pipz pipeline for an Analyze step.
// It gathers unpublished Notes, calls zyn Extract synapse for field extraction,
// and optionally calls Transform synapse for semantic summary.
func (c *analyzeConfig[T]) build(ctx context.Context, provider Provider, temperature float32) (pipz.Chainable[*Thought], error) {
	// Create zyn extract synapse (reasoning phase)
	extractSynapse, err := zyn.Extract[T](c.what, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to create extract synapse: %w", err)
	}

	// Create zyn transform synapse (introspection phase, if enabled)
	var transformSynapse *zyn.TransformSynapse
	if c.useIntrospection {
		transformSynapse, err = zyn.Transform(
			"Synthesize extracted data into context for next reasoning step",
			provider,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create transform synapse: %w", err)
		}
	}

	// Return a pipz processor that handles Thought → zyn(s) → Thought
	return pipz.Apply(pipz.Name("analyze"), func(ctx context.Context, t *Thought) (*Thought, error) {
		// Get unpublished notes (new context since last LLM call)
		unpublished := t.GetUnpublishedNotes()

		// Render to context string
		context := RenderNotesToContext(unpublished)

		// Determine reasoning temperature (use config if set, otherwise step default)
		reasoningTemp := temperature
		if c.reasoningTemperature != 0 {
			reasoningTemp = c.reasoningTemperature
		}

		// PHASE 1: REASONING - Extract structured data
		extracted, err := extractSynapse.FireWithInput(ctx, t.Session, zyn.ExtractionInput{
			Text:        context,
			Temperature: reasoningTemp,
		})
		if err != nil {
			return t, fmt.Errorf("extract synapse execution failed: %w", err)
		}

		// Serialize extracted data to JSON for storage
		extractedJSON, err := json.Marshal(extracted)
		if err != nil {
			return t, fmt.Errorf("failed to serialize extracted data: %w", err)
		}

		// Write extracted data as Note content (JSON string)
		t.SetContent(c.outputKey, string(extractedJSON), "analyze")

		// PHASE 2: INTROSPECTION - Semantic summary (optional)
		if c.useIntrospection && transformSynapse != nil {
			// Emit introspection started event
			capitan.Emit(ctx, IntrospectionStarted,
				FieldTraceID.Field(t.TraceID),
				FieldStepType.Field("analyze"),
			)

			introspectionInput := buildAnalyzeIntrospectionInput(extracted, unpublished)

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
			t.SetContent(summaryKey, summary, "analyze-introspection")

			// Emit introspection completed event
			capitan.Emit(ctx, IntrospectionCompleted,
				FieldTraceID.Field(t.TraceID),
				FieldStepType.Field("analyze"),
				FieldContextSize.Field(len(summary)),
			)
		}

		// Mark all notes as published after successful execution
		t.MarkNotesPublished()

		return t, nil
	}), nil
}

// stepType returns the semantic type of this step.
func (c *analyzeConfig[T]) stepType() string {
	return "analyze"
}

// defaultTemperature returns the default temperature for extraction.
func (c *analyzeConfig[T]) defaultTemperature() float32 {
	return zyn.DefaultTemperatureDeterministic
}

// Implement builder interface methods

func (c *analyzeConfig[T]) withIntrospection(enabled bool) stepConfig {
	newCfg := *c
	newCfg.useIntrospection = enabled
	return &newCfg
}

func (c *analyzeConfig[T]) withSummaryKey(key string) stepConfig {
	newCfg := *c
	newCfg.summaryKey = key
	return &newCfg
}

func (c *analyzeConfig[T]) withReasoningTemperature(temp float32) stepConfig {
	newCfg := *c
	newCfg.reasoningTemperature = temp
	return &newCfg
}

func (c *analyzeConfig[T]) withIntrospectionTemperature(temp float32) stepConfig {
	newCfg := *c
	newCfg.introspectionTemperature = temp
	return &newCfg
}

// Analyze creates a new structured data extraction step with introspection enabled by default.
// It asks the LLM to extract structured data of type T from unpublished Notes and writes
// the result as JSON to a Note.
//
// The step uses two zyn synapses:
//  1. Extract synapse: Pulls out structured data of type T
//  2. Transform synapse: Synthesizes a semantic summary for context accumulation
//
// The step automatically includes all Notes added since the last LLM call as context,
// enabling automatic context accumulation across the reasoning chain.
//
// Output Notes:
//   - {key}: JSON-serialized extracted data of type T
//   - {key}_summary: Semantic summary for next steps (if introspection enabled)
//
// Parameters:
//   - key: The note key to write results to (also used as step name)
//   - what: Description of what to extract (e.g., "contact information", "ticket metadata")
//
// Type Parameter:
//   - T: The type of data to extract (must implement zyn.Validator)
//
// Example:
//
//	type TicketData struct {
//	    Severity  string `json:"severity"`
//	    Component string `json:"component"`
//	}
//	func (t TicketData) Validate() error { return nil }
//
//	step := cogito.Analyze[TicketData]("ticket_data", "ticket metadata")
//	thought := cogito.New("classify ticket")
//	thought.SetContent("ticket_text", "URGENT: Login broken!", "initial")
//	result, _ := step.Process(ctx, thought)
//
//	// Get extracted data as JSON string
//	jsonData, _ := result.GetContent("ticket_data")
//	var ticketData TicketData
//	json.Unmarshal([]byte(jsonData), &ticketData)
//
//	// Next step sees semantic summary
//	summary, _ := result.GetContent("ticket_data_summary")
//
// Use WithoutIntrospection() to disable the Transform synapse and only get the extraction.
// Use WithSummaryKey(), WithReasoningTemperature(), WithIntrospectionTemperature() to customize.
func Analyze[T zyn.Validator](key, what string) *Step {
	return newStep(key, &analyzeConfig[T]{
		what:             what,
		outputKey:        key,
		useIntrospection: true, // Default: use two-synapse pattern
	})
}
