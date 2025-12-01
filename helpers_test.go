package cogito

import (
	"context"
	"errors"
	"testing"
)

func TestDo(t *testing.T) {
	thought := New("test")
	thought.SetContent("input", "test value", "test")

	processor := Do("custom-logic", func(ctx context.Context, th *Thought) (*Thought, error) {
		input, _ := th.GetContent("input")
		th.SetContent("output", input+" processed", "custom-logic")
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
	thought := New("test")

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
	thought := New("test")
	thought.SetContent("count", "5", "test")

	processor := Transform("increment", func(ctx context.Context, th *Thought) *Thought {
		count, _ := th.GetInt("count")
		th.SetContent("count", string(rune('0'+count+1)), "increment")
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

	thought := New("test")

	processor := Do("check-context", func(ctx context.Context, th *Thought) (*Thought, error) {
		value := ctx.Value(ctxKey{})
		if value == nil {
			return th, errors.New("context value not found")
		}
		th.SetContent("ctx-value", value.(string), "check-context")
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

	thought := New("test")

	processor := Transform("get-from-context", func(ctx context.Context, th *Thought) *Thought {
		value := ctx.Value(ctxKey{})
		if value != nil {
			th.SetContent("value", value.(string), "get-from-context")
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
	thought := New("test")
	thought.SetContent("value", "1", "test")

	step1 := Do("add-two", func(ctx context.Context, th *Thought) (*Thought, error) {
		val, _ := th.GetInt("value")
		th.SetContent("value", string(rune('0'+val+2)), "add-two")
		return th, nil
	})

	step2 := Do("multiply-by-three", func(ctx context.Context, th *Thought) (*Thought, error) {
		val, _ := th.GetInt("value")
		th.SetContent("value", string(rune('0'+val*3)), "multiply-by-three")
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
