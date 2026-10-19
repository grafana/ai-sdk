# Tasks

## 1. Apache provider prerequisite and independent client

- [x] 1.1 Add failing focused domain tests for generated URL originalUrl omission/empty/opaque values against the exact pinned SharedV4FileDataUrl definition, then add narrow provider representation/codec preservation; verify `go test ./provider` and document the field in godoc. Preserve the new pointer through middleware simulate-streaming, Gateway unary-to-stream projection and Grafana unary reconstruction; add a focused conversion regression test. Keep request behavior unchanged except necessary shared round-trips and retain the existing high-level generated-file API/download policy.
- [x] 1.2 Add independent Grafana unary/SSE file tests for bytes/base64/empty data, URL/originalUrl, metadata omission/empty/nested values, missing/malformed consumed fields, ordering and ignored unregistered filename; extend private decoding and verify `GOWORK=off go test ./...` in providers/grafana with the required candidate prerequisite overlay reported separately.
- [x] 1.3 Add Grafana hostile-server oversized event/response, cancel/backpressure and independent consumer capture cases using existing helpers; verify focused tests and document supported generated output in docs/providers/grafana-gateway.md.

## 2. Strict Gateway generated media projection

- [x] 2.1 Add failing unary/SSE strict schema and encoder tests, then extend private generated-file DTOs and schema arms; verify data/URL/originalUrl/mediaType/metadata fidelity and that provider-local filename never appears or consumes the emitted budget. Run the focused Gateway v4 tests and schema suite.
- [x] 2.2 Extend preflight before base64 allocation and final escaped JSON/frame checks; test below/at/above response and frame budgets, byte expansion, escaped URLs/metadata, aggregate multi-file limits and malformed union fields. Assert no 200 or partial media frame on failure and preserve existing reasoning-file tests.
- [x] 2.3 Integrate file events into existing stream state and configured fallback; test file-only streams, text/file ordering, eligible preselection fallback, file-then-error, selected encoding failure, cancellation and no secondary invocation after commitment. Run v4 and fallback-focused tests.
- [x] 2.4 Test server capture disabled/enabled independently of consumer capture with ordinary opaque application values and media metadata; verify identical caller responses and expected capture destination behavior. Update narrative support/bounds documentation alongside these tests.

## 3. Registered-client and frontend proof

- [x] 3.1 Add deterministic generated-file responses to the existing AGPL HTTP testserver and differential/runtime cases; verify registered TS doGenerate/doStream and Go DoGenerate/DoStream consume identical data/URL outputs and raw response bodies/frames pass strict schemas. Use authentic pinned source/tests for expectations and label fakes synthetic.
- [x] 3.2 Exercise high-level generated-file access and frontend assembly through the Apache Grafana client using a standalone fake HTTP Gateway, by extending the existing generated-file scenario/suite, with a deterministic data: URL where resolution is needed; parse SSE through parseJsonEventStream/uiMessageChunkSchema and run `mise run test-integration`. No Apache test may import Gateway.
- [x] 3.3 Reuse a focused existing native generated-file producer witness if one already exists, without fabricating recorded/upstream inputs. If current native adapters do not emit ordinary generated files, record that support/evidence boundary; do not introduce a new native provider capability to satisfy this transport issue. Update PARITY.md only with stable achieved evidence and explicitly state remaining native/live/published adoption limits.

## 4. Integrated validation and review handoff

- [x] 4.1 Run `TMPDIR=/private/tmp GATEWAY_TEST_GOWORK=$PWD/go.gateway.work mise exec -- pnpm --dir ai-gateway/test/providerwire-v4 test:contract`, relevant command scenarios, Gateway and Grafana Go suites, meaningful race coverage, `AI_SDK_UPSTREAM_ROOT=/path/to/registered-upstream mise run parity-check`, and required CONTRIBUTING candidate-source checks; the canonical baseline passes parity-check and all 110 client tests. Earlier direct pnpm Anthropic path mismatches were committed-pin execution without GATEWAY_TEST_GOWORK, not candidate-source known failures; do not exempt them or rewrite authentic expectations.
- [x] 4.2 Verify Apache-only GOWORK=off dependency boundaries and committed Gateway pins separately from local source overlays. Record any required immutable prerequisite version/publication gap precisely; do not introduce committed replace directives or claim source-overlay success proves publication.
- [x] 4.3 Run OpenSpec strict validation, review the complete diff against #110 and WP16 ownership, leave implementation reports untracked/ignored, and provide independent reviewer with exact tests, known failures, file ownership and unresolved delivery limits.

## Workflow follow-up

- Archive only after implementation review and repository workflow requirements are satisfied.
- Publishing immutable prerequisites, pushing and opening PRs require separate user authorization under the workday policy.
