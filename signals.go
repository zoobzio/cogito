package cogito

import "github.com/zoobzio/capitan"

// Signal definitions for cogito reasoning chain events.
// Signals follow the pattern: cogito.<entity>.<event>.
var (
	// Thought lifecycle signals.
	ThoughtCreated = capitan.NewSignal(
		"cogito.thought.created",
		"New thought chain initiated with intent and trace ID",
	)

	// Step execution signals.
	StepStarted = capitan.NewSignal(
		"cogito.step.started",
		"Reasoning step began execution",
	)
	StepCompleted = capitan.NewSignal(
		"cogito.step.completed",
		"Reasoning step finished successfully",
	)
	StepFailed = capitan.NewSignal(
		"cogito.step.failed",
		"Reasoning step encountered an error",
	)

	// Note management signals.
	NoteAdded = capitan.NewSignal(
		"cogito.note.added",
		"New note added to thought context",
	)
	NotesPublished = capitan.NewSignal(
		"cogito.notes.published",
		"Notes marked as published to LLM",
	)

	// Introspection signals.
	IntrospectionStarted = capitan.NewSignal(
		"cogito.introspection.started",
		"Transform synapse beginning semantic summary generation",
	)
	IntrospectionCompleted = capitan.NewSignal(
		"cogito.introspection.completed",
		"Transform synapse completed semantic summary",
	)

	// Context signals.
	ContextAccumulated = capitan.NewSignal(
		"cogito.context.accumulated",
		"Unpublished notes gathered for next LLM call",
	)
)

// Field keys for cogito event data.
var (
	// Thought metadata.
	FieldIntent    = capitan.NewStringKey("intent")
	FieldTraceID   = capitan.NewStringKey("trace_id")
	FieldNoteCount = capitan.NewIntKey("note_count")
	FieldStepCount = capitan.NewIntKey("step_count")

	// Step metadata.
	FieldStepName    = capitan.NewStringKey("step_name")
	FieldStepType    = capitan.NewStringKey("step_type") // decide, classify, analyze, sentiment, rank
	FieldTemperature = capitan.NewFloat32Key("temperature")
	FieldProvider    = capitan.NewStringKey("provider")

	// Note metadata.
	FieldNoteKey     = capitan.NewStringKey("note_key")
	FieldNoteSource  = capitan.NewStringKey("note_source")
	FieldContentSize = capitan.NewIntKey("content_size") // character count

	// Context metrics.
	FieldUnpublishedCount = capitan.NewIntKey("unpublished_count")
	FieldPublishedCount   = capitan.NewIntKey("published_count")
	FieldContextSize      = capitan.NewIntKey("context_size") // character count

	// Timing.
	FieldStepDuration = capitan.NewDurationKey("step_duration")

	// Error information.
	FieldError = capitan.NewErrorKey("error")
)
