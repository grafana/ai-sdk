## Why

The repository has executable evidence for the registered ProviderWire V4 HTTP contract, but no production Go handler can accept a unary request from `@ai-sdk/gateway@4.0.52`. Phase 3 must establish the strict, bounded unary runtime before streaming or service integration can build on the protocol.

## What Changes

- Add a strict ProviderWire V4 unary HTTP handler with exact envelope validation, bounded raw-body processing, lexical JSON checks, complete request-schema validation, explicit capability mapping, host-policy sequencing, catalog resolution, model invocation, and bounded response commitment.
- Support ordered system messages, user and assistant text parts, required empty text, scalar generation controls, stop sequences, text response format, and reasoning effort; reject every other schema-valid request capability deterministically before policy, resolution, or invocation.
- Add closed safe-error categories and allowlisted error envelopes that do not expose provider or transport internals.
- Add explicit unary response DTOs and schemas for text content, all registered warning variants, metadata, usage, and finish behavior; normalize successful output to canonical public model identity and remove backend request, response, and metadata material.
- Replay each phase 2 Gateway request golden to its expected unary stage, add dedicated supported-scalar and focused unsupported-capability cases, and add raw HTTP, bounds, privacy, sequencing, and pinned TypeScript client integration tests.
- **BREAKING** Change `provider.CallOptions.Reasoning` from `*ReasoningEffort` to a value enum whose zero value means provider default, preserving provider behavior while removing unnecessary presence semantics.

## Capabilities

### New Capabilities
- `providerwire-v4-unary-runtime`: Strict unary ProviderWire V4 request processing, supported text mapping, safe failures, canonical resolution, bounded response encoding, and cross-language contract evidence.

### Modified Capabilities
- `providerwire-v4-http-contract`: Replace the phase 2 no-runtime boundary with production Go replay and pinned-client unary execution evidence while retaining the registered-client contract authority.
- `provider-v4-core-types`: Represent reasoning effort as a value enum with provider-default zero semantics instead of a pointer.
- `provider-v4-content-model`: Replace the obsolete pointer-to-string reasoning contract with the value-typed reasoning effort contract.
- `typed-string-enums`: Preserve the typed reasoning enum while changing its provider-default value and `CallOptions` field from pointer to value semantics.
- `effort-level`: Treat zero-valued reasoning effort and explicit wire `provider-default` as equivalent no-op inputs in Anthropic reasoning resolution.

## Impact

- Primary code: `gateway/providerwire/v4`, its embedded request and new response schemas, `provider`, `providers/*` reasoning call sites, and `gateway/catalog` integration.
- Tests: `test/providerwire-v4`, production Go handler/schema/mapping tests, and pinned cross-language unary integration.
- Dependencies: the existing draft 2020-12 schema validation library in the root module plus bounded JSON/HTTP processing implemented in Go; no baseline package upgrade.
- Protocol scope: unary `POST /language-model` only; strict streaming, authentication, discovery service routes, Go client support, tools, files, provider options, body-header forwarding, structured output, and raw output execution remain deferred.
