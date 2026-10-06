## Context

The root `middleware` package already wraps any `provider.LanguageModel` through optional `WrapGenerate` and `WrapStream` hooks, with the first middleware in a slice running outermost (`middleware/middleware.go`). Registry integration already applies configured language-model middleware to every resolved model (`registry/registry.go`). The Sigil middleware establishes the repository pattern for dependency-heavy optional middleware as a nested Go module under `middleware/<name>/`, keeping vendor dependencies out of the root module (`middleware/sigil/doc.go`, `middleware/sigil/go.mod`).

Provider-call data needed for metrics is already available at the provider boundary. `provider.GenerateResult` carries finish reason, token usage, request metadata, and response provider/model metadata. Stream calls return `provider.StreamPart` values that include response metadata, finish usage/reason, and `*provider.APICallError` on error parts (`provider/language_model.go`, `provider/stream_part.go`, `provider/types.go`). `StreamText` calls `currentModel.DoStream` inside the retry loop, so this middleware will observe provider attempts rather than logical SDK operations (`streamtext.go`).

The registered upstream baseline is `ai@7.0.11`; upstream has lifecycle telemetry and OTel span attributes for model-call duration, time to first output, and output chunk timing, but no Prometheus metric contract. This change can borrow those concepts while defining Go/Prometheus-native names and labels without changing provider wire protocol, UI chunks, or conformance fixtures.

## Goals / Non-Goals

**Goals:**

- Add `github.com/grafana/ai-sdk/middleware/prometheus` as an opt-in nested module.
- Instrument provider-level `DoGenerate` and `DoStream` calls for direct wrapping and registry-wide wrapping.
- Keep Prometheus client dependencies out of the root module.
- Emit stable, low-cardinality Prometheus counters, gauges, and histograms for provider-call health and usage.
- Preserve stream parts exactly while observing stream metadata, payload timing, usage, finish reason, and provider stream errors.
- Prefer backend response identity when available for final metrics while keeping in-flight accounting balanced.
- Document privacy/cardinality guardrails, provider-call scope, Sigil composition, and Grafana hosted metrics independence.

**Non-Goals:**

- Do not add metrics code or Prometheus dependencies to the root `middleware` package.
- Do not create a generic telemetry framework, OpenTelemetry exporter, or logical `StreamText`/`GenerateText` operation metrics.
- Do not instrument local tool execution or orchestration callbacks.
- Do not modify provider wire protocol, SSE/UI stream format, conformance fixtures, or provider interfaces.
- Do not reuse or change `providers/grafana.GrafanaOptions.Metrics`, which controls hosted server-side middleware.

## Decisions

### Use `middleware/prometheus` as a nested module

The implementation will add `middleware/prometheus/go.mod` with module path `github.com/grafana/ai-sdk/middleware/prometheus` and `replace github.com/grafana/ai-sdk => ../../`, matching `middleware/sigil` and provider nested modules. This keeps `github.com/prometheus/client_golang` out of root `go.mod` and makes the import path short and explicit.

Alternative considered: `middleware/metrics/prometheus`. This was rejected because the repository currently uses `middleware/<name>/` for optional heavy middleware, and adding a two-level namespace would imply a generic metrics abstraction that is not part of this change.

### Use direct middleware constructor helpers

The package will expose `Middleware(opts Options) (middleware.Middleware, error)` and `Wrap(base provider.LanguageModel, opts Options) (provider.LanguageModel, error)` as the primary API, matching the repository's nested middleware convention while returning errors because Prometheus collector registration can fail. It will also expose `MustMiddleware(opts Options) middleware.Middleware` and `MustWrap(base provider.LanguageModel, opts Options) provider.LanguageModel` for examples/tests and applications that prefer panic-on-registration-failure behavior.

`Middleware` will register collectors explicitly against `Options.Registerer` or `prometheus.DefaultRegisterer` when nil, then return a reusable `middleware.Middleware` whose closures own those collectors. `Wrap` will be equivalent to constructing the middleware with `Middleware(opts)` and applying it with the root middleware wrapping helper. Users should call `Middleware` once per process/registerer and reuse the returned middleware for direct wrapping or registry-wide wrapping. The `Must*` helpers are the only APIs that panic on registration failure.

Alternative considered: expose an exported `Instrumentation` object with `New`, `MustNew`, `Middleware`, and `Wrap` methods. This was rejected because the nested module can keep reusable state behind the returned middleware while exposing the same direct constructor style as existing middleware modules. Alternative considered: use `promauto` or silently reuse `prometheus.AlreadyRegisteredError.ExistingCollector`. This was rejected because libraries should make duplicate registration visible and because safe reuse would require proving all collector definitions and const labels are compatible.

### Record provider-call metrics, not logical operation metrics

The middleware will wrap `DoGenerate` and `DoStream` through `middleware.Middleware`. SDK `GenerateText` currently uses streaming internally, and retries/fallback invoke provider calls inside orchestration loops. Each provider attempt will therefore produce its own observation. That behavior is useful for provider health dashboards and should be documented. Future core telemetry can add logical operation metrics separately.

### Use bounded metric labels and response identity for final metrics

Final request, duration, token, TTFT, stream chunk, and inter-chunk metrics will use selected provider/model labels. The default `IdentityPreferResponse` mode will use `GenerateResult.Response.Provider/ModelID` or `PartResponseMeta.Provider/ModelID` when both are present, falling back to the requested model identity. `IdentityRequested` will always use `p.Model.Provider()` and `p.Model.ModelID()`. `NormalizeProvider` and `NormalizeModel` hooks will run after identity selection for every collector that carries provider/model labels.

The in-flight gauge will increment and decrement using the requested identity captured before the inner provider call, even when final metrics switch to response identity. This avoids gauge leaks caused by label changes discovered at the end of a stream.

Alternative considered: always label by requested identity. This was rejected because gateway-style providers, including Grafana-hosted routes, can return backend provider/model metadata that is more useful for attribution. Alternative considered: change the in-flight gauge labels at finish time. This was rejected because decrementing a different label set than the increment would leave stale gauge values.

### Define Prometheus-native metric families

The module will define these collectors:

- `aisdk_model_requests_total` counter.
- `aisdk_model_inflight_requests` gauge.
- `aisdk_model_request_duration_seconds` histogram.
- `aisdk_model_tokens_total` counter.
- `aisdk_model_time_to_first_output_seconds` histogram.
- `aisdk_model_stream_chunks_total` counter.
- `aisdk_model_inter_chunk_delay_seconds` histogram.

Default histogram buckets will be seconds-based and overrideable through `Options`. Labels will be limited to operation, provider, model, status, bounded error type, HTTP status code, unified finish reason, token type, and stream chunk type. The implementation must never use prompts, outputs, tool arguments/results, request or response IDs, headers, URLs, response bodies, raw provider metadata, error messages, Go error type strings, user IDs, tenant IDs, or tool names as metric labels.

### Classify errors with a fixed vocabulary

Returned errors and stream `PartError` values will be classified into a bounded label set: `none`, `api_call_error`, `context_canceled`, `context_deadline_exceeded`, `provider_stream_error`, and `other`. `*provider.APICallError.StatusCode` is the only error detail allowed as a label, represented as a decimal string or `none`. Context cancellation/deadline wins over stream provider errors when determining final status.

### Tee streams without mutation and finalize at stream completion

For successful `DoStream` returns, the middleware will return a new `provider.StreamResult` with the same request/response metadata and a tee channel buffered at 64, matching the Sigil stream recording precedent. The tee goroutine will observe each upstream `provider.StreamPart`, forward it unchanged, and close the downstream channel exactly once. If context cancellation prevents downstream sends, it will stop sending and continue draining upstream so the provider goroutine can exit.

Stream request/duration/token metrics finalize when upstream closes or context cancellation stops the tee. TTFT will be stored when the first payload-bearing part is observed and emitted at finalization with the final status label. Inter-chunk delay will measure gaps between consecutive payload-bearing parts. `DisableStreamChunkMetrics` will suppress the chunk counter and inter-chunk histogram, but request, duration, usage, in-flight, and TTFT metrics remain enabled.

Payload-bearing parts are `PartTextDelta`, `PartReasoningDelta`, `PartToolInputDelta`, `PartToolCall`, `PartToolResult`, `PartSource`, `PartFile`, `PartCustom`, `PartReasoningFile`, and `PartToolApprovalRequest`. Framing/metadata/raw/finish/error parts are not payload-bearing.

### Integrate with docs and aggregate tasks

The module docs will show direct wrapping, registry-wide wrapping via `registry.WithLanguageModelMiddleware`, and composition with Sigil. For provider-call metrics after Sigil hooks/recording, Prometheus should be innermost/closest to the provider. Users who want to count broader wrapped-model attempts, including hook denials, can put Prometheus outside Sigil.

`mise.toml` should gain a targeted Prometheus middleware test task and include the module in aggregate test, short-test, vet, tidy, and build tasks when the implementation lands.

## Risks / Trade-offs

- Provider-call observations can surprise users expecting one metric per public SDK call → Document that retries, fallback, and multi-step tool loops can create multiple provider-call observations.
- Stream metrics finalize only when streams are drained or contexts are canceled → Document the drain/cancel requirement and follow the existing Sigil tee/drain pattern.
- Duplicate registration errors require users to reuse the constructed middleware → Return errors from `Middleware`, provide `MustMiddleware`/`MustWrap` only for examples/tests or panic-preferred applications, and document one middleware construction per registry.
- Provider/model labels can be high-cardinality → Provide normalization hooks and omit per-request custom labels from the initial API.
- Inter-chunk histograms can be high-volume → Keep labels bounded and provide `DisableStreamChunkMetrics`.
- Response identity arrives after the in-flight gauge increment → Use requested identity for the gauge and response identity only for final metrics.

## Migration Plan

- Implement as a new opt-in module with no changes to existing imports or provider interfaces.
- Add focused unit tests under `middleware/prometheus` using `prometheus.NewRegistry()` and `prometheus/testutil`.
- Update aggregate `mise` tasks to include the new nested module.
- Verify root dependency isolation by confirming root `go.mod` is unchanged and root dependency listing does not include `github.com/prometheus/client_golang`.
- Rollback is deleting the nested module and its aggregate task entries; no persisted data or wire migration is involved.

## Open Questions

- None blocking for implementation. The proposal chooses default `prometheus.DefaultRegisterer` for nil registerers, returns duplicate registration errors rather than reusing collectors, defaults to response identity for final metrics, and delays TTFT observation until stream finalization so status labels are available.
