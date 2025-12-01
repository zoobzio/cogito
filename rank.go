package cogito

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
	"github.com/zoobzio/zyn"
)

// rankConfig implements stepConfig for ranking/sorting steps.
type rankConfig struct {
	criteria                 string   // Ranking criteria
	outputKey                string
	items                    []string // Explicit items to rank (optional)
	itemsKey                 string   // Note key to read items from (optional)
	summaryKey               string   // Custom key for summary note (if empty, uses outputKey+"_summary")
	useIntrospection         bool     // Whether to use Transform synapse for semantic summary
	reasoningTemperature     float32  // Temperature for Ranking synapse (0 = use step default)
	introspectionTemperature float32  // Temperature for Transform synapse (0 = use creative default)
}

// buildRankIntrospectionInput formats ranking result and original notes
// into input for the Transform synapse to generate semantic summary.
func buildRankIntrospectionInput(rankResponse zyn.RankingResponse, originalNotes []Note) zyn.TransformInput {
	// Format the ranking result
	rankText := fmt.Sprintf("Ranked items (confidence: %.2f):\n", rankResponse.Confidence)
	for i, item := range rankResponse.Ranked {
		rankText += fmt.Sprintf("  %d. %s\n", i+1, item)
	}

	rankText += "Reasoning:\n"
	for i, reason := range rankResponse.Reasoning {
		rankText += fmt.Sprintf("  %d. %s\n", i+1, reason)
	}

	// Include original context
	originalContext := RenderNotesToContext(originalNotes)

	return zyn.TransformInput{
		Text:    rankText,
		Context: originalContext,
		Style:   "Synthesize this ranking into rich semantic context for the next reasoning step. Focus on why the top items rank highly, what patterns emerge, and actionable insights about priority. Be concise but comprehensive.",
	}
}

// build creates the pipz pipeline for a Rank step.
// It gathers unpublished Notes, calls zyn Ranking synapse,
// and optionally calls Transform synapse for semantic summary.
func (c *rankConfig) build(ctx context.Context, provider Provider, temperature float32) (pipz.Chainable[*Thought], error) {
	// Create zyn ranking synapse (reasoning phase)
	rankingSynapse, err := zyn.NewRanking(c.criteria, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to create ranking synapse: %w", err)
	}

	// Create zyn transform synapse (introspection phase, if enabled)
	var transformSynapse *zyn.TransformSynapse
	if c.useIntrospection {
		transformSynapse, err = zyn.Transform(
			"Synthesize ranking into context for next reasoning step",
			provider,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create transform synapse: %w", err)
		}
	}

	// Return a pipz processor that handles Thought → zyn(s) → Thought
	return pipz.Apply(pipz.Name("rank"), func(ctx context.Context, t *Thought) (*Thought, error) {
		var items []string

		// Resolve items to rank (two-mode resolution)

		// Mode 1: Explicit items provided at construction time
		if len(c.items) > 0 {
			items = c.items

		// Mode 2: Read items from specific note key
		} else if c.itemsKey != "" {
			itemsJSON, err := t.GetContent(c.itemsKey)
			if err != nil {
				return t, fmt.Errorf("items note %q not found: %w", c.itemsKey, err)
			}
			if err := json.Unmarshal([]byte(itemsJSON), &items); err != nil {
				return t, fmt.Errorf("failed to parse items from %q: %w", c.itemsKey, err)
			}

		} else {
			return t, fmt.Errorf("rank step requires either explicit items or itemsKey")
		}

		if len(items) == 0 {
			return t, fmt.Errorf("no items to rank")
		}

		// Get unpublished notes for introspection context
		unpublished := t.GetUnpublishedNotes()

		// Determine reasoning temperature (use config if set, otherwise step default)
		reasoningTemp := temperature
		if c.reasoningTemperature != 0 {
			reasoningTemp = c.reasoningTemperature
		}

		// PHASE 1: REASONING - Ranking

		rankResponse, err := rankingSynapse.FireWithInput(ctx, t.Session, zyn.RankingInput{
			Items:       items,
			Context:     c.criteria,
			Temperature: reasoningTemp,
		})
		if err != nil {
			return t, fmt.Errorf("ranking synapse execution failed: %w", err)
		}

		// Write ranking result as Note with metadata
		// Content is the ranked list joined by newlines
		rankedList := strings.Join(rankResponse.Ranked, "\n")

		metadata := map[string]string{
			"confidence": fmt.Sprintf("%.2f", rankResponse.Confidence),
		}

		// Add reasoning if available
		if len(rankResponse.Reasoning) > 0 {
			for i, reason := range rankResponse.Reasoning {
				metadata[fmt.Sprintf("reasoning_%d", i)] = reason
			}
		}

		t.SetNote(c.outputKey, rankedList, "rank", metadata)

		// PHASE 2: INTROSPECTION - Semantic summary (optional)
		if c.useIntrospection && transformSynapse != nil {
			// Emit introspection started event
			capitan.Emit(ctx, IntrospectionStarted,
				FieldTraceID.Field(t.TraceID),
				FieldStepType.Field("rank"),
			)

			introspectionInput := buildRankIntrospectionInput(rankResponse, unpublished)

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
			t.SetContent(summaryKey, summary, "rank-introspection")

			// Emit introspection completed event
			capitan.Emit(ctx, IntrospectionCompleted,
				FieldTraceID.Field(t.TraceID),
				FieldStepType.Field("rank"),
				FieldContextSize.Field(len(summary)),
			)
		}

		// Mark all notes as published after successful execution
		t.MarkNotesPublished()

		return t, nil
	}), nil
}

// stepType returns the semantic type of this step.
func (c *rankConfig) stepType() string {
	return "rank"
}

// defaultTemperature returns the default temperature for ranking.
func (c *rankConfig) defaultTemperature() float32 {
	return zyn.DefaultTemperatureAnalytical
}

// Implement builder interface methods

func (c *rankConfig) withIntrospection(enabled bool) stepConfig {
	newCfg := *c
	newCfg.useIntrospection = enabled
	return &newCfg
}

func (c *rankConfig) withSummaryKey(key string) stepConfig {
	newCfg := *c
	newCfg.summaryKey = key
	return &newCfg
}

func (c *rankConfig) withReasoningTemperature(temp float32) stepConfig {
	newCfg := *c
	newCfg.reasoningTemperature = temp
	return &newCfg
}

func (c *rankConfig) withIntrospectionTemperature(temp float32) stepConfig {
	newCfg := *c
	newCfg.introspectionTemperature = temp
	return &newCfg
}

// RankItems creates a new ranking step with explicit items to rank.
// The items are provided at construction time and ranked by the given criteria.
//
// The step uses two zyn synapses:
//  1. Ranking synapse: Ranks items by criteria
//  2. Transform synapse: Synthesizes a semantic summary for context accumulation
//
// Output Notes:
//   - {key}: Ranked items (newline-separated), with metadata:
//     - confidence: Ranking confidence (0.0-1.0)
//     - reasoning_N: Step-by-step reasoning
//   - {key}_summary: Semantic summary for next steps (if introspection enabled)
//
// Parameters:
//   - key: The note key to write results to (also used as step name)
//   - criteria: Ranking criteria (e.g., "urgency", "importance", "relevance")
//   - items: The items to rank
//
// Example:
//
//	items := []string{
//	    "ticket_1: Login broken",
//	    "ticket_2: Feature request",
//	    "ticket_3: Critical outage",
//	}
//	step := cogito.RankItems("ticket_priority", "urgency and impact", items)
//	thought := cogito.New("process tickets")
//	result, _ := step.Process(ctx, thought)
//
//	// Access ranked list
//	ranked, _ := result.GetContent("ticket_priority")
//	// "ticket_3: Critical outage\nticket_1: Login broken\nticket_2: Feature request"
//
// Use WithoutIntrospection() to disable the Transform synapse and only get the ranking.
func RankItems(key, criteria string, items []string) *Step {
	return newStep(key, &rankConfig{
		criteria:         criteria,
		outputKey:        key,
		items:            items,
		useIntrospection: true,
	})
}

// RankFrom creates a new ranking step that reads items from a note.
// The items are read from the specified note key (expected to be JSON array of strings).
//
// The step uses two zyn synapses:
//  1. Ranking synapse: Ranks items by criteria
//  2. Transform synapse: Synthesizes a semantic summary for context accumulation
//
// Output Notes:
//   - {key}: Ranked items (newline-separated), with metadata:
//     - confidence: Ranking confidence (0.0-1.0)
//     - reasoning_N: Step-by-step reasoning
//   - {key}_summary: Semantic summary for next steps (if introspection enabled)
//
// Parameters:
//   - key: The note key to write results to (also used as step name)
//   - criteria: Ranking criteria (e.g., "urgency", "importance", "relevance")
//   - itemsKey: The note key containing JSON array of items to rank
//
// Example:
//
//	// First, extract items into a note
//	type TicketList struct {
//	    Tickets []string `json:"tickets"`
//	}
//	cogito.Analyze[TicketList]("ticket_list", "list of all tickets"),
//
//	// Then rank them
//	cogito.RankFrom("ticket_priority", "urgency and impact", "ticket_list"),
//
// Use WithoutIntrospection() to disable the Transform synapse and only get the ranking.
func RankFrom(key, criteria, itemsKey string) *Step {
	return newStep(key, &rankConfig{
		criteria:         criteria,
		outputKey:        key,
		itemsKey:         itemsKey,
		useIntrospection: true,
	})
}
