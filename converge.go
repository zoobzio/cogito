package cogito

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
	"github.com/zoobzio/zyn"
)

// Converge is a parallel execution primitive with LLM-powered synthesis that implements pipz.Chainable[*Thought].
// It runs multiple processors concurrently, then uses an LLM to synthesize their outputs into a unified result.
//
// Unlike pipz.Concurrent which uses a programmatic reducer, Converge uses semantic synthesis
// to merge perspectives intelligently based on meaning rather than simple aggregation.
type Converge struct {
	key             string
	synthesisPrompt string
	processors      []pipz.Chainable[*Thought]

	// Configuration
	synthesisTemperature float32
	provider             Provider
	temperature          float32

	mu sync.RWMutex
}

// NewConverge creates a new parallel synthesis primitive.
//
// The primitive executes all processors concurrently on cloned thoughts, then uses
// zyn.Transform to synthesize their outputs into a unified result.
//
// Output Notes:
//   - {key}: The synthesized output combining all processor results
//   - Notes from each processor are preserved with their original keys
//
// Example:
//
//	converge := cogito.NewConverge(
//	    "unified_analysis",
//	    "Synthesize these perspectives into a unified recommendation, highlighting agreements and resolving conflicts",
//	    technicalAnalysis,
//	    businessAnalysis,
//	    riskAnalysis,
//	)
//	result, _ := converge.Process(ctx, thought)
//	synthesis, _ := converge.Scan(result)
//	fmt.Println(synthesis)
func NewConverge(key, synthesisPrompt string, processors ...pipz.Chainable[*Thought]) *Converge {
	return &Converge{
		key:             key,
		synthesisPrompt: synthesisPrompt,
		processors:      processors,
		temperature:     DefaultReasoningTemperature,
	}
}

// branchResult captures the outcome of a single parallel branch.
type branchResult struct {
	name   pipz.Name
	result *Thought
	err    error
}

// Process implements pipz.Chainable[*Thought].
func (c *Converge) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	c.mu.RLock()
	processors := make([]pipz.Chainable[*Thought], len(c.processors))
	copy(processors, c.processors)
	c.mu.RUnlock()

	if len(processors) == 0 {
		return t, nil
	}

	// Resolve provider
	provider, err := ResolveProvider(ctx, c.provider)
	if err != nil {
		return t, fmt.Errorf("converge: %w", err)
	}

	// Get unpublished notes and track original note count for merge filtering
	unpublished := t.GetUnpublishedNotes()
	originalNoteCount := len(t.AllNotes())

	// Emit step started
	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(c.key),
		FieldStepType.Field("converge"),
		FieldUnpublishedCount.Field(len(unpublished)),
		FieldBranchCount.Field(len(processors)),
		FieldTemperature.Field(c.temperature),
	)

	// PHASE 1: PARALLEL EXECUTION - Run all processors concurrently
	results := make(chan branchResult, len(processors))
	var wg sync.WaitGroup
	wg.Add(len(processors))

	for _, processor := range processors {
		go func(p pipz.Chainable[*Thought]) {
			defer wg.Done()

			// Emit branch started
			capitan.Emit(ctx, ConvergeBranchStarted,
				FieldTraceID.Field(t.TraceID),
				FieldStepName.Field(c.key),
				FieldBranchName.Field(string(p.Name())),
			)

			// Clone thought for isolated processing
			clone := t.Clone()

			// Process
			result, err := p.Process(ctx, clone)

			// Emit branch completed
			capitan.Emit(ctx, ConvergeBranchCompleted,
				FieldTraceID.Field(t.TraceID),
				FieldStepName.Field(c.key),
				FieldBranchName.Field(string(p.Name())),
				FieldError.Field(err),
			)

			results <- branchResult{
				name:   p.Name(),
				result: result,
				err:    err,
			}
		}(processor)
	}

	// Wait for all branches to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	branchResults := make(map[pipz.Name]*Thought)
	var branchErrors []error

	for br := range results {
		if br.err != nil {
			branchErrors = append(branchErrors, fmt.Errorf("branch %q: %w", br.name, br.err))
		} else {
			branchResults[br.name] = br.result
		}
	}

	// If all branches failed, return error
	if len(branchResults) == 0 && len(branchErrors) > 0 {
		joinedErr := errors.Join(branchErrors...)
		c.emitFailed(ctx, t, start, joinedErr)
		return t, fmt.Errorf("converge: all branches failed: %w", joinedErr)
	}

	// PHASE 2: MERGE NOTES - Collect notes from all successful branches
	mergedContext := c.buildMergedContext(branchResults, originalNoteCount)

	// Copy notes from successful branches to the original thought
	// Only copy notes added after the original note count (new notes from branch processing)
	for name, branchThought := range branchResults {
		branchNotes := branchThought.AllNotes()
		for i := originalNoteCount; i < len(branchNotes); i++ {
			note := branchNotes[i]
			// Tag the source with branch name
			taggedSource := fmt.Sprintf("%s[%s]", note.Source, name)
			if setErr := t.SetNote(ctx, note.Key, note.Content, taggedSource, note.Metadata); setErr != nil {
				c.emitFailed(ctx, t, start, setErr)
				return t, fmt.Errorf("converge: failed to merge note from branch %q: %w", name, setErr)
			}
		}
	}

	// PHASE 3: SYNTHESIS - LLM combines perspectives
	capitan.Emit(ctx, ConvergeSynthesisStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(c.key),
		FieldBranchCount.Field(len(branchResults)),
	)

	transformSynapse, err := zyn.Transform(c.synthesisPrompt, provider)
	if err != nil {
		c.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("converge: failed to create transform synapse: %w", err)
	}

	// Determine synthesis temperature
	synthesisTemp := c.temperature
	if c.synthesisTemperature != 0 {
		synthesisTemp = c.synthesisTemperature
	}

	synthesis, err := transformSynapse.FireWithInput(ctx, t.Session, zyn.TransformInput{
		Text:        mergedContext,
		Context:     RenderNotesToContext(unpublished),
		Style:       c.synthesisPrompt,
		Temperature: synthesisTemp,
	})
	if err != nil {
		c.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("converge: synthesis failed: %w", err)
	}

	// Store synthesis result
	if err := t.SetContent(ctx, c.key, synthesis, "converge"); err != nil {
		c.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("converge: failed to persist synthesis note: %w", err)
	}

	// Mark notes as published
	t.MarkNotesPublished()

	// Emit step completed
	duration := time.Since(start)
	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(c.key),
		FieldStepType.Field("converge"),
		FieldStepDuration.Field(duration),
		FieldNoteCount.Field(len(t.AllNotes())),
		FieldBranchCount.Field(len(branchResults)),
	)

	return t, nil
}

// buildMergedContext creates a formatted context from all branch results.
// originalNoteCount is used to filter out notes that existed before branching.
func (c *Converge) buildMergedContext(branchResults map[pipz.Name]*Thought, originalNoteCount int) string {
	var builder strings.Builder

	builder.WriteString("=== PARALLEL ANALYSIS RESULTS ===\n\n")

	for name, branchThought := range branchResults {
		builder.WriteString(fmt.Sprintf("--- Branch: %s ---\n", name))

		// Get notes created by this branch (after original notes)
		branchNotes := branchThought.AllNotes()
		for i := originalNoteCount; i < len(branchNotes); i++ {
			note := branchNotes[i]
			builder.WriteString(fmt.Sprintf("%s: %s\n", note.Key, note.Content))
		}
		builder.WriteString("\n")
	}

	return builder.String()
}

// emitFailed emits a step failed event.
func (c *Converge) emitFailed(ctx context.Context, t *Thought, start time.Time, err error) {
	capitan.Error(ctx, StepFailed,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(c.key),
		FieldStepType.Field("converge"),
		FieldStepDuration.Field(time.Since(start)),
		FieldError.Field(err),
	)
}

// Name implements pipz.Chainable[*Thought].
func (c *Converge) Name() pipz.Name {
	return pipz.Name(c.key)
}

// Close implements pipz.Chainable[*Thought].
// Propagates Close to all registered processors.
func (c *Converge) Close() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var errs []error
	for _, p := range c.processors {
		if err := p.Close(); err != nil {
			errs = append(errs, fmt.Errorf("processor %q: %w", p.Name(), err))
		}
	}

	return errors.Join(errs...)
}

// Scan retrieves the synthesis result from a thought.
func (c *Converge) Scan(t *Thought) (string, error) {
	content, err := t.GetContent(c.key)
	if err != nil {
		return "", fmt.Errorf("converge scan: %w", err)
	}
	return content, nil
}

// Builder methods

// WithProvider sets the provider for synthesis.
func (c *Converge) WithProvider(p Provider) *Converge {
	c.provider = p
	return c
}

// WithTemperature sets the default temperature for synthesis.
func (c *Converge) WithTemperature(temp float32) *Converge {
	c.temperature = temp
	return c
}

// WithSynthesisTemperature sets the temperature for the synthesis phase.
func (c *Converge) WithSynthesisTemperature(temp float32) *Converge {
	c.synthesisTemperature = temp
	return c
}

// Processor management methods

// AddProcessor adds a processor to the parallel execution list.
func (c *Converge) AddProcessor(processor pipz.Chainable[*Thought]) *Converge {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.processors = append(c.processors, processor)
	return c
}

// RemoveProcessor removes a processor by name.
func (c *Converge) RemoveProcessor(name pipz.Name) *Converge {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, p := range c.processors {
		if p.Name() == name {
			c.processors = append(c.processors[:i], c.processors[i+1:]...)
			break
		}
	}
	return c
}

// ClearProcessors removes all processors.
func (c *Converge) ClearProcessors() *Converge {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.processors = nil
	return c
}

// Processors returns a copy of the current processors list.
func (c *Converge) Processors() []pipz.Chainable[*Thought] {
	c.mu.RLock()
	defer c.mu.RUnlock()

	processors := make([]pipz.Chainable[*Thought], len(c.processors))
	copy(processors, c.processors)
	return processors
}
