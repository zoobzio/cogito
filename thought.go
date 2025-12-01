package cogito

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zoobzio/capitan"
	"github.com/zoobzio/zyn"
)

// Note represents a semantic piece of information in the reasoning chain.
// Everything in LLM space is fundamentally text-based, so Content is always a string.
// Metadata provides structured extension without breaking type safety.
type Note struct {
	Key      string            // Semantic identifier (e.g., "ticket_classification")
	Content  string            // Primary content (the "answer")
	Metadata map[string]string // Additional structured fields (all strings)
	Source   string            // Which step created this note
	Created  time.Time         // When this note was added
}

// StepRecord captures execution details of a single step.
// Input/output tracking is handled by Notes - each Note has a Source field
// indicating which step created it, providing built-in provenance.
type StepRecord struct {
	Name      string
	Type      string
	Duration  time.Duration
	Timestamp time.Time
	Error     error
}

// Thought represents the rolling context of a chain of thought.
// It maintains an append-only log of Notes, providing semantic reasoning history.
type Thought struct {
	// Metadata
	Intent  string // Business purpose/description
	TraceID string // Correlation ID for distributed tracing

	// LLM conversation state
	Session *zyn.Session // Shared session for LLM continuity

	// Append-only note history
	notes          []Note
	publishedCount int      // Number of notes that have been sent to LLM
	index          sync.Map // map[string]int for quick lookup by key (most recent)
	mu             sync.RWMutex

	// Execution history
	Steps []StepRecord

	// Timestamps
	CreatedAt time.Time
	UpdatedAt time.Time
}

// New creates a new Thought with the given intent.
// TraceID is auto-generated using UUID.
func New(intent string) *Thought {
	t := &Thought{
		Intent:         intent,
		TraceID:        uuid.New().String(),
		Session:        zyn.NewSession(),
		notes:          make([]Note, 0),
		publishedCount: 0,
		Steps:          make([]StepRecord, 0),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Emit thought creation event
	capitan.Emit(context.Background(), ThoughtCreated,
		FieldIntent.Field(t.Intent),
		FieldTraceID.Field(t.TraceID),
	)

	return t
}

// NewWithTrace creates a new Thought with an explicit trace ID.
func NewWithTrace(intent, traceID string) *Thought {
	t := &Thought{
		Intent:         intent,
		TraceID:        traceID,
		Session:        zyn.NewSession(),
		notes:          make([]Note, 0),
		publishedCount: 0,
		Steps:          make([]StepRecord, 0),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Emit thought creation event
	capitan.Emit(context.Background(), ThoughtCreated,
		FieldIntent.Field(t.Intent),
		FieldTraceID.Field(t.TraceID),
	)

	return t
}

// AddNote adds a new note to the thought.
// If a note with the same key exists, the new note becomes the current value.
func (t *Thought) AddNote(note Note) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if note.Created.IsZero() {
		note.Created = time.Now()
	}

	t.notes = append(t.notes, note)
	t.index.Store(note.Key, len(t.notes)-1)
	t.UpdatedAt = time.Now()

	// Emit note added event
	capitan.Emit(context.Background(), NoteAdded,
		FieldTraceID.Field(t.TraceID),
		FieldNoteKey.Field(note.Key),
		FieldNoteSource.Field(note.Source),
		FieldNoteCount.Field(len(t.notes)),
		FieldContentSize.Field(len(note.Content)),
	)
}

// SetContent adds a simple note with just key and content.
func (t *Thought) SetContent(key, content, source string) {
	t.AddNote(Note{
		Key:      key,
		Content:  content,
		Source:   source,
		Metadata: make(map[string]string),
		Created:  time.Now(),
	})
}

// SetNote adds a note with metadata.
func (t *Thought) SetNote(key, content, source string, metadata map[string]string) {
	if metadata == nil {
		metadata = make(map[string]string)
	}
	t.AddNote(Note{
		Key:      key,
		Content:  content,
		Source:   source,
		Metadata: metadata,
		Created:  time.Now(),
	})
}

// GetNote retrieves the most recent note with the given key.
func (t *Thought) GetNote(key string) (Note, bool) {
	idx, ok := t.index.Load(key)
	if !ok {
		return Note{}, false
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	i, ok := idx.(int)
	if !ok || i < 0 || i >= len(t.notes) {
		return Note{}, false
	}

	return t.notes[i], true
}

// GetContent retrieves the content of the most recent note with the given key.
func (t *Thought) GetContent(key string) (string, error) {
	note, ok := t.GetNote(key)
	if !ok {
		return "", fmt.Errorf("note not found: %s", key)
	}
	return note.Content, nil
}

// GetMetadata retrieves a specific metadata field from the most recent note.
func (t *Thought) GetMetadata(key, field string) (string, error) {
	note, ok := t.GetNote(key)
	if !ok {
		return "", fmt.Errorf("note not found: %s", key)
	}

	value, ok := note.Metadata[field]
	if !ok {
		return "", fmt.Errorf("metadata field not found: %s.%s", key, field)
	}

	return value, nil
}

// GetLatestNote returns the most recently added note.
func (t *Thought) GetLatestNote() (Note, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.notes) == 0 {
		return Note{}, false
	}

	return t.notes[len(t.notes)-1], true
}

// AllNotes returns all notes in chronological order.
func (t *Thought) AllNotes() []Note {
	t.mu.RLock()
	defer t.mu.RUnlock()

	notes := make([]Note, len(t.notes))
	copy(notes, t.notes)
	return notes
}

// GetBool parses the content as a boolean ("true"/"false").
func (t *Thought) GetBool(key string) (bool, error) {
	content, err := t.GetContent(key)
	if err != nil {
		return false, err
	}

	switch content {
	case "true", "yes", "1":
		return true, nil
	case "false", "no", "0":
		return false, nil
	default:
		return false, fmt.Errorf("cannot parse %q as boolean: %s", key, content)
	}
}

// GetFloat parses the content as a float64.
func (t *Thought) GetFloat(key string) (float64, error) {
	content, err := t.GetContent(key)
	if err != nil {
		return 0, err
	}

	f, err := strconv.ParseFloat(content, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot parse %q as float: %w", key, err)
	}

	return f, nil
}

// GetInt parses the content as an int.
func (t *Thought) GetInt(key string) (int, error) {
	content, err := t.GetContent(key)
	if err != nil {
		return 0, err
	}

	i, err := strconv.Atoi(content)
	if err != nil {
		return 0, fmt.Errorf("cannot parse %q as int: %w", key, err)
	}

	return i, nil
}

// AddStep records a step execution in the thought history.
func (t *Thought) AddStep(record StepRecord) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now()
	}

	t.Steps = append(t.Steps, record)
	t.UpdatedAt = time.Now()
}

// Clone creates a deep copy of the thought for concurrent processing.
// Required for pipz.Concurrent and other parallel operations.
func (t *Thought) Clone() *Thought {
	t.mu.RLock()
	defer t.mu.RUnlock()

	clone := &Thought{
		Intent:         t.Intent,
		TraceID:        t.TraceID,
		Session:        zyn.NewSession(),
		notes:          make([]Note, len(t.notes)),
		publishedCount: t.publishedCount,
		Steps:          make([]StepRecord, len(t.Steps)),
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      time.Now(),
	}

	// Copy session messages
	clone.Session.SetMessages(t.Session.Messages())

	// Deep copy notes (Note is value type, but Metadata is map)
	for i, note := range t.notes {
		clonedMeta := make(map[string]string, len(note.Metadata))
		for k, v := range note.Metadata {
			clonedMeta[k] = v
		}
		clone.notes[i] = Note{
			Key:      note.Key,
			Content:  note.Content,
			Metadata: clonedMeta,
			Source:   note.Source,
			Created:  note.Created,
		}
	}

	// Copy steps (StepRecord is now a simple value type)
	copy(clone.Steps, t.Steps)

	// Rebuild index
	for i, note := range clone.notes {
		clone.index.Store(note.Key, i)
	}

	return clone
}

// GetUnpublishedNotes returns all notes that have not yet been sent to the LLM.
// These are notes added after the last MarkNotesPublished call.
func (t *Thought) GetUnpublishedNotes() []Note {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.publishedCount >= len(t.notes) {
		return []Note{}
	}

	unpublished := make([]Note, len(t.notes)-t.publishedCount)
	copy(unpublished, t.notes[t.publishedCount:])
	return unpublished
}

// MarkNotesPublished marks all current notes as published to the LLM.
// This should be called after successfully sending notes to a synapse.
func (t *Thought) MarkNotesPublished() {
	t.mu.Lock()
	previousPublished := t.publishedCount
	t.publishedCount = len(t.notes)
	t.mu.Unlock()

	// Emit notes published event
	capitan.Emit(context.Background(), NotesPublished,
		FieldTraceID.Field(t.TraceID),
		FieldPublishedCount.Field(t.publishedCount),
		FieldUnpublishedCount.Field(t.publishedCount-previousPublished),
	)
}

// RenderNotesToContext converts a slice of notes to a formatted context string
// suitable for LLM consumption. Each note is rendered as "key: content".
func RenderNotesToContext(notes []Note) string {
	if len(notes) == 0 {
		return ""
	}

	var builder strings.Builder
	for i, note := range notes {
		if i > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(note.Key)
		builder.WriteString(": ")
		builder.WriteString(note.Content)
	}
	return builder.String()
}
