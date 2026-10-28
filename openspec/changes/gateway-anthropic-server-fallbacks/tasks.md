# Tasks

Paths are relative to `ai-gateway/` unless they start with a top-level directory. Write each task's tests first, and confirm they fail before implementing.

## 1. Policy configuration

- [ ] 1.1 Add failing cases to `cmd/grafana-ai-gateway/internal/config/file_test.go` (`TestLoadFile_ServerFallbacksPolicy`, table-driven):
  - valid on an `anthropic` primary and on a fallback entry;
  - `serverFallbacks: null` behaves as absent;
  - rejected on `openai` and `openai-compatible`;
  - rejected for an empty policy, an empty, blank, invalid-UTF-8, overlong or repeated model, or an unknown policy field;
  - a repeated `{provider, model}` with a different policy still fails as a repeated candidate;
  - errors name the path without echoing values.

  Verify with `go test ./cmd/grafana-ai-gateway/internal/config/...`, which fails.
- [ ] 1.2 Add `Primary.ServerFallbacks` and its validation in `File.Validate`, keying duplicate detection on `{Provider, Model}` (design D1/D2). Verify that 1.1 passes and that the existing config tests are unchanged.
- [ ] 1.3 Add failing cases to `cmd/grafana-ai-gateway/internal/service/catalog_test.go`:
  - `TestBuildCatalog_RejectsServerFallbacksOnNonAnthropic`: `BuildCatalog` with a policy on an `openai` or `openai-compatible` descriptor (primary and fallback entry) returns an error that echoes no policy values or secrets, and no catalog;
  - `TestBuildCatalog_ServerFallbacksLeaveDiscoveryUnchanged`: `ListModels` rows and their `catalog.ConfiguredCandidate` facts are identical with and without a policy.

  Implement the construction check in `service/catalog.go` (design D2). Verify with `go test ./cmd/grafana-ai-gateway/internal/service/...`.

## 2. Native enforcement (configured and BYOK)

- [ ] 2.1 Add failing unit cases to `cmd/grafana-ai-gateway/internal/nativeoptions/options_test.go` (`TestAnthropic_ServerFallbackPolicy`):
  - allowed chain, allowed default;
  - disallowed model, repeated entry, default without `allowDefault`, zero policy;
  - null/absent/`[]` no-op, with MCP checks still applied;
  - exact-byte comparison (case and whitespace variants refused);
  - skills still refused, MCP checks unchanged.

  Verify with `go test ./cmd/grafana-ai-gateway/internal/nativeoptions/...`, which fails.
- [ ] 2.2 Implement `AnthropicFallbackPolicy` and the `Anthropic(policy)` validator (design D3). Wire one validator per descriptor in `service/catalog.go`, and the zero policy in `byok/model.go`. Verify that 2.1 passes and that `go build ./...` succeeds.
- [ ] 2.3 Update `service/native_options_test.go`:
  - `TestNativeOptions_BypassAndOrdinaryControls`: the existing `fallback` row stays denied without a policy.
  - Add `TestNativeServerFallbacks_ConfiguredPolicy` (unary × stream × direct/configured-fallback). Allowed chain/default reach native with the unchanged body and the right `anthropic-beta`; disallowed cases give 400 with zero native requests.
  - Primary without a policy and second candidate allowing: the primary makes no request, the second is selected, and the overview shows failed then selected. The failed primary's error summary is exactly the fixed message `gateway: unsupported request` (no other fields, no model names).
  - Translator-added fallbacks are checked per candidate.

  Verify with `go test ./cmd/grafana-ai-gateway/internal/service/...`.
- [ ] 2.4 Extend the BYOK refusals:
  - `byok/model_test.go` `TestNew_AnthropicNativeOptionGuards`: chain and default;
  - `service/native_options_test.go` `TestBYOKSelection_NativeOptionGuards`: a chain naming a model a configured route allows, unary and stream.

  Assert zero account attempts and no overview. Verify with `go test ./cmd/grafana-ai-gateway/internal/byok/... ./cmd/grafana-ai-gateway/internal/service/...`.
- [ ] 2.5 Add a both-client command case to `test/providerwire-v4/gateway-command.test.ts` ("native option forwarding"). A configured policy allows the chain and `"default"`, and the fake Anthropic receives each; a disallowed model and a BYOK chain get the fixed 400. When the fake Anthropic returns a `fallback` block, both clients see `servedModelId` equal to its `to.model` on the selected attempt. Verify with `mise run test-ai-gateway-command`.

## 3. Served model in the execution overview

- [ ] 3.1 Add failing cases to `internal/execution/overview_test.go` and `attempts_test.go`:
  - the selected attempt gets `servedModelId` from the last valid `anthropic.fallback` marker;
  - no marker, an invalid marker (empty or invalid UTF-8 `to.model`), or a failed/canceled attempt gives no field;
  - recording, including markers observed after sealing, leaves the same attempts in the same order, differing only by `servedModelId`;
  - JSON with no marker is byte-identical.

  Add a stream case where the marker arrives after the first part (mid-output). Add ProviderWire cases in `providerwire/v4` asserting the error metadata carries `servedModelId` when a marker is followed by:
  - a committed error part (`processStreamPart`, `stream.go:438`);
  - each terminal wait (closed, deadline, canceled, idle timeout; `writeStreamTerminalForWait`), including a deadline after a hop;
  - an adapter-failure terminal error (`handleStreamPartResult`, `stream.go:342-344`);
  - a unary error after a selected result (`ServeHTTP`, `handler.go:110`). Verify with `go test ./internal/execution/... ./providerwire/v4/...`, which fails.
- [ ] 3.2 Implement `Attempt.ServedModelID` and the runtime recording for unary results and `Stream.Next` (design D7). Publish the current served model on every carrier of the selected attempt: `providerwire/v4/handler.go` unary result and error; `stream.go` finish and committed error (`processStreamPart`, lines 473, 438), terminal-wait errors (closed, deadline, canceled, idle timeout) via `writeStreamTerminalForWait` (lines 547-549) and the adapter-failure terminal error (`handleStreamPartResult`, lines 342-344). Do the same in `internal/testadapter`, and add a served-model case to `TestTestAdapter_MatchesProviderWire` (`internal/execution/adapter_test.go:370`). Verify with `go test -race ./internal/execution/... ./providerwire/v4/...`.
- [ ] 3.3 Add optional `servedModelId` (`minLength: 1`, selected only) to `providerwire/v4/schema/gateway_execution.json`. Add accept/reject cases in `test/providerwire-v4/schema.test.ts` and `schema-definitions.test.ts`. Verify with `mise run test-providerwire-v4`.
- [ ] 3.4 Add a service test with a fake Anthropic that returns a `fallback` block, unary and stream, asserting `modelId` is the backend and `servedModelId` is `to.model`. Verify with `go test ./cmd/grafana-ai-gateway/internal/service/...`.

## 4. Documentation

- [ ] 4.1 Document `serverFallbacks` in `ai-gateway/README.md` "Configure models": an example, per-candidate scope, deny by default, Anthropic only, and that `allowDefault` accepts Anthropic's model choice and cost. Update the limits row (`README.md:127`) so the 2,048-byte per-value bound also lists each `serverFallbacks.models` entry. Verify with `mise run lint-docs`.
- [ ] 4.2 Add a "Use Anthropic server-side fallback" subsection to `docs/providers/grafana-gateway.md` (near "Configure fallback"). Cover: administrator policy, refused-request behavior, refusal for calls with your own key, relation to Gateway fallback (a refusing candidate advances, possibly to a non-Anthropic candidate that ignores `anthropic.fallbacks`), that callers control per-entry `speed`, `max_tokens` and `thinking` within an allowed model, and `servedModelId` under "Inspect execution overviews and failures". Verify with `mise run lint-docs`.
- [ ] 4.3 Update the candidate option checks in `docs/concepts/gateway-architecture.md` step 4, and the "Diagnostics; execution overview" row in `docs/concepts/gateway-glossary.md`. Add a "Server-side fallback" row distinguishing it from Gateway fallback, retry and "native fallback output identity". Check the diagrams for drift. Verify with `mise run lint-docs`.
- [ ] 4.4 Update godoc for `config.Primary`, `nativeoptions`, and `execution.Attempt`. Verify with `go doc` on each.

## 5. Conformance matrix (gated on PR #201 merging)

- [ ] 5.1 Once #201 is on main, add a harness-owned allowlist keyed by fixture case name (`TestCase.name`, not the client-suffixed row ID): `anthropic/recorded/server-side-fallback-chain` and `anthropic/recorded/server-side-fallback-default`, two fixture cases (four client rows). Extend `test/conformance/tools/gateway-runtime.mts` `Route`/`catalog()` to emit `primary.serverFallbacks` only for rows of those cases (design D8). Add a unit test in `gateway-runtime.test.mts` asserting both client rows of each case get the policy and a case outside the allowlist gets none. Verify with `mise run test-conformance-gateway-harness`.
- [ ] 5.2 Confirm that both client rows of `anthropic/recorded/server-side-fallback-chain` and `-default` pass, and that `git status test/conformance/anthropic` is clean. Verify with `mise run test-conformance-gateway`.

## 6. Integration checks

- [ ] 6.1 Run `mise run fmt`, `mise run vet`, `mise run lint`, `mise run test`, `mise run test-ai-gateway` and `mise run test-race`, and confirm they pass.
- [ ] 6.2 Run `mise run test-providerwire-v4`, `mise run test-ai-gateway-command` and `mise run test-ai-gateway-source-integration`, and confirm they pass. `mise run test-integration` is not required, because SSE framing and UI chunks are unchanged (design D10).
- [ ] 6.3 Run `mise run validate-parity-baseline` and `mise run parity-check`, and confirm that no pins, recordings or expectations changed.
- [ ] 6.4 Run `openspec validate gateway-anthropic-server-fallbacks --strict` and confirm it passes.

## Workflow follow-up

- Get nara's confirmations in design.md (BYOK refusal, hop-only `servedModelId`) before implementation starts.
- If #201 has not merged when this change is ready, land groups 1-4 and 6, and track group 5 on #201 or as a follow-up.
- Open a PR titled `feat(ai-gateway): allow Anthropic server-side fallbacks under operator policy`. Use `Refs #562`, not a closing keyword, unless group 5 passes in the same PR. State that the policy evidence is synthetic and the matrix rows replay unchanged recordings.
- Do not archive the change or close #562 until 5.2 passes.
