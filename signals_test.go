package cogito

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/zoobzio/capitan"
	capitantesting "github.com/zoobzio/capitan/testing"
	"github.com/zoobzio/zyn"
)

// getStringField extracts a string field value from a captured event.
func getStringField(event capitantesting.CapturedEvent, keyName string) string {
	for _, f := range event.Fields {
		if f.Key().Name() == keyName {
			if v, ok := f.Value().(string); ok {
				return v
			}
		}
	}
	return ""
}

// TestThoughtCreatedEvent verifies ThoughtCreated signal emission.
func TestThoughtCreatedEvent(t *testing.T) {
	capture := capitantesting.NewEventCapture()
	listener := capitan.Hook(ThoughtCreated, capture.Handler())
	defer listener.Close()

	thought := newTestThought("test intent")

	if !capture.WaitForCount(1, time.Second) {
		t.Fatal("expected ThoughtCreated event")
	}

	events := capture.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	intent := getStringField(events[0], FieldIntent.Name())
	if intent != "test intent" {
		t.Errorf("expected intent 'test intent', got %q", intent)
	}

	traceID := getStringField(events[0], FieldTraceID.Name())
	if traceID != thought.TraceID {
		t.Errorf("expected trace_id %q, got %q", thought.TraceID, traceID)
	}
}

// TestNoteAddedEvent verifies NoteAdded signal emission.
func TestNoteAddedEvent(t *testing.T) {
	type noteData struct {
		key         string
		source      string
		count       int
		contentSize int
	}

	var mu sync.Mutex
	var events []noteData

	listener := capitan.Hook(NoteAdded, func(_ context.Context, e *capitan.Event) {
		key, _ := FieldNoteKey.From(e)
		source, _ := FieldNoteSource.From(e)
		count, _ := FieldNoteCount.From(e)
		contentSize, _ := FieldContentSize.From(e)
		mu.Lock()
		events = append(events, noteData{key, source, count, contentSize})
		mu.Unlock()
	})
	defer listener.Close()

	thought := newTestThought("test")
	thought.SetContent(context.Background(), "key1", "value1", "source1")
	thought.SetContent(context.Background(), "key2", "value2", "source2")

	// Wait for events.
	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		count := len(events)
		mu.Unlock()
		if count >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(events) != 2 {
		t.Fatalf("expected 2 NoteAdded events, got %d", len(events))
	}

	if events[0].key != "key1" {
		t.Errorf("expected note_key 'key1', got %q", events[0].key)
	}
	if events[0].source != "source1" {
		t.Errorf("expected note_source 'source1', got %q", events[0].source)
	}
	if events[0].count != 1 {
		t.Errorf("expected note_count 1, got %d", events[0].count)
	}
	if events[0].contentSize != len("value1") {
		t.Errorf("expected content_size %d, got %d", len("value1"), events[0].contentSize)
	}

	if events[1].count != 2 {
		t.Errorf("expected note_count 2, got %d", events[1].count)
	}
}

// TestNotesPublishedEvent verifies NotesPublished signal emission.
func TestNotesPublishedEvent(t *testing.T) {
	type publishedData struct {
		publishedCount   int
		unpublishedCount int
	}

	var mu sync.Mutex
	var received *publishedData

	listener := capitan.Hook(NotesPublished, func(_ context.Context, e *capitan.Event) {
		pub, _ := FieldPublishedCount.From(e)
		unpub, _ := FieldUnpublishedCount.From(e)
		mu.Lock()
		received = &publishedData{pub, unpub}
		mu.Unlock()
	})
	defer listener.Close()

	thought := newTestThought("test")
	thought.SetContent(context.Background(), "key1", "value1", "source1")
	thought.SetContent(context.Background(), "key2", "value2", "source2")
	thought.MarkNotesPublished()

	// Wait for event.
	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		got := received != nil
		mu.Unlock()
		if got || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	if received == nil {
		t.Fatal("expected NotesPublished event")
	}

	if received.publishedCount != 2 {
		t.Errorf("expected published_count 2, got %d", received.publishedCount)
	}
	if received.unpublishedCount != 2 {
		t.Errorf("expected unpublished_count 2, got %d", received.unpublishedCount)
	}
}

// mockTestProvider implements Provider interface for signal tests.
type mockTestProvider struct {
	callCount int
}

func (m *mockTestProvider) Call(_ context.Context, messages []zyn.Message, _ float32) (*zyn.ProviderResponse, error) {
	m.callCount++

	if len(messages) == 0 {
		return nil, context.DeadlineExceeded
	}

	lastMessage := messages[len(messages)-1]
	if m.callCount > 1 || len(lastMessage.Content) > 200 {
		return &zyn.ProviderResponse{
			Content: `{"output": "Synthesized context for test decision", "confidence": 0.92, "changes": ["Synthesized decision context"], "reasoning": ["Combined decision with original context"]}`,
			Usage: zyn.TokenUsage{
				Prompt:     15,
				Completion: 25,
				Total:      40,
			},
		}, nil
	}

	return &zyn.ProviderResponse{
		Content: `{"decision": true, "confidence": 0.95, "reasoning": ["test reason"]}`,
		Usage: zyn.TokenUsage{
			Prompt:     10,
			Completion: 20,
			Total:      30,
		},
	}, nil
}

func (m *mockTestProvider) Name() string {
	return "mock-test-provider"
}

// TestStepEvents verifies step lifecycle events.
func TestStepEvents(t *testing.T) {
	type stepData struct {
		name     string
		stepType string
		traceID  string
		duration time.Duration
		count    int
	}

	var mu sync.Mutex
	var started, completed *stepData

	startListener := capitan.Hook(StepStarted, func(_ context.Context, e *capitan.Event) {
		name, _ := FieldStepName.From(e)
		stype, _ := FieldStepType.From(e)
		trace, _ := FieldTraceID.From(e)
		mu.Lock()
		started = &stepData{name: name, stepType: stype, traceID: trace}
		mu.Unlock()
	})
	defer startListener.Close()

	completedListener := capitan.Hook(StepCompleted, func(_ context.Context, e *capitan.Event) {
		dur, _ := FieldStepDuration.From(e)
		count, _ := FieldNoteCount.From(e)
		mu.Lock()
		completed = &stepData{duration: dur, count: count}
		mu.Unlock()
	})
	defer completedListener.Close()

	provider := &mockTestProvider{}
	thought := newTestThought("test decision")
	thought.SetContent(context.Background(), "input", "test input", "test")

	step := NewDecide("test_decision", "Is this a test?").WithProvider(provider)
	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("step execution failed: %v", err)
	}

	// Wait for events.
	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		gotBoth := started != nil && completed != nil
		mu.Unlock()
		if gotBoth || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	if started == nil {
		t.Fatal("expected StepStarted event")
	}
	if started.name != "test_decision" {
		t.Errorf("expected step_name 'test_decision', got %q", started.name)
	}
	if started.stepType != "decide" {
		t.Errorf("expected step_type 'decide', got %q", started.stepType)
	}
	if started.traceID != thought.TraceID {
		t.Errorf("trace_id mismatch: expected %q, got %q", thought.TraceID, started.traceID)
	}

	if completed == nil {
		t.Fatal("expected StepCompleted event")
	}
	if completed.duration <= 0 {
		t.Errorf("expected positive duration, got %v", completed.duration)
	}
	if completed.count != len(result.AllNotes()) {
		t.Errorf("expected note_count %d, got %d", len(result.AllNotes()), completed.count)
	}
}

// TestIntrospectionEvents verifies introspection lifecycle events.
func TestIntrospectionEvents(t *testing.T) {
	type introData struct {
		contextSize int
		stepType    string
	}

	var mu sync.Mutex
	var completed *introData

	listener := capitan.Hook(IntrospectionCompleted, func(_ context.Context, e *capitan.Event) {
		size, _ := FieldContextSize.From(e)
		stype, _ := FieldStepType.From(e)
		mu.Lock()
		completed = &introData{size, stype}
		mu.Unlock()
	})
	defer listener.Close()

	provider := &mockTestProvider{}
	thought := newTestThought("test introspection")
	thought.SetContent(context.Background(), "input", "test input", "test")

	step := NewDecide("test_decision", "Is this a test?").
		WithProvider(provider).
		WithIntrospection()
	_, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("step execution failed: %v", err)
	}

	// Wait for event.
	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		got := completed != nil
		mu.Unlock()
		if got || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	if completed == nil {
		t.Fatal("expected IntrospectionCompleted event")
	}
	if completed.contextSize <= 0 {
		t.Errorf("expected positive context_size, got %d", completed.contextSize)
	}
	if completed.stepType != "decide" {
		t.Errorf("expected step_type 'decide', got %q", completed.stepType)
	}
}

// mockFailingProvider implements Provider interface that always fails.
type mockFailingProvider struct{}

func (m *mockFailingProvider) Call(_ context.Context, _ []zyn.Message, _ float32) (*zyn.ProviderResponse, error) {
	return nil, context.DeadlineExceeded
}

func (m *mockFailingProvider) Name() string {
	return "mock-failing-provider"
}

// TestStepFailedEvent verifies error handling emits failure events.
func TestStepFailedEvent(t *testing.T) {
	type failData struct {
		err      error
		severity capitan.Severity
	}

	var mu sync.Mutex
	var failed *failData

	listener := capitan.Hook(StepFailed, func(_ context.Context, e *capitan.Event) {
		stepErr, _ := FieldError.From(e)
		mu.Lock()
		failed = &failData{err: stepErr, severity: e.Severity()}
		mu.Unlock()
	})
	defer listener.Close()

	provider := &mockFailingProvider{}
	thought := newTestThought("test failure")
	step := NewDecide("test_decision", "Will this fail?").WithProvider(provider)
	_, err := step.Process(context.Background(), thought)
	if err == nil {
		t.Fatal("expected step to fail, but it succeeded")
	}

	// Wait for event.
	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		got := failed != nil
		mu.Unlock()
		if got || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	if failed == nil {
		t.Fatal("expected StepFailed event")
	}
	if failed.err == nil {
		t.Error("expected error field to be present")
	}
	if failed.severity != capitan.SeverityError {
		t.Errorf("expected Error severity, got %v", failed.severity)
	}
}

// TestEventTraceIDCorrelation verifies all events for a thought share the same trace ID.
func TestEventTraceIDCorrelation(t *testing.T) {
	var mu sync.Mutex
	traceIDs := make(map[string]int)

	signals := []capitan.Signal{
		ThoughtCreated,
		NoteAdded,
		NotesPublished,
		StepStarted,
		StepCompleted,
	}

	listeners := make([]*capitan.Listener, 0, len(signals))
	for _, sig := range signals {
		listener := capitan.Hook(sig, func(_ context.Context, e *capitan.Event) {
			if traceID, ok := FieldTraceID.From(e); ok {
				mu.Lock()
				traceIDs[traceID]++
				mu.Unlock()
			}
		})
		listeners = append(listeners, listener)
	}
	defer func() {
		for _, l := range listeners {
			l.Close()
		}
	}()

	provider := &mockTestProvider{}
	thought := newTestThought("correlation test")
	thought.SetContent(context.Background(), "input", "test", "test")

	step := NewDecide("decision", "test?").WithProvider(provider)
	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("step failed: %v", err)
	}
	result.MarkNotesPublished()

	// Wait for events (expect at least 5: thought created, note added, step started, step completed, notes published).
	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		total := 0
		for _, count := range traceIDs {
			total += count
		}
		mu.Unlock()
		if total >= 5 || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(traceIDs) != 1 {
		t.Errorf("expected all events to share one trace ID, got %d unique trace IDs: %v",
			len(traceIDs), traceIDs)
	}

	for traceID := range traceIDs {
		if traceID != thought.TraceID {
			t.Errorf("event trace_id %q doesn't match thought trace_id %q",
				traceID, thought.TraceID)
		}
	}
}
