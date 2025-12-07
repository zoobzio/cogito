package cogito

import "github.com/zoobzio/zyn"

// Default configuration for cogito primitives.
// These can be overridden per-step using builder methods.
var (
	// DefaultIntrospection controls whether primitives generate semantic summaries
	// after reasoning. Disabled by default for cost efficiency. Enable per-step
	// with WithIntrospection() or globally by setting this to true.
	DefaultIntrospection = false

	// DefaultReasoningTemperature is used for the primary LLM call in each primitive.
	// Defaults to deterministic (low temperature) for consistent outputs.
	DefaultReasoningTemperature = zyn.DefaultTemperatureDeterministic

	// DefaultIntrospectionTemperature is used for the transform synapse that
	// synthesizes semantic summaries. Defaults to creative (higher temperature)
	// for richer context generation.
	DefaultIntrospectionTemperature = zyn.DefaultTemperatureCreative
)
