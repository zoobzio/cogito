package cogito

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/zoobzio/astql/postgres"
	"github.com/zoobzio/soy"
	"github.com/zoobzio/zyn"
)

// SoyMemory implements Memory using soy for persistence.
type SoyMemory struct {
	thoughts *soy.Soy[Thought]
	notes    *soy.Soy[Note]
	db       *sqlx.DB
}

// NewSoyMemory creates a new soy-backed Memory implementation.
func NewSoyMemory(db *sqlx.DB) (*SoyMemory, error) {
	renderer := postgres.New()

	thoughts, err := soy.New[Thought](db, "thoughts", renderer)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize thoughts table: %w", err)
	}

	notes, err := soy.New[Note](db, "notes", renderer)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize notes table: %w", err)
	}

	return &SoyMemory{
		thoughts: thoughts,
		notes:    notes,
		db:       db,
	}, nil
}

// CreateThought persists a new thought and returns it with ID populated.
func (m *SoyMemory) CreateThought(ctx context.Context, thought *Thought) (*Thought, error) {
	inserted, err := m.thoughts.Insert().Exec(ctx, thought)
	if err != nil {
		return nil, fmt.Errorf("failed to insert thought: %w", err)
	}
	return inserted, nil
}

// GetThought loads a thought by ID, including all its notes.
func (m *SoyMemory) GetThought(ctx context.Context, id string) (*Thought, error) {
	thought, err := m.thoughts.Select().
		Where("id", "=", "id").
		Exec(ctx, map[string]any{"id": id})
	if err != nil {
		return nil, fmt.Errorf("failed to get thought: %w", err)
	}

	// Hydrate notes
	if err := m.hydrateThought(ctx, thought); err != nil {
		return nil, err
	}

	return thought, nil
}

// GetThoughtByTraceID loads a thought by trace ID, including all its notes.
func (m *SoyMemory) GetThoughtByTraceID(ctx context.Context, traceID string) (*Thought, error) {
	thought, err := m.thoughts.Select().
		Where("trace_id", "=", "trace_id").
		Exec(ctx, map[string]any{"trace_id": traceID})
	if err != nil {
		return nil, fmt.Errorf("failed to get thought by trace ID: %w", err)
	}

	// Hydrate notes
	if err := m.hydrateThought(ctx, thought); err != nil {
		return nil, err
	}

	return thought, nil
}

// GetThoughtsByTaskID loads all thoughts for a task, ordered by creation time.
func (m *SoyMemory) GetThoughtsByTaskID(ctx context.Context, taskID string) ([]*Thought, error) {
	thoughts, err := m.thoughts.Query().
		Where("task_id", "=", "task_id").
		OrderBy("created_at", "asc").
		Exec(ctx, map[string]any{"task_id": taskID})
	if err != nil {
		return nil, fmt.Errorf("failed to get thoughts by task ID: %w", err)
	}

	// Hydrate each thought
	for _, thought := range thoughts {
		if err := m.hydrateThought(ctx, thought); err != nil {
			return nil, err
		}
	}

	return thoughts, nil
}

// GetChildThoughts loads all thoughts that have the given thought as parent.
func (m *SoyMemory) GetChildThoughts(ctx context.Context, parentID string) ([]*Thought, error) {
	thoughts, err := m.thoughts.Query().
		Where("parent_id", "=", "parent_id").
		OrderBy("created_at", "asc").
		Exec(ctx, map[string]any{"parent_id": parentID})
	if err != nil {
		return nil, fmt.Errorf("failed to get child thoughts: %w", err)
	}

	// Hydrate each thought
	for _, thought := range thoughts {
		if err := m.hydrateThought(ctx, thought); err != nil {
			return nil, err
		}
	}

	return thoughts, nil
}

// AddNote persists a note and returns it with ID populated.
func (m *SoyMemory) AddNote(ctx context.Context, note *Note) (*Note, error) {
	inserted, err := m.notes.Insert().Exec(ctx, note)
	if err != nil {
		return nil, fmt.Errorf("failed to insert note: %w", err)
	}
	return inserted, nil
}

// GetNotes loads all notes for a thought.
func (m *SoyMemory) GetNotes(ctx context.Context, thoughtID string) ([]Note, error) {
	notePtrs, err := m.notes.Query().
		Where("thought_id", "=", "thought_id").
		OrderBy("created", "asc").
		Exec(ctx, map[string]any{"thought_id": thoughtID})
	if err != nil {
		return nil, fmt.Errorf("failed to get notes: %w", err)
	}

	// Convert []*Note to []Note
	notes := make([]Note, len(notePtrs))
	for i, n := range notePtrs {
		notes[i] = *n
	}
	return notes, nil
}

// UpdateThought updates thought metadata (timestamps, publishedCount).
func (m *SoyMemory) UpdateThought(ctx context.Context, thought *Thought) error {
	_, err := m.thoughts.Modify().
		Set("updated_at", "updated_at").
		Where("id", "=", "id").
		Exec(ctx, map[string]any{
			"updated_at": time.Now(),
			"id":         thought.ID,
		})
	if err != nil {
		return fmt.Errorf("failed to update thought: %w", err)
	}
	return nil
}

// DeleteThought removes a thought and all its notes.
func (m *SoyMemory) DeleteThought(ctx context.Context, id string) error {
	// Delete notes first (foreign key constraint)
	_, err := m.notes.Remove().
		Where("thought_id", "=", "thought_id").
		Exec(ctx, map[string]any{"thought_id": id})
	if err != nil {
		return fmt.Errorf("failed to delete notes: %w", err)
	}

	// Delete thought
	_, err = m.thoughts.Remove().
		Where("id", "=", "id").
		Exec(ctx, map[string]any{"id": id})
	if err != nil {
		return fmt.Errorf("failed to delete thought: %w", err)
	}

	return nil
}

// hydrateThought loads notes and session state into a thought.
func (m *SoyMemory) hydrateThought(ctx context.Context, thought *Thought) error {
	notes, err := m.GetNotes(ctx, thought.ID)
	if err != nil {
		return err
	}

	// Set memory reference for future persistence operations
	thought.SetMemory(m)

	// Create fresh session
	thought.Session = zyn.NewSession()

	// Add notes to thought (using internal method to avoid re-persistence)
	for _, note := range notes {
		thought.AddNoteWithoutPersist(note)
	}

	return nil
}

// Close closes the underlying database connection.
func (m *SoyMemory) Close() error {
	return m.db.Close()
}

// SearchNotes finds notes semantically similar to the query embedding.
// Returns notes ordered by similarity, limited to the specified count.
// Notes without embeddings are excluded from results.
func (m *SoyMemory) SearchNotes(ctx context.Context, embedding Vector, limit int) ([]NoteWithThought, error) {
	// Query notes ordered by vector distance
	notes, err := m.notes.Query().
		WhereNotNull("embedding").
		OrderByExpr("embedding", "<->", "query_embedding", "asc").
		Limit(limit).
		Exec(ctx, map[string]any{"query_embedding": embedding})
	if err != nil {
		return nil, fmt.Errorf("failed to search notes: %w", err)
	}

	if len(notes) == 0 {
		return nil, nil
	}

	// Collect unique thought IDs
	thoughtIDSet := make(map[string]struct{})
	for _, note := range notes {
		thoughtIDSet[note.ThoughtID] = struct{}{}
	}
	thoughtIDs := make([]string, 0, len(thoughtIDSet))
	for id := range thoughtIDSet {
		thoughtIDs = append(thoughtIDs, id)
	}

	// Query thoughts by IDs
	thoughts, err := m.thoughts.Query().
		Where("id", "IN", "ids").
		Exec(ctx, map[string]any{"ids": thoughtIDs})
	if err != nil {
		return nil, fmt.Errorf("failed to get thoughts: %w", err)
	}

	// Build thought map for lookup
	thoughtMap := make(map[string]*Thought, len(thoughts))
	for _, t := range thoughts {
		t.SetMemory(m)
		t.Session = zyn.NewSession()
		thoughtMap[t.ID] = t
	}

	// Build results maintaining note order (by similarity)
	results := make([]NoteWithThought, 0, len(notes))
	for _, note := range notes {
		thought := thoughtMap[note.ThoughtID]
		if thought == nil {
			continue
		}
		results = append(results, NoteWithThought{
			Note:    *note,
			Thought: thought,
		})
	}

	return results, nil
}

// SearchNotesByTask finds the most relevant note per task.
// Returns the most recent thought for each task that has matching notes.
// Notes without embeddings are excluded from results.
func (m *SoyMemory) SearchNotesByTask(ctx context.Context, embedding Vector, limit int) ([]*Thought, error) {
	// Query notes ordered by vector distance (fetch more to ensure coverage across tasks)
	notes, err := m.notes.Query().
		WhereNotNull("embedding").
		OrderByExpr("embedding", "<->", "query_embedding", "asc").
		Exec(ctx, map[string]any{"query_embedding": embedding})
	if err != nil {
		return nil, fmt.Errorf("failed to search notes: %w", err)
	}

	if len(notes) == 0 {
		return nil, nil
	}

	// Collect thought IDs from notes
	thoughtIDs := make([]string, 0, len(notes))
	thoughtIDSet := make(map[string]struct{})
	for _, note := range notes {
		if _, seen := thoughtIDSet[note.ThoughtID]; !seen {
			thoughtIDSet[note.ThoughtID] = struct{}{}
			thoughtIDs = append(thoughtIDs, note.ThoughtID)
		}
	}

	// Query thoughts by IDs
	thoughts, err := m.thoughts.Query().
		Where("id", "IN", "ids").
		Exec(ctx, map[string]any{"ids": thoughtIDs})
	if err != nil {
		return nil, fmt.Errorf("failed to get thoughts: %w", err)
	}

	// Build thought map and track task associations
	thoughtMap := make(map[string]*Thought, len(thoughts))
	for _, t := range thoughts {
		thoughtMap[t.ID] = t
	}

	// Track best note distance per task and collect task IDs
	type taskInfo struct {
		taskID   string
		distance int // index in notes slice (lower = closer)
	}
	bestNotePerTask := make(map[string]taskInfo)

	for i, note := range notes {
		thought := thoughtMap[note.ThoughtID]
		if thought == nil || thought.TaskID == nil {
			continue
		}
		taskID := *thought.TaskID
		if _, exists := bestNotePerTask[taskID]; !exists {
			bestNotePerTask[taskID] = taskInfo{taskID: taskID, distance: i}
		}
	}

	if len(bestNotePerTask) == 0 {
		return nil, nil
	}

	// Collect task IDs sorted by best note distance
	type taskWithDistance struct {
		taskID   string
		distance int
	}
	tasksWithDistance := make([]taskWithDistance, 0, len(bestNotePerTask))
	for _, info := range bestNotePerTask {
		tasksWithDistance = append(tasksWithDistance, taskWithDistance(info))
	}
	// Sort by distance (best first)
	for i := 0; i < len(tasksWithDistance)-1; i++ {
		for j := i + 1; j < len(tasksWithDistance); j++ {
			if tasksWithDistance[j].distance < tasksWithDistance[i].distance {
				tasksWithDistance[i], tasksWithDistance[j] = tasksWithDistance[j], tasksWithDistance[i]
			}
		}
	}

	// Limit to requested count
	if len(tasksWithDistance) > limit {
		tasksWithDistance = tasksWithDistance[:limit]
	}

	// Collect task IDs for final query
	taskIDs := make([]string, len(tasksWithDistance))
	for i, t := range tasksWithDistance {
		taskIDs[i] = t.taskID
	}

	// Query most recent thought per task
	allTaskThoughts, err := m.thoughts.Query().
		Where("task_id", "IN", "task_ids").
		OrderBy("created_at", "desc").
		Exec(ctx, map[string]any{"task_ids": taskIDs})
	if err != nil {
		return nil, fmt.Errorf("failed to get task thoughts: %w", err)
	}

	// Pick most recent thought per task
	latestPerTask := make(map[string]*Thought)
	for _, t := range allTaskThoughts {
		if t.TaskID == nil {
			continue
		}
		if _, exists := latestPerTask[*t.TaskID]; !exists {
			latestPerTask[*t.TaskID] = t
		}
	}

	// Build final result in distance order
	results := make([]*Thought, 0, len(tasksWithDistance))
	for _, td := range tasksWithDistance {
		thought := latestPerTask[td.taskID]
		if thought == nil {
			continue
		}
		if err := m.hydrateThought(ctx, thought); err != nil {
			return nil, err
		}
		results = append(results, thought)
	}

	return results, nil
}

var _ Memory = (*SoyMemory)(nil)
