package cogito

import "context"

// Memory defines the interface for thought persistence.
// Implementations handle the storage and retrieval of Thoughts and Notes.
type Memory interface {
	// CreateThought persists a new thought and returns it with ID populated.
	CreateThought(ctx context.Context, thought *Thought) (*Thought, error)

	// GetThought loads a thought by ID, including all its notes.
	GetThought(ctx context.Context, id string) (*Thought, error)

	// GetThoughtByTraceID loads a thought by trace ID, including all its notes.
	GetThoughtByTraceID(ctx context.Context, traceID string) (*Thought, error)

	// GetThoughtsByTaskID loads all thoughts for a task, ordered by creation time.
	GetThoughtsByTaskID(ctx context.Context, taskID string) ([]*Thought, error)

	// GetChildThoughts loads all thoughts that have the given thought as parent.
	GetChildThoughts(ctx context.Context, parentID string) ([]*Thought, error)

	// AddNote persists a note and returns it with ID populated.
	AddNote(ctx context.Context, note *Note) (*Note, error)

	// GetNotes loads all notes for a thought.
	GetNotes(ctx context.Context, thoughtID string) ([]Note, error)

	// UpdateThought updates thought metadata (timestamps, publishedCount).
	UpdateThought(ctx context.Context, thought *Thought) error

	// DeleteThought removes a thought and all its notes.
	DeleteThought(ctx context.Context, id string) error

	// SearchNotes finds notes semantically similar to the query embedding.
	// Returns notes ordered by similarity, limited to the specified count.
	SearchNotes(ctx context.Context, embedding Vector, limit int) ([]NoteWithThought, error)

	// SearchNotesByTask finds the most relevant note per task.
	// Returns the most recent thought for each task that has matching notes.
	SearchNotesByTask(ctx context.Context, embedding Vector, limit int) ([]*Thought, error)
}

// NoteWithThought pairs a note with its parent thought for search results.
type NoteWithThought struct {
	Note    Note
	Thought *Thought
}
