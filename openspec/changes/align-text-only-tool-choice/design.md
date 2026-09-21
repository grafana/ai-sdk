## Context

Issue #202 reports a 0/59 TypeScript versus 7/59 Go Anthropic Gateway matrix result. Those counts are reported evidence, not a reproduction performed for this proposal. The planning checkout (`f6b6033c`) lacks #201's `test-conformance-gateway` task and executors.

The relevant execution path is:

1. `streamtext.go:475-638` resolves configured and `PrepareStep` options, filters active tools, then creates `provider.CallOptions`. At line 607 its automatic-choice default incorrectly requires nonempty tools.
2. `generatetext.go:29-35` and `agent.go:251-280` use the same streaming engine. Go `GenerateText` does not call the provider's `DoGenerate` directly.
3. `providers/grafana/request.go:68-80` preserves an explicitly supplied choice. Neither client serialization nor fixture selection causes the omission.
4. `ai-gateway/providerwire/v4/handler.go:175-224` validates the complete schema before `mapWireRequest`; `request.go:98-100` currently rejects any choice. Unary and streaming share this mapping.
5. `ai-gateway/cmd/grafana-ai-gateway/internal/service/fallback_route.go:26-28` independently rejects any nonnil choice on fallback routes. Passing auto through the mapper without updating this guard would leave these text routes broken.
6. `providers/anthropic/convert_request.go:344-351` omits automatic choice when tools are empty, so unchanged direct Anthropic backend snapshots cannot expose the upstream/core mismatch.

### Registered upstream and coverage classification

All upstream references are from read-only `git show` at `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`, matching `test/conformance/upstream.yaml`: `ai@7.0.65`, `@ai-sdk/gateway@4.0.52`, `@ai-sdk/provider@4.0.7`, and `@ai-sdk/anthropic@4.0.38`. No latest/main source or version upgrade is used.

- `packages/ai/src/prompt/prepare-tool-choice.ts` and its adjacent tests define omitted choice as auto without a tools argument, and preserve explicit none/auto/required/named choices.
- `packages/ai/src/generate-text/stream-text.ts:1962-1985` prepares tools and choice independently, with a non-nullish `prepareStep` choice taking precedence. The matching `generate-text.ts` uses the same helper.
- `packages/gateway/src/gateway-language-model.ts:60-140` preserves call options in HTTP bodies for both execution modes.
- `packages/anthropic/src/anthropic-prepare-tools.ts:60-67` drops choice when no tools exist, explaining the lower-boundary blind spot.

Per `test/conformance/PARITY.md`, this spans core orchestration, provider call options, ProviderWire runtime/client composition, and conformance-harness evidence. Core tools-dependent defaulting is an implementation bug. Treating harmless text-only auto as an unsupported tool capability is a Gateway compatibility bug against the public client's emission, not a claim about Vercel's private Gateway service. Missing high-level cross-client HTTP assertions are a coverage gap. Go's shared streaming implementation of GenerateText is a parity-preserving adaptation. The existing Anthropic required/named-without-tools deviation and high-level TypeScript generateText body-header gap remain outside scope.

## Goals / Non-Goals

**Goals:**

- Match upstream automatic-choice preparation at the core provider-call boundary on every step and shared entry point.
- Admit exactly the no-tools automatic choice through direct and fallback Gateway text routes, in both wire execution modes, without dropping it.
- Demonstrate equivalence using actual high-level Go and pinned TypeScript streaming calls, and preserve direct/paired conformance expectations.
- Keep strict schema validation, fixed errors, pre-invocation rejection, fallback safety, and authentication/credential privacy intact.

**Non-Goals:**

- Tool definitions, execution, tool history, approvals, or multi-step Gateway tool support (#105/#106).
- Gateway acceptance of explicit none/required/named choices, body headers, structured output, raw output, or other unsupported families.
- End-to-end high-level TypeScript generateText compatibility or default unary-token-limit support.
- Changing public APIs, provider serializers, request schemas, UI chunks, SSE framing, upstream pins, or provider recording provenance.
- Building #201's matrix again or equating compatibility with a particular aggregate pass count.

## Decisions

### 1. Default only after effective per-step choice resolution

Change the existing default branch in `streamtext.go` to depend only on nil choice, not `len(provTools)`. Do not mutate configuration or retain a prior step's override. The precedence remains nonnil `PrepareStepResult.ToolChoice`, then configured choice, then a newly prepared `provider.ToolChoiceAuto`. Keep tool filtering independent.

Explicit auto, none, required, and named choices must reach the model unchanged even with absent, empty, or filtered-out tools. This does not promise that every provider or the Gateway accepts every explicit choice.

Place normative core behavior under the existing `functional-options` capability's Tool options requirement, rather than creating a second orchestration capability. Test shared callers at the provider's `DoStream` boundary in `streamtext_test.go`, `generatetext_test.go`, and `agent_test.go`. A common test helper/table may reduce duplication; no public preparation API or generalized tool refactor is needed.

**Rejected alternatives:** Defaulting in `WithTools` misses absent tools; defaulting in the Grafana client leaves other providers inconsistent; moving defaulting into provider adapters conflates core intent with provider-specific serialization.

### 2. Admit an exact no-tools auto exception after schema validation

Retain `handler.go`'s complete-schema-before-mapping order. In `request.go`, decode the validated choice into a private typed representation and distinguish absence from an object. Continue using typed provider choice constants; do not inspect JSON by substring or compare raw bytes.

| Request choice | Tools | Mapping result |
| --- | --- | --- |
| Absent | Absent or empty | Accept ordinary text; `CallOptions.ToolChoice` remains nil |
| Schema-valid `{ "type": "auto" }` | Absent or empty | Accept; forward `ToolChoiceAuto` |
| None, required, named | Absent or empty | Fixed unsupported-tools response |
| Any or absent | Nonempty function/provider tools | Fixed unsupported-tools response |
| Null, unknown type, wrong shape, extra fields | Any | Existing schema-invalid response before mapping/resolution |

The accepted choice does not bypass prompt/content or other capability checks. Text-only means the entire request is within the current supported subset, not merely that its tools list is empty. Rejections happen before catalog resolution or provider invocation; invalid streaming requests remain non-2xx JSON with no SSE commitment.

**Rejected alternatives:** Erasing auto at the Gateway hides provider-boundary intent; accepting all choices/ignoring tool declarations weakens the safety boundary; broadening the schema is unnecessary because valid auto is already modeled.

### 3. Apply the same exception to the fallback text guard

In `fallbackTextRequest`, accept nil or a pure auto choice (auto type with no tool name) when tools are empty, while retaining all existing text/history/provider-options/headers/raw-output/response-format checks. Forward the original options unchanged to the logical fallback model. Nonempty tools, other choices, tool-call/result history, and other unsupported controls still fail before any physical candidate executes.

Test `fallbackTextModel.DoGenerate` and `DoStream` independently, plus a real configured fallback chain where the primary fails before commitment and the secondary receives identical auto-bearing options. Preserve retry eligibility, commitment, privacy, candidate ordering, and restart-at-primary semantics. No root fallback algorithm changes are required.

This narrowly updates the existing `gateway-ordered-text-fallback` requirement that currently treats every choice as effectful. It is required because the mapper now preserves auto; leaving the guard unchanged would make support depend on route topology.

### 4. Prove the request boundary with real high-level clients

Extend the existing AGPL ProviderWire contract/command workspace, not the public client codec, with a focused high-level streaming case:

- Pin `ai` to exactly `7.0.65` in `ai-gateway/test/providerwire-v4/package.json` and update `test/pnpm-lock.yaml` using the existing workspace tooling. Retain all registered versions and minimum-release-age policy.
- Add a high-level mode to the Go test capture executable in `providers/grafana/internal/capture/main.go`, or an equivalently narrow adjacent test executable, that calls actual root `StreamText` over the Grafana model. It must not construct a replacement `provider.CallOptions` with auto. Keep existing low-level modes/tests intact.
- The current `go-client-capture.ts` builds with `GOWORK=off`, and `providers/grafana/go.mod` pins an older published root. Build this high-level probe with an explicitly selected repository `go.work` so it exercises the changed local core and client; keep ordinary independent client and Gateway builds on their existing `GOWORK=off` path. Do not update production module pins merely to run a local regression. Assert/document source selection in the test setup so an old dependency cannot produce false evidence.
- Use the existing `runtime-integration.test.ts`/test server and `gateway-command.test.ts`/edge-shim composition for actual HTTP execution. Observe incoming JSON without modifying it. Both high-level clients receive equivalent prompt and model settings, with no tools or choice supplied. Constructor/outer authentication headers are allowed; do not add unsupported call-level body headers.
- Assert independently that each inbound request contains `{type: "auto"}`, each reaches the provider, and each completes with the expected text. Accept harmless existing differences such as absent versus false `includeRawChunks` and empty `providerOptions`; do not require unrelated byte identity or strip fields.
- Exercise high-level streaming through the authenticated edge/real-command path so the cloud-authentication spec's revised support claim has evidence. Retain existing cancellation/flush and credential-privacy tests.

Keep unary mapper auto coverage separate from high-level streaming equivalence: TypeScript generateText still adds unsupported body headers. Go GenerateText and Agent defaulting is covered at the shared core call boundary, not misrepresented as cross-language unary HTTP parity.

### 5. Red-first layered validation; no golden manipulation

| Layer | Red-first evidence and regression contract |
| --- | --- |
| Core | Capture full provider call options for absent/empty tools, nonempty tools, global and per-step empty active-tool filtering; explicit auto/none/required/named without tools; per-step precedence and nil fallback; later-step reset. Assert default and override behavior for StreamText, GenerateText, Agent.Stream, and Agent.Generate. |
| Gateway HTTP | Use production-handler harnesses in `runtime_test.go` and `stream_test.go` in both modes. Cover the mapping table, exact forwarded choice/presence, one correct model-method invocation, stable unsupported/schema errors, zero resolution/invocations on rejected requests, and no streaming commitment on failure. |
| Fallback service | Extend `fallback_route_test.go` with both entry points, auto pass-through, all non-auto choices and tools/history rejection, and pre-commit failover option preservation. |
| Cross-language request | Actual Go StreamText and pinned TS streamText with omitted tools/choice, captured unmodified inbound request, successful handler/command execution, and expected text. Existing low-level differential tests alone are insufficient. |
| Paired conformance | Run #201's Go and TypeScript executors for existing `anthropic/upstream/text-generation` with unchanged `expected.jsonl` and `expected-requests.jsonl`. Both must reach the backend and pass their existing expectations. |
| Direct conformance | Existing provider request/UI/object snapshots remain the no-regression contract for Anthropic and all other affected providers. |

Add request-focused failing tests/captures before behavioral edits. On the #201 integration branch, capture the paired pre-fix failure before applying fixes, then replay after both sides are fixed. Existing provenance-valid fixture inputs are sufficient; synthetic handler/service responses stay focused test doubles, never new `recorded/` or `upstream/` inputs. If unexpected direct snapshots differ, investigate the provider behavior against the same baseline instead of mechanically regenerating expectations. No existing fixture goldens are expected to change.

Implementation validation commands:

- `go test ./...` for the root, with focused ToolChoice/shared-caller tests during iteration.
- `(cd ai-gateway && GOWORK=off go test ./providerwire/v4 ./cmd/grafana-ai-gateway/internal/service)`; run focused service fallback tests with `-race` as well.
- `mise run test-providerwire-v4` and `mise run test-ai-gateway-command` for pinned-client and real-command evidence.
- `mise run test-conformance` and `mise run parity-check` for direct fixtures, pin validation, provider shape, and contract replay.
- On Linux/local Docker after #201 is integrated: `SCENARIO=anthropic/upstream/text-generation CLIENT=typescript mise run test-conformance-gateway` and the same with `CLIENT=go`. Preserve generated evidence under that harness's `gateway-results/`; report residual matrix failures separately. A broader matrix rerun is diagnostic, not a demand to implement unrelated capabilities.

No UI/SSE shape changes are intended. If implementation changes frontend wire behavior, add the required deterministic `test/integration/` Go/Vitest scenario and run `mise run test-integration`; do not silently widen this request-defaulting fix.

Update `PARITY.md`'s core/provider-wire and trusted-cloud compatibility coverage with the new assertions. Remove the auto-rejection gap only after evidence passes, preserving high-level TS generateText body-header, default unary-token-limit, actual-tool, and provider-specific deviations. Spec deltas must remove the cloud-authentication blanket high-level streaming rejection without changing authentication rules.

## Risks / Trade-offs

- **Independent module pins can test old core code** → Explicitly select local root/client sources for the new high-level probe; preserve separately isolated production module checks.
- **Fallback routes reject the newly preserved default** → Include the narrow service guard and its direct/failover regression tests in the same change; no request normalization workaround.
- **An auto exception could admit effectful requests** → Require empty tools and unchanged full-subset checks; exercise tools/history/approvals and other unsupported families with auto in both modes and at the fallback guard.
- **Correcting core alone reduces current Gateway Go passes** → Deliver/deploy Gateway admission with the core change; do not treat matching failures as success.
- **Custom providers/middleware observe a new nonnil default** → Document the upstream-alignment behavior change and preserve every explicit choice; no API migration is required.
- **Missing #201 infrastructure or Docker prevents paired proof** → Mark that acceptance gate blocked, not passed. Integrate the existing harness before claiming #202 complete; focused tests do not replace the required paired replay.
- **Broader compatibility claims exceed evidence** → Limit claims to no-tools supported-subset streaming and the independently tested wire-unary mapping, with unchanged documented residual gaps.

## Migration Plan

No data, configuration, or wire-schema migration is needed. Land core default preparation, Gateway mapper admission, fallback guard consistency, and their tests together. Where SDK and Gateway releases deploy independently, deploy the Gateway-compatible change first: it accepts both old absent-choice requests and the new auto default. Then release the aligned Go core.

A server rollback after clients start sending auto restores the old text-only rejection, so retain or restore the Gateway fix when rolling back unrelated changes; coordinate any full rollback across server and client. Do not ship a client-side auto-stripping compatibility mode. Update the coverage map only to the level actually verified.

## Open Questions

There is no unresolved behavior/API decision. #201's harness is not in this checkout; its merge/integration revision and Linux/local-Docker execution are prerequisites to the final paired-fixture gate. Implementation may proceed with focused regressions, but the issue cannot be declared closed until that gate passes against the existing expectations. Report any environment or integration blocker explicitly.
