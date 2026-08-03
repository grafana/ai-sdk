package logger

import (
	"context"
	"log/slog"
	"time"
)

const (
	// DefaultMaxStringLen is the default bound for captured string payloads.
	DefaultMaxStringLen = 4096
	// DefaultMaxJSONBytes is the default bound for captured JSON payloads.
	DefaultMaxJSONBytes = 16 * 1024
)

// Options configures structured provider-call logging.
type Options struct {
	// Logger receives structured records. A nil logger defaults to slog.Default().
	Logger *slog.Logger
	// Level is used for successful lifecycle records. A nil level defaults to slog.LevelInfo.
	Level slog.Leveler
	// ErrorLevel is used for failed calls and stream error parts. A nil level defaults to slog.LevelError.
	ErrorLevel slog.Leveler
	// PartLevel is used for EventStreamPart records. A nil level defaults to slog.LevelDebug.
	PartLevel slog.Leveler
	// Attrs are added to every emitted record before redaction.
	Attrs []slog.Attr
	// DynamicAttrs adds request-scoped attributes from context to every emitted record before redaction.
	DynamicAttrs func(ctx context.Context) []slog.Attr
	// Capture controls which sensitive payloads are eligible to be logged.
	Capture CaptureOptions
	// Redactor transforms selected attributes immediately before logging. A nil redactor defaults to DefaultRedactor().
	Redactor Redactor
	// LogStreamParts enables one EventStreamPart record per observed provider stream part.
	LogStreamParts bool
	// Clock returns the current time for durations. A nil clock defaults to time.Now.
	Clock func() time.Time
}

// CaptureOptions controls opt-in logging for sensitive provider-call payloads.
type CaptureOptions struct {
	// Inputs enables prompt/message content capture.
	Inputs bool
	// Outputs enables generated text/content capture.
	Outputs bool
	// Reasoning enables reasoning text capture.
	Reasoning bool
	// ToolInputs enables tool input and tool definition payload capture.
	ToolInputs bool
	// ToolOutputs enables tool output payload capture.
	ToolOutputs bool
	// Files enables file, binary, and source-reference payload capture.
	Files bool
	// RawChunks enables provider raw chunk capture when raw chunks are already emitted.
	RawChunks bool
	// Headers enables request and response header capture.
	Headers bool
	// ProviderOptions enables provider-specific option capture.
	ProviderOptions bool
	// RequestBody enables provider request body capture when metadata is available.
	RequestBody bool
	// ResponseBody enables provider response body capture when metadata is available.
	ResponseBody bool
	// ProviderMetadata enables provider metadata capture.
	ProviderMetadata bool
	// ErrorMessages enables opaque error message capture.
	ErrorMessages bool
	// MaxStringLen bounds captured string values. Zero uses DefaultMaxStringLen.
	MaxStringLen int
	// MaxJSONBytes bounds captured JSON payloads. Zero uses DefaultMaxJSONBytes.
	MaxJSONBytes int
}

type normalizedOptions struct {
	logger         *slog.Logger
	level          slog.Level
	errorLevel     slog.Level
	partLevel      slog.Level
	attrs          []slog.Attr
	dynamicAttrs   func(context.Context) []slog.Attr
	capture        CaptureOptions
	redactor       Redactor
	logStreamParts bool
	clock          func() time.Time
}

func normalizeOptions(opts Options) normalizedOptions {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	level := slog.LevelInfo
	if opts.Level != nil {
		level = opts.Level.Level()
	}

	errorLevel := slog.LevelError
	if opts.ErrorLevel != nil {
		errorLevel = opts.ErrorLevel.Level()
	}

	partLevel := slog.LevelDebug
	if opts.PartLevel != nil {
		partLevel = opts.PartLevel.Level()
	}

	capture := opts.Capture
	if capture.MaxStringLen == 0 {
		capture.MaxStringLen = DefaultMaxStringLen
	}
	if capture.MaxJSONBytes == 0 {
		capture.MaxJSONBytes = DefaultMaxJSONBytes
	}

	redactor := opts.Redactor
	if redactor == nil {
		redactor = DefaultRedactor()
	}

	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}

	attrs := make([]slog.Attr, len(opts.Attrs))
	copy(attrs, opts.Attrs)

	return normalizedOptions{
		logger:         logger,
		level:          level,
		errorLevel:     errorLevel,
		partLevel:      partLevel,
		attrs:          attrs,
		dynamicAttrs:   opts.DynamicAttrs,
		capture:        capture,
		redactor:       redactor,
		logStreamParts: opts.LogStreamParts,
		clock:          clock,
	}
}
