# Design

## Context

See proposal.md for the motivation. Paths below are relative to `ai-gateway/pkg/internal/` unless they start with a top-level directory. Worktree base is `b171e485`.

- **Refusal.**
  - `nativeoptions/options.go:42-50`: `Anthropic(options)` resolves `AnthropicOptions` and returns `catalog.ErrUnsupportedRequest` for container skills or active fallbacks (`Default || len(Chain) > 0`). The MCP bounds follow at lines 51-79.
  - `nativeoptions.Model` (lines 23-40) runs a `Validate func(provider.CallOptions) error` before `DoGenerate`/`DoStream`. The validator has no policy input.
- **Configured wiring.** `BuildCatalog` (`service/catalog.go:43`) does not call `File.Validate`, so programmatic callers can pass an unvalidated `config.File`.
  - `service/catalog.go:64` builds the descriptors from `Primary` plus `Fallback`.
  - Lines 76-81 set `validateOptions = nativeoptions.Anthropic` for each `anthropic` candidate.
  - Lines 110-113 wrap the native model, then `nativeoptions.Model`, then `execution.Translate`.
  - Line 127 keeps each route's descriptors and wrapped candidates (`physicalRoute`, lines 37-40); `composeCandidates` (lines 155-164) composes `fallback.New` with `execution.FallbackDecider`.
- **Routing (#596).** `service/routed.go:21-72` composes a routed chain from the requested and `models` routes' prebuilt `physicalRoute.candidates`, so each routed attempt keeps its own validator. Destinations are provider instance plus backend model (line 47). `routing.Compose` (`routing/compose.go:39-75`) applies `only` and `order` within each group and skips a repeated destination, so the candidate composed first is the one attempted.
- **BYOK wiring.** `byok/model.go:78` wraps each Anthropic account with the same validator. Lines 67-68 wrap the whole chain only when every routed destination is Anthropic, so such a refused request makes zero account attempts. In a mixed chain (for example `models: ["openai/y"]`), each Anthropic account refuses before its native request and the decider advances; OpenAI accounts do not consume the `anthropic` namespace (`TestRoutedSelection_BYOKNativeOptionRefusal`, skills rows). Today both wraps also refuse fallbacks. `service/selection.go:68` passes nil destination approvals and no other policy, so there is no service-owned BYOK policy surface.
  - BYOK already reaches any native model of the caller's accounts: through the selector (gateway-request-byok "Native model absent from configured catalog") and through routed `models` (gateway-request-routing "BYOK fallback models").
- **Configuration.**
  - The catalog comes from the `model_catalog` setting (`config/settings.go:48`), bounded at 64 KiB (`MaxModelCatalogBytes`, line 33) and parsed by `ParseModelCatalog` (`config/file.go:68`).
  - `config/file.go:52-55`: `Primary{Provider, Model}` is decoded with strict YAML (`KnownFields`, line 80).
  - `Validate` (line 99) bounds candidate text with `validDiscoveryText` (lines 148-155, 233-235; `maxStringBytes`, line 19).
  - Line 174 detects duplicates with `map[Primary]struct{}`, so `Primary` is used as a comparable map key.
- **Overview.**
  - `ai-gateway/internal/execution/overview.go:20-32`: `Attempt{Provider, ModelID, ProviderInstance, Outcome, Error}` plus unexported `fallbackModel`/`fallbackCanonical`. `projectExpected` (lines 59-91) sets configured identity from each position's expected attempt and checks BYOK identity against it.
  - `attempts.go:72-98` seals and projects the overview. `Metadata` (`metadata.go:22-36`) publishes `gateway.execution` and the `gateway.routing` summary projected from it.
  - Carriers in `ai-gateway/providerwire/v4`:
    - unary result and errors: `ServeHTTP` (`handler.go:97-105`). Line 100 carries invalid output (`Call.Generate` returns `ErrInvalidOutput`, `runtime.go:124-139`, with a selected view); line 105 is the error after a selected result fails to map;
    - stream: `runStream` (`stream.go:259-297`), which seals the view at the first part (line 261);
    - finish metadata: `processStreamPart` (`stream.go:421`), once per response segment (providerwire-v4-streaming-runtime "Sequential response segments");
    - committed error metadata: `processStreamPart` (`stream.go:399`);
    - terminal-wait errors (closed, deadline, canceled, idle timeout, invalid output; `execution.Wait`, `runtime.go:210-228`): `writeStreamTerminalForWait` (`stream.go:475-477`), called from `runStream` (line 266) and `consumeStreamParts` (line 306). Finished and closed-after-error write nothing;
    - adapter-failure terminal error: `handleStreamPartResult` (`stream.go:316-325`).
  - `schema/gateway_execution.json` sets `additionalProperties: false` on the attempt.
- **Provider (already at parity).**
  - `providers/anthropic/options.go:52` (`Fallbacks *FallbackConfig`) and lines 78-171 (decode and validation, `Fallback{Model, MaxTokens, Thinking, OutputConfig, Speed}`).
  - `convert_request.go:2972-3008` (`applyFallbacks`): no-op for empty values; warns and ignores on Vertex (lines 2976-2982); default uses the `server-side-fallback-2026-07-01` beta, an explicit chain uses `-06-01` (lines 27-28).
  - `convert_response.go:31-37` and `convert_stream.go:203-208` emit `anthropic.fallback` custom content with `from.model`/`to.model` (`fallback_content.go:16-25`).
  - `ResponseMetadata.ModelID = msg.Model` (`convert_response.go:508`, `convert_stream.go:140`).
- **Upstream.** `@ai-sdk/anthropic@4.0.94` (tag resolves to registered commit `028a0253`):
  - `anthropic-language-model-options.ts:378-391`: `fallbacks` is `'default' | {model, max_tokens?, thinking?, output_config?, speed?}[]`, triggered when safety classifiers block a turn (line 363).
  - `anthropic-language-model.ts:839-842` forwards `'default'` or any non-empty array, including repeated models (for example the same model with different `thinking`); there is no allowlist. Lines 1117-1123 add the betas.
  - Response `modelId: response.model` (line 1883); stream `value.message.model` (line 3083).
  - `@ai-sdk/gateway@4.0.132` has no operator policy for provider server-side fallback; its `models`/`order`/`only` are Gateway routing controls, which `gateway-request-routing` implements.

### Verified assumptions

- **Vertex.** The Gateway `anthropic` type only constructs `anthropicprovider.New` against the direct API (`nativemodel/model.go:27-37`), so no Gateway candidate reaches the Vertex warn-and-ignore branch. Accepting the policy only on `type: anthropic` therefore covers every provider that honors fallbacks. Providers of type `openai` and `openai-compatible` reject the policy at load.
- **Decider.** `execution.FallbackDecider` (`ai-gateway/internal/execution/translate.go:53-55`) stops only on `ErrTranslation`, and otherwise applies `fallback.DefaultDecider` (`fallback/fallback.go:337-349`). That decider returns `true` for any error that is not an `APICallError`. So `ErrUnsupportedRequest` **advances** to the next candidate, in configured and routed chains alike.
  - `ClassifyProvider` maps it to invalid request (`classify.go:114-115`).
  - Existing proof: `TestNativeMCP_ActualCandidateConsumption` ("next native consumer validates": primary 503, then the Anthropic refusal gives 400), and `TestNativeOptions_BypassAndOrdinaryControls` with `fallback=true` (both candidates refuse, zero native requests, 400).
  - The refused attempt carries an error summary: without an `APICallError`, `summarize` returns `&Failure{Message: err.Error()}` (`ai-gateway/internal/execution/error.go:37-42`), and `project` stores it on non-selected attempts (`overview.go:122-124`). The message is the fixed `gateway: unsupported request` (`ai-gateway/catalog/catalog.go:11`), so it names no models.
- **Served model.** `ResponseMetadata.ModelID` does **not** reflect a hop. In the upstream fixture `test/conformance/anthropic/upstream/fallback` (byte-identical to `__fixtures__/anthropic-fallback.chunks.txt` at the 4.0.94 tag):
  - `message_start.model` is `claude-fable-5` and `response-metadata.modelId` is `claude-fable-5`;
  - the `fallback` block goes from `claude-fable-5` to `claude-opus-4-8`;
  - `usage.iterations` lists a `fallback_message` with model `claude-opus-4-8`.

  `fallback-mid-output` shows the same pattern. The recorded `server-side-fallback-{chain,default}` rows had no hop, and `message.model` is the requested `claude-opus-5`. Only the marker's `to.model` identifies the model that served the response.
- **Map key.** Adding a policy to `Primary` must not change duplicate detection. A slice field would make `Primary` non-comparable, and a pointer field would compare by identity. The duplicate check must key on `{Provider, Model}`. Routing destinations already key on instance and model (`service/routed.go:47`), so the policy does not split them.

## Goals / Non-Goals

**Goals:** policy-gated forwarding for configured candidates at the existing native consumption seam, with no provider-module change. BYOK forwards without a policy, as upstream does. A truthful served-model field that leaves existing overviews byte-identical.

**Non-Goals:**
- A BYOK policy surface.
- Exposing the policy in discovery.
- Changes to request routing.
- Cross-candidate pre-validation.
- New size constants.
- Changes to the provider adapters or UI chunks.
- Usage/billing attribution for fallback-served turns.
- Live refusal-triggered evidence.

## Decisions

### D1. Policy per configured candidate
`Primary` gains `ServerFallbacks *ServerFallbacks` with the YAML key `serverFallbacks`. The struct is `ServerFallbacks{Models []string yaml:"models,omitempty"; AllowDefault bool yaml:"allowDefault,omitempty"}` and follows `file.go`'s camelCase keys. It applies to the primary and to each `fallback` entry independently.
- *Alternative: per provider instance.* Rejected. Every route on that account, including a cheap-model route, could fall back to any allowlisted model, which leaks grants across routes.
- *Alternative: per model route.* Rejected. Candidates in one route can target different instances and models; a route-level allowlist would grant the same chain to an OpenAI fallback candidate or to a different Anthropic backend.

### D2. Load validation
In `File.Validate`, for each candidate with a policy:
- the provider type must be `anthropic`;
- the policy must have `allowDefault: true` or at least one model;
- each model must be nonempty, not blank, `validDiscoveryText`, and distinct.

Programmatic construction: `BuildCatalog` fails with a `gateway service:` error that names the model but no policy values or secrets, when a descriptor with a policy resolves to a provider type other than `anthropic`. Otherwise the policy would be silently ignored, because `service/catalog.go:57-81` wires a validator only for Anthropic candidates.

Errors follow the existing `config: models.<id>.primary.serverFallbacks…` / `fallback[<i>].serverFallbacks…` style and never echo values. Duplicate detection keys on `{Provider, Model}`. There is no count constant: the 64 KiB `model_catalog` setting bounds the list, and the per-request check is a map lookup.

### D3. Enforcement at native consumption
`nativeoptions` owns `AnthropicFallbackPolicy{models map[string]struct{}, allowDefault, unrestricted bool}`, a constructor for configured policies and `UnrestrictedAnthropicFallbacks()`. The zero value denies both forms. The validator becomes `Anthropic(policy)`, which returns `func(provider.CallOptions) error`.
- `service/catalog.go` builds one validator per descriptor from its policy (the zero policy when absent); routed chains reuse those candidates unchanged. `byok/model.go` passes `UnrestrictedAnthropicFallbacks()` at both wraps: the whole-chain wrap (lines 67-68) and the per-account wrap (line 78). See D4.
- Order inside the validator:
  1. resolve options as today;
  2. refuse container skills (unchanged, both modes);
  3. if fallbacks are inactive (null, absent or `[]`) or the policy is unrestricted, skip steps 4-5 only;
  4. `Default` requires `allowDefault`;
  5. a chain requires every `Model` to be allowlisted and the models to be pairwise distinct;
  6. run the MCP checks (unchanged, both modes). They run whether or not fallbacks are active.
- Steps 4-5 apply only to configured candidates, because the operator account pays.
- Distinctness caps the server-side generations one request can bill to the operator account at the allowlist size, without a new count constant. Upstream accepts repeated models; configured candidates refuse them.
- Comparison is exact bytes, per AGENTS.md canonical casing: no trimming or case folding. Model IDs are opaque, so near-matches are not grants.
- Other entry fields stay as decoded and are emitted by `applyFallbacks`.
- Wrapping order and per-attempt translation are unchanged (host contract: translation, then native validation).
- *Alternative: a new wrapper or a field on `nativeoptions.Model`.* Rejected. The closure keeps one validator signature for both modes.

### D4. BYOK forwards without a policy
BYOK forwards an explicit chain, including repeated models, and `"default"`, matching `@ai-sdk/anthropic@4.0.94`. The unrestricted policy applies to the requested model's accounts and to every routed fallback model's accounts, for request options and translation-added options alike. In a mixed chain, non-Anthropic accounts ignore the namespace. The whole-chain pre-check still refuses container skills and invalid MCP before any account attempt in an all-Anthropic chain.
- Server-side fallback runs inside the selected account's one native request at its destination. The selected model, account and destination do not change; a hop is reported through `servedModelId` (D7). gateway-request-byok "Unsupported controls fail explicitly" states that this is not an override.
- Rationale: the caller's own account pays, and BYOK already reaches any native model of the caller's accounts (see Context). A refusal would protect nothing the caller cannot already do.
- Configured candidate policies never apply to BYOK, and no client control grants or applies one. BYOK stays catalog-independent.
- *Alternative B: keep refusing.* Rejected. It protects no operator resource and diverges from upstream without a rationale.
- *Alternative C: an operator policy for BYOK.* Rejected. No service-owned BYOK policy surface exists (`service/selection.go:68`). It would let the operator restrict models on the caller's own account, which BYOK does not do for the requested model (advisory native model lists are not an allowlist), and it would add operator configuration to the catalog-independent path.

### D5. Combination with ordered fallback (#121) and request routing (#316)
Server-side fallback is one outbound request inside one Gateway attempt. Each candidate checks its own policy; a refusal advances through the existing decider (see Verified assumptions). No pre-validation across candidates is added.
- *Alternative: refuse the whole call if any candidate refuses.* Rejected. It would block a later candidate that the operator deliberately allowed, and it would add a second validation pass.
- *Alternative: stop on a refusal.* Rejected. That would change `FallbackDecider` for every unsupported request.

Routed chains reuse each route's startup candidates, so a routed attempt applies the policy of the route it was composed from. A route without a policy followed by `models: [B]`, where B allows the chain, refuses on the first route's candidates and selects B's. When two routes share a destination with different policies, routing attempts it once, and the policy of the candidate composed first applies (gateway-request-routing "Repeated models and candidates"). `gateway-request-routing` needs no delta: its "Candidate-native request semantics across routed attempts" already requires each routed attempt's own native validation.

### D6. Consumption audit row (gateway-native-provider-options)

| Path | Spelling/scope/precedence | Disposition and paired proof |
| --- | --- | --- |
| `ResolveOption[AnthropicOptions]` → `applyFallbacks` | Exact `anthropic` call namespace. Case-insensitive typed decoding. `null` is a nil pointer; `[]` is a no-op; `"default"` and a chain are exclusive. | Configured: admit allowlisted distinct chains and policy-allowed default; refuse other models, repeated entries and unallowed default. BYOK: forward any chain and default (upstream parity). Paired: allowed vs disallowed, configured vs BYOK, unary vs stream, translated vs supplied. Custom-part `fallbacks` scopes stay inert (existing "custom scopes cannot enable guarded features" case). |

### D7. Served model identity
`execution.Attempt` gains `ServedModelID string json:"servedModelId,omitempty"`, set only on the selected attempt.
- The runtime records the `to.model` of the latest valid `anthropic.fallback` custom part observed so far. For unary calls it reads the content of the result `DoGenerate` returned, before output validation; for streams it reads each custom part as it passes `Stream.Next`, across response segments. Decoding is the ordinary JSON of the `anthropic` metadata that `fallback_content.go` writes. A marker is invalid, and ignored, when `to.model` is empty or its raw bytes are invalid UTF-8; the check runs on the raw value with `utf8.Valid`, as `validMetadata` does (`providerwire/v4/response.go:191-198`), because ordinary decoding replaces invalid bytes with U+FFFD. No length bound is added (Non-Goal: new size constants; `config`'s unexported `maxStringBytes` is not importable from `internal/execution`).
- Recording covers every marker the selected output yields, including markers observed after sealing. It does not change the sealed attempts list or its check against expected attempts. The update happens only while the selected output is being read on the adapter goroutine, never after the call ends, and frames already written keep their value; it is not a rewrite by a late callback or native result under gateway-attempt-failure-evidence.
- Every carrier that publishes a selected attempt uses the current served model: unary result, invalid-output and mapping errors (`ServeHTTP`, `handler.go:97-105`), each segment's stream finish and committed stream error (`processStreamPart`, `stream.go:421`, 399), terminal-wait errors (closed, deadline, canceled, idle timeout, invalid output) via `writeStreamTerminalForWait` (`stream.go:475-477`) and the adapter-failure terminal error (`handleStreamPartResult`, `stream.go:316-325`). A stream with two segments, each with its own hop, publishes the first hop's model on the first finish and the second hop's on the second. The stream view stays sealed at the first part; only the served model is read later, on the adapter goroutine.
- The `gateway.routing` summary is unchanged: `servedModelId` stays on the `gateway.execution` attempt only.
- The schema adds an optional nonempty `servedModelId` on the attempt, forbidden unless the outcome is `selected`.
- This is generic for configured and BYOK overviews. A BYOK hop does not affect the identity check in `projectExpected`, because the attempt's `modelId` comes from the candidate's `ModelID()` (`fallback/fallback.go:295`), not from the response.
- The existing command test "returns and replays the Anthropic fallback marker through both clients" (`test/providerwire-v4/gateway-command.test.ts:2950`) already returns a marker from a configured route. It does not assert the overview, so it keeps passing; task 2.5 adds the `servedModelId` assertion to it.
- *Alternative: `ResponseMetadata.ModelID`.* Rejected; the evidence shows it is the requested model.
- *Alternative: raw `usage.iterations`.* Rejected; raw usage is provider-raw and unvalidated.
- *Alternative: always emit the served model.* Rejected. It would duplicate the response `modelId` and change every existing overview.
- *Alternative: a host hook in the service package.* Rejected; it adds API for one documented kind.

### D8. Recorded-fixture acceptance through the Gateway command
The issue's acceptance names `mise run test-conformance-gateway`, which is not on main. The same proof, two recorded fixtures through the Gateway with both clients, uses existing main infrastructure:
- `test/providerwire-v4/gateway-command.test.ts` runs the real `grafana-ai-gateway` binary with an operator model catalog passed as `GF_PLUGIN_MODEL_CATALOG` (`GatewayProcess.start`, :3661; config builders such as `openAIConfig` at :3529) and drives both registered clients (`createGateway`, `captureGoClient`).
- `test/providerwire-v4/runtime-integration.test.ts:761-800` already replays recorded fixtures against unchanged `expected.jsonl` and `expected-requests.jsonl` with the `test/conformance/tools/common.mts` helpers. It cannot carry this proof: its `testserver` serves `catalog.StaticEntry` models (`testserver/recorded_usage.go`), which bypass the command's `nativeoptions` validator.

The plan adds a command-level replay:
- A local backend serves each fixture's unchanged `input.chunks.txt` as SSE, as `recorded_usage.go` does in Go.
- The route's primary is `{provider: anthropic, model: claude-opus-5}`. The test writes a fixed policy per fixture: `models: [claude-opus-4-8]` for the chain, `allowDefault: true` for default. The policy is never derived from the request.
- Both clients stream with the fixture's `providerOptions`. UI chunks must equal `expected.jsonl`, and each client's normalized native requests must equal `expected-requests.jsonl`. If a compared chunk carries Gateway-owned metadata, only the `gateway` namespace (`execution` and `routing`) is removed, and the test says why.
- A paired run without a policy, and with the other fixture's policy, gets the fixed 400 with zero backend requests.
- Inputs are read in place. `git status --porcelain test/conformance` stays empty.

### D9. Parity
- The provider layer is parity-preserving and unchanged.
- The Gateway refusal was a Gateway-owned support boundary. After this change, the boundary is policy-gated configured mode and the marker-only `servedModelId`, recorded in `gateway-anthropic-server-fallbacks` and `gateway-execution-overview`.
- BYOK forwarding is parity-preserving: like upstream, it forwards any array and `"default"` (gateway-request-byok).
- Configured-mode refusals with no upstream counterpart are intentional Gateway support boundaries, recorded in `gateway-anthropic-server-fallbacks`. Upstream `@ai-sdk/anthropic@4.0.94` forwards any array, repeated models included, and has no allowlist. For configured candidates the Gateway adds:
  - the allowlist refusal (the operator account pays, so the operator grants models);
  - the distinctness refusal (caps billed generations at the allowlist size, see D3);
  - exact-byte matching (opaque model IDs, canonical casing).
- AI Gateway is a Go extension in `PARITY.md`, so no `knownGaps` entry or `PARITY.md` change is needed.
- No pin, recording or expectation changes.

### D10. Validation scope
- `gateway.execution` is Gateway-owned opaque provider metadata. SSE framing and UI chunks are unchanged, so there is no `test/integration/` scenario (same reasoning as `expose-byok-attempt-overviews`).
- Coverage instead comes from ProviderWire schema tests, both-client command tests (including a `servedModelId` assertion, task 2.5) and `parity-check`.

## Risks / Trade-offs

- [`"default"` lets Anthropic choose the model] → `allowDefault` is an explicit opt-in. Docs say the operator accepts Anthropic's choice and its cost, and `servedModelId` reveals it.
- [Fallback-served turns bill at another model] → Out of scope. Usage is already top-level per the `anthropic-executor-iteration-cache-usage` gap. Docs note this.
- [A primary that fails with a 5xx and is followed by a refusing candidate returns 400] → This is existing last-error behavior (as with MCP). Docs advise consistent policies across a route's Anthropic candidates.
- [A refusing candidate advances, possibly to a non-Anthropic candidate that ignores `anthropic.fallbacks`, including a routed `models` candidate] → Existing namespace behavior: the call succeeds without server-side fallback. Docs say so.
- [BYOK callers can send `"default"` or any chain, so Anthropic may serve and bill another model on the caller's account] → Accepted: the caller chose it and pays. Docs say so, and `servedModelId` reveals the served model.
- [Routes sharing a destination with different policies] → Routing attempts the destination once, under the policy of the candidate composed first. Docs advise one policy per destination.
- [Callers control per-entry `speed`, `max_tokens` and `thinking` within an allowed model] → These are forwarded unchanged and can raise cost. Docs say the policy grants models, not entry settings.
- [Model renames break the exact match] → The operator updates the config. The refusal stays fail-closed.
- [`servedModelId` comes from provider output] → It comes from the selected native destination only: a configured candidate, or a BYOK account's native default or service-approved destination. A value that is empty or invalid UTF-8 is omitted. No new length bound is added (Non-Goal: new size constants); the value is the provider's own model ID, also published as the custom part.
- [The issue names a matrix command that is not on main] → Task group 5 gives the same evidence through the real command. The PR states the substitution.

## Migration Plan

The field is additive and an absent policy keeps today's configured behavior, so no migration is needed. BYOK calls with server-side fallback, refused today, are forwarded. Rollback reverts the change and refuses them again; configs with `serverFallbacks` then fail strict decoding, so remove the key before rolling back.

## Resolved decisions

nara decided:

1. **BYOK forwards (D4, option A).** BYOK forwards server-side fallback, chain and `"default"`, without a policy, as `@ai-sdk/anthropic@4.0.94` does. The caller's own key pays, and BYOK already reaches any native model through the selector and routed `models`. Configured policies never apply to BYOK, and no client control grants one.
2. **`servedModelId` only after a hop (D7).** `ResponseMetadata.ModelID` is the requested model even after a hop, so only the marker's `to.model` is used. Absent means the attempt's `modelId` served. This holds for configured and BYOK overviews.
3. **Policy stays out of discovery.** Exposing it later is additive.
