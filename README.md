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

## Reasoning That Accumulates

A Thought is a reasoning context. Primitives read it, reason via LLM, and contribute Notes back. Each step sees what came before.

```go
thought, _ := cogito.New(ctx, memory, "should we approve this refund?")
thought.SetContent(ctx, "request", customerEmail, "input")

// Pipeline: each primitive builds on accumulated context
pipeline := cogito.Sequence("refund-decision",
    cogito.NewAnalyze[RefundRequest]("parse", "extract order ID, amount, and reason"),
    cogito.NewSeek("history", "this customer's previous refund requests"),
    cogito.NewDecide("approve", "does this meet our refund policy?").
        WithIntrospection(),
)

result, _ := pipeline.Process(ctx, thought)

// Every step left a Note — an auditable chain of reasoning
for _, note := range result.AllNotes() {
    fmt.Printf("[%s] %s: %s\n", note.Source, note.Key, note.Content)
}
// [input] request: "I'd like a refund for order #12345..."
// [analyze] parse: {"order_id": "12345", "amount": 49.99, "reason": "arrived damaged"}
// [seek] history: "No previous refund requests found for this customer"
// [decide] approve: "yes"
// [introspect] approve: "Approved: first-time request, clear damage claim, within policy window"
```

Introspection adds a semantic summary explaining _why_ — context for subsequent steps or human review. Notes persist with vector embeddings, enabling semantic search across all stored reasoning.

## Install

```bash
go get github.com/zoobzio/cogito
```

Requires Go 1.21+, PostgreSQL with pgvector (for Memory).

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/zoobzio/cogito"
)

type TicketData struct {
    CustomerName string   `json:"customer_name"`
    Issue        string   `json:"issue"`
    Urgency      []string `json:"urgency_indicators"`
}

func main() {
    ctx := context.Background()

    // Configure providers
    cogito.SetProvider(myLLMProvider)
    cogito.SetEmbedder(cogito.NewOpenAIEmbedder(apiKey))

    // Connect to memory
    memory, _ := cogito.NewSoyMemory(db)

    // Build a reasoning pipeline
    pipeline := cogito.Sequence("ticket-triage",
        cogito.NewAnalyze[TicketData]("parse",
            "extract customer name, issue description, and urgency indicators"),
        cogito.NewSeek("history", "similar support issues and resolutions").
            WithLimit(5),
        cogito.NewCategorize("category", "what type of support issue is this?",
            []string{"billing", "technical", "account", "feature_request"}),
        cogito.NewAssess("urgency",
            "how urgent is this ticket based on tone and content?"),
        cogito.NewDecide("escalate",
            "should this ticket be escalated to a senior agent?").
            WithIntrospection(),
    )

    // Create a thought with initial context
    thought, _ := cogito.New(ctx, memory, "triage support ticket")
    thought.SetContent(ctx, "ticket", incomingTicket, "input")

    // Execute the pipeline
    result, err := pipeline.Process(ctx, thought)
    if err != nil {
        log.Fatal(err)
    }

    // Results are Notes
    category, _ := result.GetContent("category")
    escalate, _ := result.GetContent("escalate")
    fmt.Printf("Category: %s, Escalate: %s\n", category, escalate)
}
```

## Capabilities

| Feature               | Description                                                   | Docs                                             |
| --------------------- | ------------------------------------------------------------- | ------------------------------------------------ |
| Reasoning Primitives  | Decide, Analyze, Categorize, Assess, Prioritize               | [Primitives](docs/2.learn/2.primitives.md)       |
| Semantic Control Flow | Sift (LLM gate) and Discern (LLM router)                      | [Control Flow](docs/3.guides/1.control-flow.md)  |
| Memory & Reflection   | Recall, Reflect, Checkpoint, Seek, Survey                     | [Memory](docs/3.guides/2.memory.md)              |
| Session Management    | Compress and Truncate for token control                       | [Sessions](docs/3.guides/3.sessions.md)          |
| Synthesis             | Amplify (iterative refinement), Converge (parallel synthesis) | [Synthesis](docs/3.guides/4.synthesis.md)        |
| Two-Phase Reasoning   | Deterministic decisions with optional creative introspection  | [Introspection](docs/2.learn/3.introspection.md) |

## Why cogito?

- **Composable reasoning** — Chain primitives into pipelines via [pipz](https://github.com/zoobzio/pipz)
- **Semantic memory** — Notes persist with vector embeddings for similarity search
- **Context accumulation** — Each step builds on previous reasoning
- **Two-phase reasoning** — Deterministic decisions with optional creative introspection
- **Observable** — Emits [capitan](https://github.com/zoobzio/capitan) signals throughout execution
- **Extensible** — Implement `pipz.Chainable[*Thought]` for custom primitives

## Semantic Control Flow

Traditional pipelines route on data. Cogito routes on meaning.

```go
// Sift: LLM decides whether to execute
urgent := cogito.NewSift("urgent-only",
    "is this request time-sensitive?",
    expeditedHandler,
)

// Discern: LLM classifies and routes
router := cogito.NewDiscern("route", "how should we handle this?",
    map[string]pipz.Chainable[*cogito.Thought]{
        "approve": approvalFlow,
        "review":  manualReviewFlow,
        "decline": declineFlow,
    },
)
```

Control flow adapts to domain changes without code changes — the LLM interprets intent.

Integrate with [flume](https://github.com/zoobzio/flume) for declarative, hot-reloadable pipeline definitions.

## Documentation

- [Overview](docs/1.overview.md) — Design philosophy and architecture

### Learn

- [Quickstart](docs/2.learn/1.quickstart.md) — Build your first reasoning pipeline
- [Primitives](docs/2.learn/2.primitives.md) — Decide, Analyze, Categorize, and more
- [Introspection](docs/2.learn/3.introspection.md) — Two-phase reasoning
- [Architecture](docs/2.learn/4.architecture.md) — Thought-Note-Memory model

### Guides

- [Control Flow](docs/3.guides/1.control-flow.md) — Sift and Discern for semantic routing
- [Memory](docs/3.guides/2.memory.md) — Persistence and semantic search
- [Sessions](docs/3.guides/3.sessions.md) — Token management with Compress and Truncate
- [Synthesis](docs/3.guides/4.synthesis.md) — Amplify and Converge patterns
- [Custom Primitives](docs/3.guides/5.custom-primitives.md) — Implementing Chainable[*Thought]
- [Testing](docs/3.guides/6.testing.md) — Testing reasoning pipelines

### Cookbook

- [Support Triage](docs/4.cookbook/1.support-triage.md) — Ticket classification and routing
- [Document Analysis](docs/4.cookbook/2.document-analysis.md) — Extract and reason over documents
- [Decision Workflows](docs/4.cookbook/3.decision-workflows.md) — Multi-step approval flows

### Reference

- [API](docs/5.reference/1.api.md) — Complete function documentation
- [Primitives](docs/5.reference/2.primitives.md) — All primitives with signatures
- [Types](docs/5.reference/3.types.md) — Thought, Note, Memory, Provider

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License — see [LICENSE](LICENSE) for details.
