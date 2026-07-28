# Prometheus Middleware

## Purpose

Define the dependency-isolated Prometheus middleware module for provider-level ai-sdk model metrics.

## Requirements

### Requirement: Nested Go module for Prometheus middleware

`middleware/prometheus/` SHALL be a separate Go module under the ai-sdk repository, declared with `module github.com/grafana/ai-sdk/middleware/prometheus` and `replace github.com/grafana/ai-sdk => ../../`, following the existing nested middleware module convention.

The module SHALL depend on the root `github.com/grafana/ai-sdk` module and the Prometheus Go client. The root ai-sdk module SHALL NOT import `middleware/prometheus` and SHALL NOT gain a dependency on `github.com/prometheus/client_golang`.

The module documentation SHALL describe that the middleware records local/client-side provider-call metrics and does not configure Grafana hosted server-side metrics controls.

#### Scenario: Root module dependency isolation

- **WHEN** a consumer imports only `github.com/grafana/ai-sdk` or root module tests list dependencies from the repository root
- **THEN** `github.com/prometheus/client_golang` SHALL NOT appear in the root module dependency graph

#### Scenario: Nested module path

- **WHEN** `middleware/prometheus/go.mod` is inspected
- **THEN** it SHALL declare module path `github.com/grafana/ai-sdk/middleware/prometheus`
- **AND** it SHALL replace `github.com/grafana/ai-sdk` with `../../`

#### Scenario: Grafana hosted metrics remain independent

- **WHEN** documentation describes Prometheus middleware with `providers/grafana.GrafanaOptions.Metrics`
- **THEN** it SHALL state that Prometheus middleware measures local client-side provider calls
- **AND** it SHALL state that Grafana provider options control hosted server-side middleware independently

### Requirement: Public API surface

The `middleware/prometheus` package SHALL expose direct middleware constructors and wrapping helpers. Non-`Must` APIs that construct collectors SHALL return registration errors because Prometheus registration can fail.

The package SHALL export the following symbols:

- `type IdentitySource string`
- `const IdentityPreferResponse IdentitySource = "prefer_response"`
- `const IdentityRequested IdentitySource = "requested"`
- `type Options struct`
- `func Middleware(opts Options) (middleware.Middleware, error)`
- `func MustMiddleware(opts Options) middleware.Middleware`
- `func Wrap(base provider.LanguageModel, opts Options) (provider.LanguageModel, error)`
- `func MustWrap(base provider.LanguageModel, opts Options) provider.LanguageModel`

The package SHALL NOT expose a separate `Instrumentation`, `New`, or `MustNew` API in the initial version.

`Options` SHALL include these fields: `Registerer promclient.Registerer`, `ConstLabels promclient.Labels`, `IdentitySource IdentitySource`, `NormalizeProvider func(provider string) string`, `NormalizeModel func(provider, model string) string`, `DurationBuckets []float64`, `TimeToFirstOutputBuckets []float64`, `InterChunkDelayBuckets []float64`, and `DisableStreamChunkMetrics bool`.

#### Scenario: Middleware returns a reusable language model middleware

- **WHEN** `prometheus.Middleware(opts)` succeeds
- **THEN** it SHALL return a `middleware.Middleware` with generate and stream instrumentation behavior
- **AND** calls through models wrapped with that middleware SHALL be instrumented by the registered collectors

#### Scenario: Direct wrapping API

- **WHEN** `prometheus.Wrap(base, opts)` succeeds
- **THEN** the returned value SHALL satisfy `provider.LanguageModel`
- **AND** calls to the returned model SHALL be instrumented by the registered collectors
- **AND** observable wrapping behavior SHALL match constructing middleware with `prometheus.Middleware(opts)` and applying it to `base` through the root middleware wrapping helper

#### Scenario: Registry middleware API

- **WHEN** a caller passes a middleware returned by `prometheus.Middleware(opts)` to `registry.WithLanguageModelMiddleware`
- **THEN** every registry-resolved model wrapped by that option SHALL be instrumented without requiring registry API changes

#### Scenario: Must helpers panic on registration errors

- **WHEN** `MustMiddleware(opts)` or `MustWrap(base, opts)` encounters a collector registration error
- **THEN** the helper SHALL panic with that error

### Requirement: Collector registration behavior

`Middleware` SHALL construct all enabled collectors and register them exactly once with `Options.Registerer`. If `Options.Registerer` is nil, `Middleware` SHALL use the Prometheus default registerer. `Middleware` and `Wrap` SHALL return registration errors to the caller, including duplicate collector registration, and SHALL NOT use `promauto` internally.

`MustMiddleware` and `MustWrap` SHALL panic when collector registration fails. Callers SHALL be able to construct one middleware per process/registerer and reuse the returned `middleware.Middleware` concurrently for direct wrapping or registry-wide wrapping.

`ConstLabels` SHALL be attached to every collector at registration time and SHALL be documented as process-level labels only.

#### Scenario: Registers enabled collectors against custom registry

- **WHEN** `Middleware` is called with a custom Prometheus registry
- **THEN** all enabled `aisdk_model_*` collectors SHALL be registered with that registry
- **AND** no collector SHALL be registered with the default registry for that middleware

#### Scenario: Duplicate registration returns error

- **WHEN** `Middleware` is called twice with equivalent collector definitions against the same registry
- **THEN** the second call SHALL return a non-nil registration error
- **AND** it SHALL NOT panic

#### Scenario: Must helpers panic on registration error

- **WHEN** `MustMiddleware` or `MustWrap` encounters a registration error
- **THEN** it SHALL panic with that error

### Requirement: Metric family contract

The module SHALL define the following Prometheus metric families using OpenMetrics naming and base units:

- `aisdk_model_requests_total`: counter with labels `operation`, `provider`, `model`, `status`, `error_type`, `status_code`, and `finish_reason`.
- `aisdk_model_inflight_requests`: gauge with labels `operation`, `provider`, and `model`.
- `aisdk_model_request_duration_seconds`: histogram with labels `operation`, `provider`, `model`, and `status`.
- `aisdk_model_tokens_total`: counter with labels `operation`, `provider`, `model`, and `token_type`.
- `aisdk_model_time_to_first_output_seconds`: histogram with labels `operation`, `provider`, `model`, and `status`.
- `aisdk_model_stream_chunks_total`: counter with labels `operation`, `provider`, `model`, and `chunk_type`.
- `aisdk_model_inter_chunk_delay_seconds`: histogram with labels `operation`, `provider`, `model`, and `chunk_type`.

Default histogram buckets SHALL be seconds-based. Request duration default buckets SHALL be `0.025`, `0.05`, `0.1`, `0.25`, `0.5`, `1`, `2.5`, `5`, `10`, `30`, `60`, `120`, and `300`. Time-to-first-output default buckets SHALL be `0.01`, `0.025`, `0.05`, `0.1`, `0.25`, `0.5`, `1`, `2.5`, `5`, `10`, and `30`. Inter-chunk-delay default buckets SHALL be `0.001`, `0.005`, `0.01`, `0.025`, `0.05`, `0.1`, `0.25`, `0.5`, `1`, `2.5`, and `5`.

#### Scenario: Metric descriptors expose expected labels

- **WHEN** collectors are registered and gathered from a Prometheus registry
- **THEN** each `aisdk_model_*` metric family SHALL expose exactly the labels defined for that family, plus any configured const labels

#### Scenario: Bucket overrides are honored

- **WHEN** a caller supplies non-empty duration, time-to-first-output, or inter-chunk-delay bucket overrides in `Options`
- **THEN** the corresponding histogram SHALL use those buckets instead of the package defaults

### Requirement: Generate call instrumentation

The middleware SHALL instrument `WrapGenerate` around the inner `DoGenerate` closure. It SHALL increment the in-flight gauge before calling the inner model and decrement the same label set when the call returns. It SHALL observe request duration from immediately before the inner call until return.

On success, the middleware SHALL increment `aisdk_model_requests_total` once with `operation="generate"`, `status="success"`, `error_type="none"`, `status_code="none"`, and the unified finish reason from `GenerateResult.FinishReason`. It SHALL observe duration with the final operation, provider, model, and status labels, and SHALL observe positive token usage from `GenerateResult.Usage`. It SHALL return the original result pointer unchanged.

On error, the middleware SHALL classify the error using the bounded error vocabulary, increment the request counter once, observe duration, decrement in-flight, and return the original error unchanged.

#### Scenario: Generate success records one request

- **WHEN** the inner model's `DoGenerate` returns a successful `provider.GenerateResult`
- **THEN** `aisdk_model_requests_total` SHALL increment exactly once for `operation="generate"` and `status="success"`
- **AND** `aisdk_model_request_duration_seconds` SHALL observe exactly one sample for the final operation, provider, model, and status labels
- **AND** the returned result SHALL be the same pointer returned by the inner model

#### Scenario: Generate error records one failed request

- **WHEN** the inner model's `DoGenerate` returns a non-nil error
- **THEN** `aisdk_model_requests_total` SHALL increment exactly once for `operation="generate"` with status derived from that error
- **AND** the same error SHALL be returned to the caller

#### Scenario: Generate in-flight gauge is balanced

- **WHEN** a generate call succeeds or fails
- **THEN** `aisdk_model_inflight_requests` SHALL be decremented for the same requested provider/model label set that was incremented before the inner call

### Requirement: Stream call instrumentation and tee behavior

The middleware SHALL instrument `WrapStream` around the inner `DoStream` closure. It SHALL increment the in-flight gauge before calling the inner model. If `DoStream` returns an error before a stream is available, the middleware SHALL record the failed request like a generate error, decrement in-flight, and return the original error unchanged.

When `DoStream` returns a stream result, the middleware SHALL return a new `provider.StreamResult` that preserves the upstream request and response metadata and exposes a tee stream. The tee SHALL forward every upstream `provider.StreamPart` unchanged and in order, observe stream metrics, and close the downstream channel exactly once. The tee SHALL finalize request, duration, usage, TTFT, and in-flight metrics when the upstream stream closes or the request context is canceled. If context cancellation prevents downstream sends, the tee SHALL stop sending and continue draining upstream on a best-effort basis.

A streamed `provider.PartError` SHALL make the final request status `error` unless the context was canceled first. The error part SHALL still be forwarded unchanged.

#### Scenario: Stream success forwards exact parts

- **WHEN** the inner model returns a stream that emits N parts and closes normally
- **THEN** the consumer SHALL receive the same N `provider.StreamPart` values in the same order
- **AND** the middleware SHALL record exactly one successful stream request when the upstream stream closes

#### Scenario: Initial stream error is returned unchanged

- **WHEN** the inner model's `DoStream` returns a non-nil error before returning a stream
- **THEN** the middleware SHALL record exactly one failed stream request
- **AND** it SHALL return the same error to the caller

#### Scenario: Stream part error records failed request

- **WHEN** the upstream stream emits a `provider.StreamPart{Type: provider.PartError}` and then closes
- **THEN** the error part SHALL be forwarded to the consumer unchanged
- **AND** the final request metrics SHALL use `status="error"`

#### Scenario: Stream in-flight gauge is balanced

- **WHEN** the stream closes or the request context is canceled
- **THEN** `aisdk_model_inflight_requests` SHALL be decremented for the same requested provider/model label set that was incremented before the inner call

### Requirement: Provider and model identity labels

The middleware SHALL support two provider/model identity modes. `IdentityPreferResponse` SHALL be the default and SHALL select response provider/model metadata for final metrics when both provider and model are available; otherwise it SHALL fall back to the requested model identity from `p.Model.Provider()` and `p.Model.ModelID()`. `IdentityRequested` SHALL always use the requested model identity.

For generate calls, response identity SHALL come from `GenerateResult.Response.Provider` and `GenerateResult.Response.ModelID`. For stream calls, response identity SHALL come from observed `provider.PartResponseMeta` stream parts. `NormalizeProvider` and `NormalizeModel` SHALL be applied consistently after identity selection for every metric that carries provider/model labels.

The in-flight gauge SHALL always use the requested identity captured at call start for both increment and decrement.

#### Scenario: Generate response identity overrides requested identity by default

- **GIVEN** a wrapped model whose requested provider is `grafana`
- **WHEN** `DoGenerate` succeeds with `GenerateResult.Response.Provider` equal to `anthropic` and a non-empty response model ID
- **THEN** final request, duration, and token metrics SHALL use provider label `anthropic`

#### Scenario: Requested identity mode ignores response identity

- **GIVEN** instrumentation configured with `IdentitySource` equal to `IdentityRequested`
- **WHEN** a generate or stream response supplies different response provider/model metadata
- **THEN** all final metrics SHALL use the requested model identity

#### Scenario: Normalizers apply to all provider model labels

- **WHEN** `NormalizeProvider` or `NormalizeModel` is configured
- **THEN** every emitted metric label named `provider` or `model` SHALL use the normalized value

### Requirement: Bounded status and error classification

Metric status labels SHALL be limited to `success`, `error`, and `canceled`. Metric error type labels SHALL be limited to `none`, `api_call_error`, `context_canceled`, `context_deadline_exceeded`, `provider_stream_error`, and `other`. Metric status code labels SHALL be the HTTP status code from `*provider.APICallError` as a decimal string, or `none` when unavailable.

The classifier SHALL treat `context.Canceled` as `status="canceled"` and `error_type="context_canceled"`. It SHALL treat `context.DeadlineExceeded` as `status="canceled"` and `error_type="context_deadline_exceeded"`. It SHALL treat errors matching `*provider.APICallError` as `status="error"` and `error_type="api_call_error"`. It SHALL treat stream error parts without an API call error as `status="error"` and `error_type="provider_stream_error"`. All other errors SHALL use `status="error"` and `error_type="other"`.

Finish reason labels SHALL use the unified finish reason values `stop`, `length`, `content-filter`, `tool-calls`, `error`, or `other`, and SHALL use `none` when unavailable.

#### Scenario: API call error status code is recorded

- **WHEN** a generate or stream call fails with a `*provider.APICallError` whose `StatusCode` is `429`
- **THEN** final request metrics SHALL use `error_type="api_call_error"`
- **AND** final request metrics SHALL use `status_code="429"`
- **AND** duration metrics SHALL use `status="error"` without `error_type` or `status_code` labels

#### Scenario: Context cancellation is canceled status

- **WHEN** a call fails because the context is canceled
- **THEN** final request and duration metrics SHALL use `status="canceled"`
- **AND** final request metrics SHALL use either `error_type="context_canceled"` or `error_type="context_deadline_exceeded"` according to the cancellation cause
- **AND** duration metrics SHALL NOT include an `error_type` label

#### Scenario: Error messages are not labels

- **WHEN** an error contains a URL, header value, response body, or detailed message
- **THEN** no metric label SHALL contain those values

### Requirement: Token usage metrics

The middleware SHALL increment `aisdk_model_tokens_total` only for positive token counts present in provider usage. Generate usage SHALL come from `provider.GenerateResult.Usage`. Stream usage SHALL come from the final observed `provider.PartFinish.Usage`.

Token type labels SHALL be limited to:

- `input` from `Usage.InputTokens.Total`
- `input_no_cache` from `Usage.InputTokens.NoCache`
- `input_cache_read` from `Usage.InputTokens.CacheRead`
- `input_cache_write` from `Usage.InputTokens.CacheWrite`
- `output` from `Usage.OutputTokens.Total`
- `output_text` from `Usage.OutputTokens.Text`
- `output_reasoning` from `Usage.OutputTokens.Reasoning`

The middleware SHALL NOT expose `Usage.Raw` as a metric label.

#### Scenario: Positive usage increments fixed token types

- **WHEN** a successful generate or stream call reports positive input and output usage fields
- **THEN** `aisdk_model_tokens_total` SHALL increment for the corresponding fixed `token_type` labels

#### Scenario: Missing or zero usage is ignored

- **WHEN** a usage field is nil, zero, or negative
- **THEN** the middleware SHALL NOT increment `aisdk_model_tokens_total` for that field

### Requirement: Stream chunk and timing metrics

For streams, the middleware SHALL increment `aisdk_model_stream_chunks_total` for every upstream `provider.StreamPart` observed and forwarded, with `chunk_type` equal to the exact `provider.StreamPartType` string. This metric SHALL be disabled when `Options.DisableStreamChunkMetrics` is true.

The middleware SHALL observe `aisdk_model_time_to_first_output_seconds` once per stream when at least one payload-bearing stream part is observed. The observation SHALL use the elapsed seconds between starting the provider call and observing the first payload-bearing part, and SHALL be emitted at stream finalization with the final stream status label. Streams that finish, error, or cancel before any payload-bearing part SHALL NOT observe TTFT.

The middleware SHALL observe `aisdk_model_inter_chunk_delay_seconds` for gaps between consecutive payload-bearing stream parts, labeled by the current payload-bearing part type. This metric SHALL be disabled when `Options.DisableStreamChunkMetrics` is true.

Payload-bearing stream part types SHALL be `text-delta`, `reasoning-delta`, `tool-input-delta`, `tool-call`, `tool-result`, `source`, `file`, `custom`, `reasoning-file`, and `tool-approval-request`. Framing, metadata, raw, finish, and error parts SHALL NOT be payload-bearing.

#### Scenario: Chunk counter records all parts

- **WHEN** a stream emits `text-delta`, `response-metadata`, `finish`, and `error` parts
- **THEN** `aisdk_model_stream_chunks_total` SHALL increment once for each exact chunk type observed

#### Scenario: TTFT records first payload only

- **WHEN** a stream emits metadata parts before its first `text-delta`
- **THEN** TTFT SHALL be measured to the first `text-delta`
- **AND** metadata parts before it SHALL NOT cause a TTFT observation

#### Scenario: Inter-chunk delay records payload gaps

- **WHEN** a stream emits two consecutive payload-bearing parts with non-zero elapsed time between them
- **THEN** `aisdk_model_inter_chunk_delay_seconds` SHALL observe that elapsed time labeled by the second payload-bearing part type

#### Scenario: Stream chunk metrics can be disabled

- **WHEN** instrumentation is configured with `DisableStreamChunkMetrics` true
- **THEN** stream request, duration, in-flight, token, and TTFT metrics SHALL still be recorded
- **AND** `aisdk_model_stream_chunks_total` and `aisdk_model_inter_chunk_delay_seconds` SHALL NOT be registered or observed

### Requirement: Privacy and cardinality guardrails

Default metric labels SHALL be bounded and content-free. The middleware SHALL NOT label metrics with prompt text, output text, reasoning text, tool arguments, tool results, user IDs, tenant IDs, session IDs, request IDs, response IDs, generation IDs, HTTP headers, URLs, response bodies, raw request bodies, raw provider metadata, error messages, Go error type strings, source URLs, filenames, tool names, tool call IDs, or arbitrary per-request custom labels.

Provider and model labels SHALL be the only potentially user-controlled high-cardinality labels, and the package SHALL provide normalization hooks so callers can bucket or redact those values.

#### Scenario: Prompt and output content are absent from labels

- **WHEN** a model call contains unique prompt text and produces unique output text
- **THEN** no metric label SHALL contain either string

#### Scenario: Request and response metadata are absent from labels

- **WHEN** a provider result contains response IDs, request bodies, response headers, URLs, or raw provider metadata
- **THEN** no metric label SHALL contain those values

### Requirement: Documentation and validation coverage

The module SHALL include package documentation describing the public API, metric contract, default buckets, provider-call scope, stream finalization behavior, privacy/cardinality guardrails, Agent Observability composition ordering, registry integration, and Grafana hosted metrics independence.

The implementation SHALL include tests using `prometheus.NewRegistry()` and `prometheus/testutil` that cover collector registration, duplicate registration, generate success/error/cancellation, stream success/error/cancellation, response identity preference, requested identity mode, normalizers, stream chunk/timing metrics, disabled stream chunk metrics, registry integration, privacy label exclusions, and root dependency isolation.

The repository task configuration SHALL include a targeted Prometheus middleware test task and include the nested module in aggregate test, short-test, vet, tidy, and build tasks.

#### Scenario: Package docs cover composition order

- **WHEN** a user reads the `middleware/prometheus` package documentation
- **THEN** it SHALL explain how to compose Prometheus with Agent Observability when measuring provider calls closest to the provider
- **AND** it SHALL explain that putting Prometheus outside Agent Observability measures a broader wrapped-model operation

#### Scenario: Registry integration is tested

- **WHEN** a model is resolved through a registry configured with a middleware returned by `prometheus.Middleware(opts)`
- **THEN** provider calls through that model SHALL emit Prometheus metrics

#### Scenario: Aggregate tasks include nested module

- **WHEN** contributors run aggregate test, short-test, vet, tidy, or build tasks after implementation
- **THEN** those tasks SHALL include `middleware/prometheus` alongside existing nested modules
