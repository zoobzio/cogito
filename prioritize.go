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

// Prioritize is a prioritization primitive that implements pipz.Chainable[*Thought].
// It prioritizes items by criteria and stores the full response for typed retrieval.
type Prioritize struct {
	identity                 pipz.Identity
	key                      string
	criteria                 string
	items                    []string // Explicit items to rank (mode 1)
	itemsKey                 string   // Note key to read items from (mode 2)
	summaryKey               string
	useIntrospection         bool
	reasoningTemperature     float32
	introspectionTemperature float32
	provider                 Provider
	temperature              float32
}

// NewPrioritize creates a new prioritization primitive with explicit items to prioritize.
//
// The primitive uses two zyn synapses:
//  1. Ranking synapse: Prioritizes items by criteria
//  2. Transform synapse: Synthesizes a semantic summary for context accumulation
//
// Output Notes:
//   - {key}: JSON-serialized zyn.RankingResponse
//   - {key}_summary: Semantic summary for next steps (if introspection enabled)
//
// Example:
//
//	items := []string{
//	    "ticket_1: Login broken",
//	    "ticket_2: Feature request",
//	    "ticket_3: Critical outage",
//	}
//	step := cogito.NewPrioritize("ticket_priority", "urgency and impact", items)
//	result, _ := step.Process(ctx, thought)
//	resp, _ := step.Scan(result)
//	fmt.Println(resp.Ranked, resp.Confidence, resp.Reasoning)
func NewPrioritize(key, criteria string, items []string) *Prioritize {
	return &Prioritize{
		identity:         pipz.NewIdentity(key, "Prioritization primitive"),
		key:              key,
		criteria:         criteria,
		items:            items,
		useIntrospection: DefaultIntrospection,
		temperature:      DefaultReasoningTemperature,
	}
}

// NewPrioritizeFrom creates a new prioritization primitive that reads items from a note.
// The items are read from the specified note key (expected to be JSON array of strings).
//
// The primitive uses two zyn synapses:
//  1. Ranking synapse: Prioritizes items by criteria
//  2. Transform synapse: Synthesizes a semantic summary for context accumulation
//
// Output Notes:
//   - {key}: JSON-serialized zyn.RankingResponse
//   - {key}_summary: Semantic summary for next steps (if introspection enabled)
//
// Example:
//
//	// First, extract items into a note
//	type TicketList struct {
//	    Tickets []string `json:"tickets"`
//	}
//	cogito.NewAnalyze[TicketList]("ticket_list", "list of all tickets"),
//
//	// Then rank them
//	cogito.NewPrioritizeFrom("ticket_priority", "urgency and impact", "ticket_list"),
func NewPrioritizeFrom(key, criteria, itemsKey string) *Prioritize {
	return &Prioritize{
		identity:         pipz.NewIdentity(key, "Prioritization primitive (from note)"),
		key:              key,
		criteria:         criteria,
		itemsKey:         itemsKey,
		useIntrospection: DefaultIntrospection,
		temperature:      DefaultReasoningTemperature,
	}
}

// Process implements pipz.Chainable[*Thought].
func (r *Prioritize) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	// Resolve provider
	provider, err := ResolveProvider(ctx, r.provider)
	if err != nil {
		return t, fmt.Errorf("prioritize: %w", err)
	}

	// Resolve items (two-mode resolution)
	items, err := r.resolveItems(t)
	if err != nil {
		return t, err
	}

	// Create zyn ranking synapse
	rankingSynapse, err := zyn.NewRanking(r.criteria, provider)
	if err != nil {
		return t, fmt.Errorf("prioritize: failed to create ranking synapse: %w", err)
	}

	// Get unpublished notes
	unpublished := t.GetUnpublishedNotes()

	// Emit step started
	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(r.key),
		FieldStepType.Field("prioritize"),
		FieldUnpublishedCount.Field(len(unpublished)),
		FieldTemperature.Field(r.temperature),
	)

	// Determine reasoning temperature
	reasoningTemp := r.temperature
	if r.reasoningTemperature != 0 {
		reasoningTemp = r.reasoningTemperature
	}

	// PHASE 1: REASONING - Ranking
	rankResponse, err := rankingSynapse.FireWithInput(ctx, t.Session, zyn.RankingInput{
		Items:       items,
		Context:     r.criteria,
		Temperature: reasoningTemp,
	})
	if err != nil {
		r.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("prioritize: ranking synapse execution failed: %w", err)
	}

	// Store full response as JSON
	respJSON, err := json.Marshal(rankResponse)
	if err != nil {
		r.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("prioritize: failed to marshal response: %w", err)
	}
	if err := t.SetContent(ctx, r.key, string(respJSON), "prioritize"); err != nil {
		r.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("prioritize: failed to persist note: %w", err)
	}

	// PHASE 2: INTROSPECTION - Semantic summary (optional)
	if r.useIntrospection {
		if err := r.runIntrospection(ctx, t, rankResponse, unpublished, provider); err != nil {
			r.emitFailed(ctx, t, start, err)
			return t, err
		}
	}

	// Mark notes as published
	t.MarkNotesPublished()

	// Emit step completed
	duration := time.Since(start)
	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(r.key),
		FieldStepType.Field("prioritize"),
		FieldStepDuration.Field(duration),
		FieldNoteCount.Field(len(t.AllNotes())),
	)

	return t, nil
}

// resolveItems resolves items from explicit list or note key.
func (r *Prioritize) resolveItems(t *Thought) ([]string, error) {
	// Mode 1: Explicit items provided at construction time
	if len(r.items) > 0 {
		return r.items, nil
	}

	// Mode 2: Read items from specific note key
	if r.itemsKey != "" {
		itemsJSON, err := t.GetContent(r.itemsKey)
		if err != nil {
			return nil, fmt.Errorf("prioritize: items note %q not found: %w", r.itemsKey, err)
		}
		var items []string
		if err := json.Unmarshal([]byte(itemsJSON), &items); err != nil {
			return nil, fmt.Errorf("prioritize: failed to parse items from %q: %w", r.itemsKey, err)
		}
		if len(items) == 0 {
			return nil, fmt.Errorf("prioritize: no items to rank")
		}
		return items, nil
	}

	return nil, fmt.Errorf("prioritize: requires either explicit items or itemsKey")
}

// runIntrospection executes the transform synapse for semantic summary.
func (r *Prioritize) runIntrospection(ctx context.Context, t *Thought, resp zyn.RankingResponse, originalNotes []Note, provider Provider) error {
	return runIntrospection(ctx, t, provider, r.buildIntrospectionInput(resp, originalNotes), introspectionConfig{
		stepType:                 "prioritize",
		key:                      r.key,
		summaryKey:               r.summaryKey,
		introspectionTemperature: r.introspectionTemperature,
		synapsePrompt:            "Synthesize ranking into context for next reasoning step",
	})
}

// buildIntrospectionInput formats ranking for the transform synapse.
func (r *Prioritize) buildIntrospectionInput(resp zyn.RankingResponse, originalNotes []Note) zyn.TransformInput {
	rankText := fmt.Sprintf("Ranked items (confidence: %.2f):\n", resp.Confidence)
	for i, item := range resp.Ranked {
		rankText += fmt.Sprintf("  %d. %s\n", i+1, item)
	}

	rankText += "Reasoning:\n"
	for i, reason := range resp.Reasoning {
		rankText += fmt.Sprintf("  %d. %s\n", i+1, reason)
	}

	return zyn.TransformInput{
		Text:    rankText,
		Context: RenderNotesToContext(originalNotes),
		Style:   "Synthesize this ranking into rich semantic context for the next reasoning step. Focus on why the top items rank highly, what patterns emerge, and actionable insights about priority. Be concise but comprehensive.",
	}
}

// emitFailed emits a step failed event.
func (r *Prioritize) emitFailed(ctx context.Context, t *Thought, start time.Time, err error) {
	capitan.Error(ctx, StepFailed,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(r.key),
		FieldStepType.Field("prioritize"),
		FieldStepDuration.Field(time.Since(start)),
		FieldError.Field(err),
	)
}

// Identity implements pipz.Chainable[*Thought].
func (r *Prioritize) Identity() pipz.Identity {
	return r.identity
}

// Schema implements pipz.Chainable[*Thought].
func (r *Prioritize) Schema() pipz.Node {
	return pipz.Node{Identity: r.identity, Type: "prioritize"}
}

// Close implements pipz.Chainable[*Thought].
func (r *Prioritize) Close() error {
	return nil
}

// Scan retrieves the typed ranking response from a thought.
func (r *Prioritize) Scan(t *Thought) (*zyn.RankingResponse, error) {
	content, err := t.GetContent(r.key)
	if err != nil {
		return nil, fmt.Errorf("prioritize scan: %w", err)
	}
	var resp zyn.RankingResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return nil, fmt.Errorf("prioritize scan: failed to unmarshal response: %w", err)
	}
	return &resp, nil
}

// Builder methods

// WithProvider sets the provider for this step.
func (r *Prioritize) WithProvider(p Provider) *Prioritize {
	r.provider = p
	return r
}

// WithTemperature sets the default temperature for this step.
func (r *Prioritize) WithTemperature(temp float32) *Prioritize {
	r.temperature = temp
	return r
}

// WithIntrospection enables the introspection phase.
func (r *Prioritize) WithIntrospection() *Prioritize {
	r.useIntrospection = true
	return r
}

// WithSummaryKey sets a custom key for the introspection summary note.
func (r *Prioritize) WithSummaryKey(key string) *Prioritize {
	r.summaryKey = key
	return r
}

// WithReasoningTemperature sets the temperature for the reasoning phase.
func (r *Prioritize) WithReasoningTemperature(temp float32) *Prioritize {
	r.reasoningTemperature = temp
	return r
}

// WithIntrospectionTemperature sets the temperature for the introspection phase.
func (r *Prioritize) WithIntrospectionTemperature(temp float32) *Prioritize {
	r.introspectionTemperature = temp
	return r
}
