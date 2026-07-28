// Package logger provides structured slog logging for language model calls.
//
// Wrap applies logging to a single model:
//
//	model := logger.Wrap(base, logger.Options{
//		Attrs: []slog.Attr{slog.String("component", "llm")},
//	})
//
// Middleware returns the same behavior as a middleware value, useful when
// composing wrappers or attaching logging to a provider registry:
//
//	wrapped := middleware.Wrap(middleware.WrapOptions{
//		Model: base,
//		Middleware: []middleware.Middleware{
//			logger.Middleware(logger.Options{}),
//		},
//	})
//
// Zero-value Options are usable and log through slog.Default(). Default records
// are metadata-only: event schema, call ID, call type, provider/model,
// duration, outcome, request summaries, usage, finish reason, warnings,
// response metadata, and stream part counts. Streams also report time to first
// content when content is observed. Prompt text, generated text, reasoning,
// tool payloads, files, raw chunks, headers, request/response bodies, provider
// options, and provider metadata require explicit CaptureOptions.
//
// Terminal records prefer response provider/model metadata when a gateway or
// router reports the backend that served the call. The original wrapper identity
// is retained as transport metadata when it differs.
//
// Captured attributes pass through a Redactor before emission. DefaultRedactor
// redacts common secret-bearing keys such as authorization, API keys, tokens,
// cookies, passwords, credentials, and secrets in structured maps and slog
// groups. DefaultRedactorWithExtraKeys extends that behavior with
// application-specific secret key patterns.
//
// This package writes structured logs. It does not create traces, metrics, or
// full operation-level telemetry.
package logger
