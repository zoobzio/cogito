package cogito

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// mockMemory implements Memory for testing without a database.
type mockMemory struct {
	thoughts map[string]*Thought
	notes    map[string][]Note
	mu       sync.RWMutex
}

func newMockMemory() *mockMemory {
	return &mockMemory{
		thoughts: make(map[string]*Thought),
		notes:    make(map[string][]Note),
	}
}

func (m *mockMemory) CreateThought(_ context.Context, thought *Thought) (*Thought, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	thought.ID = uuid.New().String()
	m.thoughts[thought.ID] = thought
	return thought, nil
}

func (m *mockMemory) GetThought(_ context.Context, id string) (*Thought, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	thought, ok := m.thoughts[id]
	if !ok {
		return nil, fmt.Errorf("thought not found: %s", id)
	}
	return thought, nil
}

func (m *mockMemory) GetThoughtByTraceID(_ context.Context, traceID string) (*Thought, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, thought := range m.thoughts {
		if thought.TraceID == traceID {
			return thought, nil
		}
	}
	return nil, fmt.Errorf("thought not found for trace: %s", traceID)
}

func (m *mockMemory) GetThoughtsByTaskID(_ context.Context, taskID string) ([]*Thought, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var thoughts []*Thought
	for _, thought := range m.thoughts {
		if thought.TaskID != nil && *thought.TaskID == taskID {
			thoughts = append(thoughts, thought)
		}
	}
	return thoughts, nil
}

func (m *mockMemory) GetChildThoughts(_ context.Context, parentID string) ([]*Thought, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var thoughts []*Thought
	for _, thought := range m.thoughts {
		if thought.ParentID != nil && *thought.ParentID == parentID {
			thoughts = append(thoughts, thought)
		}
	}
	return thoughts, nil
}

func (m *mockMemory) AddNote(_ context.Context, note *Note) (*Note, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	note.ID = uuid.New().String()
	if note.Created.IsZero() {
		note.Created = time.Now()
	}
	m.notes[note.ThoughtID] = append(m.notes[note.ThoughtID], *note)
	return note, nil
}

func (m *mockMemory) GetNotes(_ context.Context, thoughtID string) ([]Note, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	notes, ok := m.notes[thoughtID]
	if !ok {
		return []Note{}, nil
	}
	return notes, nil
}

func (m *mockMemory) UpdateThought(_ context.Context, thought *Thought) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.thoughts[thought.ID]; !ok {
		return fmt.Errorf("thought not found: %s", thought.ID)
	}
	m.thoughts[thought.ID] = thought
	return nil
}

func (m *mockMemory) DeleteThought(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.thoughts, id)
	delete(m.notes, id)
	return nil
}

func (m *mockMemory) SearchNotes(_ context.Context, _ Vector, limit int) ([]NoteWithThought, error) {
	// Mock implementation returns empty results
	return []NoteWithThought{}, nil
}

func (m *mockMemory) SearchNotesByTask(_ context.Context, _ Vector, limit int) ([]*Thought, error) {
	// Mock implementation returns empty results
	return []*Thought{}, nil
}

// newTestThought creates a Thought with mock memory for testing.
func newTestThought(intent string) *Thought {
	mem := newMockMemory()
	ctx := context.Background()
	thought, _ := New(ctx, mem, intent)
	return thought
}

// newTestThoughtWithTrace creates a Thought with explicit trace ID for testing.
func newTestThoughtWithTrace(intent, traceID string) *Thought {
	mem := newMockMemory()
	ctx := context.Background()
	thought, _ := NewWithTrace(ctx, mem, intent, traceID)
	return thought
}

var _ Memory = (*mockMemory)(nil)
