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

// Decide is a binary decision primitive that implements pipz.Chainable[*Thought].
// It asks the LLM a yes/no question and stores the full response for typed retrieval.
type Decide struct {
	key                      string
	question                 string
	summaryKey               string
	useIntrospection         bool
	reasoningTemperature     float32
	introspectionTemperature float32
	provider                 Provider
	temperature              float32
}

// NewDecide creates a new binary decision primitive with introspection enabled by default.
//
// The primitive uses two zyn synapses:
//  1. Binary synapse: Makes the decision and provides reasoning
//  2. Transform synapse: Synthesizes a semantic summary for context accumulation
//
// Output Notes:
//   - {key}: JSON-serialized zyn.BinaryResponse
//   - {key}_summary: Semantic summary for next steps (if introspection enabled)
//
// Example:
//
//	step := cogito.NewDecide("is_urgent", "Is this ticket urgent?")
//	result, _ := step.Process(ctx, thought)
//	resp, _ := step.Scan(result)
//	fmt.Println(resp.Decision, resp.Confidence, resp.Reasoning)
func NewDecide(key, question string) *Decide {
	return &Decide{
		key:              key,
		question:         question,
		useIntrospection: DefaultIntrospection,
		temperature:      DefaultReasoningTemperature,
	}
}

// Process implements pipz.Chainable[*Thought].
func (d *Decide) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	// Resolve provider
	provider, err := ResolveProvider(ctx, d.provider)
	if err != nil {
		return t, fmt.Errorf("decide: %w", err)
	}

	// Create zyn binary synapse
	binarySynapse, err := zyn.Binary(d.question, provider)
	if err != nil {
		return t, fmt.Errorf("decide: failed to create binary synapse: %w", err)
	}

	// Get unpublished notes
	unpublished := t.GetUnpublishedNotes()
	context := RenderNotesToContext(unpublished)

	// Emit step started
	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(d.key),
		FieldStepType.Field("decide"),
		FieldUnpublishedCount.Field(len(unpublished)),
		FieldTemperature.Field(d.temperature),
	)

	// Determine reasoning temperature
	reasoningTemp := d.temperature
	if d.reasoningTemperature != 0 {
		reasoningTemp = d.reasoningTemperature
	}

	// PHASE 1: REASONING - Binary decision
	binaryResponse, err := binarySynapse.FireWithInput(ctx, t.Session, zyn.BinaryInput{
		Subject:     d.question,
		Context:     context,
		Temperature: reasoningTemp,
	})
	if err != nil {
		d.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("decide: binary synapse execution failed: %w", err)
	}

	// Store full response as JSON
	respJSON, err := json.Marshal(binaryResponse)
	if err != nil {
		d.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("decide: failed to marshal response: %w", err)
	}
	if err := t.SetContent(ctx, d.key, string(respJSON), "decide"); err != nil {
		d.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("decide: failed to persist note: %w", err)
	}

	// PHASE 2: INTROSPECTION - Semantic summary (optional)
	if d.useIntrospection {
		if err := d.runIntrospection(ctx, t, binaryResponse, unpublished, provider); err != nil {
			d.emitFailed(ctx, t, start, err)
			return t, err
		}
	}

	// Mark notes as published
	t.MarkNotesPublished()

	// Emit step completed
	duration := time.Since(start)
	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(d.key),
		FieldStepType.Field("decide"),
		FieldStepDuration.Field(duration),
		FieldNoteCount.Field(len(t.AllNotes())),
	)

	return t, nil
}

// runIntrospection executes the transform synapse for semantic summary.
func (d *Decide) runIntrospection(ctx context.Context, t *Thought, resp zyn.BinaryResponse, originalNotes []Note, provider Provider) error {
	transformSynapse, err := zyn.Transform(
		"Synthesize decision into context for next reasoning step",
		provider,
	)
	if err != nil {
		return fmt.Errorf("decide: failed to create transform synapse: %w", err)
	}

	introspectionInput := d.buildIntrospectionInput(resp, originalNotes)

	// Determine introspection temperature
	introspectionTemp := DefaultIntrospectionTemperature
	if d.introspectionTemperature != 0 {
		introspectionTemp = d.introspectionTemperature
	}
	introspectionInput.Temperature = introspectionTemp

	summary, err := transformSynapse.FireWithInput(ctx, t.Session, introspectionInput)
	if err != nil {
		return fmt.Errorf("decide: transform synapse execution failed: %w", err)
	}

	// Determine summary key
	summaryKey := d.summaryKey
	if summaryKey == "" {
		summaryKey = d.key + "_summary"
	}
	if err := t.SetContent(ctx, summaryKey, summary, "decide-introspection"); err != nil {
		return fmt.Errorf("decide: failed to persist introspection note: %w", err)
	}

	capitan.Emit(ctx, IntrospectionCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepType.Field("decide"),
		FieldContextSize.Field(len(summary)),
	)

	return nil
}

// buildIntrospectionInput formats decision for the transform synapse.
func (d *Decide) buildIntrospectionInput(resp zyn.BinaryResponse, originalNotes []Note) zyn.TransformInput {
	decisionText := fmt.Sprintf(
		"Decision: %v (confidence: %.2f)\nReasoning:\n",
		resp.Decision,
		resp.Confidence,
	)
	for i, reason := range resp.Reasoning {
		decisionText += fmt.Sprintf("  %d. %s\n", i+1, reason)
	}

	return zyn.TransformInput{
		Text:    decisionText,
		Context: RenderNotesToContext(originalNotes),
		Style:   "Synthesize this decision into rich semantic context for the next reasoning step. Focus on implications, actionable insights, and what future steps need to know. Be concise but comprehensive.",
	}
}

// emitFailed emits a step failed event.
func (d *Decide) emitFailed(ctx context.Context, t *Thought, start time.Time, err error) {
	capitan.Error(ctx, StepFailed,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(d.key),
		FieldStepType.Field("decide"),
		FieldStepDuration.Field(time.Since(start)),
		FieldError.Field(err),
	)
}

// Name implements pipz.Chainable[*Thought].
func (d *Decide) Name() pipz.Name {
	return pipz.Name(d.key)
}

// Close implements pipz.Chainable[*Thought].
func (d *Decide) Close() error {
	return nil
}

// Scan retrieves the typed binary response from a thought.
func (d *Decide) Scan(t *Thought) (*zyn.BinaryResponse, error) {
	content, err := t.GetContent(d.key)
	if err != nil {
		return nil, fmt.Errorf("decide scan: %w", err)
	}
	var resp zyn.BinaryResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return nil, fmt.Errorf("decide scan: failed to unmarshal response: %w", err)
	}
	return &resp, nil
}

// Builder methods

// WithProvider sets the provider for this step.
func (d *Decide) WithProvider(p Provider) *Decide {
	d.provider = p
	return d
}

// WithTemperature sets the default temperature for this step.
func (d *Decide) WithTemperature(temp float32) *Decide {
	d.temperature = temp
	return d
}

// WithIntrospection enables the introspection phase.
func (d *Decide) WithIntrospection() *Decide {
	d.useIntrospection = true
	return d
}

// WithSummaryKey sets a custom key for the introspection summary note.
func (d *Decide) WithSummaryKey(key string) *Decide {
	d.summaryKey = key
	return d
}

// WithReasoningTemperature sets the temperature for the reasoning phase.
func (d *Decide) WithReasoningTemperature(temp float32) *Decide {
	d.reasoningTemperature = temp
	return d
}

// WithIntrospectionTemperature sets the temperature for the introspection phase.
func (d *Decide) WithIntrospectionTemperature(temp float32) *Decide {
	d.introspectionTemperature = temp
	return d
}
