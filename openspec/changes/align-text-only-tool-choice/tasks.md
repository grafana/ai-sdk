## 1. Confirm baseline and follow-up scope

- [x] 1.1 Reconfirm `test/conformance/upstream.yaml` and `PARITY.md`; compare the pinned `prepare-tool-choice` implementation/tests and stream/generate preparation paths. Record the same baseline commit and classify the core default, Gateway admission, and request-coverage gaps without upgrading packages.
- [x] 1.2 Identify #201's existing Gateway conformance task/executors and record their revision and Linux/local-Docker availability. Track their matrix checks as integration follow-up, not a shipping dependency. Do not create a replacement matrix.

Baseline evidence: registered baseline remains `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`; pinned helper/tests confirm tools-independent defaulting. Core defaulting and Gateway rejection are bugs; high-level HTTP evidence is a coverage gap. #201 is open on `nrbrd/ai-gateway-conformance` at `e6e690d857962b1d8325806496b5547e7d0622a7` and is not integrated here. Linux Docker server `29.6.1` is available. The owner approved shipping this fix before #201; former tasks 1.3, 5.3, and 5.5 are deferred below, not marked as executed.

## 2. Add failing focused regressions before production fixes

- [x] 2.1 Add table-driven provider-call-options tests in `streamtext_test.go` for omitted choice with absent/empty/nonempty tools, tools filtered to empty globally and by `PrepareStep`, explicit auto/none/required/named choices with empty effective tools, and exact named-choice preservation. Assert the non-nil automatic default directly and observe failures on current core.
- [x] 2.2 Add per-step precedence/reset cases (step override, nil override falling back to configuration/default, later-step non-leakage) and shared-call coverage in `generatetext_test.go` and `agent_test.go`. Assert automatic default and explicit override preservation through `DoStream` for GenerateText and both Agent entry points, including Agent per-call precedence.
- [x] 2.3 Add production-handler HTTP tests in `ai-gateway/providerwire/v4/runtime_test.go` and `stream_test.go` for absent choice versus auto, crossed with absent versus empty tools in both wire modes. Assert forwarded choice/presence, exact-once resolution/correct model method, and successful text. Auto failed on the original main baseline; on #186/#187 retain its existing admission and also verify explicit none/required/named preservation.
- [x] 2.4 Extend HTTP negatives in both modes: auto with provider tools or provider-executed history; auto with each existing unsupported family; malformed/null/unknown/extra-field choice. Retain #186/#187's direct-route function tools/history and explicit choices instead of the original blanket rejection cases. Assert existing fixed response classification, zero resolution/model calls, and no SSE commitment; leave schema constraints unchanged.
- [x] 2.5 Extend `ai-gateway/cmd/grafana-ai-gateway/internal/service/fallback_route_test.go` for auto acceptance/preservation through unary and streaming guards, pre-commit primary-to-secondary failover preserving options, and rejection of non-auto/invalid choices, auto with tool name, actual tools/history, and other unsupported controls before any physical call. Observe the current auto rejection.

## 3. Add red high-level cross-client request evidence

- [x] 3.1 Use exact `ai@7.0.65` in `ai-gateway/test/providerwire-v4/package.json`; after stacking onto #187, retain its existing dependency and lockfile unchanged. Preserve registered package pins and release-age checks. Add actual TypeScript `streamText` calls, not reconstructed `doStream` options.
- [x] 3.2 Extend the Go capture test tooling under `providers/grafana/internal/capture/` with a focused real `StreamText` path over the Grafana client. Select the repository's `go.work` explicitly only when building this high-level probe so the current local core is tested; retain existing `GOWORK=off` independent-client and Gateway checks. Verify source selection rather than silently using the older published root pin.
- [x] 3.3 Extend the existing ProviderWire runtime/command integration tests to capture unmodified inbound HTTP requests for equivalent high-level Go/TypeScript calls without tools or explicit choice. Assert auto independently for each client, backend invocation, and expected text. Include the existing authenticated edge-shim/real-command composition and retain privacy assertions. Confirm the pre-fix core/mapping failures; do not normalize away request fields or demand unrelated byte identity.

## 4. Implement the coordinated narrow fixes

- [x] 4.1 Change the existing default branch in `streamtext.go` to assign auto whenever the effective per-step choice is nil, independently of tools, without mutating configuration or altering explicit/per-step precedence. Make core/shared-caller regressions pass.
- [x] 4.2 Retain #186/#187's shared typed function-tool mapper in `ai-gateway/providerwire/v4/request.go`, which already admits and preserves text-only auto in both modes. Drop the original redundant narrow mapper patch; preserve explicit choices and direct function-tool support alongside nil omission and unchanged schema/error ordering.
- [x] 4.3 Apply the identical no-tools pure-auto exception to `fallbackTextRequest` in `fallback_route.go`; pass original options through unchanged and preserve all other guard/commitment/retry behavior. Make direct guard and failover tests pass.
- [x] 4.4 Run the high-level Go/TypeScript request scenarios after all fixes and verify both reach the provider with the actual default and expected text. Retain existing client serializers and production request schema unchanged.

## 5. Prove parity and preserve safety boundaries

- [x] 5.1 Run `go test ./...` from the root and `(cd ai-gateway && GOWORK=off go test ./providerwire/v4 ./cmd/grafana-ai-gateway/internal/service)`. Also run focused Gateway service fallback tests with `-race`; record results.
- [x] 5.2 Run `mise run test-providerwire-v4` and `mise run test-ai-gateway-command`, including unchanged low-level semantic goldens, strict schema/runtime negatives, the new high-level defaults, authenticated command behavior, and independent-module checks.
- [x] 5.4 Run `mise run test-conformance` and `mise run parity-check`. Investigate unexpected provider/UI/object snapshot differences against the registered baseline rather than regenerating goldens to force success. Add no fabricated recorded/upstream provider inputs and do not rewrite existing ones.
- [x] 5.6 Confirm no frontend wire behavior changed. If UI chunks, SSE framing, headers, or response formats did change, add the required deterministic cross-language scenario in `test/integration/` and run `mise run test-integration` before completion.

## 6. Update durable coverage and completion evidence

- [x] 6.1 Update `test/conformance/PARITY.md` with core defaulting, Gateway HTTP/high-level streaming, and fallback-guard evidence. Correct the trusted-cloud blanket high-level rejection while retaining TypeScript generateText body-header, default unary-token-limit, effectful-fallback, and existing provider-specific gaps. Record any remaining validation gap accurately.
- [x] 6.2 Review implementation against all six spec deltas, record the compatibility impact on custom models/middleware, and document Gateway-first rollout if server/SDK releases are separate. Confirm no public API, auth policy, client stripping workaround, or fallback effect boundary changed.
- [x] 6.3 Verify only intended implementation/tests/coverage/dependency-lock changes are present, run OpenSpec validation for this change, and record successful commands plus residual coverage gaps. Completion requires focused core/HTTP/fallback regressions, actual high-level cross-client command evidence, and direct conformance; #201 matrix replay remains explicit follow-up.

## Original implementation evidence (before rebase)

- Red-first core tests failed on missing automatic choice for absent/empty/filtered tools and shared callers. Handler and fallback tests failed on automatic-choice rejection. Actual high-level Go failed its inbound-auto assertion while TypeScript failed with unsupported tools. All focused regressions pass after the coordinated fixes.
- Both HTTP modes share the table-driven production-handler tests in `runtime_test.go`. Cross-language high-level evidence uses the existing authenticated edge/real-command composition; the narrow Go executable lives at `providers/grafana/internal/capture/testdata/streamtext/main.go` so isolated production-module builds do not acquire test-only root dependencies. The build asserts local core/client source selection via the explicit repository workspace.
- Passed: `go test ./...`; isolated Gateway runtime/service tests; focused root and Gateway fallback race tests; root and focused Gateway `go vet` and `golangci-lint` (zero issues); `mise run test-ai-gateway-command` (25 tests); `mise run parity-check` (including ProviderWire contract checks, baseline validation, provider shape/inventory, and direct conformance); strict OpenSpec validation; `git diff --check`.
- No UI/SSE protocol changes, production dependency-pin changes, fixture expectation changes, provider input changes, or client-side request rewriting. Coverage documentation records observable nonnil automatic choice for custom models/middleware and coordinated Gateway-first deployment/rollback.
- All in-scope implementation tasks are complete. The owner approved moving the unexecuted #201 matrix checks out of this change's completion gates; no paired Gateway fixture or aggregate matrix result is claimed.

## Integration onto #186/#187

- Rebased onto #187 at `7051714218395fe54a0ae561f8def9c9ceaebb24`, including #186. Preserved its mapper, function-tool support, dependency pins, and lockfile; remaining production changes are core defaulting and the fallback pure-auto exception.
- Updated the authenticated fallback command matrix: no-tools auto succeeds in both modes; definitions, non-auto choices, call history, and continuation still cause zero physical calls. Direct tool round trips remain enabled.
- Reconciled spec deltas and coverage claims with the combined stack, including the unary-tool fallback requirement. Original red-first evidence above is historical, not a new red run against #187.
- Combined-stack validation passed: `mise run test`, `mise run build`, `mise run parity-check`, `mise run test-ai-gateway-command` (28 tests), `mise run vet`, `mise run lint` (zero issues), focused core and Gateway fallback/runtime race tests, strict OpenSpec validation, and `git diff --check`. Direct function-tool command round trips and text-only fallback both pass.
- No mapper, production dependency, package-lock, provider-input, or fixture-expectation changes remain relative to #187. Rewritten history has not been pushed.

## Deferred validation for #201 integration

These checks remain pending follow-up for #201, not completed tasks or prerequisites to shipping this PR:

- Former 1.3: reproduce the paired pre-fix failure on a pre-fix revision (for example `ba4e6d47`) with #201's harness, retaining inbound request/backend-count evidence against unchanged fixture inputs and goldens.
- Former 5.3: after integrating this fix into #201, run `SCENARIO=anthropic/upstream/text-generation CLIENT=typescript mise run test-conformance-gateway` and the same with `CLIENT=go`. Require both to reach the backend and pass existing `expected.jsonl` and `expected-requests.jsonl`; retain harness evidence without rewriting requests or goldens.
- Former 5.5: rerun the broader Gateway matrix and report remaining unsupported-capability failures separately. Do not expand this fix to achieve arbitrary pass-count equality.
