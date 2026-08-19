## Why

The checked-in ProviderWire V4 contract proves what the registered stock Gateway client emits and consumes, but no production server enforces that contract or can carry a unary call to a Go language model. A strict unary vertical slice is needed now to prove validation, host-policy, adaptation, privacy, bounds, and pinned-client interoperability before streaming lifecycle or client adoption adds more state.

## What Changes

- Add a production `gateway/providerwire/v4` HTTP handler for unary `POST /language-model` requests selected by `ai-language-model-streaming: false`.
- Add a small request-aware model resolver and narrowly useful V4 route, header, version, and media-type constants.
- Enforce bounded HTTP envelope validation, strict JSON syntax, request-schema validation, host-control extraction, and request policy before model resolution or provider adaptation.
- Convert validated private wire values to `provider.CallOptions`, preserving explicit empty collections and opaque JSON semantics, then resolve and invoke `DoGenerate` at most once.
- Apply response disclosure policy, validate and encode the complete unary result before HTTP 200 commitment, and keep provider metadata and backend diagnostics private by default.
- Normalize envelope, syntax, schema, policy, resolver, provider, timeout, nil-result, size, and encoding failures into bounded contract-valid JSON errors.
- Add independently generated raw and policy-normalized unary conformance lanes from provenance-valid pinned provider inputs, plus pinned stock-Gateway `doGenerate` and `generateText` interoperability.
- Document the bounded initial unary fixture inventory and any authentic-provider coverage gap rather than inventing or modifying provider inputs.
- Preserve the active legacy provider-wire API, handler, Grafana defaults, and wire behavior.
- Defer streaming/SSE, idle timeout, Go client and Grafana adoption, authentication, catalog/discovery, credentials, accounting, and implemented Gateway controls.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-providerwire-v4`: Extends the strict contract capability with the production unary handler, resolver behavior, ordered validation and policy gates, private adapters, bounded safe responses, total timeout, privacy policy, and stock-client unary interoperability.
- `conformance-testing`: Adds raw and independently policy-normalized ProviderWire V4 unary evidence derived from provenance-valid pinned provider fixtures without changing or reclassifying their inputs.

## Impact

The change primarily affects `gateway/providerwire/v4`, the ProviderWire V4 interop test server and pinned TypeScript tests, unary generation support in `test/conformance`, `test/conformance/PARITY.md`, and the provider-wire server guide. It may promote or replace the existing test-only strict JSON mechanism after focused dependency review. It adds a small exported handler/resolver/options surface but no public wire DTOs or codecs, no service/auth/catalog dependencies, no streaming implementation, and no change to `gateway/providerwire` or Grafana's default transport.
