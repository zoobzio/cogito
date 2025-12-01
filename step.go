package cogito

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
)

// stepConfig is an internal interface that different step types implement.
// It handles the specifics of building the internal pipeline and data mapping.
type stepConfig interface {
	// build creates the internal pipz pipeline for this step type
	build(ctx context.Context, provider Provider, temperature float32) (pipz.Chainable[*Thought], error)

	// stepType returns the semantic type (e.g., "decide", "analyze")
	stepType() string

	// defaultTemperature returns the default temperature for this step type
	defaultTemperature() float32
}

// Optional interfaces for builder method support

// introspectionToggler supports enabling/disabling introspection phase
type introspectionToggler interface {
	stepConfig
	withIntrospection(enabled bool) stepConfig
}

// summaryKeySetter supports custom summary note keys
type summaryKeySetter interface {
	stepConfig
	withSummaryKey(key string) stepConfig
}

// reasoningTempSetter supports custom reasoning phase temperature
type reasoningTempSetter interface {
	stepConfig
	withReasoningTemperature(temp float32) stepConfig
}

// introspectionTempSetter supports custom introspection phase temperature
type introspectionTempSetter interface {
	stepConfig
	withIntrospectionTemperature(temp float32) stepConfig
}

// Step represents a single cognitive operation in a chain of thought.
// It wraps zyn synapses and handles data extraction/update via the Note system.
type Step struct {
	name string
	cfg  stepConfig

	// Configuration
	provider    Provider
	temperature float32

	// Built pipeline (lazy initialization)
	pipeline pipz.Chainable[*Thought]
	once     sync.Once
	buildErr error
}

// newStep creates a new Step with the given configuration.
// This is internal - users create steps via Decide(), Analyze(), etc.
func newStep(name string, cfg stepConfig) *Step {
	return &Step{
		name:        name,
		cfg:         cfg,
		temperature: cfg.defaultTemperature(),
	}
}

// Process implements pipz.Chainable[*Thought].
// It builds the internal pipeline on first call (lazy init) and executes it.
func (s *Step) Process(ctx context.Context, t *Thought) (*Thought, error) {
	// Lazy initialization of pipeline
	s.once.Do(func() {
		s.buildErr = s.buildPipeline(ctx)
	})

	if s.buildErr != nil {
		return t, fmt.Errorf("failed to build step %q: %w", s.name, s.buildErr)
	}

	// Record execution start
	start := time.Now()
	unpublishedCount := len(t.GetUnpublishedNotes())

	// Emit step started event
	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(s.name),
		FieldStepType.Field(s.cfg.stepType()),
		FieldUnpublishedCount.Field(unpublishedCount),
		FieldTemperature.Field(s.temperature),
	)

	// Execute pipeline
	result, err := s.pipeline.Process(ctx, t)
	duration := time.Since(start)

	// Record step execution (input/output tracked via Note.Source)
	record := StepRecord{
		Name:      s.name,
		Type:      s.cfg.stepType(),
		Duration:  duration,
		Timestamp: start,
		Error:     err,
	}

	// Add step record to the returned thought
	if result != nil {
		result.AddStep(record)
	} else {
		// If result is nil, add to original thought
		t.AddStep(record)
	}

	// Emit step completion or failure event
	if err != nil {
		capitan.Error(ctx, StepFailed,
			FieldTraceID.Field(t.TraceID),
			FieldStepName.Field(s.name),
			FieldStepType.Field(s.cfg.stepType()),
			FieldStepDuration.Field(duration),
			FieldError.Field(err),
		)
	} else {
		capitan.Emit(ctx, StepCompleted,
			FieldTraceID.Field(t.TraceID),
			FieldStepName.Field(s.name),
			FieldStepType.Field(s.cfg.stepType()),
			FieldStepDuration.Field(duration),
			FieldNoteCount.Field(len(result.AllNotes())),
		)
	}

	return result, err
}

// Name implements pipz.Chainable[*Thought]
func (s *Step) Name() pipz.Name {
	return pipz.Name(s.name)
}

// Close implements pipz.Chainable[*Thought]
func (s *Step) Close() error {
	if s.pipeline != nil {
		return s.pipeline.Close()
	}
	return nil
}

// WithProvider sets the provider for this specific step.
// This takes precedence over context and global providers.
func (s *Step) WithProvider(p Provider) *Step {
	s.provider = p
	return s
}

// WithTemperature sets the LLM temperature for this step.
// Overrides the step type's default temperature.
func (s *Step) WithTemperature(temp float32) *Step {
	s.temperature = temp
	return s
}

// WithRetry wraps the step with retry logic.
func (s *Step) WithRetry(attempts int) *Step {
	// Create new step with wrapped config
	return &Step{
		name: s.name,
		cfg: &retryConfig{
			inner:    s.cfg,
			attempts: attempts,
		},
		provider:    s.provider,
		temperature: s.temperature,
	}
}

// WithTimeout wraps the step with timeout protection.
func (s *Step) WithTimeout(d time.Duration) *Step {
	return &Step{
		name: s.name,
		cfg: &timeoutConfig{
			inner:   s.cfg,
			timeout: d,
		},
		provider:    s.provider,
		temperature: s.temperature,
	}
}

// WithBackoff wraps the step with exponential backoff retry.
func (s *Step) WithBackoff(attempts int, baseDelay time.Duration) *Step {
	return &Step{
		name: s.name,
		cfg: &backoffConfig{
			inner:     s.cfg,
			attempts:  attempts,
			baseDelay: baseDelay,
		},
		provider:    s.provider,
		temperature: s.temperature,
	}
}

// WithCircuitBreaker wraps the step with circuit breaker protection.
func (s *Step) WithCircuitBreaker(failures int, recovery time.Duration) *Step {
	return &Step{
		name: s.name,
		cfg: &circuitBreakerConfig{
			inner:    s.cfg,
			failures: failures,
			recovery: recovery,
		},
		provider:    s.provider,
		temperature: s.temperature,
	}
}

// WithoutIntrospection disables the introspection phase for supported primitives.
// Affects Decide and Analyze steps, which by default use two synapses:
// a reasoning synapse and a transform synapse for semantic summary.
// With introspection disabled, only the reasoning synapse fires.
func (s *Step) WithoutIntrospection() *Step {
	// Clone the step config and disable introspection if it supports it
	newCfg := s.cfg

	// Type switch to handle different config types
	switch cfg := s.cfg.(type) {
	case *decideConfig:
		newDecideCfg := *cfg
		newDecideCfg.useIntrospection = false
		newCfg = &newDecideCfg
	case introspectionToggler:
		// For generic configs that support introspection toggling
		newCfg = cfg.withIntrospection(false)
	}

	return &Step{
		name:        s.name,
		cfg:         newCfg,
		provider:    s.provider,
		temperature: s.temperature,
	}
}

// WithSummaryKey sets a custom key for the introspection summary note.
// Affects Decide and Analyze steps with introspection enabled.
// By default, the summary key is "{outputKey}_summary".
func (s *Step) WithSummaryKey(key string) *Step {
	newCfg := s.cfg

	switch cfg := s.cfg.(type) {
	case *decideConfig:
		newDecideCfg := *cfg
		newDecideCfg.summaryKey = key
		newCfg = &newDecideCfg
	case summaryKeySetter:
		// For generic configs that support summary key customization
		newCfg = cfg.withSummaryKey(key)
	}

	return &Step{
		name:        s.name,
		cfg:         newCfg,
		provider:    s.provider,
		temperature: s.temperature,
	}
}

// WithReasoningTemperature sets the temperature for the reasoning phase.
// Affects Decide (Binary synapse) and Analyze (Extract synapse) steps.
// If not set, uses the step's default temperature.
func (s *Step) WithReasoningTemperature(temp float32) *Step {
	newCfg := s.cfg

	switch cfg := s.cfg.(type) {
	case *decideConfig:
		newDecideCfg := *cfg
		newDecideCfg.reasoningTemperature = temp
		newCfg = &newDecideCfg
	case reasoningTempSetter:
		// For generic configs that support reasoning temperature customization
		newCfg = cfg.withReasoningTemperature(temp)
	}

	return &Step{
		name:        s.name,
		cfg:         newCfg,
		provider:    s.provider,
		temperature: s.temperature,
	}
}

// WithIntrospectionTemperature sets the temperature for the introspection phase.
// Affects Decide and Analyze steps with introspection enabled (controls Transform synapse temperature).
// If not set, uses DefaultTemperatureCreative for more expressive summaries.
func (s *Step) WithIntrospectionTemperature(temp float32) *Step {
	newCfg := s.cfg

	switch cfg := s.cfg.(type) {
	case *decideConfig:
		newDecideCfg := *cfg
		newDecideCfg.introspectionTemperature = temp
		newCfg = &newDecideCfg
	case introspectionTempSetter:
		// For generic configs that support introspection temperature customization
		newCfg = cfg.withIntrospectionTemperature(temp)
	}

	return &Step{
		name:        s.name,
		cfg:         newCfg,
		provider:    s.provider,
		temperature: s.temperature,
	}
}

// buildPipeline constructs the internal pipeline using the config.
func (s *Step) buildPipeline(ctx context.Context) error {
	// Resolve provider
	provider, err := ResolveProvider(ctx, s.provider)
	if err != nil {
		return err
	}

	// Build pipeline from config
	pipeline, err := s.cfg.build(ctx, provider, s.temperature)
	if err != nil {
		return err
	}

	s.pipeline = pipeline
	return nil
}

// Wrapper configs for reliability features

type retryConfig struct {
	inner    stepConfig
	attempts int
}

func (c *retryConfig) build(ctx context.Context, provider Provider, temp float32) (pipz.Chainable[*Thought], error) {
	inner, err := c.inner.build(ctx, provider, temp)
	if err != nil {
		return nil, err
	}
	return pipz.NewRetry("retry", inner, c.attempts), nil
}

func (c *retryConfig) stepType() string {
	return c.inner.stepType()
}

func (c *retryConfig) defaultTemperature() float32 {
	return c.inner.defaultTemperature()
}

type timeoutConfig struct {
	inner   stepConfig
	timeout time.Duration
}

func (c *timeoutConfig) build(ctx context.Context, provider Provider, temp float32) (pipz.Chainable[*Thought], error) {
	inner, err := c.inner.build(ctx, provider, temp)
	if err != nil {
		return nil, err
	}
	return pipz.NewTimeout("timeout", inner, c.timeout), nil
}

func (c *timeoutConfig) stepType() string {
	return c.inner.stepType()
}

func (c *timeoutConfig) defaultTemperature() float32 {
	return c.inner.defaultTemperature()
}

type backoffConfig struct {
	inner     stepConfig
	attempts  int
	baseDelay time.Duration
}

func (c *backoffConfig) build(ctx context.Context, provider Provider, temp float32) (pipz.Chainable[*Thought], error) {
	inner, err := c.inner.build(ctx, provider, temp)
	if err != nil {
		return nil, err
	}
	return pipz.NewBackoff("backoff", inner, c.attempts, c.baseDelay), nil
}

func (c *backoffConfig) stepType() string {
	return c.inner.stepType()
}

func (c *backoffConfig) defaultTemperature() float32 {
	return c.inner.defaultTemperature()
}

type circuitBreakerConfig struct {
	inner    stepConfig
	failures int
	recovery time.Duration
}

func (c *circuitBreakerConfig) build(ctx context.Context, provider Provider, temp float32) (pipz.Chainable[*Thought], error) {
	inner, err := c.inner.build(ctx, provider, temp)
	if err != nil {
		return nil, err
	}
	return pipz.NewCircuitBreaker("circuit-breaker", inner, c.failures, c.recovery), nil
}

func (c *circuitBreakerConfig) stepType() string {
	return c.inner.stepType()
}

func (c *circuitBreakerConfig) defaultTemperature() float32 {
	return c.inner.defaultTemperature()
}
