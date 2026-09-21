## 1. Confirm baseline and integration prerequisites

- [ ] 1.1 Reconfirm `test/conformance/upstream.yaml` and `PARITY.md`; compare the pinned `prepare-tool-choice` implementation/tests and stream/generate preparation paths. Record the same baseline commit and classify the core default, Gateway admission, and request-coverage gaps without upgrading packages.
- [ ] 1.2 Identify an implementation integration branch containing #201's existing Gateway conformance task/executors. Record its revision and Linux/local-Docker availability; if unavailable, mark the paired-fixture gate blocked while allowing focused work to proceed. Do not create a replacement matrix.
- [ ] 1.3 On that integration branch, run the existing `anthropic/upstream/text-generation` Go and TypeScript Gateway cases before behavioral edits; retain actual inbound request/backend-count and expectation-failure evidence. Use unchanged fixture inputs and UI/backend goldens.

## 2. Add failing focused regressions before production fixes

- [ ] 2.1 Add table-driven provider-call-options tests in `streamtext_test.go` for omitted choice with absent/empty/nonempty tools, tools filtered to empty globally and by `PrepareStep`, explicit auto/none/required/named choices with empty effective tools, and exact named-choice preservation. Assert the non-nil automatic default directly and observe failures on current core.
- [ ] 2.2 Add per-step precedence/reset cases (step override, nil override falling back to configuration/default, later-step non-leakage) and shared-call coverage in `generatetext_test.go` and `agent_test.go`. Assert automatic default and explicit override preservation through `DoStream` for GenerateText and both Agent entry points, including Agent per-call precedence.
- [ ] 2.3 Add production-handler HTTP tests in `ai-gateway/providerwire/v4/runtime_test.go` and `stream_test.go` for absent choice versus auto, crossed with absent versus empty tools in both wire modes. Assert forwarded choice/presence, exact-once resolution/correct model method, and successful text; confirm auto currently fails.
- [ ] 2.4 Extend HTTP negatives in both modes: none/required/named with no tools; auto with nonempty function/provider tools or tool history; auto with each existing unsupported family; malformed/null/unknown/extra-field choice. Assert existing fixed response classification, zero resolution/model calls, and no SSE commitment; leave schema constraints unchanged.
- [ ] 2.5 Extend `ai-gateway/cmd/grafana-ai-gateway/internal/service/fallback_route_test.go` for auto acceptance/preservation through unary and streaming guards, pre-commit primary-to-secondary failover preserving options, and rejection of non-auto/invalid choices, auto with tool name, actual tools/history, and other unsupported controls before any physical call. Observe the current auto rejection.

## 3. Add red high-level cross-client request evidence

- [ ] 3.1 Add exact `ai@7.0.65` to `ai-gateway/test/providerwire-v4/package.json` and update the existing `test/pnpm-lock.yaml` with normal workspace tooling; preserve registered package pins and release-age checks. Add actual TypeScript `streamText` calls, not reconstructed `doStream` options.
- [ ] 3.2 Extend the Go capture test tooling under `providers/grafana/internal/capture/` with a focused real `StreamText` path over the Grafana client. Select the repository's `go.work` explicitly only when building this high-level probe so the current local core is tested; retain existing `GOWORK=off` independent-client and Gateway checks. Verify source selection rather than silently using the older published root pin.
- [ ] 3.3 Extend the existing ProviderWire runtime/command integration tests to capture unmodified inbound HTTP requests for equivalent high-level Go/TypeScript calls without tools or explicit choice. Assert auto independently for each client, backend invocation, and expected text. Include the existing authenticated edge-shim/real-command composition and retain privacy assertions. Confirm the pre-fix core/mapping failures; do not normalize away request fields or demand unrelated byte identity.

## 4. Implement the coordinated narrow fixes

- [ ] 4.1 Change the existing default branch in `streamtext.go` to assign auto whenever the effective per-step choice is nil, independently of tools, without mutating configuration or altering explicit/per-step precedence. Make core/shared-caller regressions pass.
- [ ] 4.2 In `ai-gateway/providerwire/v4/request.go`, decode schema-validated choice into a private typed representation, accept only absent or pure auto with no tools, and preserve accepted auto in `provider.CallOptions.ToolChoice`. Keep absence nil and all other supported-subset/schema/error-order behavior intact; make both-mode HTTP tests pass.
- [ ] 4.3 Apply the identical no-tools pure-auto exception to `fallbackTextRequest` in `fallback_route.go`; pass original options through unchanged and preserve all other guard/commitment/retry behavior. Make direct guard and failover tests pass.
- [ ] 4.4 Run the high-level Go/TypeScript request scenarios after all fixes and verify both reach the provider with the actual default and expected text. Retain existing client serializers and production request schema unchanged.

## 5. Prove parity and preserve safety boundaries

- [ ] 5.1 Run `go test ./...` from the root and `(cd ai-gateway && GOWORK=off go test ./providerwire/v4 ./cmd/grafana-ai-gateway/internal/service)`. Also run focused Gateway service fallback tests with `-race`; record results.
- [ ] 5.2 Run `mise run test-providerwire-v4` and `mise run test-ai-gateway-command`, including unchanged low-level semantic goldens, strict schema/runtime negatives, the new high-level defaults, authenticated command behavior, and independent-module checks.
- [ ] 5.3 With #201 integrated, run `SCENARIO=anthropic/upstream/text-generation CLIENT=typescript mise run test-conformance-gateway` and the same with `CLIENT=go`. Require both to reach the backend and pass existing `expected.jsonl` and `expected-requests.jsonl`; retain harness evidence. Keep this task incomplete if the harness/environment is unavailable or either case fails.
- [ ] 5.4 Run `mise run test-conformance` and `mise run parity-check`. Investigate unexpected provider/UI/object snapshot differences against the registered baseline rather than regenerating goldens to force success. Add no fabricated recorded/upstream provider inputs and do not rewrite existing ones.
- [ ] 5.5 Re-run the broader Gateway matrix when the integrated harness/environment permits and report remaining unsupported-capability failures separately from the paired text acceptance gate. Do not expand scope to achieve arbitrary pass-count equality.
- [ ] 5.6 Confirm no frontend wire behavior changed. If UI chunks, SSE framing, headers, or response formats did change, add the required deterministic cross-language scenario in `test/integration/` and run `mise run test-integration` before completion.

## 6. Update durable coverage and completion evidence

- [ ] 6.1 Update `test/conformance/PARITY.md` with core defaulting, Gateway HTTP/high-level streaming, and fallback-guard evidence. Correct the trusted-cloud blanket high-level rejection while retaining TypeScript generateText body-header, default unary-token-limit, actual-tool, and existing provider-specific gaps. Record any remaining validation gap accurately.
- [ ] 6.2 Review implementation against all five spec deltas, record the compatibility impact on custom models/middleware, and document Gateway-first rollout if server/SDK releases are separate. Confirm no public API, auth policy, client stripping workaround, or fallback effect boundary changed.
- [ ] 6.3 Verify only intended implementation/tests/coverage/dependency-lock changes are present, run OpenSpec validation for this change, and record successful commands plus residual environment blockers. Do not declare #202 complete until the required paired fixture and direct conformance gates pass.
