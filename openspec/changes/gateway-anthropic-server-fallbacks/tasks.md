# Tasks

Paths are relative to `ai-gateway/` unless they start with a top-level directory. Write each task's tests first, and confirm they fail before implementing.

## 1. Policy configuration

- [ ] 1.1 Add failing cases to `pkg/internal/config/file_test.go` (`TestParseModelCatalog_ServerFallbacksPolicy`, table-driven):
  - valid on an `anthropic` primary and on a fallback entry;
  - `serverFallbacks: null` behaves as absent;
  - rejected on `openai` and `openai-compatible`;
  - rejected for an empty policy, an empty, blank, invalid-UTF-8, overlong or repeated model, or an unknown policy field;
  - a repeated `{provider, model}` with a different policy still fails as a repeated candidate;
  - errors name the path without echoing values.

  Verify with `go test ./pkg/internal/config/...`, which fails.
- [ ] 1.2 Add `Primary.ServerFallbacks` and its validation in `File.Validate`, keying duplicate detection on `{Provider, Model}` (design D1/D2). Verify that 1.1 passes and that the existing config tests are unchanged.
- [ ] 1.3 Add failing cases to `pkg/internal/service/catalog_test.go`:
  - `TestBuildCatalog_RejectsServerFallbacksOnNonAnthropic`: `BuildCatalog` with a policy on an `openai` or `openai-compatible` descriptor (primary and fallback entry) returns an error that echoes no policy values or secrets, and no catalog;
  - `TestBuildCatalog_ServerFallbacksLeaveDiscoveryUnchanged`: `ListModels` rows and their `catalog.ConfiguredCandidate` facts are identical with and without a policy.

  Implement the construction check in `service/catalog.go` (design D2). Verify with `go test ./pkg/internal/service/...`.

## 2. Native enforcement (configured and BYOK)

- [ ] 2.1 Add failing unit cases to `pkg/internal/nativeoptions/options_test.go` (`TestAnthropic_ServerFallbackPolicy`):
  - allowed chain, allowed default;
  - disallowed model, repeated entry, default without `allowDefault`, zero policy;
  - null/absent/`[]` no-op, with MCP checks still applied;
  - exact-byte comparison (case and whitespace variants refused);
  - skills still refused, MCP checks unchanged;
  - `UnrestrictedAnthropicFallbacks()`: any chain, including repeated models and models outside any allowlist, and `"default"` pass; skills are still refused; invalid MCP configuration and unconfigured MCP history are still refused.

  Verify with `go test ./pkg/internal/nativeoptions/...`, which fails.
- [ ] 2.2 Implement `AnthropicFallbackPolicy` and the `Anthropic(policy)` validator (design D3). Wire one validator per descriptor in `service/catalog.go`, and `UnrestrictedAnthropicFallbacks()` at both wraps in `byok/model.go` (whole chain, lines 67-68; each Anthropic account, line 78). Verify that 2.1 passes and that `go build ./...` succeeds.
- [ ] 2.3 Update `service/native_options_test.go`:
  - `TestNativeOptions_BypassAndOrdinaryControls`: the existing `fallback` row stays denied without a policy.
  - Add `TestNativeServerFallbacks_ConfiguredPolicy` (unary × stream × direct/configured-fallback). Allowed chain/default reach native with the unchanged body and the right `anthropic-beta`; disallowed cases give 400 with zero native requests.
  - Primary without a policy and second candidate allowing: the primary makes no request, the second is selected, and the overview shows failed then selected. The failed primary's error summary is exactly the fixed message `gateway: unsupported request` (no other fields, no model names).
  - Translator-added fallbacks are checked per candidate.

  In `service/routed_test.go`, add `TestRoutedSelection_ServerFallbackPolicy` (unary and stream): route A without a policy plus `models: [B]`, where B has another destination and allows the chain, refuses A with no native request and selects B; routes A (no policy) and B (allowing) sharing one destination give the fixed 400 with no native request for A with `models: [B]`, and one allowed native request for B with `models: [A]`. Verify with `go test ./pkg/internal/service/...`.
- [ ] 2.4 Replace the BYOK refusal tests with forwarding tests (design D4):
  - `byok/model_test.go` `TestNew_AnthropicNativeOptionGuards`: the "native fallback" row (`"default"`) becomes allowed; add a chain row with a repeated model. Assert one native request whose body carries `fallbacks` and whose `anthropic-beta` is the default or explicit server-side fallback beta.
  - `service/native_options_test.go` `TestBYOKSelection_NativeOptionGuards`: the "native routing" row (`"default"`) becomes allowed; add a chain row. Unary and stream.
  - Configured policy does not apply: a chain naming a model no configured route allows, and `"default"`, reach the BYOK native request unchanged.
  - `service/routed_test.go` `TestRoutedSelection_BYOKNativeOptionRefusal`: keep the skills rows; add chain and default forwarding rows to both subtests (all-Anthropic `models: ["anthropic/y"]`, mixed `models: ["openai/y"]`). Each attempted Anthropic account's request carries `fallbacks` and the beta; the OpenAI request carries none.
  - `service/translation_test.go`: translator-added fallbacks reach the BYOK account's native request with the beta.
  - With a fake Anthropic that returns a `fallback` block (JSON for unary, SSE for stream; `nativeServer` answers JSON only), assert the BYOK overview's selected attempt carries `modelId` (the native model) and `servedModelId` (`to.model`), unary and stream.

  `nativeRequest` (`service/translation_test.go:35`) records the body but not headers; extend it with `anthropic-beta`. Verify with `go test ./pkg/internal/byok/... ./pkg/internal/service/...`.
- [ ] 2.5 Add a both-client command case to `test/providerwire-v4/gateway-command.test.ts` ("native option forwarding"). A configured policy allows the chain and `"default"`, and the fake Anthropic receives each; a disallowed model gets the fixed 400. The existing configured refusal in "protects Anthropic execution controls without rejecting safe contextual history" (no policy) stays. For BYOK, use `startCloudGateway` (the native proxy routes `api.anthropic.com` to a `FakeAnthropic`; the "Cloud BYOK through the platform host" describe already drives both clients this way): a chain and `"default"` reach the fake unchanged with the beta through both clients, unary and stream, and with the `wp112-fallback-marker` prompt both clients see `servedModelId` `claude-b` on the BYOK selected attempt. In the existing "returns and replays the Anthropic fallback marker through both clients" test (`gateway-command.test.ts:2950`), assert that both clients see `servedModelId` equal to the marker's `to.model` on the selected attempt, unary and stream. Verify with `mise run test-ai-gateway-command`.

## 3. Served model in the execution overview

- [ ] 3.1 Add failing cases to `internal/execution/overview_test.go` and `attempts_test.go`:
  - the selected attempt gets `servedModelId` from the latest valid `anthropic.fallback` marker;
  - no marker, an invalid marker (empty `to.model`, or raw bytes that are invalid UTF-8), or a failed/canceled attempt gives no field;
  - recording, including markers observed after sealing, leaves the same attempts in the same order, differing only by `servedModelId`;
  - JSON with no marker is byte-identical, and the `gateway.routing` summary is unchanged with a marker.

  Add a stream case where the marker arrives after the first part (mid-output). Add a two-segment case to `providerwire/v4/stream_segments_test.go` (`TestStreamingSegments_ExecutionOverview`): each finish carries the latest marker so far. Add ProviderWire cases in `providerwire/v4` asserting the error metadata carries `servedModelId` when a marker is followed by:
  - a committed error part (`processStreamPart`, `stream.go:399`);
  - each terminal wait (closed, deadline, canceled, idle timeout, invalid output; `writeStreamTerminalForWait`), including a deadline after a hop;
  - an adapter-failure terminal error (`handleStreamPartResult`, `stream.go:316-325`);
  - a unary invalid-output or mapping error after a selected result (`ServeHTTP`, `handler.go:100`, 105). Verify with `go test ./internal/execution/... ./providerwire/v4/...`, which fails.
- [ ] 3.2 Implement `Attempt.ServedModelID` and the runtime recording for unary results and `Stream.Next` (design D7). Publish the current served model on every carrier of the selected attempt: `providerwire/v4/handler.go` unary result and errors (lines 97-105); `stream.go` finish and committed error (`processStreamPart`, lines 421, 399), terminal-wait errors (closed, deadline, canceled, idle timeout, invalid output) via `writeStreamTerminalForWait` (lines 475-477) and the adapter-failure terminal error (`handleStreamPartResult`, lines 316-325). Do the same in `internal/testadapter`, and add a served-model case to `TestTestAdapter_MatchesProviderWire` (`internal/execution/adapter_test.go:457`). Verify with `go test -race ./internal/execution/... ./providerwire/v4/...`.
- [ ] 3.3 Add optional `servedModelId` (`minLength: 1`, selected only) to `providerwire/v4/schema/gateway_execution.json`. Add accept/reject cases in `providerwire/v4/schema_test.go`, `test/providerwire-v4/schema.test.ts` and `schema-definitions.test.ts`. Verify with `mise run test-providerwire-v4`.
- [ ] 3.4 Add a service test with a fake Anthropic that returns a `fallback` block, unary and stream, asserting `modelId` is the backend and `servedModelId` is `to.model`. Verify with `go test ./pkg/internal/service/...`.

## 4. Documentation

- [ ] 4.1 Document `serverFallbacks` in the `model_catalog` document described by `ai-gateway/README.md` "Configure models": an example, per-candidate scope, deny by default, Anthropic only, and that `allowDefault` accepts Anthropic's model choice and cost. Update the limits row (`README.md:141`, under the 64 KiB `model_catalog` bound) so the 2,048-byte per-value bound also lists each `serverFallbacks.models` entry. Verify with `mise run lint-docs`.
- [ ] 4.2 Add a "Use Anthropic server-side fallback" subsection to `docs/providers/grafana-gateway.md` (near "Configure fallback"). Cover: administrator policy, refused-request behavior, calls with your own key (server-side fallback is forwarded without a policy and billed to the caller's account; `"default"` lets Anthropic choose the model), relation to Gateway fallback and request routing (a refusing candidate advances, possibly to a non-Anthropic or routed candidate that ignores `anthropic.fallbacks`; each routed attempt applies its own route's policy; give routes that share a destination one policy), that callers control per-entry `speed`, `max_tokens` and `thinking` within an allowed model, and `servedModelId` under "Inspect execution overviews and failures". Verify with `mise run lint-docs`.
- [ ] 4.3 Update the candidate option checks in `docs/concepts/gateway-architecture.md` step 4, and the "Diagnostics; execution overview" row in `docs/concepts/gateway-glossary.md`. Add a "Server-side fallback" row distinguishing it from Gateway fallback, routing `models`, and retry, and from the `gateway-ordered-text-fallback` requirement "Native fallback output identity". Check the diagrams for drift. Verify with `mise run lint-docs`.
- [ ] 4.4 Update godoc for `config.Primary`, `nativeoptions`, and `execution.Attempt`. Verify with `go doc` on each.

## 5. Recorded fixtures through the Gateway command

- [ ] 5.1 Before task 2.2, add a "recorded Anthropic server-side fallback" describe to `test/providerwire-v4/gateway-command.test.ts` (design D8). For each of `anthropic/recorded/server-side-fallback-chain` and `-default`:
  - start a local backend that serves the unchanged `input.chunks.txt` as SSE and records requests;
  - start `GatewayProcess` with a model catalog (`GF_PLUGIN_MODEL_CATALOG`) whose `claude-opus-5` Anthropic route carries the fixture's fixed policy;
  - stream with the TypeScript client (`streamText` + `toUIMessageStream`) and the Go client (`captureGoClient`, `recorded-ui` mode) using the fixture's `providerOptions`;
  - assert chunks equal `expected.jsonl` and each client's normalized requests (`normalizeRequestSnapshot`, `writeRequestSnapshots` from `test/conformance/tools/common.mts`) equal `expected-requests.jsonl`.

  Confirm it fails with 400 before 2.2 and passes after. Verify with `mise run test-ai-gateway-command`.
- [ ] 5.2 Add the paired refusal: the same fixtures through a route without a policy, and with the other fixture's policy, give both clients the fixed 400 with zero backend requests. Verify with `mise run test-ai-gateway-command`.
- [ ] 5.3 Confirm direct provider replay is unchanged and the inputs are untouched. Verify with `mise run test-conformance` and an empty `git status --porcelain test/conformance`.

## 6. Integration checks

- [ ] 6.1 Run `mise run fmt`, `mise run vet`, `mise run lint`, `mise run test`, `mise run test-ai-gateway` and `mise run test-race`, and confirm they pass.
- [ ] 6.2 Run `mise run test-providerwire-v4`, `mise run test-ai-gateway-command` and `mise run test-ai-gateway-source-integration`, and confirm they pass. `mise run test-integration` is not required, because SSE framing and UI chunks are unchanged (design D10).
- [ ] 6.3 Run `mise run validate-parity-baseline` and `mise run parity-check`, and confirm that no pins, recordings or expectations changed.
- [ ] 6.4 Run `openspec validate gateway-anthropic-server-fallbacks --strict` and confirm it passes.

## Workflow follow-up

- nara's decisions are recorded in design.md "Resolved decisions" (BYOK forwards, hop-only `servedModelId`, no discovery exposure).
- Open a PR titled `feat(ai-gateway): allow Anthropic server-side fallbacks under operator policy` with `Closes #562`. State that policy evidence is synthetic, that group 5 replays the unchanged recordings through the real command, and that it stands in for the issue's `mise run test-conformance-gateway` acceptance, which is not on main.
- The PR also states that, by nara's decision, BYOK forwards server-side fallback, so the issue item "Disallowed models or default are still refused before native I/O, with tests for configured and BYOK modes" is met by configured refusal tests plus BYOK tests that prove forwarding and that configured policies do not apply.
