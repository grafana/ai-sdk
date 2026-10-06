## 1. Module skeleton

- [x] 1.1 Create `middleware/logger/go.mod` with module path `github.com/grafana/ai-sdk/middleware/logger`, `go 1.26.3`, `replace github.com/grafana/ai-sdk => ../../`, and a root ai-sdk requirement only.
- [x] 1.2 Add `middleware/logger/doc.go` with package overview, privacy defaults, provider-layer scope, and examples for `Middleware` and `Wrap`.
- [x] 1.3 Add initial source files: `logger.go`, `options.go`, `events.go`, `redaction.go`, `stream.go`, and package tests.
- [x] 1.4 Run `go mod tidy` inside `middleware/logger` and verify no non-root third-party dependencies are added.
- [x] 1.5 Verify the root module does not import or depend on `middleware/logger`.

## 2. Public API and event schema

- [x] 2.1 Define `Options`, `CaptureOptions`, `EventKind`, `Redactor`, and `RedactorFunc` with package-level doc comments.
- [x] 2.2 Define event constants for generate start/finish/error, stream start/finish/error, and optional stream part records.
- [x] 2.3 Implement option normalization defaults for logger, levels, redactor, clock, and capture bounds.
- [x] 2.4 Implement `Middleware(opts) middleware.Middleware` with `WrapGenerate` and `WrapStream` hooks only; do not implement `TransformParams`.
- [x] 2.5 Implement `Wrap(base, opts)` as a convenience wrapper using `middleware.Wrap` or `middleware.WrapLanguageModel`.
- [x] 2.6 Add compile-time/API smoke tests that reference every exported type, function, and constant required by the spec.

## 3. Attribute building, capture, and redaction

- [x] 3.1 Implement common attribute builders for event name, call type, provider, model, static attrs, dynamic attrs, durations, success state, scalar request summaries, usage, finish reason, warning count, and response metadata.
- [x] 3.2 Implement opt-in capture builders for prompts, tool definitions, generated content, reasoning, tool inputs/outputs, files, raw chunks, headers, provider options, request/response bodies, and provider metadata.
- [x] 3.3 Implement bounded string and JSON serialization for captured payloads with documented finite defaults for zero `MaxStringLen` and `MaxJSONBytes`.
- [x] 3.4 Implement best-effort serialization error attrs (`ai_sdk.serialization_error`) without failing the model call.
- [x] 3.5 Implement `DefaultRedactor` with recursive redaction for maps, slices, slog groups, and JSON-compatible values using the required case-insensitive secret key patterns.
- [x] 3.6 Add redaction tests for headers, provider options, nested maps/slices, slog groups, custom `RedactorFunc`, and opaque strings.

## 4. Generate logging

- [x] 4.1 Implement generate start logging before invoking the inner model.
- [x] 4.2 Invoke `p.DoGenerate(ctx)` exactly once and return the original result or error unchanged.
- [x] 4.3 Implement generate error logging with duration, `success=false`, sanitized error type/message, and API-call status/retryability when available.
- [x] 4.4 Implement generate finish logging with duration, `success=true`, finish reason, usage, warning count, response metadata, and opt-in response captures.
- [x] 4.5 Add unit tests for generate success, generate error propagation, zero-value options, static/dynamic attrs, privacy defaults, capture opt-ins, and no mutation of `provider.CallOptions`/`GenerateResult`.

## 5. Stream logging and teeing

- [x] 5.1 Implement stream start logging before invoking the inner model.
- [x] 5.2 Handle pre-stream `DoStream` errors by logging `EventStreamError` and returning the original error unchanged.
- [x] 5.3 Return a wrapped `*provider.StreamResult` that preserves the original `Request` and `Response` fields and uses a 64-buffer tee channel.
- [x] 5.4 Implement the tee goroutine to forward every `provider.StreamPart` unchanged and in order while accumulating part counts and terminal metadata.
- [x] 5.5 Observe `PartResponseMeta`, `PartFinish`, and `PartError` for response metadata, usage, finish reason, provider metadata, and first stream error.
- [x] 5.6 Implement context cancellation behavior: stop blocking on downstream sends, drain upstream best-effort, close the output channel once, and emit one terminal error record.
- [x] 5.7 Implement optional `EventStreamPart` records when `LogStreamParts` is true, subject to capture/redaction policy.
- [x] 5.8 Add unit tests for stream success teeing, stream open error, `PartError`, context cancellation cleanup, per-part logging opt-in, privacy defaults, and no mutation of stream parts.

## 6. Composition and documentation

- [x] 6.1 Add a user-facing guide page under `docs/guides/` or a logger section linked from `docs/guides/middleware.md` following the repository docs strategy.
- [x] 6.2 Document registry usage with `registry.WithLanguageModelMiddleware(logger.Middleware(opts))`.
- [x] 6.3 Document Sigil ordering guidance: logger outside Sigil for attempted calls and inside Sigil recording for post-hook calls.
- [x] 6.4 Document that root `GenerateText` currently logs as stream calls because it uses `StreamText` internally.
- [x] 6.5 Document that this package is provider-layer structured logging, not full operation-level telemetry or upstream telemetry parity.

## 7. Validation

- [x] 7.1 Run `cd middleware/logger && go test ./...`.
- [x] 7.2 Run `cd middleware/logger && go vet ./...` if the nested module is not already covered by a workspace command.
- [x] 7.3 Run `go test ./...` or `mise run test-short` from the repository root to catch root-module/workspace regressions.
- [x] 7.4 Run `mise run validate-parity-baseline` because this is provider/wire-adjacent but should not change parity fixtures.
- [x] 7.5 Verify no conformance fixtures changed and no provider/UI wire-format deltas were introduced.
- [x] 7.6 Verify dependency isolation by checking the root module does not pull `middleware/logger` and the logger module has no non-root third-party dependencies.
