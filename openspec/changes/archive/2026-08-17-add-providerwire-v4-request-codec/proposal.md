## Why

The existing provider-wire request helpers intentionally accept historical Go encodings and delegate nested JSON behavior to provider-domain marshalers. The repository needs an independent, strict LanguageModelV4 request boundary that produces stable canonical JSON, supplies validation for later transport handlers, and can evolve without changing the legacy transport.

## What Changes

- Add an independent `gateway/providerwire/v4` package for canonical LanguageModelV4 request encoding.
- Add private request DTOs and field-by-field conversion between `provider.CallOptions` and canonical JSON.
- Add strict internal request decoding that validates required fields, discriminators, active union arms, provider references, opaque JSON boundaries, typed nulls, request privacy constraints, and the complete set of understood standard fields.
- Remove absent or empty top-level `providerOptions.gateway` values and reject non-empty or nested reserved gateway namespaces.
- Reject unknown standard request fields before invocation while preserving inactive sibling-arm fields and explicit opaque extension boundaries.
- Preserve all supported request semantics, including empty inline-text file data, tools, tool results, provider options, headers, and model settings.
- Keep the existing `gateway/providerwire` public API and tolerant request decoding unchanged.

## Capabilities

### New Capabilities

- `gateway-providerwire-v4`: Defines the independent strict LanguageModelV4 request codec, its private DTO and validation boundary, reserved namespace handling, canonical request semantics, and coexistence with the legacy provider-wire package.

### Modified Capabilities

None.

## Impact

- Adds production and focused tests under `gateway/providerwire/v4`.
- Adds compatibility checks under `gateway/providerwire` to prevent accidental legacy API or behavior changes.
- Depends only on the standard library and the transport-agnostic `provider` package.
- Does not add HTTP execution, routes, catalog resolution, result or stream decoding, error envelopes, SSE framing, or client transport selection.
