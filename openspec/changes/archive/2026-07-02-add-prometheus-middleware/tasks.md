## 1. Module Setup

- [x] 1.1 Create `middleware/prometheus` as a nested Go module with `go.mod`, root-module replace directive, package docs, and no changes to root `go.mod`.
- [x] 1.2 Add Prometheus client and test dependencies only to the nested module.
- [x] 1.3 Add a targeted `test-prometheus-middleware` task and include `middleware/prometheus` in aggregate test, short-test, vet, tidy, and build tasks.

## 2. Public API and Collector Registration

- [x] 2.1 Define `IdentitySource`, `Options`, unexported collector state, default histogram buckets, and normalizer/identity helper functions.
- [x] 2.2 Implement `Middleware`, `MustMiddleware`, `Wrap`, and `MustWrap` using explicit collector registration and direct middleware constructor semantics.
- [x] 2.3 Implement collector construction for all `aisdk_model_*` metric families, const-label support, bucket overrides, duplicate-registration error propagation, and optional omission of disabled stream chunk collectors.

## 3. Generate Instrumentation

- [x] 3.1 Implement generate wrapping that records in-flight gauge changes, request duration, request counter status, finish reason, and token usage.
- [x] 3.2 Implement bounded error classification for returned errors, including context cancellation/deadline and `*provider.APICallError` status codes.
- [x] 3.3 Ensure generate success and error paths return the original result pointer or error unchanged.

## 4. Stream Instrumentation

- [x] 4.1 Implement stream wrapping that records initial `DoStream` errors before a stream is available.
- [x] 4.2 Implement a 64-buffer tee that preserves request/response metadata, forwards every `provider.StreamPart` unchanged and in order, closes exactly once, and drains upstream after consumer cancellation.
- [x] 4.3 Observe stream response metadata, finish usage/reason, provider stream error parts, chunk counts, time to first payload-bearing output, and inter-payload chunk delay.
- [x] 4.4 Finalize stream request, duration, token, TTFT, and in-flight metrics when upstream closes or context cancellation ends the tee.

## 5. Identity, Privacy, and Documentation

- [x] 5.1 Apply `IdentityPreferResponse` and `IdentityRequested` consistently, using response identity for final labels only when provider and model are both available.
- [x] 5.2 Apply provider/model normalizers to every metric with provider/model labels and keep the in-flight gauge balanced on requested identity.
- [x] 5.3 Audit all labels so they never include prompts, outputs, headers, URLs, bodies, response IDs, raw provider metadata, error messages, tool names, tool call IDs, or arbitrary per-request labels.
- [x] 5.4 Write package documentation covering API usage, metric names/labels/buckets, stream-drain requirements, provider-call scope, privacy/cardinality guardrails, Sigil composition, registry usage, and Grafana hosted metrics independence.

## 6. Test Coverage and Verification

- [x] 6.1 Add tests for collector registration, duplicate registration, default registerer behavior, const labels, bucket overrides, and disabled stream chunk collectors.
- [x] 6.2 Add generate tests for success metrics, token usage mapping, API call errors, context cancellation/deadline, response identity preference, requested identity mode, normalizers, and privacy label exclusions.
- [x] 6.3 Add stream tests for exact part forwarding, initial `DoStream` error, `PartError` final status, response identity, finish usage/reason, chunk counts, TTFT, inter-chunk delay, cancellation cleanup, disabled stream chunk metrics, and normalizers.
- [x] 6.4 Add registry integration coverage proving `registry.WithLanguageModelMiddleware(mw)` instruments resolved models when `mw` is returned by `prometheus.Middleware(opts)`.
- [x] 6.5 Verify root dependency isolation with `go list -deps ./...` from the root and by confirming root `go.mod` does not include Prometheus dependencies.
- [x] 6.6 Run `cd middleware/prometheus && go test ./...`, root `go test ./...`, and aggregate short/build checks appropriate for nested-module changes.
