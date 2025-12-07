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
//	route := cogito.Do("route-ticket", func(ctx context.Context, t *cogito.Thought) (*cogito.Thought, error) {
//	    ticketType, _ := t.GetContent("ticket_type")
//	    if ticketType == "urgent" {
//	        t.SetContent("queue", "urgent-queue", "route-ticket")
//	    } else {
//	        t.SetContent("queue", "standard-queue", "route-ticket")
//	    }
//	    return t, nil
//	})
func Do(name string, fn func(context.Context, *Thought) (*Thought, error)) pipz.Processor[*Thought] {
	return pipz.Apply(pipz.Name(name), fn)
}

// Transform creates a processor from a pure transformation function.
// Use this when your operation cannot fail.
//
// Example:
//
//	addMetadata := cogito.Transform("add-metadata", func(ctx context.Context, t *cogito.Thought) *cogito.Thought {
//	    t.SetContent("timestamp", time.Now().Format(time.RFC3339), "add-metadata")
//	    return t
//	})
func Transform(name string, fn func(context.Context, *Thought) *Thought) pipz.Processor[*Thought] {
	return pipz.Transform(pipz.Name(name), fn)
}

// Effect creates a processor that performs a side effect without modifying the thought.
// Use this for logging, metrics, or other observational operations.
//
// Example:
//
//	logger := cogito.Effect("log-intent", func(ctx context.Context, t *cogito.Thought) error {
//	    log.Printf("Processing thought with intent: %s", t.Intent)
//	    return nil
//	})
func Effect(name string, fn func(context.Context, *Thought) error) pipz.Processor[*Thought] {
	return pipz.Effect(pipz.Name(name), fn)
}

// Mutate creates a processor that conditionally modifies a thought.
// The modification is only applied if the predicate returns true.
//
// Example:
//
//	prioritize := cogito.Mutate("prioritize",
//	    func(ctx context.Context, t *cogito.Thought) *cogito.Thought {
//	        t.SetContent("priority", "high", "prioritize")
//	        return t
//	    },
//	    func(ctx context.Context, t *cogito.Thought) bool {
//	        urgency, _ := t.GetContent("urgency")
//	        return urgency == "critical"
//	    },
//	)
func Mutate(name string, fn func(context.Context, *Thought) *Thought, predicate func(context.Context, *Thought) bool) pipz.Processor[*Thought] {
	return pipz.Mutate(pipz.Name(name), fn, predicate)
}

// Enrich creates a processor that optionally enhances a thought.
// Unlike Do, errors are logged but don't stop the pipeline.
//
// Example:
//
//	addContext := cogito.Enrich("add-context", func(ctx context.Context, t *cogito.Thought) (*cogito.Thought, error) {
//	    extra, err := fetchExternalContext(ctx, t.Intent)
//	    if err != nil {
//	        return t, err // Logged but pipeline continues
//	    }
//	    t.SetContent("external_context", extra, "add-context")
//	    return t, nil
//	})
func Enrich(name string, fn func(context.Context, *Thought) (*Thought, error)) pipz.Processor[*Thought] {
	return pipz.Enrich(pipz.Name(name), fn)
}

// -----------------------------------------------------------------------------
// Sequential Connectors - process thoughts in order
// -----------------------------------------------------------------------------

// Sequence creates a sequential pipeline of thought processors.
// Each processor receives the output of the previous one.
//
// Example:
//
//	pipeline := cogito.Sequence("reasoning-chain",
//	    cogito.NewAnalyze("analyze", "Examine the situation"),
//	    cogito.NewDecide("decide", "What action to take?", []string{"proceed", "wait", "abort"}),
//	)
func Sequence(name string, processors ...pipz.Chainable[*Thought]) *pipz.Sequence[*Thought] {
	return pipz.NewSequence(pipz.Name(name), processors...)
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
//	onlyUrgent := cogito.Filter("urgent-only",
//	    func(ctx context.Context, t *cogito.Thought) bool {
//	        priority, _ := t.GetContent("priority")
//	        return priority == "urgent"
//	    },
//	    urgentProcessor,
//	)
func Filter(name string, predicate func(context.Context, *Thought) bool, processor pipz.Chainable[*Thought]) *pipz.Filter[*Thought] {
	return pipz.NewFilter(pipz.Name(name), predicate, processor)
}

// Switch creates a router that directs thoughts to different processors.
// The condition function returns a route key that determines which processor handles the thought.
//
// Example:
//
//	router := cogito.Switch("intent-router", func(ctx context.Context, t *cogito.Thought) string {
//	    category, _ := t.GetContent("category")
//	    return category
//	})
//	router.AddRoute("question", questionHandler)
//	router.AddRoute("command", commandHandler)
//	router.AddRoute("statement", statementHandler)
func Switch[K comparable](name string, condition func(context.Context, *Thought) K) *pipz.Switch[*Thought, K] {
	return pipz.NewSwitch(pipz.Name(name), condition)
}

// Gate creates a simple pass/fail filter that blocks thoughts not meeting criteria.
// Unlike Filter which has a fallback processor, Gate simply passes through or blocks.
//
// Example:
//
//	validOnly := cogito.Gate("valid-only", func(ctx context.Context, t *cogito.Thought) bool {
//	    _, err := t.GetContent("required_field")
//	    return err == nil
//	})
func Gate(name string, predicate func(context.Context, *Thought) bool) pipz.Processor[*Thought] {
	return pipz.Apply(pipz.Name(name), func(ctx context.Context, t *Thought) (*Thought, error) {
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
//	resilient := cogito.Fallback("resilient-analysis",
//	    primaryAnalyzer,
//	    backupAnalyzer,
//	    fallbackAnalyzer,
//	)
func Fallback(name string, processors ...pipz.Chainable[*Thought]) *pipz.Fallback[*Thought] {
	return pipz.NewFallback(pipz.Name(name), processors...)
}

// Retry creates a processor that retries on failure up to maxAttempts times.
// Immediate retry without delay - for backoff, use Backoff instead.
//
// Example:
//
//	reliable := cogito.Retry("reliable-call", externalServiceCall, 3)
func Retry(name string, processor pipz.Chainable[*Thought], maxAttempts int) *pipz.Retry[*Thought] {
	return pipz.NewRetry(pipz.Name(name), processor, maxAttempts)
}

// Backoff creates a processor that retries with exponential backoff.
// Useful for operations that need time to recover between attempts.
//
// Example:
//
//	resilient := cogito.Backoff("api-call", apiProcessor, 5, time.Second)
func Backoff(name string, processor pipz.Chainable[*Thought], maxAttempts int, baseDelay time.Duration) *pipz.Backoff[*Thought] {
	return pipz.NewBackoff(pipz.Name(name), processor, maxAttempts, baseDelay)
}

// Timeout creates a processor that enforces a time limit on execution.
// If the timeout expires, the operation is canceled and an error is returned.
//
// Example:
//
//	bounded := cogito.Timeout("bounded-analysis", analyzer, 30*time.Second)
func Timeout(name string, processor pipz.Chainable[*Thought], duration time.Duration) *pipz.Timeout[*Thought] {
	return pipz.NewTimeout(pipz.Name(name), processor, duration)
}

// Handle creates a processor that handles errors without stopping the pipeline.
// When the primary processor fails, the error handler is invoked for monitoring.
// The error handler receives a pipz.Error[*Thought] with full error context.
//
// Example:
//
//	errorLogger := pipz.Effect(pipz.Name("log-error"), func(ctx context.Context, err *pipz.Error[*cogito.Thought]) error {
//	    log.Printf("thought %s failed: %v", err.InputData.TraceID, err.Err)
//	    return nil
//	})
//	observed := cogito.Handle("observed", riskyProcessor, errorLogger)
func Handle(name string, processor pipz.Chainable[*Thought], errorHandler pipz.Chainable[*pipz.Error[*Thought]]) *pipz.Handle[*Thought] {
	return pipz.NewHandle(pipz.Name(name), processor, errorHandler)
}

// -----------------------------------------------------------------------------
// Resource Protection Connectors - protect system resources
// -----------------------------------------------------------------------------

// RateLimiter creates a processor that enforces rate limits.
// Useful for protecting rate-limited external services.
//
// Example:
//
//	limited := cogito.RateLimiter("api-limit", 100, 10) // 100/sec, burst 10
//	limited.SetProcessor(apiCall)
func RateLimiter(name string, requestsPerSecond float64, burst int) *pipz.RateLimiter[*Thought] {
	return pipz.NewRateLimiter[*Thought](pipz.Name(name), requestsPerSecond, burst)
}

// CircuitBreaker creates a processor that prevents cascade failures.
// Opens the circuit after failureThreshold consecutive failures.
//
// Example:
//
//	protected := cogito.CircuitBreaker("service-call", apiProcessor, 5, 30*time.Second)
func CircuitBreaker(name string, processor pipz.Chainable[*Thought], failureThreshold int, resetTimeout time.Duration) *pipz.CircuitBreaker[*Thought] {
	return pipz.NewCircuitBreaker(pipz.Name(name), processor, failureThreshold, resetTimeout)
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
//	parallel := cogito.Concurrent("notify-all", nil, // no reducer
//	    emailNotifier,
//	    smsNotifier,
//	    webhookNotifier,
//	)
func Concurrent(name string, reducer func(original *Thought, results map[pipz.Name]*Thought, errors map[pipz.Name]error) *Thought, processors ...pipz.Chainable[*Thought]) *pipz.Concurrent[*Thought] {
	return pipz.NewConcurrent(pipz.Name(name), reducer, processors...)
}

// Race runs all processors in parallel and returns the first successful result.
// Useful for reducing latency when multiple paths can produce the same result.
//
// Example:
//
//	fastest := cogito.Race("fastest-lookup",
//	    cacheProcessor,
//	    databaseProcessor,
//	    externalAPIProcessor,
//	)
func Race(name string, processors ...pipz.Chainable[*Thought]) *pipz.Race[*Thought] {
	return pipz.NewRace(pipz.Name(name), processors...)
}

// WorkerPool creates a bounded parallel executor with a fixed number of workers.
// Useful for controlling parallelism when processing multiple thought streams.
//
// Example:
//
//	pool := cogito.WorkerPool("bounded-analysis", 5,
//	    analyzer1,
//	    analyzer2,
//	    analyzer3,
//	)
func WorkerPool(name string, workers int, processors ...pipz.Chainable[*Thought]) *pipz.WorkerPool[*Thought] {
	return pipz.NewWorkerPool(pipz.Name(name), workers, processors...)
}
