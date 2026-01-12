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

// Sift is an LLM-powered conditional gate that implements pipz.Chainable[*Thought].
// It uses semantic reasoning to decide whether to execute a wrapped processor or pass through unchanged.
//
// Unlike pipz.Filter which uses a programmatic condition, Sift uses an LLM to make the decision
// based on semantic understanding of the context. This enables gates based on meaning rather than.
// simple data inspection.
type Sift struct {
	identity  pipz.Identity
	key       string
	question  string
	processor pipz.Chainable[*Thought]

	// Configuration
	useIntrospection         bool
	reasoningTemperature     float32
	introspectionTemperature float32
	summaryKey               string
	provider                 Provider
	temperature              float32
}

// NewSift creates a new semantic gate primitive.
//
// The primitive uses zyn.Binary to decide whether to execute the processor.
// If the decision is true, the processor is executed. Otherwise, the thought passes through unchanged.
//
// Output Notes:
//   - {key}: JSON-serialized zyn.BinaryResponse (the gate decision)
//   - {key}_summary: Semantic summary for next steps (if introspection enabled)
//
// Example:
//
//	gate := cogito.NewSift(
//	    "escalation_gate",
//	    "Does this ticket require human escalation?",
//	    humanEscalationPipeline,
//	)
//	result, _ := gate.Process(ctx, thought)
//	resp, _ := gate.Scan(result)
//	fmt.Println("Escalated:", resp.Decision)
func NewSift(key, question string, processor pipz.Chainable[*Thought]) *Sift {
	return &Sift{
		identity:         pipz.NewIdentity(key, "Semantic gate primitive"),
		key:              key,
		question:         question,
		processor:        processor,
		useIntrospection: DefaultIntrospection,
		temperature:      DefaultReasoningTemperature,
	}
}

// Process implements pipz.Chainable[*Thought].
func (s *Sift) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	// Resolve provider
	provider, err := ResolveProvider(ctx, s.provider)
	if err != nil {
		return t, fmt.Errorf("sift: %w", err)
	}

	// Create zyn binary synapse for gate decision
	binarySynapse, err := zyn.Binary(s.question, provider)
	if err != nil {
		return t, fmt.Errorf("sift: failed to create binary synapse: %w", err)
	}

	// Get unpublished notes
	unpublished := t.GetUnpublishedNotes()
	noteContext := RenderNotesToContext(unpublished)

	// Emit step started
	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(s.key),
		FieldStepType.Field("sift"),
		FieldUnpublishedCount.Field(len(unpublished)),
		FieldTemperature.Field(s.temperature),
	)

	// Determine reasoning temperature
	reasoningTemp := s.temperature
	if s.reasoningTemperature != 0 {
		reasoningTemp = s.reasoningTemperature
	}

	// PHASE 1: REASONING - Gate decision
	binaryResponse, err := binarySynapse.FireWithInput(ctx, t.Session, zyn.BinaryInput{
		Subject:     s.question,
		Context:     noteContext,
		Temperature: reasoningTemp,
	})
	if err != nil {
		s.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("sift: binary synapse execution failed: %w", err)
	}

	// Store decision as JSON
	respJSON, err := json.Marshal(binaryResponse)
	if err != nil {
		s.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("sift: failed to marshal response: %w", err)
	}
	if setErr := t.SetContent(ctx, s.key, string(respJSON), "sift"); setErr != nil {
		s.emitFailed(ctx, t, start, setErr)
		return t, fmt.Errorf("sift: failed to persist note: %w", setErr)
	}

	// Emit gate decision
	capitan.Emit(ctx, SiftDecided,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(s.key),
		FieldDecision.Field(binaryResponse.Decision),
		FieldConfidence.Field(binaryResponse.Confidence),
	)

	// PHASE 2: INTROSPECTION - Semantic summary (optional)
	if s.useIntrospection {
		if introErr := s.runIntrospection(ctx, t, binaryResponse, unpublished, provider); introErr != nil {
			s.emitFailed(ctx, t, start, introErr)
			return t, introErr
		}
	}

	// Mark notes as published
	t.MarkNotesPublished()

	// PHASE 3: CONDITIONAL EXECUTION - Execute processor if gate opened
	if binaryResponse.Decision {
		t, err = s.processor.Process(ctx, t)
		if err != nil {
			s.emitFailed(ctx, t, start, err)
			return t, fmt.Errorf("sift: processor execution failed: %w", err)
		}
	}

	// Emit step completed
	duration := time.Since(start)
	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(s.key),
		FieldStepType.Field("sift"),
		FieldStepDuration.Field(duration),
		FieldNoteCount.Field(len(t.AllNotes())),
	)

	return t, nil
}

// runIntrospection executes the transform synapse for semantic summary.
func (s *Sift) runIntrospection(ctx context.Context, t *Thought, resp zyn.BinaryResponse, originalNotes []Note, provider Provider) error {
	return runIntrospection(ctx, t, provider, s.buildIntrospectionInput(resp, originalNotes), introspectionConfig{
		stepType:                 "sift",
		key:                      s.key,
		summaryKey:               s.summaryKey,
		introspectionTemperature: s.introspectionTemperature,
		synapsePrompt:            "Synthesize gate decision into context for next reasoning step",
	})
}

// buildIntrospectionInput formats gate decision for the transform synapse.
func (s *Sift) buildIntrospectionInput(resp zyn.BinaryResponse, originalNotes []Note) zyn.TransformInput {
	decisionText := fmt.Sprintf(
		"Gate Decision: %v (confidence: %.2f)\nQuestion: %s\nReasoning:\n",
		resp.Decision,
		resp.Confidence,
		s.question,
	)
	for i, reason := range resp.Reasoning {
		decisionText += fmt.Sprintf("  %d. %s\n", i+1, reason)
	}

	return zyn.TransformInput{
		Text:    decisionText,
		Context: RenderNotesToContext(originalNotes),
		Style:   "Synthesize this gate decision into rich semantic context for the next reasoning step. Focus on why the gate opened or closed, implications for downstream processing, and actionable insights. Be concise but comprehensive.",
	}
}

// emitFailed emits a step failed event.
func (s *Sift) emitFailed(ctx context.Context, t *Thought, start time.Time, err error) {
	capitan.Error(ctx, StepFailed,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(s.key),
		FieldStepType.Field("sift"),
		FieldStepDuration.Field(time.Since(start)),
		FieldError.Field(err),
	)
}

// Identity implements pipz.Chainable[*Thought].
func (s *Sift) Identity() pipz.Identity {
	return s.identity
}

// Schema implements pipz.Chainable[*Thought].
func (s *Sift) Schema() pipz.Node {
	return pipz.Node{Identity: s.identity, Type: "sift"}
}

// Close implements pipz.Chainable[*Thought].
// Propagates Close to the wrapped processor.
func (s *Sift) Close() error {
	if s.processor != nil {
		return s.processor.Close()
	}
	return nil
}

// Scan retrieves the typed binary response from a thought.
func (s *Sift) Scan(t *Thought) (*zyn.BinaryResponse, error) {
	content, err := t.GetContent(s.key)
	if err != nil {
		return nil, fmt.Errorf("sift scan: %w", err)
	}
	var resp zyn.BinaryResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return nil, fmt.Errorf("sift scan: failed to unmarshal response: %w", err)
	}
	return &resp, nil
}

// Builder methods

// WithProvider sets the provider for this step.
func (s *Sift) WithProvider(p Provider) *Sift {
	s.provider = p
	return s
}

// WithTemperature sets the default temperature for this step.
func (s *Sift) WithTemperature(temp float32) *Sift {
	s.temperature = temp
	return s
}

// WithIntrospection enables the introspection phase.
func (s *Sift) WithIntrospection() *Sift {
	s.useIntrospection = true
	return s
}

// WithSummaryKey sets a custom key for the introspection summary note.
func (s *Sift) WithSummaryKey(key string) *Sift {
	s.summaryKey = key
	return s
}

// WithReasoningTemperature sets the temperature for the gate decision phase.
func (s *Sift) WithReasoningTemperature(temp float32) *Sift {
	s.reasoningTemperature = temp
	return s
}

// WithIntrospectionTemperature sets the temperature for the introspection phase.
func (s *Sift) WithIntrospectionTemperature(temp float32) *Sift {
	s.introspectionTemperature = temp
	return s
}

// SetProcessor updates the wrapped processor.
func (s *Sift) SetProcessor(processor pipz.Chainable[*Thought]) *Sift {
	s.processor = processor
	return s
}
