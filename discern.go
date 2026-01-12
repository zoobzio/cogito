package cogito

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
	"github.com/zoobzio/zyn"
)

// Discern is an LLM-powered semantic routing connector that implements pipz.Chainable[*Thought].
// It uses zyn.Classification directly to determine which route to take based on semantic analysis.
type Discern struct {
	identity   pipz.Identity
	key        string
	question   string
	categories []string
	routes     map[string]pipz.Chainable[*Thought]
	fallback   pipz.Chainable[*Thought]

	// Configuration
	useIntrospection         bool
	reasoningTemperature     float32
	introspectionTemperature float32
	summaryKey               string
	provider                 Provider
	temperature              float32

	mu sync.RWMutex
}

// NewDiscern creates a new semantic routing connector.
//
// The connector uses zyn.Classification to determine which route to take.
// Routes are matched against the Primary category from the classification response.
//
// Output Notes:
//   - {key}: JSON-serialized zyn.ClassificationResponse
//   - {key}_summary: Semantic summary for next steps (if introspection enabled)
//
// Example:
//
//	router := cogito.NewDiscern(
//	    "ticket_route",
//	    "What type of support ticket is this?",
//	    []string{"billing", "technical", "general"},
//	)
//	router.AddRoute("billing", billingPipeline)
//	router.AddRoute("technical", technicalPipeline)
//	router.SetFallback(generalPipeline)
func NewDiscern(key, question string, categories []string) *Discern {
	return &Discern{
		identity:         pipz.NewIdentity(key, "Semantic routing connector"),
		key:              key,
		question:         question,
		categories:       categories,
		routes:           make(map[string]pipz.Chainable[*Thought]),
		useIntrospection: DefaultIntrospection,
		temperature:      DefaultReasoningTemperature,
	}
}

// Process implements pipz.Chainable[*Thought].
func (d *Discern) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	// Resolve provider
	provider, err := ResolveProvider(ctx, d.provider)
	if err != nil {
		return t, fmt.Errorf("discern: %w", err)
	}

	// Create zyn classification synapse
	classificationSynapse, err := zyn.Classification(d.question, d.categories, provider)
	if err != nil {
		return t, fmt.Errorf("discern: failed to create classification synapse: %w", err)
	}

	// Get unpublished notes
	unpublished := t.GetUnpublishedNotes()
	noteContext := RenderNotesToContext(unpublished)

	// Emit step started
	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(d.key),
		FieldStepType.Field("discern"),
		FieldUnpublishedCount.Field(len(unpublished)),
		FieldTemperature.Field(d.temperature),
	)

	// Determine reasoning temperature
	reasoningTemp := d.temperature
	if d.reasoningTemperature != 0 {
		reasoningTemp = d.reasoningTemperature
	}

	// PHASE 1: CLASSIFICATION - Determine route
	classResponse, err := classificationSynapse.FireWithInput(ctx, t.Session, zyn.ClassificationInput{
		Subject:     d.question,
		Context:     noteContext,
		Temperature: reasoningTemp,
	})
	if err != nil {
		d.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("discern: classification failed: %w", err)
	}

	// Store classification response as JSON
	respJSON, err := json.Marshal(classResponse)
	if err != nil {
		d.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("discern: failed to marshal response: %w", err)
	}
	if setErr := t.SetContent(ctx, d.key, string(respJSON), "discern"); setErr != nil {
		d.emitFailed(ctx, t, start, setErr)
		return t, fmt.Errorf("discern: failed to persist note: %w", setErr)
	}

	// PHASE 2: INTROSPECTION - Semantic summary (optional)
	if d.useIntrospection {
		if introErr := d.runIntrospection(ctx, t, classResponse, unpublished, provider); introErr != nil {
			d.emitFailed(ctx, t, start, introErr)
			return t, introErr
		}
	}

	// Mark notes as published
	t.MarkNotesPublished()

	// PHASE 3: ROUTING - Execute appropriate processor
	d.mu.RLock()
	processor, exists := d.routes[classResponse.Primary]
	fallback := d.fallback
	d.mu.RUnlock()

	if exists {
		t, err = processor.Process(ctx, t)
		if err != nil {
			d.emitFailed(ctx, t, start, err)
			return t, fmt.Errorf("discern: route %q failed: %w", classResponse.Primary, err)
		}
	} else if fallback != nil {
		t, err = fallback.Process(ctx, t)
		if err != nil {
			d.emitFailed(ctx, t, start, err)
			return t, fmt.Errorf("discern: fallback failed: %w", err)
		}
	}
	// If no route and no fallback, pass through unchanged

	// Emit step completed
	duration := time.Since(start)
	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(d.key),
		FieldStepType.Field("discern"),
		FieldStepDuration.Field(duration),
		FieldNoteCount.Field(len(t.AllNotes())),
	)

	return t, nil
}

// runIntrospection executes the transform synapse for semantic summary.
func (d *Discern) runIntrospection(ctx context.Context, t *Thought, resp zyn.ClassificationResponse, originalNotes []Note, provider Provider) error {
	return runIntrospection(ctx, t, provider, d.buildIntrospectionInput(resp, originalNotes), introspectionConfig{
		stepType:                 "discern",
		key:                      d.key,
		summaryKey:               d.summaryKey,
		introspectionTemperature: d.introspectionTemperature,
		synapsePrompt:            "Synthesize routing decision into context for next reasoning step",
	})
}

// buildIntrospectionInput formats classification for the transform synapse.
func (d *Discern) buildIntrospectionInput(resp zyn.ClassificationResponse, originalNotes []Note) zyn.TransformInput {
	classText := fmt.Sprintf(
		"Routing Decision: %s (confidence: %.2f)\n",
		resp.Primary,
		resp.Confidence,
	)

	if resp.Secondary != "" {
		classText += fmt.Sprintf("Alternative: %s\n", resp.Secondary)
	}

	classText += "Reasoning:\n"
	for i, reason := range resp.Reasoning {
		classText += fmt.Sprintf("  %d. %s\n", i+1, reason)
	}

	return zyn.TransformInput{
		Text:    classText,
		Context: RenderNotesToContext(originalNotes),
		Style:   "Synthesize this routing decision into rich semantic context for the next reasoning step. Focus on why this route was chosen, what it implies for downstream processing, and actionable insights. Be concise but comprehensive.",
	}
}

// emitFailed emits a step failed event.
func (d *Discern) emitFailed(ctx context.Context, t *Thought, start time.Time, err error) {
	capitan.Error(ctx, StepFailed,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(d.key),
		FieldStepType.Field("discern"),
		FieldStepDuration.Field(time.Since(start)),
		FieldError.Field(err),
	)
}

// Identity implements pipz.Chainable[*Thought].
func (d *Discern) Identity() pipz.Identity {
	return d.identity
}

// Schema implements pipz.Chainable[*Thought].
func (d *Discern) Schema() pipz.Node {
	return pipz.Node{Identity: d.identity, Type: "discern"}
}

// Close implements pipz.Chainable[*Thought].
// Propagates Close to all registered routes and fallback.
func (d *Discern) Close() error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var errs []error

	for name, route := range d.routes {
		if err := route.Close(); err != nil {
			errs = append(errs, fmt.Errorf("route %q: %w", name, err))
		}
	}

	if d.fallback != nil {
		if err := d.fallback.Close(); err != nil {
			errs = append(errs, fmt.Errorf("fallback: %w", err))
		}
	}

	return errors.Join(errs...)
}

// Scan retrieves the typed classification response from a thought.
func (d *Discern) Scan(t *Thought) (*zyn.ClassificationResponse, error) {
	content, err := t.GetContent(d.key)
	if err != nil {
		return nil, fmt.Errorf("discern scan: %w", err)
	}
	var resp zyn.ClassificationResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return nil, fmt.Errorf("discern scan: failed to unmarshal response: %w", err)
	}
	return &resp, nil
}

// Builder methods

// WithProvider sets the provider for classification.
func (d *Discern) WithProvider(p Provider) *Discern {
	d.provider = p
	return d
}

// WithTemperature sets the default temperature for classification.
func (d *Discern) WithTemperature(temp float32) *Discern {
	d.temperature = temp
	return d
}

// WithIntrospection enables the introspection phase.
func (d *Discern) WithIntrospection() *Discern {
	d.useIntrospection = true
	return d
}

// WithSummaryKey sets a custom key for the introspection summary note.
func (d *Discern) WithSummaryKey(key string) *Discern {
	d.summaryKey = key
	return d
}

// WithReasoningTemperature sets the temperature for the classification phase.
func (d *Discern) WithReasoningTemperature(temp float32) *Discern {
	d.reasoningTemperature = temp
	return d
}

// WithIntrospectionTemperature sets the temperature for the introspection phase.
func (d *Discern) WithIntrospectionTemperature(temp float32) *Discern {
	d.introspectionTemperature = temp
	return d
}

// Route management methods

// AddRoute adds or updates a route for a category.
func (d *Discern) AddRoute(category string, processor pipz.Chainable[*Thought]) *Discern {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.routes[category] = processor
	return d
}

// RemoveRoute removes a route for a category.
func (d *Discern) RemoveRoute(category string) *Discern {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.routes, category)
	return d
}

// SetFallback sets the fallback processor for unmatched categories.
func (d *Discern) SetFallback(processor pipz.Chainable[*Thought]) *Discern {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.fallback = processor
	return d
}

// HasRoute checks if a route exists for a category.
func (d *Discern) HasRoute(category string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	_, exists := d.routes[category]
	return exists
}

// Routes returns a copy of the current routes map.
func (d *Discern) Routes() map[string]pipz.Chainable[*Thought] {
	d.mu.RLock()
	defer d.mu.RUnlock()

	routes := make(map[string]pipz.Chainable[*Thought], len(d.routes))
	for k, v := range d.routes {
		routes[k] = v
	}
	return routes
}

// ClearRoutes removes all routes.
func (d *Discern) ClearRoutes() *Discern {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.routes = make(map[string]pipz.Chainable[*Thought])
	return d
}
