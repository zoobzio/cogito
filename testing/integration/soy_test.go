//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/zoobzio/cogito"
)

func getTestDB(t *testing.T) *sqlx.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}

	return db
}

func TestSoyMemory_CreateThought(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	memory, err := cogito.NewSoyMemory(db)
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}

	ctx := context.Background()
	thought, err := cogito.New(ctx, memory, "test intent")
	if err != nil {
		t.Fatalf("failed to create thought: %v", err)
	}

	if thought.ID == "" {
		t.Error("expected thought to have ID")
	}
	if thought.Intent != "test intent" {
		t.Errorf("expected intent 'test intent', got %q", thought.Intent)
	}

	// Clean up.
	_ = memory.DeleteThought(ctx, thought.ID)
}

func TestSoyMemory_AddNote(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	memory, err := cogito.NewSoyMemory(db)
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}

	ctx := context.Background()
	thought, err := cogito.New(ctx, memory, "test intent")
	if err != nil {
		t.Fatalf("failed to create thought: %v", err)
	}
	defer func() { _ = memory.DeleteThought(ctx, thought.ID) }()

	err = thought.SetContent(ctx, "key1", "value1", "test")
	if err != nil {
		t.Fatalf("failed to set content: %v", err)
	}

	content, err := thought.GetContent("key1")
	if err != nil {
		t.Fatalf("failed to get content: %v", err)
	}
	if content != "value1" {
		t.Errorf("expected content 'value1', got %q", content)
	}
}

func TestSoyMemory_GetThought(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	memory, err := cogito.NewSoyMemory(db)
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}

	ctx := context.Background()
	thought, err := cogito.New(ctx, memory, "test intent")
	if err != nil {
		t.Fatalf("failed to create thought: %v", err)
	}
	defer func() { _ = memory.DeleteThought(ctx, thought.ID) }()

	err = thought.SetContent(ctx, "key1", "value1", "test")
	if err != nil {
		t.Fatalf("failed to set content: %v", err)
	}

	// Retrieve thought from database.
	retrieved, err := memory.GetThought(ctx, thought.ID)
	if err != nil {
		t.Fatalf("failed to get thought: %v", err)
	}

	if retrieved.ID != thought.ID {
		t.Errorf("expected ID %q, got %q", thought.ID, retrieved.ID)
	}
	if retrieved.Intent != "test intent" {
		t.Errorf("expected intent 'test intent', got %q", retrieved.Intent)
	}

	// Check hydrated notes.
	notes := retrieved.AllNotes()
	if len(notes) != 1 {
		t.Errorf("expected 1 note, got %d", len(notes))
	}
}

func TestSoyMemory_GetThoughtByTraceID(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	memory, err := cogito.NewSoyMemory(db)
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}

	ctx := context.Background()
	thought, err := cogito.NewWithTrace(ctx, memory, "test intent", "trace-123")
	if err != nil {
		t.Fatalf("failed to create thought: %v", err)
	}
	defer func() { _ = memory.DeleteThought(ctx, thought.ID) }()

	retrieved, err := memory.GetThoughtByTraceID(ctx, "trace-123")
	if err != nil {
		t.Fatalf("failed to get thought by trace ID: %v", err)
	}

	if retrieved.ID != thought.ID {
		t.Errorf("expected ID %q, got %q", thought.ID, retrieved.ID)
	}
}

func TestSoyMemory_DeleteThought(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	memory, err := cogito.NewSoyMemory(db)
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}

	ctx := context.Background()
	thought, err := cogito.New(ctx, memory, "test intent")
	if err != nil {
		t.Fatalf("failed to create thought: %v", err)
	}

	err = thought.SetContent(ctx, "key1", "value1", "test")
	if err != nil {
		t.Fatalf("failed to set content: %v", err)
	}

	err = memory.DeleteThought(ctx, thought.ID)
	if err != nil {
		t.Fatalf("failed to delete thought: %v", err)
	}

	_, err = memory.GetThought(ctx, thought.ID)
	if err == nil {
		t.Error("expected error when getting deleted thought")
	}
}
