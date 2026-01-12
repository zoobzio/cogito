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

// Categorize is a multi-class categorization primitive that implements pipz.Chainable[*Thought].
// It asks the LLM to place input into one of the provided categories.
type Categorize struct {
	identity                 pipz.Identity
	key                      string
	question                 string
	categories               []string
	summaryKey               string
	useIntrospection         bool
	reasoningTemperature     float32
	introspectionTemperature float32
	provider                 Provider
	temperature              float32
}

// NewCategorize creates a new multi-class categorization primitive with introspection enabled by default.
//
// The primitive uses two zyn synapses:
//  1. Classification synapse: Places input into best matching category
//  2. Transform synapse: Synthesizes a semantic summary for context accumulation
//
// Output Notes:
//   - {key}: JSON-serialized zyn.ClassificationResponse
//   - {key}_summary: Semantic summary for next steps (if introspection enabled)
//
// Example:
//
//	step := cogito.NewCategorize("ticket_type", "What type of ticket is this?", []string{"bug", "feature", "question"})
//	result, _ := step.Process(ctx, thought)
//	resp, _ := step.Scan(result)
//	fmt.Println(resp.Primary, resp.Confidence, resp.Reasoning)
func NewCategorize(key, question string, categories []string) *Categorize {
	return &Categorize{
		identity:         pipz.NewIdentity(key, "Multi-class categorization primitive"),
		key:              key,
		question:         question,
		categories:       categories,
		useIntrospection: DefaultIntrospection,
		temperature:      DefaultReasoningTemperature,
	}
}

// Process implements pipz.Chainable[*Thought].
func (c *Categorize) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	// Resolve provider
	provider, err := ResolveProvider(ctx, c.provider)
	if err != nil {
		return t, fmt.Errorf("categorize: %w", err)
	}

	// Create zyn classification synapse
	classificationSynapse, err := zyn.Classification(c.question, c.categories, provider)
	if err != nil {
		return t, fmt.Errorf("categorize: failed to create classification synapse: %w", err)
	}

	// Get unpublished notes
	unpublished := t.GetUnpublishedNotes()
	noteContext := RenderNotesToContext(unpublished)

	// Emit step started
	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(c.key),
		FieldStepType.Field("categorize"),
		FieldUnpublishedCount.Field(len(unpublished)),
		FieldTemperature.Field(c.temperature),
	)

	// Determine reasoning temperature
	reasoningTemp := c.temperature
	if c.reasoningTemperature != 0 {
		reasoningTemp = c.reasoningTemperature
	}

	// PHASE 1: REASONING - Classification
	classResponse, err := classificationSynapse.FireWithInput(ctx, t.Session, zyn.ClassificationInput{
		Subject:     c.question,
		Context:     noteContext,
		Temperature: reasoningTemp,
	})
	if err != nil {
		c.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("categorize: classification synapse execution failed: %w", err)
	}

	// Store full response as JSON
	respJSON, err := json.Marshal(classResponse)
	if err != nil {
		c.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("categorize: failed to marshal response: %w", err)
	}
	if err := t.SetContent(ctx, c.key, string(respJSON), "categorize"); err != nil {
		c.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("categorize: failed to persist note: %w", err)
	}

	// PHASE 2: INTROSPECTION - Semantic summary (optional)
	if c.useIntrospection {
		if err := c.runIntrospection(ctx, t, classResponse, unpublished, provider); err != nil {
			c.emitFailed(ctx, t, start, err)
			return t, err
		}
	}

	// Mark notes as published
	t.MarkNotesPublished()

	// Emit step completed
	duration := time.Since(start)
	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(c.key),
		FieldStepType.Field("categorize"),
		FieldStepDuration.Field(duration),
		FieldNoteCount.Field(len(t.AllNotes())),
	)

	return t, nil
}

// runIntrospection executes the transform synapse for semantic summary.
func (c *Categorize) runIntrospection(ctx context.Context, t *Thought, resp zyn.ClassificationResponse, originalNotes []Note, provider Provider) error {
	return runIntrospection(ctx, t, provider, c.buildIntrospectionInput(resp, originalNotes), introspectionConfig{
		stepType:                 "categorize",
		key:                      c.key,
		summaryKey:               c.summaryKey,
		introspectionTemperature: c.introspectionTemperature,
		synapsePrompt:            "Synthesize classification into context for next reasoning step",
	})
}

// buildIntrospectionInput formats classification for the transform synapse.
func (c *Categorize) buildIntrospectionInput(resp zyn.ClassificationResponse, originalNotes []Note) zyn.TransformInput {
	classText := fmt.Sprintf(
		"Classification: %s (confidence: %.2f)\n",
		resp.Primary,
		resp.Confidence,
	)

	if resp.Secondary != "" {
		classText += fmt.Sprintf("Secondary: %s\n", resp.Secondary)
	}

	classText += "Reasoning:\n"
	for i, reason := range resp.Reasoning {
		classText += fmt.Sprintf("  %d. %s\n", i+1, reason)
	}

	return zyn.TransformInput{
		Text:    classText,
		Context: RenderNotesToContext(originalNotes),
		Style:   "Synthesize this classification into rich semantic context for the next reasoning step. Focus on implications of the category choice, what it means for downstream actions, and actionable insights. Be concise but comprehensive.",
	}
}

// emitFailed emits a step failed event.
func (c *Categorize) emitFailed(ctx context.Context, t *Thought, start time.Time, err error) {
	capitan.Error(ctx, StepFailed,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(c.key),
		FieldStepType.Field("categorize"),
		FieldStepDuration.Field(time.Since(start)),
		FieldError.Field(err),
	)
}

// Identity implements pipz.Chainable[*Thought].
func (c *Categorize) Identity() pipz.Identity {
	return c.identity
}

// Schema implements pipz.Chainable[*Thought].
func (c *Categorize) Schema() pipz.Node {
	return pipz.Node{Identity: c.identity, Type: "categorize"}
}

// Close implements pipz.Chainable[*Thought].
func (c *Categorize) Close() error {
	return nil
}

// Scan retrieves the typed classification response from a thought.
func (c *Categorize) Scan(t *Thought) (*zyn.ClassificationResponse, error) {
	content, err := t.GetContent(c.key)
	if err != nil {
		return nil, fmt.Errorf("categorize scan: %w", err)
	}
	var resp zyn.ClassificationResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return nil, fmt.Errorf("categorize scan: failed to unmarshal response: %w", err)
	}
	return &resp, nil
}

// Builder methods

// WithProvider sets the provider for this step.
func (c *Categorize) WithProvider(p Provider) *Categorize {
	c.provider = p
	return c
}

// WithTemperature sets the default temperature for this step.
func (c *Categorize) WithTemperature(temp float32) *Categorize {
	c.temperature = temp
	return c
}

// WithIntrospection enables the introspection phase.
func (c *Categorize) WithIntrospection() *Categorize {
	c.useIntrospection = true
	return c
}

// WithSummaryKey sets a custom key for the introspection summary note.
func (c *Categorize) WithSummaryKey(key string) *Categorize {
	c.summaryKey = key
	return c
}

// WithReasoningTemperature sets the temperature for the reasoning phase.
func (c *Categorize) WithReasoningTemperature(temp float32) *Categorize {
	c.reasoningTemperature = temp
	return c
}

// WithIntrospectionTemperature sets the temperature for the introspection phase.
func (c *Categorize) WithIntrospectionTemperature(temp float32) *Categorize {
	c.introspectionTemperature = temp
	return c
}
