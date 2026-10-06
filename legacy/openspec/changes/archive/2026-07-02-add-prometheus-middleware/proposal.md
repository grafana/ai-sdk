## Why

Applications need provider-call Prometheus metrics for model requests without pulling the Prometheus client into the root ai-sdk module or changing orchestration, providers, registry routing, or wire compatibility. The existing `middleware.Middleware` hooks and `middleware/sigil` nested-module precedent provide the right extension point for opt-in instrumentation around `provider.LanguageModel.DoGenerate` and `DoStream`.

## What Changes

- Add a dependency-isolated nested Go module at `middleware/prometheus` for local/client-side Prometheus instrumentation.
- Expose direct constructor helpers that register collectors once and return a `middleware.Middleware` or wrapped `provider.LanguageModel` for direct and registry-wide use.
- Instrument provider-level generate and stream calls with bounded, content-free Prometheus metrics for request volume, in-flight calls, request duration, token usage, stream chunks, time to first output, and inter-payload chunk delay.
- Preserve stream behavior by teeing `provider.StreamPart` values unchanged, finalizing stream metrics when the upstream stream closes or the call is canceled, and following the existing Sigil-style drain pattern.
- Prefer response provider/model metadata for final request, duration, and token labels when available, while keeping in-flight gauge accounting tied to the requested identity used at call start.
- Document privacy/cardinality guardrails and the distinction from Grafana hosted server-side metrics controls and any future core telemetry framework.

## Capabilities

### New Capabilities

- `prometheus-middleware`: Defines the nested `middleware/prometheus` module, public API, metric contract, stream observation behavior, dependency isolation, privacy/cardinality rules, and validation expectations for provider-level Prometheus metrics.

### Modified Capabilities

- None.

## Impact

- Affected code: new `middleware/prometheus` module, module docs, tests, and aggregate `mise.toml` tasks if needed for nested-module verification.
- Affected APIs: new exported package API under `github.com/grafana/ai-sdk/middleware/prometheus`; no changes to root `middleware`, `provider.LanguageModel`, `registry`, provider wire types, SSE/UI chunks, or Grafana provider options.
- Dependencies: `github.com/prometheus/client_golang` is added only to the nested Prometheus module and must not appear in root `go.mod` or root-only dependency graphs.
- Compatibility: provider-call metrics are additive and opt-in. The change is not an upstream wire/parity change; upstream `ai@7.0.11` has lifecycle telemetry/OTel semantics but no Prometheus metric contract to mirror.
