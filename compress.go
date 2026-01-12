package cogito

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zoobzio/capitan"
	"github.com/zoobzio/pipz"
	"github.com/zoobzio/zyn"
)

// Compress is a session management primitive that implements pipz.Chainable[*Thought].
// It summarizes the current session via LLM and replaces it with a fresh session
// containing the summary as context.
type Compress struct {
	identity    pipz.Identity
	key         string
	threshold   int    // Minimum message count to trigger compression (0 = always)
	summaryKey  string // Note key to store the summary
	temperature float32
	provider    Provider
}

// NewCompress creates a new session compression primitive.
//
// The primitive uses a zyn Transform synapse to summarize the session history,
// then replaces the session with a new one containing the summary as the first message.
//
// Output Notes:
//   - {key}: The generated summary text
//
// Example:
//
//	compress := cogito.NewCompress("session_compress").
//	    WithThreshold(10)  // Only compress if >= 10 messages
//	result, _ := compress.Process(ctx, thought)
func NewCompress(key string) *Compress {
	return &Compress{
		identity:    pipz.NewIdentity(key, "Session compression primitive"),
		key:         key,
		threshold:   0,
		temperature: DefaultReasoningTemperature,
	}
}

// Process implements pipz.Chainable[*Thought].
func (c *Compress) Process(ctx context.Context, t *Thought) (*Thought, error) {
	start := time.Now()

	// Check threshold
	messageCount := t.Session.Len()
	if c.threshold > 0 && messageCount < c.threshold {
		// Below threshold, pass through unchanged
		return t, nil
	}

	// Nothing to compress
	if messageCount == 0 {
		return t, nil
	}

	// Resolve provider
	provider, err := ResolveProvider(ctx, c.provider)
	if err != nil {
		return t, fmt.Errorf("compress: %w", err)
	}

	// Emit step started
	capitan.Emit(ctx, StepStarted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(c.key),
		FieldStepType.Field("compress"),
		FieldNoteCount.Field(messageCount),
		FieldTemperature.Field(c.temperature),
	)

	// Build session text for summarization
	sessionText := c.buildSessionText(t.Session.Messages())

	// Create transform synapse for summarization
	transformSynapse, err := zyn.Transform(
		"Summarize this conversation history into a concise context that preserves key information, decisions made, and important details for continuing the conversation",
		provider,
	)
	if err != nil {
		c.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("compress: failed to create transform synapse: %w", err)
	}

	// Generate summary
	summary, err := transformSynapse.FireWithInput(ctx, t.Session, zyn.TransformInput{
		Text:        sessionText,
		Style:       "Be concise but comprehensive. Preserve factual details, decisions, and context needed to continue the conversation coherently.",
		Temperature: c.temperature,
	})
	if err != nil {
		c.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("compress: summarization failed: %w", err)
	}

	// Store summary as a note
	summaryKey := c.summaryKey
	if summaryKey == "" {
		summaryKey = c.key
	}
	if err := t.SetContent(ctx, summaryKey, summary, "compress"); err != nil {
		c.emitFailed(ctx, t, start, err)
		return t, fmt.Errorf("compress: failed to persist summary note: %w", err)
	}

	// Replace session with fresh one containing summary
	t.Session.Clear()
	t.Session.Append("system", fmt.Sprintf("Previous conversation summary:\n%s", summary))

	// Emit step completed
	duration := time.Since(start)
	capitan.Emit(ctx, StepCompleted,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(c.key),
		FieldStepType.Field("compress"),
		FieldStepDuration.Field(duration),
		FieldContextSize.Field(len(summary)),
	)

	return t, nil
}

// buildSessionText formats session messages for summarisation.
func (c *Compress) buildSessionText(messages []zyn.Message) string {
	var builder strings.Builder
	for _, msg := range messages {
		builder.WriteString(msg.Role)
		builder.WriteString(": ")
		builder.WriteString(msg.Content)
		builder.WriteString("\n\n")
	}
	return builder.String()
}

// emitFailed emits a step failed event.
func (c *Compress) emitFailed(ctx context.Context, t *Thought, start time.Time, err error) {
	capitan.Error(ctx, StepFailed,
		FieldTraceID.Field(t.TraceID),
		FieldStepName.Field(c.key),
		FieldStepType.Field("compress"),
		FieldStepDuration.Field(time.Since(start)),
		FieldError.Field(err),
	)
}

// Identity implements pipz.Chainable[*Thought].
func (c *Compress) Identity() pipz.Identity {
	return c.identity
}

// Schema implements pipz.Chainable[*Thought].
func (c *Compress) Schema() pipz.Node {
	return pipz.Node{Identity: c.identity, Type: "compress"}
}

// Close implements pipz.Chainable[*Thought].
func (c *Compress) Close() error {
	return nil
}

// Builder methods

// WithThreshold sets the minimum message count to trigger compression.
// If the session has fewer messages, the primitive passes through unchanged.
func (c *Compress) WithThreshold(n int) *Compress {
	c.threshold = n
	return c
}

// WithSummaryKey sets the note key where the summary will be stored.
func (c *Compress) WithSummaryKey(key string) *Compress {
	c.summaryKey = key
	return c
}

// WithTemperature sets the temperature for summarisation.
func (c *Compress) WithTemperature(temp float32) *Compress {
	c.temperature = temp
	return c
}

// WithProvider sets the provider for this step.
func (c *Compress) WithProvider(p Provider) *Compress {
	c.provider = p
	return c
}
