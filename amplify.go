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

// AmplifyResult captures the outcome of an iterative refinement.
type AmplifyResult struct {
	Content    string   `json:"content"`    // Final refined content
	Iterations int      `json:"iterations"` // Number of iterations performed
	Completed  bool     `json:"completed"`  // Whether completion criteria was met
	Reasoning  []string `json:"reasoning"`  // Reasoning from final completion check
}

// Amplify is an iterative refinement primitive that implements pipz.Chainable[*Thought].
// It repeatedly refines content until an LLM determines completion criteria are met.
//
// Unlike pipz.Retry which retries on errors, Amplify iterates based on semantic quality.
// Each iteration transforms the content, then checks if the result meets the specified criteria.
type Amplify struct {
	identity           pipz.Identity
	key                string
	sourceKey          string
	refinementPrompt   string
	completionCriteria string
	maxIterations      int

	// Configuration
	refinementTemperature float32
	completionTemperature float32
	provider              Provider
	temperature           float32
}

// NewAmplify creates a new iterative refinement primitive.
//
// The primitive uses two zyn synapses per iteration:
//  1. Transform synapse: Refines the content based on refinementPrompt
//  2. Binary synapse: Checks if completionCriteria are met
//
// The loop continues until either:
//   - The completion criteria are satisfied (Binary returns true)
//   - maxIterations is reached
//
// Output Notes:
//   - {key}: JSON-serialized AmplifyResult
//
// Example:
//
//	refine := cogito.NewAmplify(
//	    "refined_response",
//	    "draft_response",
//	    "Improve clarity, remove redundancy, and ensure actionable recommendations",
//	    "The response is clear, concise, and provides specific actionable steps",
//	    3,
//	)
//	result, _ := refine.Process(ctx, thought)
//	output, _ := refine.Scan(result)
//	fmt.Println(output.Content, "in", output.Iterations, "iterations")
func NewAmplify(key, sourceKey, refinementPrompt, completionCriteria string, maxIterations int) *Amplify {
	if maxIterations < 1 {
		maxIterations = 1
	}
	return &Amplify{
		identity:           pipz.NewIdentity(key, "Iterative refinement primitive"),
		key:                key,
		sourceKey:          sourceKey,
		refinementPrompt:   refinementPrompt,
		completionCriteria: completionCriteria,
		maxIterations:      maxIterations,
		temperature:        DefaultReasoningTemperature,
	}
}

// Process implements pipz.Chainable[*Thought].
func (a *Amplify) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	// Resolve provider
	provider, err := ResolveProvider(ctx, a.provider)
	if err != nil {
		return t, fmt.Errorf("amplify: %w", err)
	}

	// Get source content
	content, err := t.GetContent(a.sourceKey)
	if err != nil {
		return t, fmt.Errorf("amplify: source key %q not found: %w", a.sourceKey, err)
	}

	// Get unpublished notes for context
	unpublished := t.GetUnpublishedNotes()
	noteContext := RenderNotesToContext(unpublished)

	// Emit step started
	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(a.key),
		FieldStepType.Field("amplify"),
		FieldUnpublishedCount.Field(len(unpublished)),
		FieldTemperature.Field(a.temperature),
	)

	// Create synapses
	transformSynapse, err := zyn.Transform(a.refinementPrompt, provider)
	if err != nil {
		a.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("amplify: failed to create transform synapse: %w", err)
	}

	binarySynapse, err := zyn.Binary(a.completionCriteria, provider)
	if err != nil {
		a.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("amplify: failed to create binary synapse: %w", err)
	}

	// Determine temperatures
	refinementTemp := a.temperature
	if a.refinementTemperature != 0 {
		refinementTemp = a.refinementTemperature
	}
	completionTemp := a.temperature
	if a.completionTemperature != 0 {
		completionTemp = a.completionTemperature
	}

	// Iterative refinement loop
	var completed bool
	var reasoning []string
	iteration := 0

	for iteration < a.maxIterations {
		iteration++

		// PHASE 1: REFINEMENT - Transform content
		var refined string
		refined, err = transformSynapse.FireWithInput(ctx, t.Session, zyn.TransformInput{
			Text:        content,
			Context:     noteContext,
			Style:       a.refinementPrompt,
			Temperature: refinementTemp,
		})
		if err != nil {
			a.emitFailed(ctx, t, start, err)
			return t, fmt.Errorf("amplify: refinement failed at iteration %d: %w", iteration, err)
		}
		content = refined

		// PHASE 2: COMPLETION CHECK - Binary decision
		var binaryResponse zyn.BinaryResponse
		binaryResponse, err = binarySynapse.FireWithInput(ctx, t.Session, zyn.BinaryInput{
			Subject:     a.completionCriteria,
			Context:     fmt.Sprintf("Content to evaluate:\n%s\n\nOriginal context:\n%s", content, noteContext),
			Temperature: completionTemp,
		})
		if err != nil {
			a.emitFailed(ctx, t, start, err)
			return t, fmt.Errorf("amplify: completion check failed at iteration %d: %w", iteration, err)
		}

		reasoning = binaryResponse.Reasoning

		// Emit iteration completed
		capitan.Emit(ctx, AmplifyIterationCompleted,
			FieldTraceID.Field(t.TraceID),
			FieldStepName.Field(a.key),
			FieldIterationCount.Field(iteration),
			FieldDecision.Field(binaryResponse.Decision),
			FieldConfidence.Field(binaryResponse.Confidence),
		)

		if binaryResponse.Decision {
			completed = true
			capitan.Emit(ctx, AmplifyCompleted,
				FieldTraceID.Field(t.TraceID),
				FieldStepName.Field(a.key),
				FieldIterationCount.Field(iteration),
			)
			break
		}
	}

	// Store result
	result := AmplifyResult{
		Content:    content,
		Iterations: iteration,
		Completed:  completed,
		Reasoning:  reasoning,
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		a.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("amplify: failed to marshal result: %w", err)
	}
	if err := t.SetContent(ctx, a.key, string(resultJSON), "amplify"); err != nil {
		a.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("amplify: failed to persist note: %w", err)
	}

	// Mark notes as published
	t.MarkNotesPublished()

	// Emit step completed
	duration := time.Since(start)
	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(a.key),
		FieldStepType.Field("amplify"),
		FieldStepDuration.Field(duration),
		FieldNoteCount.Field(len(t.AllNotes())),
		FieldIterationCount.Field(iteration),
	)

	return t, nil
}

// emitFailed emits a step failed event.
func (a *Amplify) emitFailed(ctx context.Context, t *Thought, start time.Time, err error) {
	capitan.Error(ctx, StepFailed,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(a.key),
		FieldStepType.Field("amplify"),
		FieldStepDuration.Field(time.Since(start)),
		FieldError.Field(err),
	)
}

// Identity implements pipz.Chainable[*Thought].
func (a *Amplify) Identity() pipz.Identity {
	return a.identity
}

// Schema implements pipz.Chainable[*Thought].
func (a *Amplify) Schema() pipz.Node {
	return pipz.Node{Identity: a.identity, Type: "amplify"}
}

// Close implements pipz.Chainable[*Thought].
func (a *Amplify) Close() error {
	return nil
}

// Scan retrieves the typed amplify result from a thought.
func (a *Amplify) Scan(t *Thought) (*AmplifyResult, error) {
	content, err := t.GetContent(a.key)
	if err != nil {
		return nil, fmt.Errorf("amplify scan: %w", err)
	}
	var result AmplifyResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("amplify scan: failed to unmarshal result: %w", err)
	}
	return &result, nil
}

// Builder methods

// WithProvider sets the provider for this step.
func (a *Amplify) WithProvider(p Provider) *Amplify {
	a.provider = p
	return a
}

// WithTemperature sets the default temperature for both refinement and completion phases.
func (a *Amplify) WithTemperature(temp float32) *Amplify {
	a.temperature = temp
	return a
}

// WithRefinementTemperature sets the temperature for the refinement phase.
func (a *Amplify) WithRefinementTemperature(temp float32) *Amplify {
	a.refinementTemperature = temp
	return a
}

// WithCompletionTemperature sets the temperature for the completion check phase.
func (a *Amplify) WithCompletionTemperature(temp float32) *Amplify {
	a.completionTemperature = temp
	return a
}

// WithMaxIterations sets the maximum number of refinement iterations.
func (a *Amplify) WithMaxIterations(maxIter int) *Amplify {
	if maxIter < 1 {
		maxIter = 1
	}
	a.maxIterations = maxIter
	return a
}
