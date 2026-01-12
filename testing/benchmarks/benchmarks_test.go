package benchmarks_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/zoobzio/cogito"
	cogitotest "github.com/zoobzio/cogito/testing"
)

func BenchmarkThoughtCreation(b *testing.B) {
	ctx := context.Background()
	mem := cogitotest.NewMockMemory()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := cogito.New(ctx, mem, "benchmark intent")
		if err != nil {
			b.Fatalf("failed to create thought: %v", err)
		}
	}
}

func BenchmarkNoteAccumulation(b *testing.B) {
	ctx := context.Background()
	mem := cogitotest.NewMockMemory()
	thought, _ := cogito.New(ctx, mem, "benchmark intent")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("note_%d", i)
		err := thought.SetContent(ctx, key, "benchmark content value", "benchmark")
		if err != nil {
			b.Fatalf("failed to add note: %v", err)
		}
	}
}

func BenchmarkNoteRetrieval(b *testing.B) {
	ctx := context.Background()
	mem := cogitotest.NewMockMemory()
	thought, _ := cogito.New(ctx, mem, "benchmark intent")

	// Pre-populate with notes.
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("note_%d", i)
		_ = thought.SetContent(ctx, key, "benchmark content value", "benchmark")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("note_%d", i%100)
		_, err := thought.GetContent(key)
		if err != nil {
			b.Fatalf("failed to get content: %v", err)
		}
	}
}

func BenchmarkThoughtClone(b *testing.B) {
	ctx := context.Background()
	mem := cogitotest.NewMockMemory()
	thought, _ := cogito.New(ctx, mem, "benchmark intent")

	// Pre-populate with notes.
	for i := 0; i < 50; i++ {
		key := fmt.Sprintf("note_%d", i)
		_ = thought.SetContent(ctx, key, "benchmark content value", "benchmark")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = thought.Clone()
	}
}

func BenchmarkAllNotes(b *testing.B) {
	ctx := context.Background()
	mem := cogitotest.NewMockMemory()
	thought, _ := cogito.New(ctx, mem, "benchmark intent")

	// Pre-populate with notes.
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("note_%d", i)
		_ = thought.SetContent(ctx, key, "benchmark content value", "benchmark")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = thought.AllNotes()
	}
}

func BenchmarkRenderNotesToContext(b *testing.B) {
	ctx := context.Background()
	mem := cogitotest.NewMockMemory()
	thought, _ := cogito.New(ctx, mem, "benchmark intent")

	// Pre-populate with notes.
	for i := 0; i < 20; i++ {
		key := fmt.Sprintf("note_%d", i)
		_ = thought.SetContent(ctx, key, "benchmark content value that might be somewhat longer", "benchmark")
	}

	notes := thought.AllNotes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cogito.RenderNotesToContext(notes)
	}
}
