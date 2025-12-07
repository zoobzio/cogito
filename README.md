# cogito

[![CI Status](https://github.com/zoobzio/cogito/workflows/CI/badge.svg)](https://github.com/zoobzio/cogito/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/zoobzio/cogito/graph/badge.svg?branch=main)](https://codecov.io/gh/zoobzio/cogito)
[![Go Report Card](https://goreportcard.com/badge/github.com/zoobzio/cogito)](https://goreportcard.com/report/github.com/zoobzio/cogito)
[![CodeQL](https://github.com/zoobzio/cogito/workflows/CodeQL/badge.svg)](https://github.com/zoobzio/cogito/security/code-scanning)
[![Go Reference](https://pkg.go.dev/badge/github.com/zoobzio/cogito.svg)](https://pkg.go.dev/github.com/zoobzio/cogito)
[![License](https://img.shields.io/github/license/zoobzio/cogito)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/zoobzio/cogito)](go.mod)
[![Release](https://img.shields.io/github/v/release/zoobzio/cogito)](https://github.com/zoobzio/cogito/releases)

LLM-powered reasoning chains with semantic memory for Go.

Build autonomous systems that reason, remember, and adapt.

## The Model

Cogito implements a **Thought-Note-Memory** architecture:

- **Thought** - A reasoning context that accumulates information across pipeline steps
- **Note** - Atomic units of information (key-value pairs with optional embeddings)
- **Memory** - Persistent storage with semantic search via vector embeddings

Each primitive in a pipeline reads context from accumulated Notes, reasons via LLM, and emits new Notes for subsequent steps.

```go
// Create a thought with intent
thought, _ := cogito.New(ctx, memory, "analyse customer feedback")

// Add initial context
thought.SetContent(ctx, "feedback", customerMessage, "input")

// Build reasoning pipeline
pipeline := cogito.Sequence("feedback-analysis",
    cogito.NewAnalyze[FeedbackData]("parse", "extract sentiment and key topics"),
    cogito.NewCategorize("category", "classify the feedback type",
        []string{"bug", "feature", "praise", "complaint"}),
    cogito.NewDecide("escalate", "should this be escalated to management?").
        WithIntrospection(),
)

// Execute
result, _ := pipeline.Process(ctx, thought)

// Results are Notes
category, _ := result.GetContent("category")
shouldEscalate, _ := result.GetContent("escalate")
```

## Why cogito?

- **Composable reasoning** - Chain primitives like Decide, Analyze, Categorize into pipelines
- **Semantic memory** - Notes persist with vector embeddings for similarity search
- **Context accumulation** - Each step builds on previous reasoning
- **Two-phase reasoning** - Deterministic decisions with optional creative introspection
- **Pipeline integration** - Built on [pipz](https://github.com/zoobzio/pipz) for orchestration
- **Observable** - Emits [capitan](https://github.com/zoobzio/capitan) signals throughout execution
- **Extensible** - Implement `pipz.Chainable[*Thought]` for custom primitives

## Installation

```bash
go get github.com/zoobzio/cogito
```

Requirements: Go 1.21+, PostgreSQL with pgvector (for Memory)

## Core Concepts

### Thought

The central abstraction. A Thought maintains:

- An append-only log of Notes
- A tracking cursor for published vs unpublished Notes
- An LLM session for conversation continuity
- A reference to Memory for persistence

```go
thought, _ := cogito.New(ctx, memory, "process support ticket")
thought.SetContent(ctx, "ticket", ticketText, "input")

// Notes accumulate as primitives execute
notes := thought.AllNotes()
```

### Notes

Information atoms with optional vector embeddings:

```go
// Simple content
thought.SetContent(ctx, "summary", "Customer requests refund", "analyze")

// With metadata
thought.SetNote(ctx, "decision", "approved", "decide", map[string]string{
    "confidence": "0.95",
    "reasoning":  "Clear policy violation",
})
```

### Memory

Persistent storage with semantic search:

```go
memory, _ := cogito.NewCerealMemory(db)

// Thoughts and Notes persist automatically
thought, _ := cogito.New(ctx, memory, "analyse document")

// Semantic search across all stored Notes
results, _ := memory.SearchNotes(ctx, embedding, 10)
```

## Primitives

All primitives implement `pipz.Chainable[*Thought]` and can be composed into pipelines.

### Decision & Analysis

| Primitive | Purpose |
|-----------|---------|
| `Decide` | Binary yes/no decisions with confidence scores |
| `Analyze[T]` | Extract structured data into typed results |
| `Categorize` | Classify into one of N categories |
| `Assess` | Sentiment analysis with emotional scoring |
| `Prioritize` | Rank items by specified criteria |

### Control Flow

| Primitive | Purpose |
|-----------|---------|
| `Sift` | Semantic gate - LLM decides whether to execute wrapped processor |
| `Discern` | Semantic router - LLM classifies and routes to different processors |

### Memory & Reflection

| Primitive | Purpose |
|-----------|---------|
| `Recall` | Load another Thought and summarise its context |
| `Reflect` | Consolidate current Thought's Notes into a summary |
| `Checkpoint` | Create persistent snapshot for branching |
| `Seek` | Semantic search across Notes |
| `Survey` | Search grouped by task |

### Session Management

| Primitive | Purpose |
|-----------|---------|
| `Compress` | LLM-summarise session history to reduce tokens |
| `Truncate` | Sliding window session trimming (no LLM) |

### Synthesis

| Primitive | Purpose |
|-----------|---------|
| `Amplify` | Iterative refinement until criteria met |
| `Converge` | Parallel execution with semantic synthesis |

## Pipeline Helpers

Cogito wraps [pipz](https://github.com/zoobzio/pipz) connectors for Thought processing:

```go
// Sequential execution
pipeline := cogito.Sequence("my-pipeline",
    cogito.NewDecide("check", "is this valid?"),
    cogito.NewAnalyze[Result]("parse", "extract fields"),
)

// Conditional execution
filtered := cogito.Filter("only-valid",
    func(ctx context.Context, t *cogito.Thought) bool {
        content, _ := t.GetContent("check")
        return content == "yes"
    },
    cogito.NewCategorize("type", "what type?", categories),
)

// Error handling
resilient := cogito.Fallback("with-fallback",
    cogito.NewAnalyze[Detailed]("detailed", "full analysis"),
    cogito.NewAnalyze[Simple]("simple", "basic analysis"),
)
```

## Provider & Embedder

LLM and embedding access uses a resolution hierarchy:

1. Explicit parameter (`.WithProvider(p)`)
2. Context value (`cogito.WithProvider(ctx, p)`)
3. Global default (`cogito.SetProvider(p)`)

```go
// Set globally
cogito.SetProvider(myProvider)
cogito.SetEmbedder(cogito.NewOpenAIEmbedder(apiKey))

// Override per-step
decide := cogito.NewDecide("key", "question").
    WithProvider(differentProvider).
    WithTemperature(0.3)
```

## Introspection

Primitives support two-phase reasoning:

1. **Reasoning phase** - Deterministic (temperature 0) decision/extraction
2. **Introspection phase** - Creative (temperature 0.7) semantic summary

```go
decide := cogito.NewDecide("escalate", "should we escalate?").
    WithIntrospection()  // Adds semantic summary of reasoning
```

The introspection phase generates context for subsequent primitives, enabling richer reasoning chains.

## Example: Support Ticket Triage

```go
func NewTriagePipeline() pipz.Chainable[*cogito.Thought] {
    return cogito.Sequence("ticket-triage",
        // Parse ticket structure
        cogito.NewAnalyze[TicketData]("parse",
            "extract customer name, issue description, and urgency indicators"),

        // Search for similar past tickets
        cogito.NewSeek("history", "similar support issues and resolutions").
            WithLimit(5),

        // Classify the ticket
        cogito.NewCategorize("category",
            "what type of support issue is this?",
            []string{"billing", "technical", "account", "feature_request"}),

        // Assess urgency
        cogito.NewAssess("urgency",
            "how urgent is this ticket based on tone and content?"),

        // Decide escalation
        cogito.NewDecide("escalate",
            "should this ticket be escalated to a senior agent?").
            WithIntrospection(),
    )
}

func main() {
    ctx := context.Background()
    memory, _ := cogito.NewCerealMemory(db)

    cogito.SetProvider(myProvider)
    cogito.SetEmbedder(cogito.NewOpenAIEmbedder(apiKey))

    pipeline := NewTriagePipeline()

    thought, _ := cogito.New(ctx, memory, "triage support ticket")
    thought.SetContent(ctx, "ticket", incomingTicket, "input")

    result, err := pipeline.Process(ctx, thought)
    if err != nil {
        log.Fatal(err)
    }

    category, _ := result.GetContent("category")
    escalate, _ := result.GetContent("escalate")

    fmt.Printf("Category: %s, Escalate: %s\n", category, escalate)
}
```

## Observability

Cogito emits [capitan](https://github.com/zoobzio/capitan) signals throughout execution:

- `ThoughtCreated` - New thought initialised
- `StepStarted` / `StepCompleted` / `StepFailed` - Primitive lifecycle
- `NoteAdded` - Note persisted
- `NotesPublished` - Notes sent to LLM context
- `SeekResultsFound` / `SurveyResultsFound` - Search results

Use [shotel](https://github.com/zoobzio/shotel) to bridge these signals to OpenTelemetry for logs, metrics, and traces.

## Integration with flume

For declarative, hot-reloadable pipelines, use [flume](https://github.com/zoobzio/flume):

```go
factory := flume.NewFactory[*cogito.Thought]()

// Register cogito primitives
factory.RegisterProcessor("decide", cogito.NewDecide("key", "question"))
factory.RegisterProcessor("analyze", cogito.NewAnalyze[Data]("key", "prompt"))

// Define pipeline in YAML
schema := `
version: 1
pipeline:
  type: sequence
  nodes:
    - processor: decide
    - processor: analyze
`

pipeline, _ := factory.BuildFromYAML("my-pipeline", []byte(schema))
result, _ := pipeline.Process(ctx, thought)
```

## Custom Primitives

Implement `pipz.Chainable[*Thought]` for domain-specific reasoning:

```go
type MyPrimitive struct {
    key      string
    provider cogito.Provider
}

func (p *MyPrimitive) Process(ctx context.Context, t *cogito.Thought) (*cogito.Thought, error) {
    // Read context from unpublished notes
    notes := t.GetUnpublishedNotes()
    context := cogito.RenderNotesToContext(notes)

    // Resolve provider (explicit → context → global)
    provider, err := cogito.ResolveProvider(ctx, p.provider)
    if err != nil {
        return t, err
    }

    // Your LLM logic here using provider
    // ...

    // Store result as note
    t.SetContent(ctx, p.key, result, "my_primitive")

    // Mark notes as published
    t.MarkNotesPublished()

    return t, nil
}

func (p *MyPrimitive) Name() pipz.Name { return pipz.Name(p.key) }
func (p *MyPrimitive) Close() error    { return nil }
```

## Performance

- Primitives minimise LLM calls through context accumulation
- Published/unpublished note tracking prevents redundant context
- Session management (Compress/Truncate) controls token usage
- Parallel execution via Converge with Clone-based isolation

## Contributing

Contributions welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

```bash
# Run tests
make test

# Run linter
make lint

# Run benchmarks
make bench
```

## License

MIT License - see [LICENSE](LICENSE) file for details.
