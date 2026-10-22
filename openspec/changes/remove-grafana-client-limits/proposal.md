# Proposal

## Why

`providers/grafana` enforces its own configurable response limits, which can only disagree with the bounds the Gateway already applies on the server (16 MiB per provider response, `max_model_duration`, the platform's 60s route deadline). Upstream `@ai-sdk/gateway@4.0.103` has no configurable limits. Removing the client caps first ensures the client never rejects output the Gateway emits once the adapter output budgets are removed (#480). Part of #394, tracked in #479.

## What Changes

- **BREAKING** Remove `Limits`, `DefaultLimits` and the `Limits` field from `TokenExchangeConfig`, `ServiceAccountConfig` and `AccessTokenConfig`, along with constructor limit validation. There are no compatibility aliases.
- **BREAKING** Streams are unbounded by the client. Remove the cumulative stream byte cap, the per-event byte cap and the event count cap. Stream body read failures are always retryable transport failures.
- Remove the 1 MiB cap on retained `usage.raw`. Shape and pre-decode UTF-8/JSON validation stay.
- Replace the configurable discovery, unary and error-body limits with one fixed, unexported 2 GiB backstop for all buffered reads. This includes non-2xx stream setup responses. It mirrors upstream's `DEFAULT_MAX_DOWNLOAD_SIZE`. Exceeding it fails with a non-retryable error and never truncates the body.
- Update the godoc and `docs/providers/grafana-gateway.md`. They drop the limit-tuning guidance and state that the server owns resource bounds and callers bound requests through `http.Client` and `context`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `grafana-gateway-client`: replace configurable client processing limits with server-owned bounds and a fixed buffered-read backstop. This affects unary, discovery, error-body, SSE, raw-usage, generated-file, metadata, constructor and test-evidence requirements.

## Impact

- **Code:** `providers/grafana` (`provider.go`, `model.go`, `discovery.go`, `errors.go`, `stream.go`, `doc.go`) and its tests (`public_surface_test.go`, `provider_test.go`, `stream_test.go`, `model_test.go`, `discovery_test.go`, `generated_file_test.go`, `metadata_test.go`).
- **Public API:** breaking removal in the separately versioned `providers/grafana` module. No other module in the repository references the removed symbols.
- **Docs:** `docs/providers/grafana-gateway.md` (raw output, large catalogs, generated files, "Bound work and handle errors").
- **Parity:** the backstop is a parity-preserving match of upstream `readResponseWithSizeLimit`, and unbounded streams match `createEventSourceResponseHandler`. No `knownGaps` entry is needed.
- **Out of scope:** decoder shape simplification (#375), any Gateway server change (#480) and the TypeScript configured-discovery example's `maxBytes`.
