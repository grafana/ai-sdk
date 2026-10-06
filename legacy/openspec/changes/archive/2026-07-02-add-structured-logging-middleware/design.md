## Context

The ai-sdk repository already exposes a provider-agnostic middleware layer around `provider.LanguageModel`. `middleware.Middleware` has optional `WrapGenerate` and `WrapStream` hooks that receive transformed `provider.CallOptions`, the inner `provider.LanguageModel`, and closures for both call modes (`middleware/middleware.go`). Registry-resolved models can be wrapped once with `registry.WithLanguageModelMiddleware`, so provider middleware is the right layer for cross-cutting model-call logging.

The provider-facing surface contains the data a logger can safely summarize or explicitly capture: `provider.CallOptions` contains prompts, tools, headers, provider options, and scalar generation settings; `provider.GenerateResult` contains content, usage, finish reason, warnings, request metadata, response metadata, headers, body, and provider metadata; `provider.StreamResult` carries request/response metadata plus a stream of `provider.StreamPart` values. Streaming logs must tee `StreamResult.Stream` instead of draining it before returning. Existing patterns are `middleware.TransformStream` for simple transforms and `middleware/sigil`'s 64-buffer tee for recording/finalization.

`GenerateText` currently runs through `StreamText`, and `StreamText` calls `DoStream` for each step. A logger-wrapped model therefore observes root `GenerateText` calls as stream calls, not direct generate calls. Direct `provider.LanguageModel.DoGenerate` callers and middleware cross-mode calls still exercise the generate path.

The registered upstream baseline is `ai@7.0.11` in `test/conformance/upstream.yaml`. Upstream has broader telemetry/devtools integrations, but this change intentionally adds only a lightweight provider-layer logging middleware. It must not change provider request parameters, stream wire output, UI chunks, or the root module dependency graph.

## Goals / Non-Goals

**Goals:**

- Add an opt-in nested module `middleware/logger` for structured provider-call logs.
- Use only the Go standard library (`log/slog`) and the root ai-sdk module; do not add root dependencies.
- Support both `WrapGenerate` and `WrapStream` with unchanged pass-through behavior.
- Emit stable event names and `ai_sdk.*` attribute keys suitable for machines and dashboards.
- Default to privacy-safe metadata and summaries, with explicit capture controls for sensitive payloads.
- Run redaction after capture selection and before `slog.Logger.LogAttrs`.
- Tee streams without mutating, reordering, or eagerly draining parts before returning the `StreamResult`.
- Document registry integration, Sigil composition ordering, and the boundary with future core telemetry.

**Non-Goals:**

- Do not add logging APIs or dependencies to the root `middleware` package.
- Do not add OpenTelemetry, Sigil, gRPC, provider SDK, or vendor-specific dependencies.
- Do not port upstream `ai@7.0.11` telemetry/integration registry or `@ai-sdk/otel` behavior.
- Do not change `provider.LanguageModel`, provider request/result structs, stream part structs, UI message chunks, SSE framing, `StreamText` callbacks, or root global logger registration.
- Do not force `CallOptions.IncludeRawChunks` or otherwise mutate `CallOptions` to make logs richer.
- Do not claim operation-level visibility over `StreamText`/`GenerateText`; the logger observes only provider model calls.

## Decisions

### 1. Use a nested `middleware/logger` module

**Decision:** Create `middleware/logger` with module path `github.com/grafana/ai-sdk/middleware/logger`, `replace github.com/grafana/ai-sdk => ../../`, and a dependency only on `github.com/grafana/ai-sdk` plus the Go standard library.

**Rationale:** The issue asks for an independent middleware module, and nesting keeps the root dependency graph unchanged. Even though `log/slog` is standard library, isolation avoids growing the root `middleware` API with a heavier, policy-rich built-in and follows the nested middleware convention established by `middleware/sigil`.

**Alternatives considered:**

- Root `middleware` package: simpler import path, but contrary to the requested independent module and privacy-policy surface.
- Generic telemetry package: too broad and likely to conflict with future core telemetry.

### 2. Public API uses `log/slog` with explicit options

**Decision:** Expose the following primary API:

```go
func Middleware(opts Options) middleware.Middleware
func Wrap(base provider.LanguageModel, opts Options) provider.LanguageModel

type Options struct {
    Logger         *slog.Logger
    Level          slog.Leveler
    ErrorLevel     slog.Leveler
    Attrs          []slog.Attr
    DynamicAttrs   func(ctx context.Context) []slog.Attr
    Capture        CaptureOptions
    Redactor       Redactor
    LogStreamParts bool
    Clock          func() time.Time
}

type CaptureOptions struct {
    Inputs           bool
    Outputs          bool
    Reasoning        bool
    ToolInputs       bool
    ToolOutputs      bool
    Files            bool
    RawChunks        bool
    Headers          bool
    ProviderOptions  bool
    RequestBody      bool
    ResponseBody     bool
    ProviderMetadata bool
    MaxStringLen     int
    MaxJSONBytes     int
}

type EventKind string
const (
    EventGenerateStart  EventKind = "aisdk.model.generate.start"
    EventGenerateFinish EventKind = "aisdk.model.generate.finish"
    EventGenerateError  EventKind = "aisdk.model.generate.error"
    EventStreamStart    EventKind = "aisdk.model.stream.start"
    EventStreamFinish   EventKind = "aisdk.model.stream.finish"
    EventStreamError    EventKind = "aisdk.model.stream.error"
    EventStreamPart     EventKind = "aisdk.model.stream.part"
)

type Redactor interface {
    RedactAttrs(ctx context.Context, event EventKind, attrs []slog.Attr) []slog.Attr
}
type RedactorFunc func(ctx context.Context, event EventKind, attrs []slog.Attr) []slog.Attr
func DefaultRedactor() Redactor
```

`Options.Logger == nil` defaults to `slog.Default()`. `Options.Level == nil` defaults to `slog.LevelInfo`; `Options.ErrorLevel == nil` defaults to `slog.LevelError`; `Options.Redactor == nil` defaults to `DefaultRedactor()`; `Options.Clock == nil` defaults to `time.Now`.

`MaxStringLen` and `MaxJSONBytes` are included in v1 because the API allows payload capture. Zero values use finite documented package defaults; positive values override them. This keeps accidental opt-in payload logging bounded without requiring custom redactors.

**Alternatives considered:**

- Custom sink interface: more abstract but unnecessary because `slog.Handler` already lets callers route records elsewhere.
- Nil logger means no-op: safer for accidental imports, but Go's standard logging pattern is to default to the process logger. Users who want no logging can avoid installing the middleware or pass a discard `slog.Handler`.
- No size limits in v1: simpler API but unsafe once content capture is enabled.

### 3. Stable events and attributes

**Decision:** Log one start event before each inner model call and one terminal finish or error event after the call completes. For streams, per-part records are disabled by default and emitted only when `LogStreamParts` is true.

Each record uses the event string as the slog message and includes `ai_sdk.event` as an attribute. Stable common keys include:

- `ai_sdk.event`
- `ai_sdk.call.type` (`generate` or `stream`)
- `ai_sdk.provider`
- `ai_sdk.model`
- `ai_sdk.duration_ms` on terminal records
- `ai_sdk.success` on terminal records
- `ai_sdk.error.type`, `ai_sdk.error.message`, and API-call status/retryability when available
- `ai_sdk.finish_reason`, `ai_sdk.finish_reason.raw`
- `ai_sdk.usage.input_tokens.total`, `ai_sdk.usage.output_tokens.total`, `ai_sdk.usage.output_tokens.reasoning`
- `ai_sdk.warnings.count`
- `ai_sdk.response.id`, `ai_sdk.response.provider`, `ai_sdk.response.model`, `ai_sdk.response.timestamp`
- `ai_sdk.stream.parts.count` and per-type stream count keys such as `ai_sdk.stream.parts.text_delta.count`, `ai_sdk.stream.parts.reasoning_delta.count`, `ai_sdk.stream.parts.tool_call.count`, `ai_sdk.stream.parts.tool_result.count`, and `ai_sdk.stream.parts.error.count`

Default request summary keys are limited to scalar controls and counts, such as max output tokens, sampling settings, seed, reasoning effort, stop sequence count, tool count, response format type, and `include_raw_chunks`. Tool names are not logged by default because they can reveal application internals.

Optional captured keys include `ai_sdk.request.prompt`, `ai_sdk.request.tools`, `ai_sdk.request.headers`, `ai_sdk.request.provider_options`, `ai_sdk.request.body`, `ai_sdk.response.content`, `ai_sdk.response.body`, `ai_sdk.stream.part`, `ai_sdk.stream.text`, `ai_sdk.stream.reasoning`, `ai_sdk.stream.tool.input`, `ai_sdk.stream.tool.output`, and `ai_sdk.provider_metadata`.

**Alternatives considered:**

- Single terminal record only: less log volume, but start records are useful for in-flight call auditing and failures that happen before a provider returns.
- Always log stream parts: too expensive and likely to leak content; keep it opt-in.

### 4. Generate logging behavior

**Decision:** Implement `WrapGenerate` only as an observer. It builds start attributes from `middleware.WrapGenerateParams`, logs `EventGenerateStart`, calls `p.DoGenerate(ctx)` exactly once, and returns the original result or error unchanged. On success it logs `EventGenerateFinish` with duration, finish reason, usage, warning count, response metadata, and opt-in captured result fields. On error it logs `EventGenerateError` with duration and sanitized error details.

The wrapper must never mutate `p.Params`, `p.Model`, or the returned `*provider.GenerateResult`. Attribute construction and serialization failures become best-effort `ai_sdk.serialization_error` attributes and do not affect the provider call.

**Alternatives considered:**

- Use `TransformParams` to force raw chunks or inject tracing metadata: rejected because the logger must not change provider behavior.

### 5. Stream logging behavior uses a bounded tee

**Decision:** Implement `WrapStream` by logging `EventStreamStart`, calling `p.DoStream(ctx)` exactly once, and returning a new `*provider.StreamResult` with the original `Request` and `Response` pointers and a tee channel. If `p.DoStream` returns an error before a stream opens, log `EventStreamError` and return that original error unchanged.

The tee goroutine uses a 64-depth buffered output channel, matching `middleware.TransformStream` and `middleware/sigil` precedent. It observes each upstream `provider.StreamPart`, forwards it unchanged and in order, accumulates counts, response metadata from `provider.PartResponseMeta`, finish reason/usage/provider metadata from `provider.PartFinish`, and the first `provider.PartError`. It logs exactly one terminal event when the upstream channel closes: `EventStreamFinish` if no observed part error and no cancellation error, otherwise `EventStreamError`.

If the downstream consumer stops reading and `ctx.Done()` fires, the tee stops blocking on downstream sends, drains any immediately available buffered upstream parts for summary accuracy, closes the output channel exactly once, and logs `EventStreamError` with `success=false`. It does not wait indefinitely for an idle upstream channel to close because cancellation must release downstream consumers promptly. Providers are expected to observe the same request context and stop producing after cancellation; no dedicated abort event is introduced in v1.

**Alternatives considered:**

- Use `middleware.TransformStream` directly: useful for one-to-one transforms, but this middleware needs terminal finalization and drain-on-cancel behavior.
- Drain upstream until it closes after downstream cancellation: maximizes producer cleanup for providers that require a drain, but can hang the returned stream forever when the upstream channel is idle and the request context is already cancelled.

### 6. Privacy model: capture gates first, redaction second

**Decision:** Defaults log only metadata, timing, usage, finish reason, warning/error summaries, response metadata, scalar request summaries, and counts. Sensitive fields are omitted unless their corresponding `CaptureOptions` flag is true.

Capture selection happens before redaction. The redactor then receives the selected `[]slog.Attr` and can remove, replace, or add attributes before logging. `DefaultRedactor` recursively redacts known secret-looking keys in maps, slices, slog groups, and JSON-compatible values using case-insensitive key matching for patterns including `authorization`, `x-api-key`, `api-key`, `apikey`, `token`, `access_token`, `refresh_token`, `id_token`, `password`, `secret`, `credential`, `cookie`, and `set-cookie`.

The default redactor should prefer typed traversal over brittle JSON string rewriting. If an attr is already a string containing serialized JSON, the implementation should not attempt broad substring replacement; instead it should bound and emit it as selected by capture policy.

**Alternatives considered:**

- Redaction-only privacy: insufficient because omitted fields are safer than serialized-and-redacted fields.
- Provider metadata summary by default: rejected because `provider.ProviderMetadata` is opaque and can contain provider-specific payloads or identifiers.

### 7. Composition with registry, fallback, and Sigil

**Decision:** Logger middleware is an ordinary `middleware.Middleware` and must compose through existing middleware utilities. Registry integration is just `registry.WithLanguageModelMiddleware(logger.Middleware(opts))`. Fallback wrappers and provider-specific wrappers remain transparent because the logger uses only `provider.LanguageModel` methods.

Middleware order remains caller-controlled. Because `middleware.Wrap` applies the first middleware as outermost, docs should recommend:

- Logger outside Sigil to log all attempted calls, including Sigil hook denials and errors.
- Logger inside Sigil recording to log only calls that pass hooks and to observe post-hook transformed params.

The docs must also explain that root `GenerateText` currently appears as stream calls because it is implemented through `StreamText`.

**Alternatives considered:**

- Special-case Sigil integration: rejected; logger remains independent and should not import `middleware/sigil`.

### 8. Future core telemetry boundary

**Decision:** Document `middleware/logger` as a lightweight provider-layer utility, not a full telemetry system. It does not provide operation-level context, tracing spans, integration registries, context allow-lists, or tool execution hooks. It can coexist with future core telemetry and may later serve as a sink, but this API should not reserve global process registration or telemetry-specific package names.

**Alternatives considered:**

- Implement upstream-like telemetry now: out of scope and much broader than the issue.

## Risks / Trade-offs

| Risk | Mitigation |
| --- | --- |
| Sensitive prompts, outputs, headers, or provider options leak into logs | Privacy-safe defaults, explicit capture gates, size limits, default recursive redaction, and tests that assert secrets are absent by default. |
| Stream tee blocks or leaks when consumers disconnect | Use a bounded 64-buffer channel, select on `ctx.Done()` for downstream sends, drain immediately available upstream parts, close the returned stream promptly, and rely on providers to observe the shared request context after cancellation. |
| Logging changes provider behavior by requesting raw chunks | Do not implement `TransformParams` and do not mutate `CallOptions`; raw chunks are logged only if the provider emits them and `Capture.RawChunks` is enabled. |
| Log schema becomes dashboard contract too early | Keep v1 keys minimal, stable, and documented; add new keys additively. |
| Error messages may contain sensitive data | Bound and redact error message attributes; capture bodies/headers only behind explicit flags. |
| Per-part logging is expensive | Default `LogStreamParts=false`; summary-only stream logs remain one start and one terminal record. |
| Provider metadata may contain opaque sensitive data | Omit by default; require `Capture.ProviderMetadata`. |
| `GenerateText` logs as stream, surprising users | Document the current orchestration path and scope the logger to provider calls. |
| Upstream telemetry parity confusion | State that upstream devtools/telemetry are references only; run parity baseline validation but do not change conformance fixtures unless provider/wire behavior changes. |

## Migration Plan

This is additive only.

1. Add the `middleware/logger` nested module, API, tests, and package docs.
2. Add user-facing docs under `docs/guides/` and link from `docs/guides/middleware.md`.
3. Consumers opt in by importing `github.com/grafana/ai-sdk/middleware/logger` and wrapping a model directly or registering the middleware with `registry.WithLanguageModelMiddleware`.
4. Rollback is deleting the new module/docs or removing the logger wrapper from consumers; root ai-sdk consumers are unaffected because the root module never imports `middleware/logger`.

## Open Questions

- Should a future telemetry package reuse the `EventKind` strings or define separate operation-level event names? This proposal keeps the logger provider-layer only.
- Should a future version add `EventStreamAbort` to distinguish cancellation from provider errors? V1 logs cancellation as `EventStreamError` with `success=false` to match the approved event set and avoid expanding the schema prematurely.
- Should tool names ever be logged by default? V1 logs only counts by default; applications can capture tool definitions explicitly if they consider names safe.
