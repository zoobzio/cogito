package cogito

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/zyn"
)

// TestThoughtCreatedEvent verifies ThoughtCreated signal emission.
func TestThoughtCreatedEvent(t *testing.T) {
	var mu sync.Mutex
	var receivedEvents []*capitan.Event

	// Hook listener to capture events
	listener := capitan.Hook(ThoughtCreated, func(ctx context.Context, e *capitan.Event) {
		mu.Lock()
		receivedEvents = append(receivedEvents, e)
		mu.Unlock()
	})
	defer listener.Close()

	// Create thought
	thought := New("test intent")

	// Give async event processing time to complete
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(receivedEvents) != 1 {
		t.Fatalf("expected 1 ThoughtCreated event, got %d", len(receivedEvents))
	}

	event := receivedEvents[0]

	// Verify fields
	intent, ok := FieldIntent.From(event)
	if !ok || intent != "test intent" {
		t.Errorf("expected intent 'test intent', got %q", intent)
	}

	traceID, ok := FieldTraceID.From(event)
	if !ok || traceID != thought.TraceID {
		t.Errorf("expected trace_id %q, got %q", thought.TraceID, traceID)
	}
}

// TestNoteAddedEvent verifies NoteAdded signal emission.
func TestNoteAddedEvent(t *testing.T) {
	var mu sync.Mutex
	var receivedEvents []*capitan.Event

	listener := capitan.Hook(NoteAdded, func(ctx context.Context, e *capitan.Event) {
		mu.Lock()
		receivedEvents = append(receivedEvents, e)
		mu.Unlock()
	})
	defer listener.Close()

	thought := New("test")

	// Add notes
	thought.SetContent("key1", "value1", "source1")
	thought.SetContent("key2", "value2", "source2")

	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(receivedEvents) != 2 {
		t.Fatalf("expected 2 NoteAdded events, got %d", len(receivedEvents))
	}

	// Verify first event
	event1 := receivedEvents[0]
	key1, ok := FieldNoteKey.From(event1)
	if !ok || key1 != "key1" {
		t.Errorf("expected note_key 'key1', got %q", key1)
	}

	source1, ok := FieldNoteSource.From(event1)
	if !ok || source1 != "source1" {
		t.Errorf("expected note_source 'source1', got %q", source1)
	}

	count1, ok := FieldNoteCount.From(event1)
	if !ok || count1 != 1 {
		t.Errorf("expected note_count 1, got %d", count1)
	}

	contentSize1, ok := FieldContentSize.From(event1)
	if !ok || contentSize1 != len("value1") {
		t.Errorf("expected content_size %d, got %d", len("value1"), contentSize1)
	}

	// Verify second event
	event2 := receivedEvents[1]
	count2, ok := FieldNoteCount.From(event2)
	if !ok || count2 != 2 {
		t.Errorf("expected note_count 2, got %d", count2)
	}
}

// TestNotesPublishedEvent verifies NotesPublished signal emission.
func TestNotesPublishedEvent(t *testing.T) {
	var mu sync.Mutex
	var receivedEvents []*capitan.Event

	listener := capitan.Hook(NotesPublished, func(ctx context.Context, e *capitan.Event) {
		mu.Lock()
		receivedEvents = append(receivedEvents, e)
		mu.Unlock()
	})
	defer listener.Close()

	thought := New("test")
	thought.SetContent("key1", "value1", "source1")
	thought.SetContent("key2", "value2", "source2")

	// Mark published
	thought.MarkNotesPublished()

	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(receivedEvents) != 1 {
		t.Fatalf("expected 1 NotesPublished event, got %d", len(receivedEvents))
	}

	event := receivedEvents[0]

	publishedCount, ok := FieldPublishedCount.From(event)
	if !ok || publishedCount != 2 {
		t.Errorf("expected published_count 2, got %d", publishedCount)
	}

	unpublishedCount, ok := FieldUnpublishedCount.From(event)
	if !ok || unpublishedCount != 2 {
		t.Errorf("expected unpublished_count 2, got %d", unpublishedCount)
	}
}

// mockTestProvider implements Provider interface for signal tests
type mockTestProvider struct {
	callCount int
}

func (m *mockTestProvider) Call(ctx context.Context, messages []zyn.Message, temperature float32) (*zyn.ProviderResponse, error) {
	m.callCount++

	if len(messages) == 0 {
		return nil, context.DeadlineExceeded
	}

	// Check if this looks like a Transform call by inspecting message structure
	// Transform calls have specific formatting in their messages
	lastMessage := messages[len(messages)-1]
	if m.callCount > 1 || len(lastMessage.Content) > 200 {
		// This is likely the Transform synapse (second call or longer content)
		return &zyn.ProviderResponse{
			Content: `{"output": "Synthesized context for test decision", "confidence": 0.92, "changes": ["Synthesized decision context"], "reasoning": ["Combined decision with original context"]}`,
			Usage: zyn.TokenUsage{
				Prompt:     15,
				Completion: 25,
				Total:      40,
			},
		}, nil
	}

	// Binary synapse call (first call, shorter content)
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
	var mu sync.Mutex
	var startedEvents []*capitan.Event
	var completedEvents []*capitan.Event

	startListener := capitan.Hook(StepStarted, func(ctx context.Context, e *capitan.Event) {
		mu.Lock()
		startedEvents = append(startedEvents, e)
		mu.Unlock()
	})
	defer startListener.Close()

	completedListener := capitan.Hook(StepCompleted, func(ctx context.Context, e *capitan.Event) {
		mu.Lock()
		completedEvents = append(completedEvents, e)
		mu.Unlock()
	})
	defer completedListener.Close()

	// Mock provider
	provider := &mockTestProvider{}

	// Create and execute a decision step
	thought := New("test decision")
	thought.SetContent("input", "test input", "test")

	// Disable introspection to avoid Transform synapse complexity
	step := Decide("test_decision", "Is this a test?").
		WithProvider(provider).
		WithoutIntrospection()
	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("step execution failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Verify started event
	if len(startedEvents) != 1 {
		t.Fatalf("expected 1 StepStarted event, got %d", len(startedEvents))
	}

	startEvent := startedEvents[0]
	stepName, ok := FieldStepName.From(startEvent)
	if !ok || stepName != "test_decision" {
		t.Errorf("expected step_name 'test_decision', got %q", stepName)
	}

	stepType, ok := FieldStepType.From(startEvent)
	if !ok || stepType != "decide" {
		t.Errorf("expected step_type 'decide', got %q", stepType)
	}

	traceID, ok := FieldTraceID.From(startEvent)
	if !ok || traceID != thought.TraceID {
		t.Errorf("trace_id mismatch: expected %q, got %q", thought.TraceID, traceID)
	}

	// Verify completed event
	if len(completedEvents) != 1 {
		t.Fatalf("expected 1 StepCompleted event, got %d", len(completedEvents))
	}

	completedEvent := completedEvents[0]
	duration, ok := FieldStepDuration.From(completedEvent)
	if !ok || duration <= 0 {
		t.Errorf("expected positive duration, got %v", duration)
	}

	noteCount, ok := FieldNoteCount.From(completedEvent)
	if !ok || noteCount != len(result.AllNotes()) {
		t.Errorf("expected note_count %d, got %d", len(result.AllNotes()), noteCount)
	}
}

// TestIntrospectionEvents verifies introspection lifecycle events.
func TestIntrospectionEvents(t *testing.T) {
	var mu sync.Mutex
	var startedEvents []*capitan.Event
	var completedEvents []*capitan.Event

	startListener := capitan.Hook(IntrospectionStarted, func(ctx context.Context, e *capitan.Event) {
		mu.Lock()
		startedEvents = append(startedEvents, e)
		mu.Unlock()
	})
	defer startListener.Close()

	completedListener := capitan.Hook(IntrospectionCompleted, func(ctx context.Context, e *capitan.Event) {
		mu.Lock()
		completedEvents = append(completedEvents, e)
		mu.Unlock()
	})
	defer completedListener.Close()

	// Mock provider
	provider := &mockTestProvider{}

	// Create step with introspection enabled (default)
	thought := New("test introspection")
	thought.SetContent("input", "test input", "test")

	step := Decide("test_decision", "Is this a test?").WithProvider(provider)
	_, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("step execution failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Verify introspection events were emitted
	if len(startedEvents) != 1 {
		t.Fatalf("expected 1 IntrospectionStarted event, got %d", len(startedEvents))
	}

	if len(completedEvents) != 1 {
		t.Fatalf("expected 1 IntrospectionCompleted event, got %d", len(completedEvents))
	}

	completedEvent := completedEvents[0]
	contextSize, ok := FieldContextSize.From(completedEvent)
	if !ok || contextSize <= 0 {
		t.Errorf("expected positive context_size, got %d", contextSize)
	}

	stepType, ok := FieldStepType.From(completedEvent)
	if !ok || stepType != "decide" {
		t.Errorf("expected step_type 'decide', got %q", stepType)
	}
}

// mockFailingProvider implements Provider interface that always fails
type mockFailingProvider struct{}

func (m *mockFailingProvider) Call(ctx context.Context, messages []zyn.Message, temperature float32) (*zyn.ProviderResponse, error) {
	return nil, context.DeadlineExceeded
}

func (m *mockFailingProvider) Name() string {
	return "mock-failing-provider"
}

// TestStepFailedEvent verifies error handling emits failure events.
func TestStepFailedEvent(t *testing.T) {
	var mu sync.Mutex
	var failedEvents []*capitan.Event

	listener := capitan.Hook(StepFailed, func(ctx context.Context, e *capitan.Event) {
		mu.Lock()
		failedEvents = append(failedEvents, e)
		mu.Unlock()
	})
	defer listener.Close()

	// Mock provider that returns an error
	provider := &mockFailingProvider{}

	thought := New("test failure")
	step := Decide("test_decision", "Will this fail?").WithProvider(provider)
	_, err := step.Process(context.Background(), thought)
	if err == nil {
		t.Fatal("expected step to fail, but it succeeded")
	}

	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(failedEvents) != 1 {
		t.Fatalf("expected 1 StepFailed event, got %d", len(failedEvents))
	}

	event := failedEvents[0]
	stepError, ok := FieldError.From(event)
	if !ok || stepError == nil {
		t.Error("expected error field to be present")
	}

	// Verify it's an Error severity event
	if event.Severity() != capitan.SeverityError {
		t.Errorf("expected Error severity, got %v", event.Severity())
	}
}

// TestEventTraceIDCorrelation verifies all events for a thought share the same trace ID.
func TestEventTraceIDCorrelation(t *testing.T) {
	var mu sync.Mutex
	traceIDs := make(map[string]int)

	// Hook all cogito signals
	signals := []capitan.Signal{
		ThoughtCreated,
		NoteAdded,
		NotesPublished,
		StepStarted,
		StepCompleted,
		IntrospectionStarted,
		IntrospectionCompleted,
	}

	var listeners []*capitan.Listener
	for _, sig := range signals {
		listener := capitan.Hook(sig, func(ctx context.Context, e *capitan.Event) {
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

	// Execute a complete reasoning flow
	provider := &mockTestProvider{}

	thought := New("correlation test")
	thought.SetContent("input", "test", "test")

	// Disable introspection to avoid Transform synapse complexity
	step := Decide("decision", "test?").
		WithProvider(provider).
		WithoutIntrospection()
	result, err := step.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("step failed: %v", err)
	}

	result.MarkNotesPublished()

	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// All events should have the same trace ID
	if len(traceIDs) != 1 {
		t.Errorf("expected all events to share one trace ID, got %d unique trace IDs: %v",
			len(traceIDs), traceIDs)
	}

	// Verify it's the thought's trace ID
	for traceID := range traceIDs {
		if traceID != thought.TraceID {
			t.Errorf("event trace_id %q doesn't match thought trace_id %q",
				traceID, thought.TraceID)
		}
	}
}
