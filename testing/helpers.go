// Package cogitotest provides test utilities for cogito.
package cogitotest

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zoobzio/cogito"
)

// MockMemory implements cogito.Memory for testing without a database.
type MockMemory struct {
	thoughts map[string]*cogito.Thought
	notes    map[string][]cogito.Note
	mu       sync.RWMutex
}

// NewMockMemory creates a new in-memory mock for cogito.Memory.
func NewMockMemory() *MockMemory {
	return &MockMemory{
		thoughts: make(map[string]*cogito.Thought),
		notes:    make(map[string][]cogito.Note),
	}
}

// CreateThought persists a new thought and returns it with ID populated.
func (m *MockMemory) CreateThought(_ context.Context, thought *cogito.Thought) (*cogito.Thought, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	thought.ID = uuid.New().String()
	m.thoughts[thought.ID] = thought
	return thought, nil
}

// GetThought loads a thought by ID, including all its notes.
func (m *MockMemory) GetThought(_ context.Context, id string) (*cogito.Thought, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	thought, ok := m.thoughts[id]
	if !ok {
		return nil, fmt.Errorf("thought not found: %s", id)
	}
	return thought, nil
}

// GetThoughtByTraceID loads a thought by trace ID, including all its notes.
func (m *MockMemory) GetThoughtByTraceID(_ context.Context, traceID string) (*cogito.Thought, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, thought := range m.thoughts {
		if thought.TraceID == traceID {
			return thought, nil
		}
	}
	return nil, fmt.Errorf("thought not found for trace: %s", traceID)
}

// GetThoughtsByTaskID loads all thoughts for a task, ordered by creation time.
func (m *MockMemory) GetThoughtsByTaskID(_ context.Context, taskID string) ([]*cogito.Thought, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var thoughts []*cogito.Thought
	for _, thought := range m.thoughts {
		if thought.TaskID != nil && *thought.TaskID == taskID {
			thoughts = append(thoughts, thought)
		}
	}
	return thoughts, nil
}

// GetChildThoughts loads all thoughts that have the given thought as parent.
func (m *MockMemory) GetChildThoughts(_ context.Context, parentID string) ([]*cogito.Thought, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var thoughts []*cogito.Thought
	for _, thought := range m.thoughts {
		if thought.ParentID != nil && *thought.ParentID == parentID {
			thoughts = append(thoughts, thought)
		}
	}
	return thoughts, nil
}

// AddNote persists a note and returns it with ID populated.
func (m *MockMemory) AddNote(_ context.Context, note *cogito.Note) (*cogito.Note, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	note.ID = uuid.New().String()
	if note.Created.IsZero() {
		note.Created = time.Now()
	}
	m.notes[note.ThoughtID] = append(m.notes[note.ThoughtID], *note)
	return note, nil
}

// GetNotes loads all notes for a thought.
func (m *MockMemory) GetNotes(_ context.Context, thoughtID string) ([]cogito.Note, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	notes, ok := m.notes[thoughtID]
	if !ok {
		return []cogito.Note{}, nil
	}
	return notes, nil
}

// UpdateThought updates thought metadata (timestamps, publishedCount).
func (m *MockMemory) UpdateThought(_ context.Context, thought *cogito.Thought) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.thoughts[thought.ID]; !ok {
		return fmt.Errorf("thought not found: %s", thought.ID)
	}
	m.thoughts[thought.ID] = thought
	return nil
}

// DeleteThought removes a thought and all its notes.
func (m *MockMemory) DeleteThought(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.thoughts, id)
	delete(m.notes, id)
	return nil
}

// SearchNotes finds notes semantically similar to the query embedding.
func (m *MockMemory) SearchNotes(_ context.Context, _ cogito.Vector, _ int) ([]cogito.NoteWithThought, error) {
	return []cogito.NoteWithThought{}, nil
}

// SearchNotesByTask finds the most relevant note per task.
func (m *MockMemory) SearchNotesByTask(_ context.Context, _ cogito.Vector, _ int) ([]*cogito.Thought, error) {
	return []*cogito.Thought{}, nil
}

// Verify MockMemory implements cogito.Memory.
var _ cogito.Memory = (*MockMemory)(nil)

// NewTestThought creates a Thought with mock memory for testing.
// This is a convenience function that wraps cogito.New with MockMemory.
func NewTestThought(t *testing.T, intent string) *cogito.Thought {
	t.Helper()
	mem := NewMockMemory()
	ctx := context.Background()
	thought, err := cogito.New(ctx, mem, intent)
	if err != nil {
		t.Fatalf("failed to create test thought: %v", err)
	}
	return thought
}

// NewTestThoughtWithTrace creates a Thought with explicit trace ID for testing.
func NewTestThoughtWithTrace(t *testing.T, intent, traceID string) *cogito.Thought {
	t.Helper()
	mem := NewMockMemory()
	ctx := context.Background()
	thought, err := cogito.NewWithTrace(ctx, mem, intent, traceID)
	if err != nil {
		t.Fatalf("failed to create test thought with trace: %v", err)
	}
	return thought
}

// RequireContent asserts that the thought has the expected content at the given key.
func RequireContent(t *testing.T, thought *cogito.Thought, key, expected string) {
	t.Helper()
	content, err := thought.GetContent(key)
	if err != nil {
		t.Fatalf("expected content at key %q, got error: %v", key, err)
	}
	if content != expected {
		t.Fatalf("expected content %q at key %q, got %q", expected, key, content)
	}
}

// RequireNoContent asserts that the thought does not have content at the given key.
func RequireNoContent(t *testing.T, thought *cogito.Thought, key string) {
	t.Helper()
	_, err := thought.GetContent(key)
	if err == nil {
		t.Fatalf("expected no content at key %q, but found some", key)
	}
}
