# Structured Logging Middleware

## Purpose

Provide an opt-in provider-layer structured logging middleware for language model calls, with stable event records, privacy-first capture defaults, and dependency isolation from the root ai-sdk module.
## Requirements
### Requirement: Nested logger middleware module

The repository SHALL provide `middleware/logger`, a nested Go module at `github.com/grafana/ai-sdk/middleware/logger`, depending only on root ai-sdk and the standard library, not OpenTelemetry SDKs, Agent Observability, gRPC, vendor SDKs, provider modules, or third-party logging libraries. Root ai-sdk SHALL NOT import it; root-only consumers SHALL gain no logger-specific dependencies or public API.

#### Scenario: Root consumers do not import logger

- **WHEN** a consumer imports only `github.com/grafana/ai-sdk`
- **THEN** `github.com/grafana/ai-sdk/middleware/logger` SHALL NOT appear in the consumer's transitive import graph

#### Scenario: Logger module depends only on root ai-sdk and standard library

- **WHEN** running dependency inspection for `./middleware/logger/...`
- **THEN** dependencies outside the Go standard library SHALL be limited to `github.com/grafana/ai-sdk`
- **AND** the dependency graph SHALL NOT include Agent Observability, OpenTelemetry SDKs, gRPC, vendor provider SDKs, or `github.com/grafana/ai-sdk/providers/*`

### Requirement: Public logger API

`middleware/logger` SHALL expose `Middleware(opts Options) middleware.Middleware` and `Wrap(base provider.LanguageModel, opts Options) provider.LanguageModel`. It SHALL expose a `Redactor` interface, `RedactorFunc` adapter, `DefaultRedactor() Redactor`, and `DefaultRedactorWithExtraKeys(keys ...string) Redactor`.

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

`EventKind` SHALL be a typed string with stable lifecycle and part constants. Every record SHALL use its event string as the `slog` message and `ai_sdk.event`, plus `ai_sdk.event.schema` with the current schema version. Optional captured payloads SHALL use documented stable keys; future keys SHALL be additive only.

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

#### Scenario: Lifecycle event constants select stable messages
- **WHEN** a caller selects a generate, stream, or part event kind
- **THEN** the package SHALL expose `EventGenerateStart = "aisdk.model.generate.start"`, `EventGenerateFinish = "aisdk.model.generate.finish"`, and `EventGenerateError = "aisdk.model.generate.error"`
- **AND** it SHALL expose `EventStreamStart = "aisdk.model.stream.start"`, `EventStreamFinish = "aisdk.model.stream.finish"`, `EventStreamError = "aisdk.model.stream.error"`, `EventStreamCancelled = "aisdk.model.stream.cancelled"`, and `EventStreamPart = "aisdk.model.stream.part"`

### Requirement: Privacy-first capture policy

By default the logger SHALL NOT log prompt/message content, generated or reasoning text, tool inputs/outputs, file data, raw chunks, request/response bodies, headers, provider options/metadata, or opaque error messages. Sensitive fields SHALL become eligible only via their `CaptureOptions` flags. Captured strings/JSON SHALL be bounded by `MaxStringLen`, `MaxJSONBytes`, or documented finite defaults when zero.

#### Scenario: Default logging omits sensitive request fields

- **WHEN** a request includes a secret in the prompt, headers, provider options, tool input, and request body
- **AND** the logger is configured with zero-value `CaptureOptions`
- **THEN** no emitted log record SHALL contain that secret
- **AND** emitted records MAY include counts and scalar settings for the request

#### Scenario: Capture options opt in to payload attrs

- **WHEN** `Capture.Inputs`, `Capture.Headers`, and `Capture.ProviderOptions` are enabled
- **THEN** prompt, header, and provider option attributes MAY be emitted
- **AND** those attributes SHALL still pass through the configured redactor before logging

#### Scenario: Metadata-only errors omit opaque messages

- **WHEN** a unary error, stream-open error, streamed error part, context cancellation, or timeout contains an opaque message
- **AND** `Capture.ErrorMessages` is false
- **THEN** no emitted record SHALL contain the opaque message
- **AND** error class/type, HTTP status, retryability, operation, outcome, timing, and model identity metadata SHALL remain available when applicable

#### Scenario: Error message capture is opt-in

- **WHEN** `Capture.ErrorMessages` is true
- **THEN** the bounded opaque error message MAY be emitted
- **AND** the message SHALL still pass through the configured redactor before logging

#### Scenario: Captured payloads are bounded

- **WHEN** a captured prompt or JSON payload exceeds the configured capture limit
- **THEN** the logged attribute value SHALL be summarized to stay within the configured or default bound
- **AND** the model call SHALL continue unchanged

#### Scenario: Safe scalar summaries remain eligible by default
- **WHEN** capture flags are not enabled
- **THEN** the logger MAY log safe summaries/counts: provider/model identity, transport identity when a routed backend differs, duration, outcome, success/failure, max output tokens, temperature, top-p, top-k, seed, reasoning effort value, stop sequence count, tool count, response format type, usage totals/subfields, finish reason, warning count/types, response metadata, stream part counts, and stream time to first content

### Requirement: Default redaction

`DefaultRedactor()` SHALL redact known secret-bearing keys even in capture-eligible objects. It SHALL recurse through maps, slices, slog groups, and JSON-compatible values where feasible. Custom redactors SHALL receive context, event kind, and selected attrs immediately before logging. `DefaultRedactorWithExtraKeys` SHALL preserve defaults while adding caller secret-key patterns.

#### Scenario: Secret header is redacted after capture

- **WHEN** `Capture.Headers` is enabled and request headers include `Authorization: Bearer secret`
- **THEN** the emitted header attribute SHALL replace the authorization value with a redaction marker
- **AND** the unredacted token SHALL NOT appear in any emitted record

#### Scenario: Custom redactor can remove attrs

- **WHEN** `Options.Redactor` removes an attr from the provided attr slice
- **THEN** the removed attr SHALL NOT be emitted in the log record

#### Scenario: Default secret patterns match case-insensitively
- **WHEN** a capture-eligible structured value contains secret-bearing keys
- **THEN** default redaction SHALL match case-insensitively at least `authorization`, `x-api-key`, `api-key`, `apikey`, `token`, `access_token`, `refresh_token`, `id_token`, `password`, `secret`, `credential`, `cookie`, and `set-cookie`

#### Scenario: Opaque captured strings do not invite substring rewriting
- **WHEN** a captured attr is already an opaque string
- **THEN** the default redactor SHALL NOT rely on brittle substring rewriting
- **AND** it SHALL redact only fields represented as structured attrs or typed values

### Requirement: Generate call logging

`WrapGenerate` SHALL observe without mutating requests/results, build start attrs from `middleware.WrapGenerateParams` (call type, provider/model, safe request summary, opted-in captures), log `EventGenerateStart` before calling `p.DoGenerate(ctx)` exactly once, then log error or finish and return the original error/result unchanged. Serialization, capture, redaction, and logging failures SHALL NOT fail the call; serialization failures SHALL add `ai_sdk.serialization_error` when possible.

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

#### Scenario: Generate error record includes bounded diagnostics
- **WHEN** `p.DoGenerate(ctx)` returns an error
- **THEN** `EventGenerateError` SHALL include duration, `ai_sdk.outcome="error"`, `ai_sdk.success=false`, stable error classification, Go error type, and available API-call status/retryability
- **AND** a bounded opaque message SHALL be included only when `Capture.ErrorMessages` is true
- **AND** the original error SHALL be returned unchanged

#### Scenario: Generate finish record includes response summary and captures
- **WHEN** `p.DoGenerate(ctx)` succeeds
- **THEN** `EventGenerateFinish` SHALL include duration, `ai_sdk.outcome="success"`, `ai_sdk.success=true`, finish reason, usage, warning count/types, response metadata, and opted-in captured response fields
- **AND** the original `*provider.GenerateResult` SHALL be returned unchanged

### Requirement: Stream call logging

`WrapStream` SHALL log `EventStreamStart` before invoking `p.DoStream(ctx)` exactly once. Opening errors SHALL log `EventStreamError` with duration and sanitized details and return the original error. Success SHALL return a new `*provider.StreamResult` preserving upstream `Request`/`Response`, with a tee forwarding every part unchanged in order. The channel SHALL close exactly once and use a bounded 64-entry buffer unless a future measured change updates this requirement.

#### Scenario: Stream usage preserves strongest values

- **GIVEN** usage is split across multiple stream parts
- **AND** a later finish part omits or reports lower provisional normalized counters
- **WHEN** the stream completes
- **THEN** the terminal log record SHALL contain the independently aggregated strongest normalized counters

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

#### Scenario: Stream observations feed terminal summaries
- **WHEN** the tee observes stream parts
- **THEN** it SHALL accumulate counts, time to first content, response metadata from `provider.PartResponseMeta`, normalized usage from every usage-bearing part using shared aggregation, finish reason/provider metadata from `provider.PartFinish`, and the first `provider.PartError`

#### Scenario: Per-part level applies except to errors
- **WHEN** `Options.LogStreamParts` is true
- **THEN** `EventStreamPart` records SHALL use `Options.PartLevel` except for error parts
- **AND** part records SHALL be logged only when `Options.LogStreamParts` is true

#### Scenario: Upstream close selects one terminal event
- **WHEN** the upstream stream closes
- **THEN** exactly one terminal event SHALL be logged: `EventStreamFinish` for success, `EventStreamError` for a part error or deadline timeout, or `EventStreamCancelled` for cancellation

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

Logger SHALL compose as ordinary `middleware.Middleware` with `middleware.Wrap`, `middleware.WrapLanguageModel`, `registry.WithLanguageModelMiddleware`, fallback models, and `middleware/agentobservability`. Package and user docs SHALL explain single-model and registry wrapping, privacy/capture/redaction, Agent Observability ordering, the `GenerateText` stream-call boundary, and the lightweight provider-layer rather than full upstream telemetry scope.

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

#### Scenario: Documentation explains provider-call and ordering boundaries
- **WHEN** users read package documentation or user-facing docs
- **THEN** they SHALL explain that root `GenerateText` currently appears as stream calls to provider middleware because it uses `StreamText` internally
- **AND** middleware order SHALL be documented as controlling whether logs are outside or inside Agent Observability hooks/recording
- **AND** docs SHALL identify the package as a lightweight provider-layer logger, not the full upstream telemetry integration system

### Requirement: Selectable requested model identity
Structured logging options SHALL provide a named identity-source setting. Its zero value SHALL preserve the current response-preferred behavior. Requested identity mode SHALL use the wrapped model provider and model ID for start and terminal records, SHALL NOT replace them from unary or streaming response metadata, and SHALL omit response/transport identity fields including provider response IDs. It SHALL observe and return all response metadata unchanged.

#### Scenario: Requested identity suppresses backend identity
- **WHEN** requested identity mode wraps provider `grafana` and model `grafana/assistant`, and a response names provider `anthropic`, a backend model ID, and a response ID
- **THEN** start and terminal records SHALL identify only provider `grafana` and model `grafana/assistant`
- **AND** response and transport identity attributes SHALL be absent
- **AND** the original result or stream part SHALL retain its response metadata

#### Scenario: Zero value preserves existing behavior
- **WHEN** identity source is not configured and complete response identity differs from wrapped model identity
- **THEN** terminal identity and transport metadata SHALL retain the existing response-preferred behavior

### Requirement: Configurable bounded stream drain
Structured logging options SHALL provide an optional positive stream-drain duration. When configured, cancellation cleanup SHALL drain the immediate upstream only until channel close or the absolute deadline, including for a continuously ready channel. Zero SHALL preserve the existing direct-consumer drain behavior.

#### Scenario: Configured drain expires
- **WHEN** downstream cancellation occurs and the immediate upstream never closes or remains continuously ready
- **THEN** the logger-owned drain goroutine SHALL exit no later than the configured absolute deadline
- **AND** terminal logging and downstream channel closure SHALL occur exactly once without waiting for the drain

### Requirement: Logger options and capture controls

Logger options SHALL configure logging, levels, request attributes, capture, redaction, per-part logging, and time. Capture SHALL be explicit opt-in with bounded payload controls.

#### Scenario: Configure logger lifecycle options
- **WHEN** a caller constructs `Options`
- **THEN** it SHALL include `Logger *slog.Logger`, `Level slog.Leveler`, `ErrorLevel slog.Leveler`, `PartLevel slog.Leveler`, `Attrs []slog.Attr`, `DynamicAttrs func(ctx context.Context) []slog.Attr`, `Capture CaptureOptions`, `Redactor Redactor`, `LogStreamParts bool`, and `Clock func() time.Time`

#### Scenario: Configure capture categories and bounds
- **WHEN** a caller constructs `CaptureOptions`
- **THEN** explicit opt-in controls SHALL include `Inputs`, `Outputs`, `Reasoning`, `ToolInputs`, `ToolOutputs`, `Files`, `RawChunks`, `Headers`, `ProviderOptions`, `RequestBody`, `ResponseBody`, `ProviderMetadata`, and `ErrorMessages`
- **AND** bounded payload controls SHALL include `MaxStringLen` and `MaxJSONBytes`

#### Scenario: Nil options select standard defaults
- **WHEN** logger, level, redactor, or clock options are nil
- **THEN** nil `Logger` SHALL use `slog.Default()`, nil `Level` SHALL use `slog.LevelInfo`, nil `ErrorLevel` SHALL use `slog.LevelError`, and nil `PartLevel` SHALL use `slog.LevelDebug`
- **AND** nil `Redactor` SHALL use `DefaultRedactor()` and nil `Clock` SHALL use `time.Now`

### Requirement: Logger common and terminal summaries

Common records SHALL include stable `ai_sdk.*` call ID/type, provider/model, terminal outcome, and caller static/dynamic attrs, plus available `gen_ai.*` aliases for common GenAI provider/model/usage fields. Terminal records SHALL include fractional-millisecond duration, nanosecond duration, and success/failure. Stream terminal records SHALL include total and per-type part counts and time to first content when observed.

#### Scenario: Successful terminal summaries include available fields
- **WHEN** a model call completes successfully
- **THEN** the terminal record SHALL include available finish reason, token usage with available cache/text/reasoning subfields, warning count/types, and response metadata
