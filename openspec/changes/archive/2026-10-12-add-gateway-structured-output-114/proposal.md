## Why

Issue #114 / WP20 requires both Gateway clients to request structured output. Main already validates the wire shape but rejects JSON response formats at runtime, while Go's string name loses absent versus explicitly empty provider-default semantics.

## What Changes

- **BREAKING**: make `provider.ResponseFormat.Name` a `*string`, updating all producers and native adapters so only absence defaults; keep description a normalized string.
- Map JSON/schema response formats through Gateway unary and streaming paths, retaining opaque object schema values and existing response text/event codecs.
- Preserve response-schema numeric values through native OpenAI, Anthropic and Bedrock serialization, correcting accidental float64 rounding while retaining intentional native transformations and strict single-JSON decoding.
- Admit this newly mapped capability on configured fallback with native candidate semantics, existing commitment/cancellation, useful attributed errors and independent operator/consumer capture.
- Add pinned-client, native-boundary and high-level output tests, narrow docs and honest parity evidence.

## Capabilities

### New Capabilities
- `gateway-structured-output`: complete WP20 request, native execution, fallback and observability contract.

### Modified Capabilities
- `providerwire-v4-unary-runtime`: activate structured output instead of rejecting it.
- `gateway-ordered-text-fallback`: classify structured output as mapped rather than unsupported.

## Impact

Root provider/output API, Grafana encoder, native provider name consumers, Gateway v4 mapper, deterministic Go/TS tests, docs and PARITY.md. No new dependency on deferred #321–324, no routing/BYOK work, no invented recordings or deployment. Existing warnings/metadata/error machinery is reused; missing proof is distinguished from missing implementation.
