# Testing

This directory contains test utilities, benchmarks, and integration tests for cogito.

## Directory Structure

```
testing/
├── helpers.go          # Test utilities (cogitotest package)
├── helpers_test.go     # Tests for helpers
├── README.md           # This file
├── benchmarks/         # Performance benchmarks
│   ├── README.md
│   └── benchmarks_test.go
└── integration/        # Integration tests
    ├── README.md
    └── soy_test.go
```

## Test Utilities

The `cogitotest` package provides utilities for testing cogito primitives:

```go
import "github.com/zoobzio/cogito/testing"

func TestMyPrimitive(t *testing.T) {
    // Create a thought with mock memory
    thought := cogitotest.NewTestThought(t, "test intent")

    // Add content
    thought.SetContent(ctx, "input", "test value", "test")

    // Assert content
    cogitotest.RequireContent(t, thought, "input", "test value")
}
```

### Available Helpers

- `NewMockMemory()` - Creates an in-memory mock for `cogito.Memory`
- `NewTestThought(t, intent)` - Creates a thought with mock memory
- `NewTestThoughtWithTrace(t, intent, traceID)` - Creates a thought with explicit trace ID
- `RequireContent(t, thought, key, expected)` - Asserts content exists and matches
- `RequireNoContent(t, thought, key)` - Asserts no content at key

## Running Tests

### Unit Tests

```bash
# Run all unit tests
make test

# Run unit tests only (short mode)
make test-unit
```

### Integration Tests

Integration tests require PostgreSQL with pgvector extension.

```bash
# Set up test database
export TEST_DATABASE_URL="postgres://user:pass@localhost:5432/cogito_test?sslmode=disable"

# Run integration tests
make test-integration
```

### Benchmarks

```bash
# Run all benchmarks
make bench
```

## Writing Tests

### Unit Tests

Place unit tests alongside the source files they test:

```
cogito/
├── decide.go
├── decide_test.go
├── analyze.go
├── analyze_test.go
└── ...
```

### Integration Tests

Place integration tests in `testing/integration/`:

```go
//go:build integration

package integration_test

import (
    "testing"
    "github.com/zoobzio/cogito"
)

func TestSoyMemory(t *testing.T) {
    // Tests requiring real database
}
```

### Benchmarks

Place benchmarks in `testing/benchmarks/`:

```go
package benchmarks_test

import (
    "testing"
    "github.com/zoobzio/cogito"
)

func BenchmarkThoughtCreation(b *testing.B) {
    // Benchmark code
}
```
