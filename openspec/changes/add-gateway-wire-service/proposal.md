## Why

The deployed `gateway/providerwire` transport combines canonical LanguageModelV4 JSON with legacy Go compatibility. Replacing it in place would break deployed clients, but new gateway services need a strict, independently owned V4 codec, safer public errors, bounded transport behavior, and direct model-catalog composition.

## What Changes

- Add `gateway/providerwire/v4` as a strict bidirectional codec and HTTP handler for the pinned `@ai-sdk/provider@4.0.4` and `@ai-sdk/gateway@4.0.33` LanguageModelV4 wire.
- Compose the strict handler directly with a non-nil `catalog.ModelResolver`; resolve the exact requested public model ID with the request context and invoke the returned non-nil `provider.LanguageModel` directly.
- Keep total and streaming-idle lifecycle ownership in the handler without a public runtime, middleware, call policy, identity model, metadata extractor, request-ID facility, stream session, invocation goroutine, or proxy channel.
- Keep safe error classification and projection private to the V4 adapter. Preserve redacted mappings for catalog misses, provider rate limits, timeout/cancellation, permanent or transient provider dependencies, and adapter defects.
- Remove an absent or empty top-level `providerOptions.gateway` object and reject non-empty keys as unsupported before catalog resolution. Continue rejecting the reserved namespace in nested provider options. Reject raw-chunk requests before resolution because no host policy approves backend raw exposure.
- Keep the existing `gateway/providerwire` package behavior and API unchanged. Support side-by-side legacy and strict endpoints under distinct base URLs.
- Keep Grafana's binary `WithStrictProviderWire()` opt-in and positive response-limit options while removing the unreleased general mode enum. Apply new response limits only in strict mode so legacy readers remain unchanged.
- Let canonical discriminators select union arms and ignore inactive sibling properties while preserving required/active type validation, unknown-discriminator rejection, provider-reference validation, typed-null correctness, explicit legacy/private-field rejection, privacy, and fail-closed ambiguous domain encoding.
- Expose only the V4 handler/options/constants and strict Grafana client codec operations; keep server-only codecs, DTOs, failures, and limit helpers private.
- Preserve exhaustive literal discriminator goldens plus representative strict handler, Grafana, TypeScript, privacy, limit, and dual-deployment evidence. Fix empty inline-text file data round-tripping.

## Capabilities

### New Capabilities

- `gateway-providerwire-v4`: Strict private-DTO LanguageModelV4 codec and catalog-backed HTTP/SSE handler.

### Modified Capabilities

- `gateway-model-catalog`: The strict V4 handler composes directly with `catalog.ModelResolver` while legacy host composition remains unchanged.
- `grafana-provider`: Binary opt-in strict codec mode and strict-only bounded remote reads remain available without changing legacy behavior.
- `gateway-error-normalization`: Grafana continues decoding the registered strict error vocabulary, including safe dependency categories.

## Impact

- Adds the unreleased `gateway/providerwire/v4` package and removes the proposed `gateway/runtime` and `gateway/failure` packages.
- Leaves prerequisite provider APIs and the deployed legacy provider-wire package unchanged.
- Keeps the registered upstream baseline at `@ai-sdk/provider@4.0.4`, `@ai-sdk/gateway@4.0.33`, and `ai@7.0.44`; server lifecycle and catalog composition remain Go adaptations because upstream supplies no server implementation.
