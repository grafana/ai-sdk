## Why

Phase 1 established the exact pinned ProviderWire V4 request contract and Phase 2 made the Go provider request model preserve it, but the repository still has no production strict V4 runtime. A bounded unary and streaming text vertical is needed now to prove strict validation, protocol-neutral failure handling, catalog resolution, explicit response encoding, and pinned-client interoperability before Phase 4 expands the full protocol surface.

## What Changes

- Add an isolated `gateway/providerwire/v4` HTTP handler that accepts only the strict V4 dialect and exposes relative language-model route and header constants.
- Enforce the request envelope, body limits, valid raw UTF-8, scope-aware duplicate rejection, strict JSON syntax, the normative request schema, and an explicit Phase 3 text/scalar subset before policy, resolution, or provider invocation.
- Add a narrow pre-resolution host-policy seam and resolve models through `catalog.ModelResolver`, preserving the canonical `ResolvedModel.ID` as the public response identity.
- Add a protocol-neutral safe failure value and a complete ProviderWire V4 mapping for the initial failure categories without exposing provider or backend details.
- Encode bounded unary text results and normalize provider differences into one server-owned SSE `stream-start`, a minimal bounded text lifecycle through `finish`, and clean EOF without `[DONE]`.
- Add deterministic Go tests and exact-pinned `@ai-sdk/gateway@4.0.52` direct unary and streaming tests, while retaining local schemas and goldens as the strict response authority.
- Extend the existing non-mutating ProviderWire V4 check and parity map with the narrowly proven runtime vertical.
- Keep the tolerant legacy `gateway/providerwire` dialect unchanged and reject, rather than ignore, valid V4 features deferred to Phase 4.

## Capabilities

### New Capabilities

- `gateway-safe-failure`: Defines the small protocol-neutral failure value, category vocabulary, privacy boundary, and safe translation inputs shared by protocol adapters.
- `providerwire-v4-runtime`: Defines strict request processing, text-subset adaptation, policy and catalog ordering, bounded unary/SSE output, V4 error rendering, and exact-pinned client interoperability for the Phase 3 vertical.

### Modified Capabilities

- `providerwire-v4-contract-evidence`: Extends the public non-mutating check and parity reporting to cover the strict Phase 3 runtime and direct pinned-client tests without broadening the original request-capture or smoke-probe claims.

## Impact

- New root-module packages under `gateway/` for safe failures and `gateway/providerwire/v4`.
- The normative request schema becomes available to production code and remains the single schema consumed by the TypeScript evidence workspace.
- Existing `gateway/catalog`, `provider.LanguageModel`, request evidence, and exact registered package pins are consumed without API redesign.
- Focused runtime and cross-language tests, `mise` verification, and `test/conformance/PARITY.md` gain bounded Phase 3 coverage.
- No authlib, service process, `/config`, concrete provider construction, Go V4 client, full tool/file/content adaptation, raw-chunk disclosure, or legacy behavior changes are introduced.
