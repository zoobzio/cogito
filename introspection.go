package cogito

import (
	"context"
	"fmt"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/zyn"
)

// introspectionConfig holds parameters for running introspection.
type introspectionConfig struct {
	stepType                 string
	key                      string
	summaryKey               string
	introspectionTemperature float32
	synapsePrompt            string
}

// runIntrospection executes the transform synapse for semantic summary.
// This is shared logic used by all primitives that support introspection.
func runIntrospection(
	ctx context.Context,
	t *Thought,
	provider Provider,
	input zyn.TransformInput,
	cfg introspectionConfig,
) error {
	transformSynapse, err := zyn.Transform(cfg.synapsePrompt, provider)
	if err != nil {
		return fmt.Errorf("%s: failed to create transform synapse: %w", cfg.stepType, err)
	}

	// Determine introspection temperature
	introspectionTemp := DefaultIntrospectionTemperature
	if cfg.introspectionTemperature != 0 {
		introspectionTemp = cfg.introspectionTemperature
	}
	input.Temperature = introspectionTemp

	summary, err := transformSynapse.FireWithInput(ctx, t.Session, input)
	if err != nil {
		return fmt.Errorf("%s: transform synapse execution failed: %w", cfg.stepType, err)
	}

	// Determine summary key
	summaryKey := cfg.summaryKey
	if summaryKey == "" {
		summaryKey = cfg.key + "_summary"
	}

	source := cfg.stepType + "-introspection"
	if err := t.SetContent(ctx, summaryKey, summary, source); err != nil {
		return fmt.Errorf("%s: failed to persist introspection note: %w", cfg.stepType, err)
	}

	capitan.Emit(ctx, IntrospectionCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepType.Field(cfg.stepType),
		FieldContextSize.Field(len(summary)),
	)

	return nil
}
