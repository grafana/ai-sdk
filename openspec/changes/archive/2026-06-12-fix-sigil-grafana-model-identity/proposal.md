## Why

Sigil recording currently uses the wrapped `LanguageModel` identity as the canonical generation model before the call starts. When a caller wraps the Grafana hosted provider client-side, Sigil records `grafana` as the model provider instead of the real backend provider, which prevents cost attribution from resolving provider/model pricing.

## What Changes

- Update Sigil recording to prefer backend response model metadata when available, so generated records identify the real provider/model used by the hosted endpoint.
- Preserve the fact that a call was routed through Grafana as generation metadata instead of replacing the canonical model provider with the transport provider.
- Keep the Grafana provider's `Provider()` value as `grafana`; it remains a gateway-style transport provider compatible with current registry and middleware behavior.
- Extend stream recording to observe response metadata events, matching the generate path's ability to inspect `GenerateResult.Response`.
- Add tests covering client-side Sigil wrapping around a Grafana-like model for both generate and stream calls.

## Capabilities

### New Capabilities

- None

### Modified Capabilities

- `sigil-middleware`: Sigil recording must distinguish canonical backend model identity from transport/routing provider metadata when response metadata supplies the backend provider/model.

## Impact

- Affected code: `middleware/sigil` recording, generation mapping, stream recording, and tests.
- Affected behavior: Sigil generation rows for gateway-style providers can record backend provider/model for cost attribution while exposing routing information in metadata.
- Public API: no required change to `provider.LanguageModel` or `providers/grafana` identity semantics.
- Dependencies: no new dependencies; the Sigil middleware must continue avoiding imports of provider modules such as `providers/grafana` or `providers/anthropic`.
