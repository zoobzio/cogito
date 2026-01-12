// Package cogito provides LLM-powered reasoning chains with semantic memory for Go.
//
// cogito implements a Thought-Note-Memory architecture for building autonomous
// systems that reason, remember, and adapt.
//
// # Core Types
//
// The package is built around three core concepts:
//
//   - [Thought] - A reasoning context that accumulates information across pipeline steps
//   - [Note] - Atomic units of information (key-value pairs with optional embeddings)
//   - [Memory] - Persistent storage with semantic search via vector embeddings
//
// # Creating Thoughts
//
// Use [New], [NewWithTrace], or [NewForTask] to create thoughts:
//
//	thought, err := cogito.New(ctx, memory, "analyze customer feedback")
//	thought.SetContent(ctx, "feedback", customerMessage, "input")
//
// # Primitives
//
// Cogito provides a comprehensive set of reasoning primitives:
//
// Decision & Analysis:
//   - [NewDecide] - Binary yes/no decisions with confidence scores
//   - [NewAnalyze] - Extract structured data into typed results
//   - [NewCategorize] - Classify into one of N categories
//   - [NewAssess] - Sentiment analysis with emotional scoring
//   - [NewPrioritize] - Rank items by specified criteria
//
// Control Flow:
//   - [NewSift] - Semantic gate - LLM decides whether to execute wrapped processor
//   - [NewDiscern] - Semantic router - LLM classifies and routes to different processors
//
// Memory & Reflection:
//   - [NewRecall] - Load another Thought and summarize its context
//   - [NewReflect] - Consolidate current Thought's Notes into a summary
//   - [NewCheckpoint] - Create persistent snapshot for branching
//   - [NewSeek] - Semantic search across Notes
//   - [NewSurvey] - Search grouped by task
//   - [NewForget] - Remove notes by key pattern
//   - [NewRestore] - Restore thought state from checkpoint
//   - [NewReset] - Clear all notes from thought
//
// Session Management:
//   - [NewCompress] - LLM-summarize session history to reduce tokens
//   - [NewTruncate] - Sliding window session trimming (no LLM)
//
// Synthesis:
//   - [NewAmplify] - Iterative refinement until criteria met
//   - [NewConverge] - Parallel execution with semantic synthesis
//
// # Pipeline Helpers
//
// Cogito wraps pipz connectors for Thought processing:
//
//   - [Sequence] - Sequential execution
//   - [Filter] - Conditional execution
//   - [Switch] - Route to different processors
//   - [Fallback] - Try alternatives on failure
//   - [Retry] - Retry on failure
//   - [Backoff] - Retry with exponential backoff
//   - [Timeout] - Enforce time limits
//   - [Concurrent] - Run processors in parallel
//   - [Race] - Return first successful result
//
// # Provider & Embedder
//
// LLM and embedding access uses a resolution hierarchy:
//
//  1. Explicit parameter (.WithProvider(p))
//  2. Context value (cogito.WithProvider(ctx, p))
//  3. Global default (cogito.SetProvider(p))
//
// Use [SetProvider] and [SetEmbedder] to configure global defaults:
//
//	cogito.SetProvider(myProvider)
//	cogito.SetEmbedder(cogito.NewOpenAIEmbedder(apiKey))
//
// # Memory Implementation
//
// The [SoyMemory] implementation uses soy for PostgreSQL persistence
// with pgvector for semantic search:
//
//	memory, err := cogito.NewSoyMemory(db)
//
// # Observability
//
// Cogito emits capitan signals throughout execution. See [signals.go] for
// the complete list of events including ThoughtCreated, StepStarted,
// StepCompleted, StepFailed, NoteAdded, and NotesPublished.
package cogito
