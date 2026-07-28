## Why

Consumers that record per-call telemetry (token usage, cost, latency, error and
stop-reason metrics) need to label each metric with the provider that actually
served the request. Today the served model id is exposed on the response
metadata, but the provider is not. When a `fallback.Model` switches candidates
(e.g. `anthropic` -> `anthropic.vertex`), the served provider is unrecoverable
by consumers: the fallback wrapper only exposes the primary candidate's
`Provider()`/`ModelID()`, and the response carries no provider field. This
forces downstream metrics to mislabel fallback traffic as the primary provider.

## What Changes

- Add a `Provider` field to `provider.ResponseMetadata` (carried through to
  `GenerateResponse`), alongside the existing `ID`, `ModelID`, and `Timestamp`.
- Add a `Provider` field to `provider.StreamPart` so the `PartResponseMeta`
  stream event can carry the serving provider, mirroring the existing
  `ModelID`/`ResponseID` fields.
- Populate `Provider` from the concrete provider in the Anthropic provider:
  `convertResponse` (generate) and the `message_start` -> `PartResponseMeta`
  event (stream) set it to the model's provider name (`anthropic` or
  `anthropic.vertex`).
- Propagate the streamed `PartResponseMeta.Provider` into the orchestration
  `ResponseMetadata` in `StreamText`'s step processing, so `StreamTextResult`
  and step results expose the served provider.
- `fallback.Model` requires no change: because the served candidate tags its own
  response/stream output and the fallback wrapper forwards that output verbatim,
  the active provider rides along automatically.

These are additive fields with `omitempty`; no existing behavior changes and the
wire format remains backward compatible.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `provider-v4-core-types`: the `ResponseMetadata` requirement currently pins the
  struct to exactly three fields (`ID`, `ModelID`, `Timestamp`); it must allow a
  fourth field `Provider`. A new requirement covers the provider being carried on
  the response/stream metadata and populated by providers.
- `provider-wire`: the `StreamPart` wire round-trip requirement must include the
  new `Provider` field so the `PartResponseMeta` event round-trips it.

## Impact

- `provider/language_model.go`: `ResponseMetadata` gains `Provider string`.
- `provider/stream_part.go`: `StreamPart` gains `Provider string`.
- `providers/anthropic/convert_response.go` and `convert_stream.go`: populate
  `Provider` (threaded from `providers/anthropic/model.go`).
- `streamtext.go`: map `PartResponseMeta.Provider` into `ResponseMetadata`.
- Downstream consumers (e.g. the Grafana Assistant metrics middleware) can read
  the served provider from `GenerateResult.Response.Provider` /
  `StreamTextResult.Response()` / `PartResponseMeta.Provider`.
- No breaking changes; all new fields are optional and `omitempty`.
