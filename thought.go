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
	ID        string            `db:"id" type:"uuid" constraints:"primarykey" default:"gen_random_uuid()"`
	ThoughtID string            `db:"thought_id" type:"uuid" constraints:"notnull" references:"thoughts(id)"`
	Key       string            `db:"key" type:"text" constraints:"notnull"`
	Content   string            `db:"content" type:"text" constraints:"notnull"`
	Metadata  map[string]string `db:"metadata" type:"jsonb" default:"'{}'"`
	Source    string            `db:"source" type:"text" constraints:"notnull"`
	Created   time.Time         `db:"created" type:"timestamp" constraints:"notnull"`
	Embedding Vector            `db:"embedding" type:"vector(1536)"`
}

// Thought represents the rolling context of a chain of thought.
// It maintains an append-only log of Notes, providing semantic reasoning history.
//
// # Concurrency
//
// Thought is safe for concurrent reads but not concurrent writes. Multiple
// goroutines may call read methods (GetNote, GetContent, AllNotes, etc.).
// simultaneously, but write methods (AddNote, SetContent, MarkNotesPublished).
// must not be called concurrently with each other or with reads.
//
// For parallel processing, use Clone to create independent copies for each
// goroutine. The cloned thoughts can then be merged or selected as needed.
//
// # Failure Behavior
//
// Primitive steps (Decide, Classify, Analyze, etc.) may modify the thought
// before encountering an error. If a step returns an error, the thought may.
// be in a partially-modified state. Callers requiring atomicity should Clone.
// the thought before processing and discard it on failure.
type Thought struct {
	// Identity
	ID      string `db:"id" type:"uuid" constraints:"primarykey" default:"gen_random_uuid()"`
	Intent  string `db:"intent" type:"text" constraints:"notnull"`
	TraceID string `db:"trace_id" type:"text" constraints:"notnull,unique"`

	// Lineage
	ParentID *string `db:"parent_id" type:"uuid" references:"thoughts(id)"`
	TaskID   *string `db:"task_id" type:"uuid"`

	// LLM conversation state
	Session *zyn.Session // Shared session for LLM continuity (not persisted)

	// Persistence
	memory   Memory   // Reference to memory for note persistence
	embedder Embedder // Reference to embedder for note embeddings (optional)

	// Append-only note history
	notes          []Note
	publishedCount int      // Number of notes that have been sent to LLM
	index          sync.Map // map[string]int for quick lookup by key (most recent)
	mu             sync.RWMutex

	// Timestamps
	CreatedAt time.Time `db:"created_at" type:"timestamp" constraints:"notnull"`
	UpdatedAt time.Time `db:"updated_at" type:"timestamp" constraints:"notnull"`
}

// New creates a new Thought with the given intent and persists it.
// TraceID is auto-generated using UUID. ID is assigned by the database.
func New(ctx context.Context, memory Memory, intent string) (*Thought, error) {
	t := &Thought{
		Intent:         intent,
		TraceID:        uuid.New().String(),
		Session:        zyn.NewSession(),
		memory:         memory,
		notes:          make([]Note, 0),
		publishedCount: 0,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	persisted, err := memory.CreateThought(ctx, t)
	if err != nil {
		return nil, fmt.Errorf("failed to persist thought: %w", err)
	}

	// Copy the database-assigned ID back
	t.ID = persisted.ID

	// Emit thought creation event
	capitan.Emit(ctx, ThoughtCreated,
		FieldIntent.Field(t.Intent),
		FieldTraceID.Field(t.TraceID),
	)

	return t, nil
}

// NewWithTrace creates a new Thought with an explicit trace ID and persists it.
func NewWithTrace(ctx context.Context, memory Memory, intent, traceID string) (*Thought, error) {
	t := &Thought{
		Intent:         intent,
		TraceID:        traceID,
		Session:        zyn.NewSession(),
		memory:         memory,
		notes:          make([]Note, 0),
		publishedCount: 0,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	persisted, err := memory.CreateThought(ctx, t)
	if err != nil {
		return nil, fmt.Errorf("failed to persist thought: %w", err)
	}

	// Copy the database-assigned ID back
	t.ID = persisted.ID

	// Emit thought creation event
	capitan.Emit(ctx, ThoughtCreated,
		FieldIntent.Field(t.Intent),
		FieldTraceID.Field(t.TraceID),
	)

	return t, nil
}

// NewForTask creates a new Thought associated with a task and persists it.
func NewForTask(ctx context.Context, memory Memory, intent, taskID string) (*Thought, error) {
	t := &Thought{
		Intent:         intent,
		TraceID:        uuid.New().String(),
		TaskID:         &taskID,
		Session:        zyn.NewSession(),
		memory:         memory,
		notes:          make([]Note, 0),
		publishedCount: 0,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	persisted, err := memory.CreateThought(ctx, t)
	if err != nil {
		return nil, fmt.Errorf("failed to persist thought: %w", err)
	}

	t.ID = persisted.ID

	capitan.Emit(ctx, ThoughtCreated,
		FieldIntent.Field(t.Intent),
		FieldTraceID.Field(t.TraceID),
	)

	return t, nil
}

// AddNote adds a new note to the thought and persists it.
// If a note with the same key exists, the new note becomes the current value.
// If an embedder is configured, the note content will be embedded for semantic search.
func (t *Thought) AddNote(ctx context.Context, note Note) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if note.Created.IsZero() {
		note.Created = time.Now()
	}
	note.ThoughtID = t.ID

	// Generate embedding if embedder is available
	embedder, err := ResolveEmbedder(ctx, t.embedder)
	if err == nil && embedder != nil {
		embedding, embedErr := embedder.Embed(ctx, note.Content)
		if embedErr != nil {
			// Log but don't fail - embedding is optional
			capitan.Emit(ctx, NoteAdded,
				FieldTraceID.Field(t.TraceID),
				FieldNoteKey.Field(note.Key),
				FieldError.Field(fmt.Errorf("embedding failed: %w", embedErr)),
			)
		} else {
			note.Embedding = embedding
		}
	}

	// Persist the note
	persisted, err := t.memory.AddNote(ctx, &note)
	if err != nil {
		return fmt.Errorf("failed to persist note: %w", err)
	}
	note.ID = persisted.ID

	t.notes = append(t.notes, note)
	t.index.Store(note.Key, len(t.notes)-1)
	t.UpdatedAt = time.Now()

	// Emit note added event
	capitan.Emit(ctx, NoteAdded,
		FieldTraceID.Field(t.TraceID),
		FieldNoteKey.Field(note.Key),
		FieldNoteSource.Field(note.Source),
		FieldNoteCount.Field(len(t.notes)),
		FieldContentSize.Field(len(note.Content)),
	)

	return nil
}

// SetContent adds a simple note with just key and content.
func (t *Thought) SetContent(ctx context.Context, key, content, source string) error {
	return t.AddNote(ctx, Note{
		Key:      key,
		Content:  content,
		Source:   source,
		Metadata: make(map[string]string),
		Created:  time.Now(),
	})
}

// SetNote adds a note with metadata.
func (t *Thought) SetNote(ctx context.Context, key, content, source string, metadata map[string]string) error {
	if metadata == nil {
		metadata = make(map[string]string)
	}
	return t.AddNote(ctx, Note{
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

// Clone creates a deep copy of the thought for concurrent processing.
// Required for pipz.Concurrent and other parallel operations.
//
// The clone has an independent Session and notes. Modifications
// to the clone do not affect the original and vice versa.
//
// Note: Clone should only be called when the original thought is not being
// concurrently modified. The Session.Messages() call is internally synchronized,.
// but concurrent writes to the original thought during clone could result in.
// an inconsistent snapshot.
func (t *Thought) Clone() *Thought {
	t.mu.RLock()
	defer t.mu.RUnlock()

	clone := &Thought{
		ID:             t.ID,
		Intent:         t.Intent,
		TraceID:        t.TraceID,
		ParentID:       t.ParentID,
		TaskID:         t.TaskID,
		Session:        zyn.NewSession(),
		memory:         t.memory,
		embedder:       t.embedder,
		notes:          make([]Note, len(t.notes)),
		publishedCount: t.publishedCount,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      time.Now(),
	}

	// Copy session messages (Session.Messages() returns a copy and is internally synchronized)
	clone.Session.SetMessages(t.Session.Messages())

	// Deep copy notes (Note is value type, but Metadata is map and Embedding is slice)
	for i, note := range t.notes {
		clonedMeta := make(map[string]string, len(note.Metadata))
		for k, v := range note.Metadata {
			clonedMeta[k] = v
		}
		var clonedEmbedding Vector
		if note.Embedding != nil {
			clonedEmbedding = make(Vector, len(note.Embedding))
			copy(clonedEmbedding, note.Embedding)
		}
		clone.notes[i] = Note{
			ID:        note.ID,
			ThoughtID: note.ThoughtID,
			Key:       note.Key,
			Content:   note.Content,
			Metadata:  clonedMeta,
			Source:    note.Source,
			Created:   note.Created,
			Embedding: clonedEmbedding,
		}
	}

	// Rebuild index
	for i, note := range clone.notes {
		clone.index.Store(note.Key, i)
	}

	return clone
}

// PublishedCount returns the number of notes that have been published to the LLM.
func (t *Thought) PublishedCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.publishedCount
}

// SetPublishedCount sets the published count directly.
// This is primarily used for restoring thought state from persistence.
func (t *Thought) SetPublishedCount(count int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.publishedCount = count
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

// SetMemory sets the memory reference for persistence operations.
// This is used when hydrating a Thought from the database.
func (t *Thought) SetMemory(m Memory) {
	t.memory = m
}

// Memory returns the memory reference for this Thought.
func (t *Thought) Memory() Memory {
	return t.memory
}

// SetEmbedder sets the embedder for generating note embeddings.
// When set, notes will be embedded on creation for semantic search.
func (t *Thought) SetEmbedder(e Embedder) {
	t.embedder = e
}

// Embedder returns the embedder reference for this Thought.
func (t *Thought) Embedder() Embedder {
	return t.embedder
}

// AddNoteWithoutPersist adds a note to the in-memory state without persisting.
// This is used when hydrating a Thought from the database.
func (t *Thought) AddNoteWithoutPersist(note Note) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.notes = append(t.notes, note)
	t.index.Store(note.Key, len(t.notes)-1)
	t.UpdatedAt = time.Now()
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

// Compile-time check: *Thought must implement pipz.Cloner[*Thought].
var _ interface{ Clone() *Thought } = (*Thought)(nil)

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
