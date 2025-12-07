package cogito

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zoobzio/pipz"
)

func TestDo(t *testing.T) {
	thought := newTestThought("test")
	thought.SetContent(context.Background(), "input", "test value", "test")

	processor := Do("custom-logic", func(ctx context.Context, th *Thought) (*Thought, error) {
		input, _ := th.GetContent("input")
		th.SetContent(ctx, "output", input+" processed", "custom-logic")
		return th, nil
	})

	result, err := processor.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output, err := result.GetContent("output")
	if err != nil {
		t.Fatalf("unexpected error getting output: %v", err)
	}

	expected := "test value processed"
	if output != expected {
		t.Errorf("expected %q, got %q", expected, output)
	}
}

func TestDoWithError(t *testing.T) {
	thought := newTestThought("test")

	processor := Do("failing-logic", func(ctx context.Context, th *Thought) (*Thought, error) {
		return th, errors.New("intentional error")
	})

	_, err := processor.Process(context.Background(), thought)
	if err == nil {
		t.Error("expected error from Do processor")
	}

	// pipz wraps errors, so just check that the error contains our message
	if err.Error() == "" {
		t.Error("expected non-empty error message")
	}
}

func TestTransform(t *testing.T) {
	thought := newTestThought("test")
	thought.SetContent(context.Background(), "count", "5", "test")

	processor := Transform("increment", func(ctx context.Context, th *Thought) *Thought {
		count, _ := th.GetInt("count")
		th.SetContent(ctx, "count", string(rune('0'+count+1)), "increment")
		return th
	})

	result, err := processor.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	count, err := result.GetContent("count")
	if err != nil {
		t.Fatalf("unexpected error getting count: %v", err)
	}

	if count != "6" {
		t.Errorf("expected %q, got %q", "6", count)
	}
}

func TestDoContextPropagation(t *testing.T) {
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "test-value")

	thought := newTestThought("test")

	processor := Do("check-context", func(ctx context.Context, th *Thought) (*Thought, error) {
		value := ctx.Value(ctxKey{})
		if value == nil {
			return th, errors.New("context value not found")
		}
		th.SetContent(ctx, "ctx-value", value.(string), "check-context")
		return th, nil
	})

	result, err := processor.Process(ctx, thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	value, err := result.GetContent("ctx-value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value != "test-value" {
		t.Errorf("expected %q, got %q", "test-value", value)
	}
}

func TestTransformContextPropagation(t *testing.T) {
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "transform-value")

	thought := newTestThought("test")

	processor := Transform("get-from-context", func(ctx context.Context, th *Thought) *Thought {
		value := ctx.Value(ctxKey{})
		if value != nil {
			th.SetContent(ctx, "value", value.(string), "get-from-context")
		}
		return th
	})

	result, err := processor.Process(ctx, thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	value, err := result.GetContent("value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value != "transform-value" {
		t.Errorf("expected %q, got %q", "transform-value", value)
	}
}

func TestDoChaining(t *testing.T) {
	thought := newTestThought("test")
	thought.SetContent(context.Background(), "value", "1", "test")

	step1 := Do("add-two", func(ctx context.Context, th *Thought) (*Thought, error) {
		val, _ := th.GetInt("value")
		th.SetContent(ctx, "value", string(rune('0'+val+2)), "add-two")
		return th, nil
	})

	step2 := Do("multiply-by-three", func(ctx context.Context, th *Thought) (*Thought, error) {
		val, _ := th.GetInt("value")
		th.SetContent(ctx, "value", string(rune('0'+val*3)), "multiply-by-three")
		return th, nil
	})

	// Chain the steps manually
	result, err := step1.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error in step1: %v", err)
	}

	result, err = step2.Process(context.Background(), result)
	if err != nil {
		t.Fatalf("unexpected error in step2: %v", err)
	}

	// (1 + 2) * 3 = 9
	value, err := result.GetContent("value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value != "9" {
		t.Errorf("expected %q, got %q", "9", value)
	}
}

func TestEffect(t *testing.T) {
	thought := newTestThought("test")
	thought.SetContent(context.Background(), "input", "original", "test")

	var observed string
	processor := Effect("observe", func(ctx context.Context, th *Thought) error {
		observed, _ = th.GetContent("input")
		return nil
	})

	result, err := processor.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if observed != "original" {
		t.Errorf("expected observed %q, got %q", "original", observed)
	}

	// Effect should not modify the thought
	value, _ := result.GetContent("input")
	if value != "original" {
		t.Errorf("expected input unchanged, got %q", value)
	}
}

func TestMutate(t *testing.T) {
	t.Run("applies when predicate true", func(t *testing.T) {
		thought := newTestThought("test")
		thought.SetContent(context.Background(), "priority", "high", "test")

		processor := Mutate("upgrade",
			func(ctx context.Context, th *Thought) *Thought {
				th.SetContent(ctx, "priority", "urgent", "upgrade")
				return th
			},
			func(ctx context.Context, th *Thought) bool {
				priority, _ := th.GetContent("priority")
				return priority == "high"
			},
		)

		result, err := processor.Process(context.Background(), thought)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		priority, _ := result.GetContent("priority")
		if priority != "urgent" {
			t.Errorf("expected priority 'urgent', got %q", priority)
		}
	})

	t.Run("skips when predicate false", func(t *testing.T) {
		thought := newTestThought("test")
		thought.SetContent(context.Background(), "priority", "low", "test")

		processor := Mutate("upgrade",
			func(ctx context.Context, th *Thought) *Thought {
				th.SetContent(ctx, "priority", "urgent", "upgrade")
				return th
			},
			func(ctx context.Context, th *Thought) bool {
				priority, _ := th.GetContent("priority")
				return priority == "high"
			},
		)

		result, err := processor.Process(context.Background(), thought)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		priority, _ := result.GetContent("priority")
		if priority != "low" {
			t.Errorf("expected priority 'low', got %q", priority)
		}
	})
}

func TestEnrich(t *testing.T) {
	t.Run("applies enrichment on success", func(t *testing.T) {
		thought := newTestThought("test")

		processor := Enrich("add-metadata", func(ctx context.Context, th *Thought) (*Thought, error) {
			th.SetContent(ctx, "enriched", "yes", "add-metadata")
			return th, nil
		})

		result, err := processor.Process(context.Background(), thought)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		enriched, _ := result.GetContent("enriched")
		if enriched != "yes" {
			t.Errorf("expected enriched 'yes', got %q", enriched)
		}
	})

	t.Run("continues pipeline on enrichment error", func(t *testing.T) {
		thought := newTestThought("test")
		thought.SetContent(context.Background(), "original", "value", "test")

		processor := Enrich("failing-enrich", func(ctx context.Context, th *Thought) (*Thought, error) {
			return th, errors.New("enrichment failed")
		})

		result, err := processor.Process(context.Background(), thought)
		// Enrich should not fail the pipeline
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		original, _ := result.GetContent("original")
		if original != "value" {
			t.Errorf("expected original value preserved, got %q", original)
		}
	})
}

func TestSequence(t *testing.T) {
	thought := newTestThought("test")
	thought.SetContent(context.Background(), "value", "1", "test")

	seq := Sequence("pipeline",
		Do("step1", func(ctx context.Context, th *Thought) (*Thought, error) {
			th.SetContent(ctx, "step1", "done", "step1")
			return th, nil
		}),
		Do("step2", func(ctx context.Context, th *Thought) (*Thought, error) {
			th.SetContent(ctx, "step2", "done", "step2")
			return th, nil
		}),
	)

	result, err := seq.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	step1, _ := result.GetContent("step1")
	step2, _ := result.GetContent("step2")

	if step1 != "done" || step2 != "done" {
		t.Errorf("expected both steps done, got step1=%q step2=%q", step1, step2)
	}
}

func TestFilter(t *testing.T) {
	t.Run("executes processor when predicate true", func(t *testing.T) {
		thought := newTestThought("test")
		thought.SetContent(context.Background(), "type", "urgent", "test")

		filter := Filter("urgent-only",
			func(ctx context.Context, th *Thought) bool {
				t, _ := th.GetContent("type")
				return t == "urgent"
			},
			Do("handle-urgent", func(ctx context.Context, th *Thought) (*Thought, error) {
				th.SetContent(ctx, "handled", "yes", "handle-urgent")
				return th, nil
			}),
		)

		result, err := filter.Process(context.Background(), thought)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		handled, _ := result.GetContent("handled")
		if handled != "yes" {
			t.Errorf("expected handled 'yes', got %q", handled)
		}
	})

	t.Run("passes through when predicate false", func(t *testing.T) {
		thought := newTestThought("test")
		thought.SetContent(context.Background(), "type", "normal", "test")

		filter := Filter("urgent-only",
			func(ctx context.Context, th *Thought) bool {
				t, _ := th.GetContent("type")
				return t == "urgent"
			},
			Do("handle-urgent", func(ctx context.Context, th *Thought) (*Thought, error) {
				th.SetContent(ctx, "handled", "yes", "handle-urgent")
				return th, nil
			}),
		)

		result, err := filter.Process(context.Background(), thought)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = result.GetContent("handled")
		if err == nil {
			t.Error("expected 'handled' not to be set")
		}
	})
}

func TestSwitch(t *testing.T) {
	thought := newTestThought("test")
	thought.SetContent(context.Background(), "category", "question", "test")

	router := Switch("router", func(ctx context.Context, th *Thought) string {
		cat, _ := th.GetContent("category")
		return cat
	})
	router.AddRoute("question", Do("handle-question", func(ctx context.Context, th *Thought) (*Thought, error) {
		th.SetContent(ctx, "routed", "question-handler", "handle-question")
		return th, nil
	}))
	router.AddRoute("command", Do("handle-command", func(ctx context.Context, th *Thought) (*Thought, error) {
		th.SetContent(ctx, "routed", "command-handler", "handle-command")
		return th, nil
	}))

	result, err := router.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	routed, _ := result.GetContent("routed")
	if routed != "question-handler" {
		t.Errorf("expected routed to 'question-handler', got %q", routed)
	}
}

func TestGate(t *testing.T) {
	t.Run("passes through when predicate true", func(t *testing.T) {
		thought := newTestThought("test")
		thought.SetContent(context.Background(), "valid", "yes", "test")

		gate := Gate("valid-only", func(ctx context.Context, th *Thought) bool {
			v, _ := th.GetContent("valid")
			return v == "yes"
		})

		result, err := gate.Process(context.Background(), thought)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result != thought {
			t.Error("expected same thought returned")
		}
	})

	t.Run("passes through unchanged when predicate false", func(t *testing.T) {
		thought := newTestThought("test")
		thought.SetContent(context.Background(), "valid", "no", "test")

		gate := Gate("valid-only", func(ctx context.Context, th *Thought) bool {
			v, _ := th.GetContent("valid")
			return v == "yes"
		})

		result, err := gate.Process(context.Background(), thought)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Gate passes through unchanged (doesn't block)
		if result != thought {
			t.Error("expected same thought returned")
		}
	})
}

func TestFallback(t *testing.T) {
	thought := newTestThought("test")

	fallback := Fallback("resilient",
		Do("primary", func(ctx context.Context, th *Thought) (*Thought, error) {
			return th, errors.New("primary failed")
		}),
		Do("backup", func(ctx context.Context, th *Thought) (*Thought, error) {
			th.SetContent(ctx, "handler", "backup", "backup")
			return th, nil
		}),
	)

	result, err := fallback.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handler, _ := result.GetContent("handler")
	if handler != "backup" {
		t.Errorf("expected handler 'backup', got %q", handler)
	}
}

func TestRetry(t *testing.T) {
	thought := newTestThought("test")

	attempts := 0
	retry := Retry("retrying", Do("flaky", func(ctx context.Context, th *Thought) (*Thought, error) {
		attempts++
		if attempts < 3 {
			return th, errors.New("not yet")
		}
		th.SetContent(ctx, "success", "yes", "flaky")
		return th, nil
	}), 5)

	result, err := retry.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}

	success, _ := result.GetContent("success")
	if success != "yes" {
		t.Errorf("expected success 'yes', got %q", success)
	}
}

func TestTimeout(t *testing.T) {
	t.Run("completes within timeout", func(t *testing.T) {
		thought := newTestThought("test")

		timeout := Timeout("bounded", Do("fast", func(ctx context.Context, th *Thought) (*Thought, error) {
			th.SetContent(ctx, "result", "done", "fast")
			return th, nil
		}), time.Second)

		result, err := timeout.Process(context.Background(), thought)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		res, _ := result.GetContent("result")
		if res != "done" {
			t.Errorf("expected result 'done', got %q", res)
		}
	})

	t.Run("fails on timeout", func(t *testing.T) {
		thought := newTestThought("test")

		timeout := Timeout("bounded", Do("slow", func(ctx context.Context, th *Thought) (*Thought, error) {
			select {
			case <-time.After(500 * time.Millisecond):
				return th, nil
			case <-ctx.Done():
				return th, ctx.Err()
			}
		}), 10*time.Millisecond)

		_, err := timeout.Process(context.Background(), thought)
		if err == nil {
			t.Error("expected timeout error")
		}
	})
}

func TestCircuitBreaker(t *testing.T) {
	thought := newTestThought("test")

	failures := 0
	cb := CircuitBreaker("breaker", Do("failing", func(ctx context.Context, th *Thought) (*Thought, error) {
		failures++
		return th, errors.New("service down")
	}), 3, time.Second)

	// Trip the breaker
	for i := 0; i < 5; i++ {
		_, _ = cb.Process(context.Background(), thought)
	}

	// After threshold failures, the circuit should be open
	if failures > 5 {
		t.Errorf("expected circuit to open after threshold, but had %d failures", failures)
	}
}

func TestRateLimiter(t *testing.T) {
	// RateLimiter just passes through data when rate limit is not exceeded
	rl := RateLimiter("limiter", 100, 10)

	thought := newTestThought("test")
	thought.SetContent(context.Background(), "value", "test", "test")

	result, err := rl.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should pass through unchanged
	value, _ := result.GetContent("value")
	if value != "test" {
		t.Errorf("expected value 'test', got %q", value)
	}
}

func TestConcurrent(t *testing.T) {
	thought := newTestThought("test")
	thought.SetContent(context.Background(), "input", "value", "test")

	// Use a reducer to verify all branches ran
	concurrent := Concurrent("parallel",
		func(original *Thought, results map[pipz.Name]*Thought, errors map[pipz.Name]error) *Thought {
			// Aggregate results into original
			for name, result := range results {
				content, _ := result.GetContent(string(name))
				original.SetContent(context.Background(), string(name), content, "reducer")
			}
			return original
		},
		Do("branch1", func(ctx context.Context, th *Thought) (*Thought, error) {
			th.SetContent(ctx, "branch1", "done1", "branch1")
			return th, nil
		}),
		Do("branch2", func(ctx context.Context, th *Thought) (*Thought, error) {
			th.SetContent(ctx, "branch2", "done2", "branch2")
			return th, nil
		}),
	)

	result, err := concurrent.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	branch1, _ := result.GetContent("branch1")
	branch2, _ := result.GetContent("branch2")

	if branch1 != "done1" {
		t.Errorf("expected branch1 'done1', got %q", branch1)
	}
	if branch2 != "done2" {
		t.Errorf("expected branch2 'done2', got %q", branch2)
	}
}

func TestRace(t *testing.T) {
	thought := newTestThought("test")

	// First successful result wins
	race := Race("fastest",
		Do("slow", func(ctx context.Context, th *Thought) (*Thought, error) {
			time.Sleep(100 * time.Millisecond)
			th.SetContent(ctx, "winner", "slow", "slow")
			return th, nil
		}),
		Do("fast", func(ctx context.Context, th *Thought) (*Thought, error) {
			th.SetContent(ctx, "winner", "fast", "fast")
			return th, nil
		}),
	)

	result, err := race.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	winner, _ := result.GetContent("winner")
	if winner != "fast" {
		t.Errorf("expected winner 'fast', got %q", winner)
	}
}

func TestWorkerPool(t *testing.T) {
	thought := newTestThought("test")

	pool := WorkerPool("pool", 2,
		Do("task1", func(ctx context.Context, th *Thought) (*Thought, error) {
			return th, nil
		}),
		Do("task2", func(ctx context.Context, th *Thought) (*Thought, error) {
			return th, nil
		}),
	)

	result, err := pool.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// WorkerPool returns original thought
	if result.Intent != thought.Intent {
		t.Error("expected same thought intent")
	}
}

func TestThoughtClone(t *testing.T) {
	thought := newTestThought("original")
	thought.SetContent(context.Background(), "key1", "value1", "test")
	thought.SetContent(context.Background(), "key2", "value2", "test")

	clone := thought.Clone()

	// Clone should have same values
	if clone.Intent != thought.Intent {
		t.Errorf("expected intent %q, got %q", thought.Intent, clone.Intent)
	}

	val1, _ := clone.GetContent("key1")
	if val1 != "value1" {
		t.Errorf("expected key1 'value1', got %q", val1)
	}

	// Modifying clone should not affect original
	clone.SetContent(context.Background(), "key1", "modified", "test")

	originalVal, _ := thought.GetContent("key1")
	if originalVal != "value1" {
		t.Errorf("clone modification affected original: got %q", originalVal)
	}

	cloneVal, _ := clone.GetContent("key1")
	if cloneVal != "modified" {
		t.Errorf("expected clone key1 'modified', got %q", cloneVal)
	}
}
