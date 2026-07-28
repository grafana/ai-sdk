package logger

// EventKind identifies a structured logger middleware event.
type EventKind string

const (
	// EventGenerateStart is emitted before a generate model call starts.
	EventGenerateStart EventKind = "aisdk.model.generate.start"
	// EventGenerateFinish is emitted after a generate model call succeeds.
	EventGenerateFinish EventKind = "aisdk.model.generate.finish"
	// EventGenerateError is emitted after a generate model call fails.
	EventGenerateError EventKind = "aisdk.model.generate.error"
	// EventStreamStart is emitted before a stream model call starts.
	EventStreamStart EventKind = "aisdk.model.stream.start"
	// EventStreamFinish is emitted after a stream model call succeeds.
	EventStreamFinish EventKind = "aisdk.model.stream.finish"
	// EventStreamError is emitted after a stream model call fails.
	EventStreamError EventKind = "aisdk.model.stream.error"
	// EventStreamCancelled is emitted after a stream model call is cancelled by context cancellation.
	EventStreamCancelled EventKind = "aisdk.model.stream.cancelled"
	// EventStreamPart is emitted for stream parts when per-part logging is enabled.
	EventStreamPart EventKind = "aisdk.model.stream.part"
)
