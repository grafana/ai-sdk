## 1. Consolidate protocol files and tests

- [x] 1.1 Create `gateway/providerwire/doc.go` documenting that `provider` is the transport-agnostic in-process contract and `gateway/providerwire` owns the complete remote LanguageModel protocol plus server lifecycle.
- [x] 1.2 Move `provider/wire/routes.go`, `request.go`, `response.go`, `sse.go`, and `errors.go` into `gateway/providerwire`, preserving all exported names, constant values, encoding logic, SSE framing, and error-envelope semantics.
- [x] 1.3 Move `provider/wire/request_test.go`, `response_test.go`, `sse_test.go`, and `errors_test.go` into `gateway/providerwire`; update package declarations and helper references without changing existing assertions, expected bytes, or helper error behavior, and add focused assertions if needed to lock existing `wire: ...` error strings during the move.
- [x] 1.4 Delete `provider/wire` after the move. Do not create an alias package, forwarding wrapper, compatibility re-export, deprecated symbol copy, or other shim at the old path.
- [x] 1.5 Update godoc and source comments that identify `provider/wire` as the transport owner, including references under `provider`, so they name `gateway/providerwire` without making `provider` depend on it.

## 2. Public handler construction contract

- [x] 2.1 Add the request-aware `ModelResolver` interface and `ModelResolverFunc` adapter, including compile-time interface coverage.
- [x] 2.2 Add `Handler`, `NewHandler`, option types, Assistant-compatible default constants, and exported idle/total timeout sentinels in the same `gateway/providerwire` package as the codecs.
- [x] 2.3 Add construction tests for nil resolver, function-adapter behavior, omitted defaults, positive overrides, nil options, and non-positive option rejection.
- [x] 2.4 Verify imports preserve the dependency graph `gateway/providerwire -> provider` and do not introduce a dependency from `provider` back to gateway transport code.

## 3. Request validation and co-located decoding

- [x] 3.1 Add failing table-driven tests for POST-only handling; missing/blank model, spec-version, and streaming headers; unsupported specification versions; and invalid streaming values, asserting status and retryability.
- [x] 3.2 Add failing content-negotiation tests for omitted/parameterized/invalid Content-Type and omitted/exact/wildcard/incompatible Accept values in both unary and streaming modes; include compatible exact and wildcard entries with `q=0`, plus empty stripped entries such as `,application/xml` and `;q=0`, to lock in Assistant's parameter-stripping and permissive-empty-entry behavior rather than standards-aware negotiation.
- [x] 3.3 Add failing body tests for canonical CallOptions decoding, malformed JSON, read failure, exact configured size boundary, and oversized bodies.
- [x] 3.4 Add resolver-order tests proving invalid requests never resolve and valid requests pass the original request/context plus trimmed model ID exactly once.
- [x] 3.5 Implement method/header/content-negotiation validation and bounded body decoding through the co-located `gateway/providerwire` constants and `DecodeCallOptions` until the validation and resolver-order tests pass.

## 4. Unary dispatch and error normalization

- [x] 4.1 Add failing unary tests proving decoded options and total-timeout context reach `DoGenerate`, `DoStream` is not called, and successful results round-trip through the co-located canonical JSON codec.
- [x] 4.2 Add failing error tests for wrapped `APICallError` preservation, unencodable, invalid-status, and typed-nil API-call errors, arbitrary resolver/model error normalization, nil and typed-nil resolved models, request cancellation, total deadline, nil generate result, and result-encoding failure; assert invalid pre-commit responses use an encodable retryable HTTP 500 canonical error response instead of an empty implicit HTTP 200 or panic.
- [x] 4.3 Implement shared error normalization with the specified 4xx, 499, 500, 502, and 504 status/retryability/message mappings while preserving valid encodable API-call error fields and falling back to an encodable internal 500 for invalid envelopes.
- [x] 4.4 Implement unary dispatch that calls the co-located `EncodeGenerateResult` before committing HTTP 200 and preflights the co-located error envelope for all pre-commit failures.

## 5. Streaming dispatch and HTTP lifecycle

- [x] 5.1 Add failing stream setup tests for pre-stream errors, nil result/channel, successful response headers, and initial flush before the first part.
- [x] 5.2 Add failing stream forwarding tests for canonical ordered SSE parts, per-event flush, clean EOF without `[DONE]`, and termination immediately after a forwarded upstream `PartError`.
- [x] 5.3 Implement stream dispatch, delayed success commitment, canonical response headers, initial flush, co-located `WriteSSEStreamPartTo` forwarding, and clean termination behavior.
- [x] 5.4 Add failing output tests proving a canonical SSE encoding or response-writer write failure cancels the model, attempts no second event, stops timers, returns without panic, and leaks no handler-owned goroutine. Do not assert detection of `http.Flusher.Flush()` failure because `Flush` has no error return.
- [x] 5.5 Implement observable SSE encoding/write-failure cancellation and lifecycle cleanup through the co-located `WriteSSEStreamPartTo`; do not add alternate framing or flushing paths.

## 6. Cancellation and timeout behavior

- [x] 6.1 Add deterministic tests for total timeout before stream commitment and during an established stream, asserting `ErrTotalTimeout`, retryable 504 JSON/PartError behavior, and model cancellation.
- [x] 6.2 Add deterministic tests for idle timeout before the first part, between parts, and reset-on-activity, asserting `ErrIdleTimeout`, retryable 504 PartError behavior, and model cancellation.
- [x] 6.3 Add an `httptest.Server` consumer-disconnect test proving the request context cancels the producer promptly; separately test best-effort post-commit 499 emission with a writable canceled request.
- [x] 6.4 Implement one total-timeout context for model invocation/stream consumption and a stream-only resettable idle timer with cancel causes until all timeout and cancellation tests pass.
- [x] 6.5 Run the new package tests with the race detector and fix timer, channel, or cancellation races without adding detached provider-call goroutines.

## 7. Update clients, conformance, and live references

- [x] 7.1 Update `providers/grafana` imports from `github.com/grafana/ai-sdk/provider/wire` to `github.com/grafana/ai-sdk/gateway/providerwire`, update selector names/package aliases consistently, and keep its direct `provider` dependency for runtime types.
- [x] 7.2 Update Grafana provider tests for the new package path without changing request headers, JSON bodies, response decoding, SSE ordering, EOF handling, or error expectations.
- [x] 7.3 Add an external-package test in the separate `providers/grafana` module that constructs `NewWithAccessToken`, mounts the public handler on `httptest.Server`, and resolves a hand-written model without importing the child module from root tests.
- [x] 7.4 Cover real-client unary success and equivalent CallOptions/request metadata, ordered streaming success with immediate delivery and clean EOF, and retryable mid-stream `PartError` field preservation.
- [x] 7.5 Update `test/conformance/grafana/conformance_test.go` imports and replace its hand-written validation/dispatch with the public handler, retaining an outer host wrapper or mux guard that validates the exact `providerwire.PathLanguageModel` path and authorization header before dispatch, plus an Anthropic replay-backed request-aware resolver.
- [x] 7.6 Update current godoc, examples, tests, source references, and actual provider-wire documentation from `provider/wire` to `gateway/providerwire`; add or update a provider-wire server guide in the appropriate `docs/guides/` surface and index it without repurposing `docs/concepts/wire-protocol.md`, which remains the frontend UI-message protocol page. Do not rewrite archived OpenSpec history.
- [x] 7.7 Search all live Go source, tests, docs, module files, and conformance code for stale `provider/wire` imports or package references and resolve every match; separately verify no compatibility shim or re-export was introduced.

## 8. Parity, migration handoff, and byte stability

- [x] 8.1 Keep all moved codec test vectors and assertions unchanged and run the Grafana shared-fixture conformance tests, confirming existing `expected.jsonl` outputs remain byte-identical; do not regenerate snapshots for the package move or extraction.
- [x] 8.2 Update the provider-implementation row in `test/conformance/PARITY.md` to name the real Grafana client/public-server confidence source while retaining cancellation/abort as a conformance gap rather than claiming upstream coverage.
- [x] 8.3 Verify the public surface supports the documented Assistant migration shape—auth middleware, `ModelResolverFunc` catalog/policy adapter, timeout options, and host route mount—without adding Assistant, auth, logger, or router dependencies.
- [x] 8.4 Record and verify the Assistant follow-up migration expectations: side-by-side tests SHALL adopt retryable HTTP 500 canonical errors for nil/unencodable unary results and invalid API-call error envelopes, and host-owned observability tests SHALL recognize `providerwire.ErrIdleTimeout` / `providerwire.ErrTotalTimeout` through `errors.Is` or verify translation to existing Assistant sentinels while preserving backend-error plus idle/total-timeout classifications.
- [x] 8.5 Classify the import-path removal as source-breaking and unchanged valid protocol bytes as parity-preserving provider-transport work; classify retryable HTTP 500 responses for invalid unary results and invalid API-call error envelopes as intentional pre-commit response fixes.

## 9. Validation

- [x] 9.1 Run repository formatting and `go test -race ./gateway/providerwire/...` from the root module.
- [x] 9.2 Run `go test ./...` in the root module and `go test ./...` from `providers/grafana`.
- [x] 9.3 Run the Grafana conformance package from its nested module with the repository's conformance build tag, then run `mise run test-short` so root and all configured nested modules are covered.
- [x] 9.4 Run `mise run validate-parity-baseline` and `mise run parity-check`, confirming no provider-wire or UI fixture drift.
- [x] 9.5 Run live-reference searches such as `git grep -n 'github.com/grafana/ai-sdk/provider/wire\|provider/wire' -- '*.go' '*.md' '*.mod' '*.sum'` and confirm remaining matches, if any, are intentional historical change records rather than source, current docs, godoc, tests, or conformance code.
- [x] 9.6 Confirm `git status --short` contains no generated snapshot changes, no `provider/wire` files, and no staged files before review.
- [x] 9.7 Run `openspec validate extract-provider-wire-server --strict` and fix every validation error before implementation review.
- [x] 9.8 Validate API-call error statuses before HTTP commitment, add panic regression coverage, restore the Go-to-Go compatibility boundary in package godoc, and rerun focused and aggregate validation.
- [x] 9.9 Process non-empty final SSE line bytes returned with `io.EOF`, add valid single-line, valid multiline, and invalid-JSON regression coverage, and rerun focused and aggregate validation.
- [x] 9.10 Reconcile the canonical wire-output stability requirement with the explicitly documented SSE reader EOF correction and rerun strict OpenSpec validation.
- [x] 9.11 Reject body-forbidden HTTP 304 API-call error responses before commitment, add helper and handler regression coverage, align change artifacts, and rerun validation.
- [x] 9.12 Narrow proposal stability claims to canonical encoded bytes and protocol shapes except for the documented robustness corrections, then rerun strict OpenSpec validation.
