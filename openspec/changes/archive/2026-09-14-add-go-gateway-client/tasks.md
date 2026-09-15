## 1. Establish module and contract evidence

- [x] 1.1 Add the Apache-licensed `providers/grafana` module, package documentation, immutable root-SDK/authlib pins, repository build/test registration, and dependency tests proving the root SDK and client build with `GOWORK=off` and no `ai-gateway` dependency.
- [x] 1.2 Add compile-time assertions for `registry.Provider` and `provider.LanguageModel`, plus a public-surface test that rejects exported low-level ProviderWire/legacy codec or mode declarations.
- [x] 1.3 Extend the exact-pinned ProviderWire test workspace with a test-only Go semantic-request capture runner and differential harness that consumes the existing `@ai-sdk/gateway@4.0.52` pins rather than creating a second npm workspace.
- [x] 1.4 Add failing differential request cases for unary/streaming text, scalar presence, configured/call header precedence, body-header duplication, selected binary/URL values, opaque JSON, and the documented Go reasoning-default adaptation.

## 2. Provider construction and authentication

- [x] 2.1 Define validated cloud-auth and direct-access-token configurations, shared client limits/defaults, configured headers, HTTP-client selection, safe base-URL normalization, and strict requested-model validation.
- [x] 2.2 Implement authlib-backed CAP exchange with namespace and default/explicit audience, delegating token caching to authlib and propagating context cancellation without Gateway I/O on failure.
- [x] 2.3 Implement caller-managed access-token forwarding and prove it never contacts the token-exchange endpoint.
- [x] 2.4 Implement the acting-user context helper and deterministic header precedence so access token, optional user token, content negotiation, and three protocol headers cannot be overridden or duplicated.
- [x] 2.5 Add constructor/auth tests for empty and whitespace credentials, unsafe URLs, invalid limits/options/headers, HTTP-client precedence, exchange errors/empty tokens, token reuse behavior owned by authlib, acting-user presence, and cancellation.

## 3. Explicit ProviderWire request mapping

- [x] 3.1 Add private client request DTOs, a compile-time witness covering every current `provider.CallOptions` field, and an exhaustive executable growth witness covering every registered finite discriminator.
  - Go string enums are open, so discriminator growth is guarded by the complete reachable-type/constant AST inventory, finite-arm classification tests, and mutation-proven baseline failures; see `PARITY.md` for the exact Go adaptation.
- [x] 3.2 Implement explicit prompt/content/tool/result/approval, response-format, reasoning, provider-option, call-header, scalar, URL, and selected binary-data projection with representable nil/zero/false/empty/null semantics and base64 conversion.
- [x] 3.3 Reject invalid UTF-8, non-finite numerics, invalid raw JSON, unknown discriminators, conflicting selected arms, and unrepresentable values before token acquisition or HTTP.
- [x] 3.4 Complete request differential goldens and classify the reasoning-default presence gap in `test/conformance/PARITY.md`; make baseline validation fail on unmapped provider-contract changes.

## 4. Discovery client

- [x] 4.1 Define the public discovery `ModelInfo` projection and implement authenticated `ListModels` using exact safe URL composition and the configured discovery byte bound.
- [x] 4.2 Implement atomic discovery decoding that accepts additive unknown members while validating required public text, IDs, duplicate rows, `v4`/`grafana` specification, and row/model-ID consistency without importing Gateway catalog types.
- [x] 4.3 Add exact-limit and over-limit, malformed/trailing JSON, additive-field, duplicate/invalid-row, cancellation, auth/error, canonical/alias order, and privacy tests for discovery.

## 5. Unary response and Gateway errors

- [x] 5.1 Add a bounded media-type-aware JSON response reader shared by unary/discovery/error handling that detects limit-plus-one, trailing documents, short reads, context errors, and guaranteed body closure.
- [x] 5.2 Implement the closed WP5 unary text mapper for ordered content, finish reason, and JavaScript-safe usage, replacing server `request`, `response`, and `warnings` with local request bytes, HTTP headers/body, and an empty warning slice.
- [x] 5.3 Define the closed `GatewayError` category API and registered error-envelope decoder for authentication, forbidden, invalid request, model not found, rate limit, failed dependency, and internal server responses, retaining a bounded `APICallError` cause and exact status-derived retryability.
- [x] 5.4 Map malformed/wrong-media-type/oversized HTTP responses, transport failures, cancellation, and deadlines to bounded local errors without copying arbitrary response bytes into primary messages.
- [x] 5.5 Add unary/error differential cases and hostile fake-server tests covering client-owned replacement, unknown output families, every registered status/type/code combination, retryability, exact/over bounds, privacy, body closure, and `errors.As`/`errors.Is` behavior.

## 6. Incremental SSE runtime

- [x] 6.1 Implement a private incremental SSE data reader with cumulative-byte, complete-event-byte, and event-count limits, correct CRLF/data framing for the emitted contract, exact `[DONE]` handling, and no full-stream or unbounded-line allocation.
- [x] 6.2 Implement one-owner streaming setup and consumption: validate status/media type before returning, populate local request/HTTP response metadata, deliver with context-aware backpressure, close the body/channel exactly once, and perform no implicit retry.
- [x] 6.3 Implement the closed WP5 stream mapper for start, response metadata with timestamp conversion, text start/delta/end, ordered public error parts, finish, and bounded raw parts; reject every unsupported or malformed family with at most one terminal protocol `PartError`.
- [x] 6.4 Match pinned consumption behavior for raw filtering, required empty deltas, `[DONE]`, finish-before-close, clean EOF with or without finish, context cancellation without a synthetic provider error, and ordered non-terminal server error parts.
- [x] 6.5 Add stream differential tests plus adversarial exact/over byte/event/count limits, fragmented reads, long lines, malformed JSON/SSE, wrong content type, unsupported families, slow consumers, cancellation races, body/channel closure, one-owner cleanup, and goroutine-leak tests.

## 7. Authenticated service integration and documentation

- [x] 7.1 Extend repository black-box integration to build/spawn the WP5 command and exercise Go discovery, canonical/alias unary text, streaming text, cloud/static auth paths where deterministic, acting-user propagation, abort/cancellation, and every public error reachable through the production command over HTTP; cover other registered client errors at the generic runtime/client boundary.
  - The command covers all ten reachable status rows. Its production composition has no permission policy yielding 403, so forbidden remains covered by generic runtime/client HTTP differential tests without a test-only command path.
- [x] 7.2 Compare equivalent Go and Vercel calls in one matrix for method/path/headers/body, unary replacement, stream normalization, error category/retryability, discovery, cancellation, `[DONE]`, raw filtering, timestamp conversion, and EOF; record every runtime-only representation difference.
- [x] 7.3 Add credential/backend/topology privacy assertions across returned errors/results, logs captured by black-box tests, discovery, request metadata, response metadata, and stream parts.
- [x] 7.4 Document Go client setup, both auth modes, API-prefix base URL, model discovery/registry use, text capability boundary, caller-owned cancellation/retry behavior, limits, and the separation from server-side fallback and deployment.

## 8. Validation and one-PR closure

- [x] 8.1 Run formatting, module tidiness, focused client tests, race-sensitive stream tests, and root/client `GOWORK=off` build and test checks.
- [x] 8.2 Run the ProviderWire TypeScript request/consumption/differential suite, authenticated command integration, `mise run validate-parity-baseline`, and the applicable parity check without regenerating provider-provenance fixtures.
- [x] 8.3 Run repository lint/vet/test checks, verify committed module pins resolve without `replace` directives or root-workspace registration of `ai-gateway`, and inspect the final diff for production/test code outside the WP7 client and contract-evidence boundary.
- [x] 8.4 Confirm WP6 image/capacity, WP8 observability controls, WP9 fallback/backend selection, and WP10 deployed rollout/migration remain absent and leave explicit handoff notes for later capability packages that extend the client response mapper.
