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

// Analyze is a structured data extraction primitive that implements pipz.Chainable[*Thought].
// It extracts typed data from unstructured input using generics.
type Analyze[T zyn.Validator] struct {
	key                      string
	what                     string
	summaryKey               string
	useIntrospection         bool
	reasoningTemperature     float32
	introspectionTemperature float32
	provider                 Provider
	temperature              float32
}

// NewAnalyze creates a new structured data extraction primitive with introspection enabled by default.
//
// The primitive uses two zyn synapses:
//  1. Extract synapse: Pulls out structured data of type T
//  2. Transform synapse: Synthesizes a semantic summary for context accumulation
//
// Output Notes:
//   - {key}: JSON-serialized T (the extracted data)
//   - {key}_summary: Semantic summary for next steps (if introspection enabled)
//
// Example:
//
//	type TicketData struct {
//	    Severity  string `json:"severity"`
//	    Component string `json:"component"`
//	}
//	func (t TicketData) Validate() error { return nil }
//
//	step := cogito.NewAnalyze[TicketData]("ticket_data", "ticket metadata")
//	result, _ := step.Process(ctx, thought)
//	data, _ := step.Scan(result)
//	fmt.Println(data.Severity, data.Component)
func NewAnalyze[T zyn.Validator](key, what string) *Analyze[T] {
	return &Analyze[T]{
		key:              key,
		what:             what,
		useIntrospection: DefaultIntrospection,
		temperature:      DefaultReasoningTemperature,
	}
}

// Process implements pipz.Chainable[*Thought].
func (a *Analyze[T]) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	// Resolve provider
	provider, err := ResolveProvider(ctx, a.provider)
	if err != nil {
		return t, fmt.Errorf("analyze: %w", err)
	}

	// Create zyn extract synapse
	extractSynapse, err := zyn.Extract[T](a.what, provider)
	if err != nil {
		return t, fmt.Errorf("analyze: failed to create extract synapse: %w", err)
	}

	// Get unpublished notes
	unpublished := t.GetUnpublishedNotes()
	context := RenderNotesToContext(unpublished)

	// Emit step started
	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(a.key),
		FieldStepType.Field("analyze"),
		FieldUnpublishedCount.Field(len(unpublished)),
		FieldTemperature.Field(a.temperature),
	)

	// Determine reasoning temperature
	reasoningTemp := a.temperature
	if a.reasoningTemperature != 0 {
		reasoningTemp = a.reasoningTemperature
	}

	// PHASE 1: REASONING - Extract structured data
	extracted, err := extractSynapse.FireWithInput(ctx, t.Session, zyn.ExtractionInput{
		Text:        context,
		Temperature: reasoningTemp,
	})
	if err != nil {
		a.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("analyze: extract synapse execution failed: %w", err)
	}

	// Store extracted data as JSON
	extractedJSON, err := json.Marshal(extracted)
	if err != nil {
		a.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("analyze: failed to marshal extracted data: %w", err)
	}
	if err := t.SetContent(ctx, a.key, string(extractedJSON), "analyze"); err != nil {
		a.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("analyze: failed to persist note: %w", err)
	}

	// PHASE 2: INTROSPECTION - Semantic summary (optional)
	if a.useIntrospection {
		if err := a.runIntrospection(ctx, t, extracted, unpublished, provider); err != nil {
			a.emitFailed(ctx, t, start, err)
			return t, err
		}
	}

	// Mark notes as published
	t.MarkNotesPublished()

	// Emit step completed
	duration := time.Since(start)
	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(a.key),
		FieldStepType.Field("analyze"),
		FieldStepDuration.Field(duration),
		FieldNoteCount.Field(len(t.AllNotes())),
	)

	return t, nil
}

// runIntrospection executes the transform synapse for semantic summary.
func (a *Analyze[T]) runIntrospection(ctx context.Context, t *Thought, extracted T, originalNotes []Note, provider Provider) error {
	transformSynapse, err := zyn.Transform(
		"Synthesize extracted data into context for next reasoning step",
		provider,
	)
	if err != nil {
		return fmt.Errorf("analyze: failed to create transform synapse: %w", err)
	}

	introspectionInput := a.buildIntrospectionInput(extracted, originalNotes)

	// Determine introspection temperature
	introspectionTemp := DefaultIntrospectionTemperature
	if a.introspectionTemperature != 0 {
		introspectionTemp = a.introspectionTemperature
	}
	introspectionInput.Temperature = introspectionTemp

	summary, err := transformSynapse.FireWithInput(ctx, t.Session, introspectionInput)
	if err != nil {
		return fmt.Errorf("analyze: transform synapse execution failed: %w", err)
	}

	// Determine summary key
	summaryKey := a.summaryKey
	if summaryKey == "" {
		summaryKey = a.key + "_summary"
	}
	if err := t.SetContent(ctx, summaryKey, summary, "analyze-introspection"); err != nil {
		return fmt.Errorf("analyze: failed to persist introspection note: %w", err)
	}

	capitan.Emit(ctx, IntrospectionCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepType.Field("analyze"),
		FieldContextSize.Field(len(summary)),
	)

	return nil
}

// buildIntrospectionInput formats extracted data for the transform synapse.
func (a *Analyze[T]) buildIntrospectionInput(extracted T, originalNotes []Note) zyn.TransformInput {
	// Serialize extracted data to JSON for display
	extractedJSON, err := json.MarshalIndent(extracted, "", "  ")
	if err != nil {
		extractedJSON = []byte(fmt.Sprintf("%+v", extracted))
	}

	extractedText := fmt.Sprintf("Extracted data:\n%s", string(extractedJSON))

	return zyn.TransformInput{
		Text:    extractedText,
		Context: RenderNotesToContext(originalNotes),
		Style:   "Synthesize this extracted data into rich semantic context for the next reasoning step. Focus on implications, relationships between fields, and actionable insights. Be concise but comprehensive.",
	}
}

// emitFailed emits a step failed event.
func (a *Analyze[T]) emitFailed(ctx context.Context, t *Thought, start time.Time, err error) {
	capitan.Error(ctx, StepFailed,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(a.key),
		FieldStepType.Field("analyze"),
		FieldStepDuration.Field(time.Since(start)),
		FieldError.Field(err),
	)
}

// Name implements pipz.Chainable[*Thought].
func (a *Analyze[T]) Name() pipz.Name {
	return pipz.Name(a.key)
}

// Close implements pipz.Chainable[*Thought].
func (a *Analyze[T]) Close() error {
	return nil
}

// Scan retrieves the typed extracted data from a thought.
// Returns T directly (the user-defined type), not a wrapper.
func (a *Analyze[T]) Scan(t *Thought) (T, error) {
	var zero T
	content, err := t.GetContent(a.key)
	if err != nil {
		return zero, fmt.Errorf("analyze scan: %w", err)
	}
	var result T
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return zero, fmt.Errorf("analyze scan: failed to unmarshal data: %w", err)
	}
	return result, nil
}

// Builder methods

// WithProvider sets the provider for this step.
func (a *Analyze[T]) WithProvider(p Provider) *Analyze[T] {
	a.provider = p
	return a
}

// WithTemperature sets the default temperature for this step.
func (a *Analyze[T]) WithTemperature(temp float32) *Analyze[T] {
	a.temperature = temp
	return a
}

// WithIntrospection enables the introspection phase.
func (a *Analyze[T]) WithIntrospection() *Analyze[T] {
	a.useIntrospection = true
	return a
}

// WithSummaryKey sets a custom key for the introspection summary note.
func (a *Analyze[T]) WithSummaryKey(key string) *Analyze[T] {
	a.summaryKey = key
	return a
}

// WithReasoningTemperature sets the temperature for the reasoning phase.
func (a *Analyze[T]) WithReasoningTemperature(temp float32) *Analyze[T] {
	a.reasoningTemperature = temp
	return a
}

// WithIntrospectionTemperature sets the temperature for the introspection phase.
func (a *Analyze[T]) WithIntrospectionTemperature(temp float32) *Analyze[T] {
	a.introspectionTemperature = temp
	return a
}
