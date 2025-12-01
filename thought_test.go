package cogito

import (
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	thought := New("test intent")

	if thought.Intent != "test intent" {
		t.Errorf("expected intent %q, got %q", "test intent", thought.Intent)
	}

	if thought.TraceID == "" {
		t.Error("expected TraceID to be generated")
	}

	if thought.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	if thought.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}
}

func TestNewWithTrace(t *testing.T) {
	thought := NewWithTrace("test intent", "trace-123")

	if thought.TraceID != "trace-123" {
		t.Errorf("expected trace ID %q, got %q", "trace-123", thought.TraceID)
	}
}

func TestAddNote(t *testing.T) {
	thought := New("test")

	note := Note{
		Key:      "test-key",
		Content:  "test content",
		Source:   "test-source",
		Metadata: map[string]string{"foo": "bar"},
	}

	thought.AddNote(note)

	retrieved, ok := thought.GetNote("test-key")
	if !ok {
		t.Fatal("expected note to be found")
	}

	if retrieved.Content != "test content" {
		t.Errorf("expected content %q, got %q", "test content", retrieved.Content)
	}

	if retrieved.Metadata["foo"] != "bar" {
		t.Errorf("expected metadata foo=%q, got %q", "bar", retrieved.Metadata["foo"])
	}

	if retrieved.Source != "test-source" {
		t.Errorf("expected source %q, got %q", "test-source", retrieved.Source)
	}

	if retrieved.Created.IsZero() {
		t.Error("expected Created to be set")
	}
}

func TestSetContent(t *testing.T) {
	thought := New("test")

	thought.SetContent("greeting", "hello", "test")

	content, err := thought.GetContent("greeting")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if content != "hello" {
		t.Errorf("expected %q, got %q", "hello", content)
	}
}

func TestSetNote(t *testing.T) {
	thought := New("test")

	metadata := map[string]string{
		"confidence": "0.95",
		"reasoning":  "test reasoning",
	}

	thought.SetNote("decision", "yes", "decide-step", metadata)

	note, ok := thought.GetNote("decision")
	if !ok {
		t.Fatal("expected note to be found")
	}

	if note.Content != "yes" {
		t.Errorf("expected content %q, got %q", "yes", note.Content)
	}

	if note.Metadata["confidence"] != "0.95" {
		t.Errorf("expected confidence %q, got %q", "0.95", note.Metadata["confidence"])
	}
}

func TestGetNonexistentNote(t *testing.T) {
	thought := New("test")

	_, ok := thought.GetNote("nonexistent")
	if ok {
		t.Error("expected note not to be found")
	}

	_, err := thought.GetContent("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent note")
	}
}

func TestNoteOverwrite(t *testing.T) {
	thought := New("test")

	thought.SetContent("status", "pending", "step1")
	thought.SetContent("status", "completed", "step2")

	content, err := thought.GetContent("status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if content != "completed" {
		t.Errorf("expected latest value %q, got %q", "completed", content)
	}

	// All notes should still be in history
	allNotes := thought.AllNotes()
	if len(allNotes) != 2 {
		t.Errorf("expected 2 notes in history, got %d", len(allNotes))
	}
}

func TestGetMetadata(t *testing.T) {
	thought := New("test")

	thought.SetNote("result", "success", "test",
		map[string]string{
			"code":    "200",
			"message": "OK",
		})

	code, err := thought.GetMetadata("result", "code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if code != "200" {
		t.Errorf("expected %q, got %q", "200", code)
	}

	message, err := thought.GetMetadata("result", "message")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if message != "OK" {
		t.Errorf("expected %q, got %q", "OK", message)
	}
}

func TestGetMetadataErrors(t *testing.T) {
	thought := New("test")

	// Note doesn't exist
	_, err := thought.GetMetadata("nonexistent", "field")
	if err == nil {
		t.Error("expected error for nonexistent note")
	}

	// Field doesn't exist
	thought.SetNote("result", "ok", "test", map[string]string{"foo": "bar"})
	_, err = thought.GetMetadata("result", "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent field")
	}
}

func TestGetLatestNote(t *testing.T) {
	thought := New("test")

	// No notes yet
	_, ok := thought.GetLatestNote()
	if ok {
		t.Error("expected no notes")
	}

	thought.SetContent("first", "1", "test")
	thought.SetContent("second", "2", "test")
	thought.SetContent("third", "3", "test")

	latest, ok := thought.GetLatestNote()
	if !ok {
		t.Fatal("expected latest note to be found")
	}

	if latest.Key != "third" || latest.Content != "3" {
		t.Errorf("expected latest note to be third=3, got %s=%s", latest.Key, latest.Content)
	}
}

func TestAllNotes(t *testing.T) {
	thought := New("test")

	thought.SetContent("a", "1", "test")
	thought.SetContent("b", "2", "test")
	thought.SetContent("c", "3", "test")

	notes := thought.AllNotes()

	if len(notes) != 3 {
		t.Fatalf("expected 3 notes, got %d", len(notes))
	}

	if notes[0].Key != "a" || notes[1].Key != "b" || notes[2].Key != "c" {
		t.Error("notes not in chronological order")
	}
}

func TestGetBool(t *testing.T) {
	thought := New("test")

	tests := []struct {
		value    string
		expected bool
		hasError bool
	}{
		{"true", true, false},
		{"false", false, false},
		{"yes", true, false},
		{"no", false, false},
		{"1", true, false},
		{"0", false, false},
		{"invalid", false, true},
	}

	for _, tt := range tests {
		thought.SetContent("bool", tt.value, "test")
		result, err := thought.GetBool("bool")

		if tt.hasError {
			if err == nil {
				t.Errorf("expected error for value %q", tt.value)
			}
		} else {
			if err != nil {
				t.Errorf("unexpected error for value %q: %v", tt.value, err)
			}
			if result != tt.expected {
				t.Errorf("for value %q, expected %v, got %v", tt.value, tt.expected, result)
			}
		}
	}
}

func TestGetFloat(t *testing.T) {
	thought := New("test")

	thought.SetContent("score", "0.95", "test")

	score, err := thought.GetFloat("score")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if score != 0.95 {
		t.Errorf("expected 0.95, got %f", score)
	}

	// Test invalid float
	thought.SetContent("invalid", "not-a-number", "test")
	_, err = thought.GetFloat("invalid")
	if err == nil {
		t.Error("expected error for invalid float")
	}
}

func TestGetInt(t *testing.T) {
	thought := New("test")

	thought.SetContent("count", "42", "test")

	count, err := thought.GetInt("count")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if count != 42 {
		t.Errorf("expected 42, got %d", count)
	}

	// Test invalid int
	thought.SetContent("invalid", "not-a-number", "test")
	_, err = thought.GetInt("invalid")
	if err == nil {
		t.Error("expected error for invalid int")
	}
}

func TestAddStep(t *testing.T) {
	thought := New("test")

	step := StepRecord{
		Name:     "test-step",
		Type:     "custom",
		Duration: 100 * time.Millisecond,
	}

	thought.AddStep(step)

	if len(thought.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(thought.Steps))
	}

	if thought.Steps[0].Name != "test-step" {
		t.Errorf("expected name %q, got %q", "test-step", thought.Steps[0].Name)
	}

	if thought.Steps[0].Type != "custom" {
		t.Errorf("expected type %q, got %q", "custom", thought.Steps[0].Type)
	}

	if thought.Steps[0].Timestamp.IsZero() {
		t.Error("expected Timestamp to be set")
	}
}

func TestClone(t *testing.T) {
	original := New("test")

	original.SetNote("result", "success", "test",
		map[string]string{
			"code":    "200",
			"message": "OK",
		})

	original.AddStep(StepRecord{
		Name:     "step1",
		Type:     "custom",
		Duration: 100 * time.Millisecond,
	})

	clone := original.Clone()

	// Verify basic fields
	if clone.Intent != original.Intent {
		t.Error("intent not cloned correctly")
	}

	if clone.TraceID != original.TraceID {
		t.Error("traceID not cloned correctly")
	}

	// Verify notes are cloned
	note, ok := clone.GetNote("result")
	if !ok {
		t.Fatal("note not found in clone")
	}

	if note.Content != "success" {
		t.Error("note content not cloned correctly")
	}

	if note.Metadata["code"] != "200" {
		t.Error("note metadata not cloned correctly")
	}

	// Verify deep copy - modifying clone shouldn't affect original
	clone.SetContent("result", "modified", "test")

	originalContent, _ := original.GetContent("result")
	cloneContent, _ := clone.GetContent("result")

	if originalContent == cloneContent {
		t.Error("notes not deep copied - modification affected original")
	}

	// Verify steps are cloned
	if len(clone.Steps) != 1 {
		t.Fatalf("expected 1 step in clone, got %d", len(clone.Steps))
	}

	if clone.Steps[0].Name != "step1" {
		t.Error("step not cloned correctly")
	}

	if clone.Steps[0].Type != "custom" {
		t.Error("step type not cloned correctly")
	}
}

func TestConcurrentAccess(t *testing.T) {
	thought := New("test")

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent writes
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(n int) {
			defer wg.Done()
			thought.SetContent("counter", string(rune('0'+n%10)), "test")
		}(i)
	}
	wg.Wait()

	// Concurrent reads
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = thought.GetContent("counter")
			_ = thought.AllNotes()
		}()
	}
	wg.Wait()

	// Should not panic and should have notes
	notes := thought.AllNotes()
	if len(notes) == 0 {
		t.Error("expected notes after concurrent writes")
	}
}

func TestConcurrentClone(t *testing.T) {
	original := New("test")

	// Add some data
	for i := 0; i < 10; i++ {
		original.SetContent("key", "value", "test")
	}

	var wg sync.WaitGroup
	numClones := 50

	// Concurrent clones
	wg.Add(numClones)
	for i := 0; i < numClones; i++ {
		go func() {
			defer wg.Done()
			clone := original.Clone()
			if len(clone.AllNotes()) == 0 {
				t.Error("clone has no notes")
			}
		}()
	}
	wg.Wait()
}
