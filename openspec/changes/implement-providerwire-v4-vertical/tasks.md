## 1. Establish the production schema authority

- [x] 1.1 Record the registered baseline and current parent/legacy verification results, then move the normative request schema from `test/providerwire-v4` into an embeddable `gateway/providerwire/v4/schema` location without changing its bytes.
- [x] 1.2 Update the TypeScript evidence validator, source paths, and evidence documentation to consume the production-owned request schema, remove the old copy, and prove all existing semantic captures and negative schema cases remain unchanged.
- [x] 1.3 Add the strict package skeleton, relative route/header/MIME constants, embedded schema loading, named limit/timeout defaults, construction options, and constructor tests proving request/unary limits only require positivity while JSON-error and complete framed terminal-event minima are enforced separately.

## 2. Add protocol-neutral safe failures

- [x] 2.1 Add failing table-driven tests for all eleven safe-failure categories including failed dependency, fixed retryability, invalid categories, empty messages, zero-value rendering fallback, and the absence of unwrapping or rich provider/HTTP fields.
- [x] 2.2 Implement the immutable `gateway/failure` value, typed category constants, validated constructor, and accessors without protocol serialization or provider-error classification.

## 3. Implement strict request validation and mapping

- [x] 3.1 Add handler-pipeline tests proving invalid methods, missing/duplicate/invalid required headers, duplicate or parameterized `Content-Type`, body read failures, oversized bodies, and cancellation precedence bypass policy, resolution, and model invocation while additional host headers remain accepted.
- [x] 3.2 Add strict JSON tests for invalid raw UTF-8 inside strings, nested same-scope duplicate members, equal names in separate object scopes, comments, trailing commas, invalid numbers, trailing values/data, and valid exactly-one-value bodies before implementing the parser.
- [x] 3.3 Implement bounded `utf8.Valid` rejection before the scope-aware duplicate token walk, then complete embedded request-schema validation in syntax-before-schema order with audited structural invalid-request messages.
- [x] 3.4 Add failing mapper tests over Phase 1 semantic requests for ordered system/user/assistant text, required empty text, exact language-model numbers, absent/zero scalar settings, reasoning, and nil versus explicit empty stop sequences.
- [x] 3.5 Implement private request DTOs and explicit presence-correct mapping to `provider.CallOptions` without using provider generic JSON as the wire authority.
- [x] 3.6 Add and satisfy fail-closed tests for every deferred feature, including explicit empty tools/headers/provider options and explicit `includeRawChunks: false`, proving rejection occurs before policy or resolution.

## 4. Implement policy, catalog resolution, and safe errors

- [x] 4.1 Add policy and catalog tests for context propagation, the trusted no-mutation/no-retention policy contract without defensive-copy assertions, policy rejection, cancellation precedence, alias resolution, canonical identity retention, unknown models, resolver failures, nil models, empty canonical IDs, and at-most-once calls.
- [x] 4.2 Implement the narrow host-policy seam and fixed validation → mapping → policy → `catalog.ModelResolver` → invocation ordering.
- [x] 4.3 Add the closed safe-error response schema and table-driven golden tests for all eleven category mappings, zero-value fallback, separate JSON/event fallback minima, pinned-client retryability, raw authentication-message bytes, and negative assertions for backend URLs, bodies, headers, causes, provider identity, backend model IDs, and extra members.
- [x] 4.4 Implement cancellation/deadline precedence and the fixed reduction table: catalog unknown → not found, active-context resolver errors → internal, provider cancellation/deadline and 408/504 → cancellation/timeout, provider 429 → rate limit, provider 503 → overload, remaining retryable `APICallError` → upstream, remaining non-retryable `APICallError` → failed dependency/424, and other provider failures → upstream; use fixed messages and canonical fallback.

## 5. Implement bounded unary text

- [x] 5.1 Add failing unary encoder and handler tests for ordered and empty text, required `warnings: []`, rejection of non-empty provider warnings, finish reasons, optional usage counters, canonical alias identity, exactly-once dispatch, metadata stripping, nil/unsupported results, invalid values, and output-limit failure before HTTP 200.
- [x] 5.2 Add the closed unary text response schema, private DTO mapper, schema validation, golden bytes, and bounded pre-commit encoding that emits required empty warnings and only canonical public response identity.
- [x] 5.3 Implement unary handler dispatch with request-derived total timeout and safe reduction of provider, timeout, and cancellation failures.

## 6. Implement the minimal bounded SSE lifecycle

- [x] 6.1 Add failing state-machine tests for exactly one server-owned start when provider start is present or absent, validation/consumption of provider start, non-empty start warnings, empty/non-empty finish warnings, optional response metadata, sequential text blocks, matching IDs, empty deltas, finish, premature close, invalid first parts after commitment, unsupported parts, and provider failures.
- [x] 6.2 Add the closed Phase 3 stream-part schema and explicit mapper that emits the server-owned empty-warning start, canonicalizes model identity, consumes provider starts, ignores only empty finish warnings, and strips provider identity, headers, metadata, backend IDs, and unsupported fields.
- [x] 6.3 Define golden/schema coverage for HTTP 200 event-stream and no-cache/no-transform headers plus the closed terminal error payload, then implement size checks over the entire `data: <json>\n\n` frame, flushing, clean EOF after finish, no `[DONE]`, no required `Connection` header, and at most one bounded error event after commitment.
- [x] 6.4 Add and satisfy lifecycle tests for the non-nil result/channel commitment boundary, pre-stream errors, nil streams, post-commit invalid first parts, total timeout, first/inter-event idle timeout and reset, cancellation precedence, event overflow, context cancellation on observable encoding/write failure, and no second write to a failed writer.

## 7. Add exact-pinned cross-language runtime coverage

- [x] 7.1 Add `@ai-sdk/gateway@4.0.52` as an exact registered dependency of `test/integration`, update the shared lockfile, and verify baseline validation and upgrade-consumer coverage still recognize every tracked dependency.
- [x] 7.2 Add deterministic integration testserver models that respectively emit and omit provider `stream-start`, then mount the strict relative handler beneath a test-owned `/api/v1/aisdk` prefix with canonical and alias IDs.
- [x] 7.3 Add direct public Gateway-client unary request-mapping/content/finish/usage consumption, normalized single-start streaming with observable canonical identity, safe pre-stream error, and exact safe post-commit error-part assertions; use raw HTTP plus local schemas/goldens for unary canonical identity, explicit empty warnings, response headers, authentication messages, and omitted `[DONE]` because the pinned client normalizes or ignores those server details.

## 8. Integrate maintenance and parity reporting

- [x] 8.1 Extend `mise run check-providerwire-v4` with focused strict Go tests and the exact-pinned runtime integration scenario while preserving non-mutation and the existing request-evidence checks.
- [x] 8.2 Update `test/providerwire-v4/README.md` and `test/conformance/PARITY.md` to describe the automated Phase 3 vertical, single schema authority, pinned-client consumption boundary, unchanged legacy evidence, and remaining Phase 4 gaps.
- [x] 8.3 Run legacy request corpus, `gateway/providerwire`, Grafana provider, interop, and conformance checks and confirm no tolerant request/response bytes, SSE framing, or existing client behavior changed.

## 9. Verify the completed change

- [x] 9.1 Run focused root and nested-module tests, including race coverage for the new gateway packages and deterministic integration testserver tests.
- [x] 9.2 Run `mise run check-providerwire-v4`, `mise run validate-parity-baseline`, `mise run parity-check`, `mise run test-integration`, `mise run test-interop`, `mise run test-conformance`, `mise run test`, `mise run vet`, `mise run lint`, and `mise run lint-docs`.
- [x] 9.3 Run `openspec validate --all --strict` and `git diff --check`, then perform a final implementation/spec coherence review against every Phase 3 requirement and document any remaining coverage gap without broadening the parity claim.

## 10. Synchronize and close the phase

- [x] 10.1 Synchronize the completed delta specifications into the canonical OpenSpec specifications after implementation verification passes.
- [x] 10.2 Present the verified implementation, synchronized specs, validation evidence, residual risks, and proposed archive action for explicit approval; do not archive before that approval.
- [ ] 10.3 After explicit approval, archive `implement-providerwire-v4-vertical` and confirm `openspec list --json` reports zero active changes.
- [ ] 10.4 Report the final branch head SHA, parent/restack state, changed and generated artifacts, validation commands, and any residual risk.
