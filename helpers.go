package cogito

import (
	"context"
	"time"

	"github.com/zoobzio/pipz"
)

// -----------------------------------------------------------------------------
// Adapter Functions - wrap functions to create Thought processors
// -----------------------------------------------------------------------------

// Do creates a processor from a custom function that can fail.
// This is the easiest way to add custom logic to a chain.
//
// Example:
//
//	route := cogito.Do(pipz.NewIdentity("route-ticket", "Route tickets to queues"), func(ctx context.Context, t *cogito.Thought) (*cogito.Thought, error) {
//	    ticketType, _ := t.GetContent("ticket_type")
//	    if ticketType == "urgent" {
//	        t.SetContent("queue", "urgent-queue", "route-ticket")
//	    } else {
//	        t.SetContent("queue", "standard-queue", "route-ticket")
//	    }
//	    return t, nil
//	})
func Do(identity pipz.Identity, fn func(context.Context, *Thought) (*Thought, error)) pipz.Processor[*Thought] {
	return pipz.Apply(identity, fn)
}

// Transform creates a processor from a pure transformation function.
// Use this when your operation cannot fail.
//
// Example:
//
//	addMetadata := cogito.Transform(pipz.NewIdentity("add-metadata", "Add timestamp"), func(ctx context.Context, t *cogito.Thought) *cogito.Thought {
//	    t.SetContent("timestamp", time.Now().Format(time.RFC3339), "add-metadata")
//	    return t
//	})
func Transform(identity pipz.Identity, fn func(context.Context, *Thought) *Thought) pipz.Processor[*Thought] {
	return pipz.Transform(identity, fn)
}

// Effect creates a processor that performs a side effect without modifying the thought.
// Use this for logging, metrics, or other observational operations.
//
// Example:
//
//	logger := cogito.Effect(pipz.NewIdentity("log-intent", "Log processing intent"), func(ctx context.Context, t *cogito.Thought) error {
//	    log.Printf("Processing thought with intent: %s", t.Intent)
//	    return nil
//	})
func Effect(identity pipz.Identity, fn func(context.Context, *Thought) error) pipz.Processor[*Thought] {
	return pipz.Effect(identity, fn)
}

// Mutate creates a processor that conditionally modifies a thought.
// The modification is only applied if the predicate returns true.
//
// Example:
//
//	prioritize := cogito.Mutate(pipz.NewIdentity("prioritize", "Set high priority for critical items"),
//	    func(ctx context.Context, t *cogito.Thought) *cogito.Thought {
//	        t.SetContent("priority", "high", "prioritize")
//	        return t
//	    },
//	    func(ctx context.Context, t *cogito.Thought) bool {
//	        urgency, _ := t.GetContent("urgency")
//	        return urgency == "critical"
//	    },
//	)
func Mutate(identity pipz.Identity, fn func(context.Context, *Thought) *Thought, predicate func(context.Context, *Thought) bool) pipz.Processor[*Thought] {
	return pipz.Mutate(identity, fn, predicate)
}

// Enrich creates a processor that optionally enhances a thought.
// Unlike Do, errors are logged but don't stop the pipeline.
//
// Example:
//
//	addContext := cogito.Enrich(pipz.NewIdentity("add-context", "Fetch external context"), func(ctx context.Context, t *cogito.Thought) (*cogito.Thought, error) {
//	    extra, err := fetchExternalContext(ctx, t.Intent)
//	    if err != nil {
//	        return t, err // Logged but pipeline continues
//	    }
//	    t.SetContent("external_context", extra, "add-context")
//	    return t, nil
//	})
func Enrich(identity pipz.Identity, fn func(context.Context, *Thought) (*Thought, error)) pipz.Processor[*Thought] {
	return pipz.Enrich(identity, fn)
}

// -----------------------------------------------------------------------------
// Sequential Connectors - process thoughts in order
// -----------------------------------------------------------------------------

// Sequence creates a sequential pipeline of thought processors.
// Each processor receives the output of the previous one.
//
// Example:
//
//	pipeline := cogito.Sequence(pipz.NewIdentity("reasoning-chain", "Main reasoning pipeline"),
//	    cogito.NewAnalyze("analyze", "Examine the situation"),
//	    cogito.NewDecide("decide", "What action to take?"),
//	)
func Sequence(identity pipz.Identity, processors ...pipz.Chainable[*Thought]) *pipz.Sequence[*Thought] {
	return pipz.NewSequence(identity, processors...)
}

// -----------------------------------------------------------------------------
// Control Flow Connectors - route thoughts based on conditions
// -----------------------------------------------------------------------------

// Filter creates a conditional processor that either processes or passes through.
// When the predicate returns true, the processor is executed.
// When false, the thought passes through unchanged.
//
// Example:
//
//	onlyUrgent := cogito.Filter(pipz.NewIdentity("urgent-only", "Process only urgent items"),
//	    func(ctx context.Context, t *cogito.Thought) bool {
//	        priority, _ := t.GetContent("priority")
//	        return priority == "urgent"
//	    },
//	    urgentProcessor,
//	)
func Filter(identity pipz.Identity, predicate func(context.Context, *Thought) bool, processor pipz.Chainable[*Thought]) *pipz.Filter[*Thought] {
	return pipz.NewFilter(identity, predicate, processor)
}

// Switch creates a router that directs thoughts to different processors.
// The condition function returns a route key string that determines which processor handles the thought.
//
// Example:
//
//	router := cogito.Switch(pipz.NewIdentity("intent-router", "Route by category"), func(ctx context.Context, t *cogito.Thought) string {
//	    category, _ := t.GetContent("category")
//	    return category
//	})
//	router.AddRoute("question", questionHandler)
//	router.AddRoute("command", commandHandler)
func Switch(identity pipz.Identity, condition func(context.Context, *Thought) string) *pipz.Switch[*Thought] {
	return pipz.NewSwitch(identity, condition)
}

// Gate creates a simple pass/fail filter that blocks thoughts not meeting criteria.
// Unlike Filter which has a fallback processor, Gate simply passes through or blocks.
//
// Example:
//
//	validOnly := cogito.Gate(pipz.NewIdentity("valid-only", "Filter invalid thoughts"), func(ctx context.Context, t *cogito.Thought) bool {
//	    _, err := t.GetContent("required_field")
//	    return err == nil
//	})
func Gate(identity pipz.Identity, predicate func(context.Context, *Thought) bool) pipz.Processor[*Thought] {
	return pipz.Apply(identity, func(ctx context.Context, t *Thought) (*Thought, error) {
		if predicate(ctx, t) {
			return t, nil
		}
		return t, nil // Pass through unchanged when predicate fails
	})
}

// -----------------------------------------------------------------------------
// Error Handling Connectors - handle failures gracefully
// -----------------------------------------------------------------------------

// Fallback creates a processor that tries alternatives on failure.
// Each processor is tried in order until one succeeds.
//
// Example:
//
//	resilient := cogito.Fallback(pipz.NewIdentity("resilient-analysis", "Try multiple analyzers"),
//	    primaryAnalyzer,
//	    backupAnalyzer,
//	    fallbackAnalyzer,
//	)
func Fallback(identity pipz.Identity, processors ...pipz.Chainable[*Thought]) *pipz.Fallback[*Thought] {
	return pipz.NewFallback(identity, processors...)
}

// Retry creates a processor that retries on failure up to maxAttempts times.
// Immediate retry without delay - for backoff, use Backoff instead.
//
// Example:
//
//	reliable := cogito.Retry(pipz.NewIdentity("reliable-call", "Retry external service"), externalServiceCall, 3)
func Retry(identity pipz.Identity, processor pipz.Chainable[*Thought], maxAttempts int) *pipz.Retry[*Thought] {
	return pipz.NewRetry(identity, processor, maxAttempts)
}

// Backoff creates a processor that retries with exponential backoff.
// Useful for operations that need time to recover between attempts.
//
// Example:
//
//	resilient := cogito.Backoff(pipz.NewIdentity("api-call", "API with backoff"), apiProcessor, 5, time.Second)
func Backoff(identity pipz.Identity, processor pipz.Chainable[*Thought], maxAttempts int, baseDelay time.Duration) *pipz.Backoff[*Thought] {
	return pipz.NewBackoff(identity, processor, maxAttempts, baseDelay)
}

// Timeout creates a processor that enforces a time limit on execution.
// If the timeout expires, the operation is canceled and an error is returned.
//
// Example:
//
//	bounded := cogito.Timeout(pipz.NewIdentity("bounded-analysis", "Time-limited analysis"), analyzer, 30*time.Second)
func Timeout(identity pipz.Identity, processor pipz.Chainable[*Thought], duration time.Duration) *pipz.Timeout[*Thought] {
	return pipz.NewTimeout(identity, processor, duration)
}

// Handle creates a processor that handles errors without stopping the pipeline.
// When the primary processor fails, the error handler is invoked for monitoring.
// The error handler receives a pipz.Error[*Thought] with full error context.
//
// Example:
//
//	errorLogger := pipz.Effect(pipz.NewIdentity("log-error", "Log errors"), func(ctx context.Context, err *pipz.Error[*cogito.Thought]) error {
//	    log.Printf("thought %s failed: %v", err.InputData.TraceID, err.Err)
//	    return nil
//	})
//	observed := cogito.Handle(pipz.NewIdentity("observed", "Observed processor"), riskyProcessor, errorLogger)
func Handle(identity pipz.Identity, processor pipz.Chainable[*Thought], errorHandler pipz.Chainable[*pipz.Error[*Thought]]) *pipz.Handle[*Thought] {
	return pipz.NewHandle(identity, processor, errorHandler)
}

// -----------------------------------------------------------------------------
// Resource Protection Connectors - protect system resources
// -----------------------------------------------------------------------------

// RateLimiter creates a processor that enforces rate limits.
// Useful for protecting rate-limited external services.
//
// Example:
//
//	limited := cogito.RateLimiter(pipz.NewIdentity("api-limit", "API rate limiter"), 100, 10, apiCall) // 100/sec, burst 10
func RateLimiter(identity pipz.Identity, requestsPerSecond float64, burst int, processor pipz.Chainable[*Thought]) *pipz.RateLimiter[*Thought] {
	return pipz.NewRateLimiter(identity, requestsPerSecond, burst, processor)
}

// CircuitBreaker creates a processor that prevents cascade failures.
// Opens the circuit after failureThreshold consecutive failures.
//
// Example:
//
//	protected := cogito.CircuitBreaker(pipz.NewIdentity("service-call", "Protected service call"), apiProcessor, 5, 30*time.Second)
func CircuitBreaker(identity pipz.Identity, processor pipz.Chainable[*Thought], failureThreshold int, resetTimeout time.Duration) *pipz.CircuitBreaker[*Thought] {
	return pipz.NewCircuitBreaker(identity, processor, failureThreshold, resetTimeout)
}

// -----------------------------------------------------------------------------
// Parallel Connectors - process thoughts concurrently
// These require *Thought to implement pipz.Cloner[*Thought] (see thought.go Clone())
// -----------------------------------------------------------------------------

// Concurrent runs all processors in parallel and returns the original thought.
// Each processor receives an isolated clone. Use the reducer to aggregate results.
//
// Example:
//
//	parallel := cogito.Concurrent(pipz.NewIdentity("notify-all", "Parallel notifications"), nil,
//	    emailNotifier,
//	    smsNotifier,
//	    webhookNotifier,
//	)
func Concurrent(identity pipz.Identity, reducer func(original *Thought, results map[pipz.Identity]*Thought, errors map[pipz.Identity]error) *Thought, processors ...pipz.Chainable[*Thought]) *pipz.Concurrent[*Thought] {
	return pipz.NewConcurrent(identity, reducer, processors...)
}

// Race runs all processors in parallel and returns the first successful result.
// Useful for reducing latency when multiple paths can produce the same result.
//
// Example:
//
//	fastest := cogito.Race(pipz.NewIdentity("fastest-lookup", "First successful lookup"),
//	    cacheProcessor,
//	    databaseProcessor,
//	    externalAPIProcessor,
//	)
func Race(identity pipz.Identity, processors ...pipz.Chainable[*Thought]) *pipz.Race[*Thought] {
	return pipz.NewRace(identity, processors...)
}

// WorkerPool creates a bounded parallel executor with a fixed number of workers.
// Useful for controlling parallelism when processing multiple thought streams.
//
// Example:
//
//	pool := cogito.WorkerPool(pipz.NewIdentity("bounded-analysis", "Bounded worker pool"), 5,
//	    analyzer1,
//	    analyzer2,
//	    analyzer3,
//	)
func WorkerPool(identity pipz.Identity, workers int, processors ...pipz.Chainable[*Thought]) *pipz.WorkerPool[*Thought] {
	return pipz.NewWorkerPool(identity, workers, processors...)
}
