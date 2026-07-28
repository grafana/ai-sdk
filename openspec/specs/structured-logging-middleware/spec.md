# Structured Logging Middleware

## Purpose

Provide an opt-in provider-layer structured logging middleware for language model calls, with stable event records, privacy-first capture defaults, and dependency isolation from the root ai-sdk module.

## Requirements

### Requirement: Nested logger middleware module

The repository SHALL provide a nested Go module at `middleware/logger` with module path `github.com/grafana/ai-sdk/middleware/logger`.

The module SHALL depend on the root `github.com/grafana/ai-sdk` module and the Go standard library. It SHALL NOT introduce dependencies on OpenTelemetry SDKs, Agent Observability, gRPC, vendor SDKs, provider modules, or other third-party logging libraries.

The root `github.com/grafana/ai-sdk` module SHALL NOT import `middleware/logger`, so consumers who import only the root module do not gain logger-specific dependencies or public API.

#### Scenario: Root consumers do not import logger

- **WHEN** a consumer imports only `github.com/grafana/ai-sdk`
- **THEN** `github.com/grafana/ai-sdk/middleware/logger` SHALL NOT appear in the consumer's transitive import graph

#### Scenario: Logger module depends only on root ai-sdk and standard library

- **WHEN** running dependency inspection for `./middleware/logger/...`
- **THEN** dependencies outside the Go standard library SHALL be limited to `github.com/grafana/ai-sdk`
- **AND** the dependency graph SHALL NOT include Agent Observability, OpenTelemetry SDKs, gRPC, vendor provider SDKs, or `github.com/grafana/ai-sdk/providers/*`

### Requirement: Public logger API

The `middleware/logger` package SHALL expose a middleware constructor and direct wrapping helper:

- `Middleware(opts Options) middleware.Middleware`
- `Wrap(base provider.LanguageModel, opts Options) provider.LanguageModel`

The `Options` type SHALL include:

- `Logger *slog.Logger`
- `Level slog.Leveler`
- `ErrorLevel slog.Leveler`
- `PartLevel slog.Leveler`
- `Attrs []slog.Attr`
- `DynamicAttrs func(ctx context.Context) []slog.Attr`
- `Capture CaptureOptions`
- `Redactor Redactor`
- `LogStreamParts bool`
- `Clock func() time.Time`

The `CaptureOptions` type SHALL include explicit opt-in controls for `Inputs`, `Outputs`, `Reasoning`, `ToolInputs`, `ToolOutputs`, `Files`, `RawChunks`, `Headers`, `ProviderOptions`, `RequestBody`, `ResponseBody`, and `ProviderMetadata`, plus bounded payload controls `MaxStringLen` and `MaxJSONBytes`.

The package SHALL expose a `Redactor` interface, a `RedactorFunc` adapter, `DefaultRedactor() Redactor`, and `DefaultRedactorWithExtraKeys(keys ...string) Redactor`.

`Options.Logger == nil` SHALL default to `slog.Default()`. `Options.Level == nil` SHALL default to `slog.LevelInfo`. `Options.ErrorLevel == nil` SHALL default to `slog.LevelError`. `Options.PartLevel == nil` SHALL default to `slog.LevelDebug`. `Options.Redactor == nil` SHALL default to `DefaultRedactor()`. `Options.Clock == nil` SHALL default to `time.Now`.

#### Scenario: Middleware returns a language model middleware

- **WHEN** `logger.Middleware(logger.Options{})` is called
- **THEN** it SHALL return a `middleware.Middleware` with generate and stream wrapping behavior

#### Scenario: Wrap is equivalent to middleware.Wrap

- **WHEN** `logger.Wrap(base, opts)` is called
- **THEN** it SHALL return a `provider.LanguageModel`
- **AND** the observable behavior SHALL match `middleware.Wrap(middleware.WrapOptions{Model: base, Middleware: []middleware.Middleware{logger.Middleware(opts)}})`

#### Scenario: Zero-value options are usable

- **WHEN** `logger.Middleware(logger.Options{})` wraps a model
- **THEN** calls through the wrapped model SHALL log through `slog.Default()` at info level for successful lifecycle records
- **AND** error records SHALL log at error level
- **AND** the wrapped model call SHALL remain pass-through except for logging

### Requirement: Stable event names and attributes

The package SHALL define `EventKind` as a typed string and SHALL expose these event constants:

- `EventGenerateStart = "aisdk.model.generate.start"`
- `EventGenerateFinish = "aisdk.model.generate.finish"`
- `EventGenerateError = "aisdk.model.generate.error"`
- `EventStreamStart = "aisdk.model.stream.start"`
- `EventStreamFinish = "aisdk.model.stream.finish"`
- `EventStreamError = "aisdk.model.stream.error"`
- `EventStreamCancelled = "aisdk.model.stream.cancelled"`
- `EventStreamPart = "aisdk.model.stream.part"`

Every log record SHALL use the event string as the `slog` message and SHALL include `ai_sdk.event` with the same value and `ai_sdk.event.schema` with the current schema version.

Common records SHALL include stable `ai_sdk.*` attributes for call ID, call type, provider, model, outcome on terminal records, and caller-provided static/dynamic attrs. Records SHALL include `gen_ai.*` aliases for common GenAI provider/model/usage fields where available. Terminal records SHALL include duration in milliseconds with fractional precision, duration in nanoseconds, and success/failure. Success records SHALL include available finish reason, token usage including cache/text/reasoning subfields when available, warning count/types, and response metadata. Stream terminal records SHALL include total part count, per-type counts, and time to first content when content is observed.

The logger SHALL use documented stable keys for optional captured payloads and SHALL add new keys only additively in future changes.

#### Scenario: Generate start record has stable identity attrs

- **WHEN** a generate call starts through the logger middleware
- **THEN** the emitted record SHALL have message `"aisdk.model.generate.start"`
- **AND** it SHALL include `ai_sdk.event="aisdk.model.generate.start"`
- **AND** it SHALL include `ai_sdk.call.id`, `ai_sdk.call.type="generate"`, `ai_sdk.provider`, and `ai_sdk.model`

#### Scenario: Stream finish record has summary attrs

- **WHEN** a stream completes successfully after emitting text and finish parts
- **THEN** the emitted terminal record SHALL have message `"aisdk.model.stream.finish"`
- **AND** it SHALL include `ai_sdk.success=true`
- **AND** it SHALL include `ai_sdk.duration_ms`
- **AND** it SHALL include stream part count attributes
- **AND** it SHALL include available finish reason and usage attributes

#### Scenario: Static and dynamic attrs are attached

- **WHEN** `Options.Attrs` and `Options.DynamicAttrs` both return attributes for a request
- **THEN** those attributes SHALL be included on every record for that request after the logger's stable attributes are built and before redaction runs

### Requirement: Privacy-first capture policy

By default, the logger SHALL NOT log prompt/message content, generated text, reasoning text, tool inputs, tool outputs, file data, raw chunks, request bodies, response bodies, headers, provider options, or provider metadata.

By default, the logger MAY log safe scalar summaries and counts, including provider/model identity, transport identity when a routed backend differs, duration, outcome, success/failure, max output tokens, temperature, top-p, top-k, seed, reasoning effort value, stop sequence count, tool count, response format type, usage totals/subfields, finish reason, warning count/types, response metadata, stream part counts, and stream time to first content.

Sensitive fields SHALL only become eligible for logging when the corresponding `CaptureOptions` flag is enabled. Captured string and JSON payload attributes SHALL be bounded by `MaxStringLen`, `MaxJSONBytes`, or documented finite package defaults when those fields are zero.

#### Scenario: Default logging omits sensitive request fields

- **WHEN** a request includes a secret in the prompt, headers, provider options, tool input, and request body
- **AND** the logger is configured with zero-value `CaptureOptions`
- **THEN** no emitted log record SHALL contain that secret
- **AND** emitted records MAY include counts and scalar settings for the request

#### Scenario: Capture options opt in to payload attrs

- **WHEN** `Capture.Inputs`, `Capture.Headers`, and `Capture.ProviderOptions` are enabled
- **THEN** prompt, header, and provider option attributes MAY be emitted
- **AND** those attributes SHALL still pass through the configured redactor before logging

#### Scenario: Captured payloads are bounded

- **WHEN** a captured prompt or JSON payload exceeds the configured capture limit
- **THEN** the logged attribute value SHALL be summarized to stay within the configured or default bound
- **AND** the model call SHALL continue unchanged

### Requirement: Default redaction

`DefaultRedactor()` SHALL redact known secret-bearing keys even when capture options make their parent object eligible for logging.

The default redactor SHALL perform case-insensitive key matching for at least these patterns: `authorization`, `x-api-key`, `api-key`, `apikey`, `token`, `access_token`, `refresh_token`, `id_token`, `password`, `secret`, `credential`, `cookie`, and `set-cookie`.

The default redactor SHALL recurse through maps, slices, slog groups, and JSON-compatible values where feasible. If a captured attr is already an opaque string, the default redactor SHALL NOT rely on brittle substring rewriting; it SHALL redact only fields represented as structured attrs or typed values.

A custom `Redactor` SHALL receive the request context, event kind, and selected attrs immediately before logging. `DefaultRedactorWithExtraKeys` SHALL preserve default behavior while adding caller-supplied secret key patterns.

#### Scenario: Secret header is redacted after capture

- **WHEN** `Capture.Headers` is enabled and request headers include `Authorization: Bearer secret`
- **THEN** the emitted header attribute SHALL replace the authorization value with a redaction marker
- **AND** the unredacted token SHALL NOT appear in any emitted record

#### Scenario: Custom redactor can remove attrs

- **WHEN** `Options.Redactor` removes an attr from the provided attr slice
- **THEN** the removed attr SHALL NOT be emitted in the log record

### Requirement: Generate call logging

The middleware SHALL implement `WrapGenerate` by observing the call without mutating the request or response.

For each generate call, the middleware SHALL:

1. Build start attributes from `middleware.WrapGenerateParams`, including call type, provider, model, safe request summary, and any opted-in captured request fields.
2. Log `EventGenerateStart` before invoking the inner call.
3. Invoke `p.DoGenerate(ctx)` exactly once.
4. On returned error, log `EventGenerateError` with duration, `ai_sdk.outcome="error"`, `ai_sdk.success=false`, stable error classification/message, optional Go error type, and API-call status/retryability when available; then return the original error unchanged.
5. On success, log `EventGenerateFinish` with duration, `ai_sdk.outcome="success"`, `ai_sdk.success=true`, finish reason, usage, warning count/types, response metadata, and opted-in captured response fields; then return the original `*provider.GenerateResult` unchanged.

Serialization, capture, redaction, or logging failures SHALL NOT fail the model call. Best-effort serialization failures SHALL be represented as `ai_sdk.serialization_error` attrs when possible.

#### Scenario: Generate success logs start and finish

- **WHEN** the inner model's `DoGenerate` returns a successful `*provider.GenerateResult`
- **THEN** exactly one generate start record and one generate finish record SHALL be emitted for that call
- **AND** the returned result pointer SHALL be the same pointer returned by the inner model

#### Scenario: Generate error logs and propagates original error

- **WHEN** the inner model's `DoGenerate` returns a sentinel error
- **THEN** exactly one generate error record SHALL be emitted for that call after the start record
- **AND** the wrapped call SHALL return the original sentinel error unchanged

#### Scenario: Generate logging does not mutate params

- **WHEN** the logger captures or summarizes request fields for a generate call
- **THEN** the `provider.CallOptions` passed to the inner model SHALL be equal to the options provided to the wrapped model after any outer middleware transformations

### Requirement: Stream call logging

The middleware SHALL implement `WrapStream` by observing and teeing streams without mutating stream parts.

For each stream call, the middleware SHALL:

1. Log `EventStreamStart` before invoking the inner stream call.
2. Invoke `p.DoStream(ctx)` exactly once.
3. If opening the stream returns an error, log `EventStreamError` with duration and sanitized error details, then return the original error unchanged.
4. If opening succeeds, return a new `*provider.StreamResult` that preserves the upstream `Request` and `Response` fields and exposes a tee stream channel.
5. Forward every upstream `provider.StreamPart` to the downstream consumer unchanged and in order.
6. Observe stream parts to accumulate counts, time to first content, response metadata from `provider.PartResponseMeta`, usage/finish reason/provider metadata from `provider.PartFinish`, and the first `provider.PartError`.
7. Log `EventStreamPart` records only when `Options.LogStreamParts` is true, using `Options.PartLevel` except for error parts.
8. Log exactly one terminal event when upstream closes: `EventStreamFinish` for successful completion, `EventStreamError` when a part error or deadline timeout is observed, or `EventStreamCancelled` when cancellation is observed.
9. Close the returned stream channel exactly once.

The tee channel SHALL use a bounded buffer of 64 entries unless a future measured change updates this requirement.

#### Scenario: Stream success tees unmodified parts

- **GIVEN** an upstream stream emits multiple `provider.StreamPart` values
- **WHEN** a consumer reads from the logger-wrapped stream
- **THEN** the consumer SHALL receive the same parts in the same order
- **AND** the terminal log record SHALL include counts derived from those parts

#### Scenario: Stream open error logs and propagates

- **WHEN** the inner model's `DoStream` returns an error before a stream is opened
- **THEN** the wrapped call SHALL return that original error unchanged
- **AND** one `EventStreamError` record SHALL be emitted after the start record

#### Scenario: Stream part error logs terminal error

- **WHEN** the upstream stream emits a `provider.PartError` containing an `APICallError`
- **THEN** the part SHALL still be forwarded to the consumer unchanged
- **AND** the terminal record SHALL be `EventStreamError` with `ai_sdk.outcome="error"` and `ai_sdk.success=false`
- **AND** the record SHALL include sanitized API-call status/retryability details when available

#### Scenario: Stream context cancellation finalizes once

- **WHEN** the request context is cancelled while the stream tee is active
- **THEN** the tee SHALL stop blocking on downstream sends
- **AND** it SHALL close the returned stream channel exactly once
- **AND** it SHALL emit exactly one terminal `EventStreamCancelled` record for the call with `ai_sdk.outcome="cancelled"`

#### Scenario: Per-part logging is opt-in

- **WHEN** `Options.LogStreamParts` is false
- **THEN** no `EventStreamPart` records SHALL be emitted
- **WHEN** `Options.LogStreamParts` is true
- **THEN** the logger MAY emit one `EventStreamPart` record per observed stream part subject to capture and redaction policy

### Requirement: Provider behavior remains unchanged

The logger middleware SHALL NOT mutate `provider.CallOptions`, SHALL NOT force `IncludeRawChunks`, SHALL NOT mutate `provider.GenerateResult`, and SHALL NOT mutate any `provider.StreamPart`.

The middleware SHALL preserve the upstream `StreamResult.Request` and `StreamResult.Response` values when wrapping streams.

#### Scenario: Raw chunks are not forced

- **WHEN** a stream call has `CallOptions.IncludeRawChunks=false`
- **THEN** the logger middleware SHALL pass `IncludeRawChunks=false` to the inner model
- **AND** it SHALL NOT synthesize or request raw chunks for logging

#### Scenario: Stream metadata is preserved

- **WHEN** the inner stream result has non-nil `Request` and `Response` metadata
- **THEN** the logger-wrapped stream result SHALL expose the same `Request` and `Response` values

### Requirement: Composition and documentation

The logger middleware SHALL compose as an ordinary `middleware.Middleware` with `middleware.Wrap`, `middleware.WrapLanguageModel`, `registry.WithLanguageModelMiddleware`, fallback models, and `middleware/agentobservability`.

The package documentation and user-facing docs SHALL explain:

- how to wrap a single model;
- how to attach the logger through `registry.WithLanguageModelMiddleware`;
- privacy defaults and capture/redaction controls;
- that root `GenerateText` currently appears to provider middleware as stream calls because it uses `StreamText` internally;
- that middleware ordering controls whether logs are outside or inside Agent Observability hooks/recording;
- that this package is a lightweight provider-layer logger, not the full upstream telemetry integration system.

#### Scenario: Registry applies logger middleware

- **WHEN** a `registry.ProviderRegistry` is created with `registry.WithLanguageModelMiddleware(logger.Middleware(opts))`
- **AND** a model is resolved from the registry
- **THEN** calls through the resolved model SHALL emit logger middleware records

#### Scenario: Agent Observability ordering is caller controlled

- **WHEN** logger middleware is placed before Agent Observability middleware in the `middleware.Wrap` slice
- **THEN** logger SHALL be the outer middleware according to existing middleware ordering
- **AND** docs SHALL describe that this logs attempted calls including Agent Observability denials

#### Scenario: Future telemetry boundary is documented

- **WHEN** users read the logger package or guide documentation
- **THEN** the documentation SHALL state that the logger observes provider calls only
- **AND** it SHALL NOT claim to replace operation-level telemetry, tracing integration registries, or tool execution telemetry
