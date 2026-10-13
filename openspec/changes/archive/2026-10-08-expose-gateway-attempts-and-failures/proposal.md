## Why

Callers need actual configured attempt outcomes and useful native failures through ordinary Gateway results and errors. Runtime activation is implemented; the owner-approved review follow-up protects configured API-key echoes in Gateway-produced summaries without returning to broad credential filtering.

## What Changes

- Preserve request-owned SDK attempt observation, copied configured identity and optional execution metadata on existing unary/SSE carriers.
- Register configured provider API keys and the enabled exporter credential once at startup, excluding static Gateway access keys and redact exact matches in summary message, type and string code for every caller.
- Preserve BYOK-only echoes, numeric code/status, classification, original errors, opaque native metadata and existing transport/lifecycle behavior.
- Reuse this existing change and update current specs, tests and docs. No new proposal or wire shape.

## Capabilities

### New Capabilities

- `gateway-attempt-failure-evidence`: Runtime compact execution delivery and configured-key protection limited to Gateway-produced summaries.

### Modified Capabilities

- `gateway-execution-overview`: Keep pure projection reusable and govern protected runtime delivery separately.
- `gateway-provider-metadata`: Preserve opaque native metadata and optional result/finish enrichment.
- `gateway-ordered-text-fallback`: Keep execution/observation policy unchanged while protecting candidate-local summary fields.
- `providerwire-v4-unary-runtime`: Protect native summaries in optional success/failure attribution.
- `providerwire-v4-streaming-runtime`: Protect finish-attempt and event-local error summaries without stream buffering.

## Impact

AGPL service startup, ProviderWire handler and internal summary policy; focused and both-client tests, current specs/docs and this reused change. No SDK/client API, schema, provider fixture, dependency or minimum-Go change. No generic output filtering, request credential inventories, encoded-secret inference or new HTTP nativeError carrier. Request-account observation (#317), raw diagnostics (#323) and shared transport policy (#394) remain separately owned.

Registered reference remains eb77f09e3c06c28e860d92e0de941b143c2eecec (ai7.0.127/Gateway4.0.103). This Gateway security policy is not a claim about upstream private-service behavior or live provider echoes. Earlier archived validation describes historical implementations, not approval of this follow-up.
