# Structured logging

Use `middleware/logger` for consistent `log/slog` records around provider calls.
It is useful for latency and failure diagnostics, usage reporting, and request
correlation across models.

The middleware records provider calls, not one logical application operation. A
multi-step agent, retry, or fallback can produce several call records.

## Install and wrap a model

```bash
go get github.com/grafana/ai-sdk/middleware/logger
```

```go
log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

model := logger.Wrap(baseModel, logger.Options{
	Logger: log,
	Attrs: []slog.Attr{
		slog.String("service", "chat-api"),
	},
})
```

The wrapped value remains a `LanguageModel`. Attach `logger.Middleware` to a
provider registry when every resolved model should use the same policy.

## Start with metadata-only logs

Default records include call identity, provider/model identity, duration,
outcome, request summaries, usage, finish reason, warnings, response metadata,
and stream timing/counts. Streaming usage combines every usage-bearing part and
preserves the greatest value observed for each normalized counter. Error records
retain structured classification, Go type, HTTP status, and retryability when
available. Prompt text, output text, reasoning, tool payloads, files, headers,
bodies, provider options, raw chunks, and opaque error messages are not captured
by default.

This default is appropriate for most production environments. A host can make
its metadata-only policy explicit with `ErrorMessages: false`. Add payload
capture only for a defined debugging or audit requirement:

```go
model := logger.Wrap(baseModel, logger.Options{
	Logger: log,
	Capture: logger.CaptureOptions{
		Inputs:       true,
		Outputs:      true,
		MaxStringLen: 2_048,
		MaxJSONBytes: 8_192,
	},
})
```

Keep capture bounded, sampled, and short-lived. Confirm that retention,
regional, tenant, and user-consent requirements allow the selected content.

## Choose whether to capture error messages

Error class/type, HTTP status, retryability, operation, outcome, timing, and
model identity are structured metadata and remain available when opaque error
messages are disabled. This policy applies to generate errors, stream-open
errors, streamed error parts, cancellation, and timeouts.

Set `ErrorMessages` only when the provider, transport, middleware, and hook
error text is safe for your logging environment:

```go
model := logger.Wrap(baseModel, logger.Options{
	Logger: log,
	Capture: logger.CaptureOptions{
		ErrorMessages: true,
	},
})
```

Captured messages are bounded by `MaxStringLen` and pass through the configured
redactor. They remain opaque strings, however, so the default redactor cannot
reliably remove request details, response details, URLs, or other sensitive
values embedded in them.

## Add request correlation

Use static attributes for deployment-wide values and `DynamicAttrs` for values
read from the call context:

```go
model := logger.Wrap(baseModel, logger.Options{
	Logger: log,
	DynamicAttrs: func(ctx context.Context) []slog.Attr {
		return []slog.Attr{
			slog.String("request_id", requestIDFromContext(ctx)),
			slog.String("trace_id", traceIDFromContext(ctx)),
		}
	},
})
```

Each provider call also gets an SDK call ID that ties start, finish, error, and
optional per-part records together.

## Redact before emission

Captured structured values pass through a redactor. The default redactor handles
common secret-bearing keys, but it cannot infer secrets embedded in opaque text.
Use structured values and extend the redactor with organization-specific keys.

Do not treat redaction as permission to log everything. Avoid payload capture in
the first place when the content is unnecessary.

## Use per-part logging sparingly

`LogStreamParts` helps investigate stream ordering and latency, but can produce
high volume and sensitive output. Keep it at debug level, scope it to a short
window, and apply sampling in the slog handler.

## Choose middleware order

Place logging outside policy middleware when you need a record for denied
attempts. Place it inside when logs should represent only requests that passed
policy and should see transformed parameters. See the [middleware overview](overview.md)
for ordering semantics.

## Reference

- [`middleware/logger`](https://pkg.go.dev/github.com/grafana/ai-sdk/middleware/logger)
- [Prometheus metrics](prometheus.md)
- [Context enrichment](context-enrichment.md)

---

← [Middleware overview](overview.md) · [Docs index](../README.md) · [Context enrichment →](context-enrichment.md)
