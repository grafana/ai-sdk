# Prometheus Middleware

## Purpose

Define the dependency-isolated Prometheus middleware module for provider-level ai-sdk model metrics.
## Requirements
### Requirement: Nested Go module for Prometheus middleware

`middleware/prometheus/` SHALL be a separate module: `module github.com/grafana/ai-sdk/middleware/prometheus`, `replace github.com/grafana/ai-sdk => ../../`, following nested middleware conventions. It SHALL depend on root ai-sdk and the Prometheus Go client. Root ai-sdk SHALL NOT import it or gain `github.com/prometheus/client_golang`. Docs SHALL state that these local/client-side provider-call metrics do not configure remote hosted-service metrics controls.

#### Scenario: Root module dependency isolation

- **WHEN** a consumer imports only `github.com/grafana/ai-sdk` or root module tests list dependencies from the repository root
- **THEN** `github.com/prometheus/client_golang` SHALL NOT appear in the root module dependency graph

#### Scenario: Nested module path

- **WHEN** `middleware/prometheus/go.mod` is inspected
- **THEN** it SHALL declare module path `github.com/grafana/ai-sdk/middleware/prometheus`
- **AND** it SHALL replace `github.com/grafana/ai-sdk` with `../../`

#### Scenario: Hosted metrics controls remain independent

- **WHEN** documentation describes Prometheus middleware alongside remote hosted-service controls
- **THEN** it SHALL state that Prometheus middleware measures local client-side provider calls
- **AND** it SHALL NOT require option types from a removed provider module

### Requirement: Public API surface

`middleware/prometheus` SHALL expose middleware constructors and wrappers. Non-`Must` collector constructors SHALL return registration errors. The initial API SHALL NOT expose separate `Instrumentation`, `New`, or `MustNew` APIs. It SHALL export `type IdentitySource string`, `const IdentityPreferResponse IdentitySource = "prefer_response"`, `const IdentityRequested IdentitySource = "requested"`, and `type Options struct`.

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

#### Scenario: Call direct constructors and wrappers
- **WHEN** callers construct middleware or wrap models directly
- **THEN** the package SHALL export `func Middleware(opts Options) (middleware.Middleware, error)` and `func MustMiddleware(opts Options) middleware.Middleware`
- **AND** it SHALL export `func Wrap(base provider.LanguageModel, opts Options) (provider.LanguageModel, error)` and `func MustWrap(base provider.LanguageModel, opts Options) provider.LanguageModel`

### Requirement: Collector registration behavior

`Middleware` SHALL construct all enabled collectors and register each once with `Options.Registerer`, using Prometheus's default registerer if nil. `Middleware`/`Wrap` SHALL return registration errors, including duplicates, without internal `promauto`; `MustMiddleware`/`MustWrap` SHALL panic on failure. One middleware per process/registerer SHALL support concurrent direct/registry-wide reuse. `ConstLabels` SHALL attach to every collector at registration and be documented as process-level only.

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

Metric families SHALL use OpenMetrics naming and base units with exactly their specified labels plus configured const labels. Default histogram buckets SHALL be seconds-based. Non-empty duration, time-to-first-output, and inter-chunk-delay bucket overrides SHALL replace their respective defaults.

#### Scenario: Metric descriptors expose expected labels

- **WHEN** collectors are registered and gathered from a Prometheus registry
- **THEN** each `aisdk_model_*` metric family SHALL expose exactly the labels defined for that family, plus any configured const labels

#### Scenario: Bucket overrides are honored

- **WHEN** a caller supplies non-empty duration, time-to-first-output, or inter-chunk-delay bucket overrides in `Options`
- **THEN** the corresponding histogram SHALL use those buckets instead of the package defaults

#### Scenario: Request and usage families expose fixed descriptors
- **WHEN** request, in-flight, duration, and token collectors are constructed
- **THEN** `aisdk_model_requests_total` SHALL be a counter with labels `operation`, `provider`, `model`, `status`, `error_type`, `status_code`, and `finish_reason`
- **AND** `aisdk_model_inflight_requests` SHALL be a gauge with labels `operation`, `provider`, and `model`
- **AND** `aisdk_model_request_duration_seconds` SHALL be a histogram with labels `operation`, `provider`, `model`, and `status`
- **AND** `aisdk_model_tokens_total` SHALL be a counter with labels `operation`, `provider`, `model`, and `token_type`

#### Scenario: Stream families expose fixed descriptors
- **WHEN** stream timing and chunk collectors are constructed
- **THEN** `aisdk_model_time_to_first_output_seconds` SHALL be a histogram with labels `operation`, `provider`, `model`, and `status`
- **AND** `aisdk_model_stream_chunks_total` SHALL be a counter with labels `operation`, `provider`, `model`, and `chunk_type`
- **AND** `aisdk_model_inter_chunk_delay_seconds` SHALL be a histogram with labels `operation`, `provider`, `model`, and `chunk_type`

### Requirement: Generate call instrumentation

Middleware SHALL instrument `WrapGenerate` around `DoGenerate`: increment in-flight before invocation and decrement the same labels on return; duration SHALL run from immediately before invocation to return. Each call SHALL increment the request counter once and observe duration with final operation/provider/model/status labels. Success SHALL observe positive `GenerateResult.Usage` and return the original result pointer; errors SHALL use bounded classification and return the original error.

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

#### Scenario: Successful generate sets neutral error labels and finish reason
- **WHEN** the inner `DoGenerate` succeeds
- **THEN** its request counter SHALL use `operation="generate"`, `status="success"`, `error_type="none"`, `status_code="none"`, and unified `GenerateResult.FinishReason`
- **AND** duration SHALL use final operation, provider, model, and status labels
- **AND** positive `GenerateResult.Usage` SHALL be observed

### Requirement: Stream call instrumentation and tee behavior

`WrapStream` SHALL increment in-flight before inner `DoStream`. Opening errors SHALL record failure like generate errors, decrement in-flight, and return unchanged. A result SHALL be wrapped in a new `provider.StreamResult` preserving request/response metadata, with a tee forwarding every part unchanged in order, observing metrics, and closing downstream once. Request/duration/usage/TTFT/in-flight SHALL finalize on upstream close or context cancellation.

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

#### Scenario: Cancellation stops blocked downstream sends
- **WHEN** context cancellation prevents downstream sends
- **THEN** the tee SHALL stop sending and continue draining upstream on a best-effort basis

#### Scenario: Stream error classification respects earlier cancellation
- **WHEN** an upstream `provider.PartError` is observed
- **THEN** final request status SHALL be `error` unless the context was canceled first
- **AND** the error part SHALL still be forwarded unchanged

### Requirement: Provider and model identity labels

Default `IdentityPreferResponse` SHALL select response metadata for final metrics only when both provider and model are available; otherwise use requested `p.Model.Provider()` / `p.Model.ModelID()`. `IdentityRequested` SHALL always use requested identity. `NormalizeProvider` / `NormalizeModel` SHALL apply consistently after selection to every provider/model metric. In-flight increment/decrement SHALL use requested identity captured at start.

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

#### Scenario: Response identity comes from provider metadata
- **WHEN** response-preferred instrumentation observes a generate result or stream response metadata
- **THEN** generate identity SHALL come from `GenerateResult.Response.Provider` and `GenerateResult.Response.ModelID`
- **AND** stream identity SHALL come from observed `provider.PartResponseMeta` stream parts

### Requirement: Bounded status and error classification

Status labels SHALL be `success`, `error`, or `canceled`. Error type labels SHALL be `none`, `api_call_error`, `context_canceled`, `context_deadline_exceeded`, `provider_stream_error`, or `other`. Status code SHALL be the decimal HTTP status from `*provider.APICallError`, or `none` if unavailable. Finish reason SHALL be unified `stop`, `length`, `content-filter`, `tool-calls`, `error`, or `other`, or `none` if unavailable.

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

#### Scenario: Errors map to the bounded vocabulary
- **WHEN** an error is classified for request metrics
- **THEN** `context.Canceled` SHALL map to `status="canceled"`, `error_type="context_canceled"`
- **AND** `context.DeadlineExceeded` SHALL map to `status="canceled"`, `error_type="context_deadline_exceeded"`
- **AND** errors matching `*provider.APICallError` SHALL map to `status="error"`, `error_type="api_call_error"`
- **AND** stream error parts without an API call error SHALL map to `status="error"`, `error_type="provider_stream_error"`
- **AND** all other errors SHALL map to `status="error"`, `error_type="other"`

### Requirement: Token usage metrics

`aisdk_model_tokens_total` SHALL increment only for positive provider usage counts. Generate SHALL use `provider.GenerateResult.Usage`; stream SHALL observe every part with non-nil `Usage` and use shared aggregation, independently retaining greatest normalized values. `Usage.Raw` SHALL NOT be a metric label. Token types SHALL use only the specified normalized field mappings.

#### Scenario: Stream usage preserves strongest values

- **GIVEN** usage is split across multiple stream parts
- **AND** a later finish part omits or reports lower provisional normalized counters
- **WHEN** the stream completes
- **THEN** token counters SHALL increment from the independently aggregated strongest normalized values

#### Scenario: Positive usage increments fixed token types

- **WHEN** a successful generate or stream call reports positive input and output usage fields
- **THEN** `aisdk_model_tokens_total` SHALL increment for the corresponding fixed `token_type` labels

#### Scenario: Missing or zero usage is ignored

- **WHEN** a usage field is nil, zero, or negative
- **THEN** the middleware SHALL NOT increment `aisdk_model_tokens_total` for that field

#### Scenario: Normalized usage fields select fixed token labels
- **WHEN** positive normalized token fields are recorded
- **THEN** `token_type` SHALL be limited to `input` from `Usage.InputTokens.Total`, `input_no_cache` from `Usage.InputTokens.NoCache`, `input_cache_read` from `Usage.InputTokens.CacheRead`, and `input_cache_write` from `Usage.InputTokens.CacheWrite`
- **AND** output labels SHALL be `output` from `Usage.OutputTokens.Total`, `output_text` from `Usage.OutputTokens.Text`, and `output_reasoning` from `Usage.OutputTokens.Reasoning`

### Requirement: Stream chunk and timing metrics

`aisdk_model_stream_chunks_total` SHALL increment for every upstream part observed and forwarded, with registered `provider.StreamPartType` strings or `other` for unknowns. Inter-chunk delay SHALL observe gaps between consecutive payload-bearing parts, labeled by the current part type. Both chunk and inter-chunk metrics SHALL be disabled by `Options.DisableStreamChunkMetrics`; TTFT SHALL remain independent.

#### Scenario: Chunk counter records all parts

- **WHEN** a stream emits `text-delta`, `response-metadata`, `finish`, and `error` parts
- **THEN** `aisdk_model_stream_chunks_total` SHALL increment once for each known chunk type observed

#### Scenario: Unknown chunk types use one closed label

- **WHEN** a stream emits one or more unregistered `provider.StreamPartType` values
- **THEN** `aisdk_model_stream_chunks_total` SHALL bucket every such part under `chunk_type="other"`
- **AND** no unregistered value SHALL become a metric label

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

Default labels SHALL be bounded and content-free, excluding request/response content, identifying metadata, opaque errors, and arbitrary per-request labels. Provider/model SHALL be the only potentially user-controlled high-cardinality labels; normalization hooks SHALL let callers bucket or redact them.

#### Scenario: Prompt and output content are absent from labels

- **WHEN** a model call contains unique prompt text and produces unique output text
- **THEN** no metric label SHALL contain either string

#### Scenario: Request and response metadata are absent from labels

- **WHEN** a provider result contains response IDs, request bodies, response headers, URLs, or raw provider metadata
- **THEN** no metric label SHALL contain those values

#### Scenario: Content and identifiers cannot become metric labels
- **WHEN** a request, response, stream, or error includes content or identifying metadata
- **THEN** metrics SHALL NOT label prompt text, output text, reasoning text, tool arguments, tool results, user IDs, tenant IDs, session IDs, request IDs, response IDs, generation IDs, HTTP headers, URLs, response bodies, raw request bodies, raw provider metadata, error messages, Go error type strings, source URLs, filenames, tool names, tool call IDs, or arbitrary per-request custom labels

### Requirement: Documentation and validation coverage

Package docs SHALL describe public API, metric contract, default buckets, provider-call scope, stream finalization, privacy/cardinality, Agent Observability ordering, registry integration, and local metrics versus remote controls. Tests SHALL use `prometheus.NewRegistry()` and `prometheus/testutil`. Task configuration SHALL provide targeted Prometheus tests and include the nested module in aggregate test, short-test, vet, tidy, and build tasks.

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

#### Scenario: Registry-based tests cover lifecycle and privacy
- **WHEN** the Prometheus middleware test suite runs
- **THEN** tests using `prometheus.NewRegistry()` and `prometheus/testutil` SHALL cover collector registration, duplicate registration, generate success/error/cancellation, stream success/error/cancellation, response identity preference, requested identity mode, normalizers, stream chunk/timing metrics, disabled stream chunk metrics, registry integration, privacy label exclusions, and root dependency isolation

### Requirement: Configurable bounded stream drain
Prometheus middleware options SHALL provide an optional positive stream-drain duration. When configured, cancellation cleanup SHALL drain the immediate upstream only until channel close or the absolute deadline, including for a continuously ready channel. Zero SHALL preserve the existing direct-consumer drain behavior.

#### Scenario: Configured drain expires
- **WHEN** downstream cancellation occurs and the immediate upstream never closes or remains continuously ready
- **THEN** the Prometheus-owned drain goroutine SHALL exit no later than the configured absolute deadline
- **AND** terminal metrics, in-flight decrement, and downstream channel closure SHALL each occur exactly once without waiting for the drain

### Requirement: Prometheus collector configuration

`Options` SHALL configure registration, constant labels, identity selection/normalization, histogram buckets, and stream chunk metrics.

#### Scenario: Configure collector options
- **WHEN** a caller constructs Prometheus `Options`
- **THEN** fields SHALL include `Registerer promclient.Registerer`, `ConstLabels promclient.Labels`, `IdentitySource IdentitySource`, `NormalizeProvider func(provider string) string`, `NormalizeModel func(provider, model string) string`, `DurationBuckets []float64`, `TimeToFirstOutputBuckets []float64`, `InterChunkDelayBuckets []float64`, and `DisableStreamChunkMetrics bool`

### Requirement: Default Prometheus histogram buckets

Absent non-empty bucket overrides, request duration, time to first output, and inter-chunk delay histograms SHALL use their specified default seconds-based buckets.

#### Scenario: Default request duration buckets
- **WHEN** `DurationBuckets` is empty
- **THEN** request duration buckets SHALL be `0.025`, `0.05`, `0.1`, `0.25`, `0.5`, `1`, `2.5`, `5`, `10`, `30`, `60`, `120`, and `300`

#### Scenario: Default time to first output buckets
- **WHEN** `TimeToFirstOutputBuckets` is empty
- **THEN** time-to-first-output buckets SHALL be `0.01`, `0.025`, `0.05`, `0.1`, `0.25`, `0.5`, `1`, `2.5`, `5`, `10`, and `30`

#### Scenario: Default inter-chunk delay buckets
- **WHEN** `InterChunkDelayBuckets` is empty
- **THEN** inter-chunk-delay buckets SHALL be `0.001`, `0.005`, `0.01`, `0.025`, `0.05`, `0.1`, `0.25`, `0.5`, `1`, `2.5`, and `5`

### Requirement: Prometheus payload timing boundary

Payload-bearing parts SHALL be `text-delta`, `reasoning-delta`, `tool-input-delta`, `tool-call`, `tool-result`, `source`, `file`, `custom`, `reasoning-file`, and `tool-approval-request`, not framing, metadata, raw, finish, or error. TTFT SHALL be observed once for streams with payload, from provider-call start to first payload, emitted at finalization with final status. Finish/error/cancel before payload SHALL NOT observe TTFT.

#### Scenario: First payload determines TTFT
- **WHEN** a stream emits its first payload-bearing part after framing, metadata, or raw parts
- **THEN** `aisdk_model_time_to_first_output_seconds` SHALL measure elapsed seconds from provider-call start to that first payload
- **AND** exactly one observation SHALL be emitted at finalization with the final stream status label

#### Scenario: No payload means no TTFT sample
- **WHEN** a stream finishes, errors, or cancels before any payload-bearing part
- **THEN** it SHALL NOT observe TTFT
