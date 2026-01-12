# Integration Tests

End-to-end tests that require external dependencies (database, LLM providers).

## Prerequisites

### PostgreSQL with pgvector

1. Install PostgreSQL 15+
2. Install pgvector extension:
   ```sql
   CREATE EXTENSION IF NOT EXISTS vector;
   ```

3. Create test database:
   ```sql
   CREATE DATABASE cogito_test;
   ```

## Running Integration Tests

```bash
# Set database URL
export TEST_DATABASE_URL="postgres://user:pass@localhost:5432/cogito_test?sslmode=disable"

# Run integration tests
make test-integration

# Or directly with go test
go test -v -race -tags=integration ./testing/integration/...
```

## Test Structure

Integration tests use the `integration` build tag:

```go
//go:build integration

package integration_test
```

This ensures they don't run during normal `go test` execution.

## Coverage

### SoyMemory Tests

- `TestSoyMemory_CreateThought` - Thought creation and persistence
- `TestSoyMemory_AddNote` - Note persistence with embeddings
- `TestSoyMemory_GetThought` - Thought retrieval with hydration
- `TestSoyMemory_SearchNotes` - Semantic search functionality
- `TestSoyMemory_SearchNotesByTask` - Task-grouped search

## Writing Integration Tests

```go
//go:build integration

package integration_test

import (
    "os"
    "testing"

    "github.com/jmoiron/sqlx"
    _ "github.com/lib/pq"
    "github.com/zoobzio/cogito"
)

func TestSoyMemory(t *testing.T) {
    dsn := os.Getenv("TEST_DATABASE_URL")
    if dsn == "" {
        t.Skip("TEST_DATABASE_URL not set")
    }

    db, err := sqlx.Connect("postgres", dsn)
    if err != nil {
        t.Fatalf("failed to connect: %v", err)
    }
    defer db.Close()

    memory, err := cogito.NewSoyMemory(db)
    if err != nil {
        t.Fatalf("failed to create memory: %v", err)
    }

    // Test cases...
}
```
