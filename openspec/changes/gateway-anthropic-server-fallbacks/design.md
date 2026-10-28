# Design

## Context

See proposal.md for the motivation. Paths below are relative to `ai-gateway/cmd/grafana-ai-gateway/internal/` unless they start with a top-level directory. Worktree HEAD is `1493f760`.

- **Refusal.**
  - `nativeoptions/options.go:42-50`: `Anthropic(options)` resolves `AnthropicOptions` and returns `catalog.ErrUnsupportedRequest` for container skills or active fallbacks (`Default || len(Chain) > 0`). The MCP bounds follow at lines 51-79.
  - `nativeoptions.Model` (lines 23-40) runs a `Validate func(provider.CallOptions) error` before `DoGenerate`/`DoStream`. The validator has no policy input.
- **Configured wiring.** `BuildCatalog` (`service/catalog.go:27`) does not call `File.Validate`, so programmatic callers can pass an unvalidated `config.File`.
  - `service/catalog.go:43` builds the descriptors from `Primary` plus `Fallback`.
  - Lines 58-61 set `validateOptions = nativeoptions.Anthropic` for each `anthropic` candidate.
  - Lines 88-91 wrap the native model, then `nativeoptions.Model`, then `execution.Translate`.
  - Lines 102-111 compose `fallback.New` with `execution.FallbackDecider`.
- **BYOK wiring.** `byok/model.go:21` uses the same validator. Lines 37-41 wrap each account, and lines 47-50 wrap the whole chain, so a refused request makes zero account attempts. `service/selection.go:48` passes nil approvals, so there is no service-owned BYOK policy surface.
- **Configuration.**
  - `config/file.go:51-54`: `Primary{Provider, Model}` is decoded with strict YAML (`KnownFields`, line 108).
  - `Validate` (line 127) bounds candidate text with `validDiscoveryText` (lines 173-180, 258-260).
  - Line 199 detects duplicates with `map[Primary]struct{}`, so `Primary` is used as a comparable map key.
- **Overview.**
  - `ai-gateway/internal/execution/overview.go:20-26`: `Attempt{Provider, ModelID, ProviderInstance, Outcome, Error}`. Configured identity comes from `catalog.ConfiguredCandidate` (line 53); BYOK identity comes from the decision (line 93).
  - `attempts.go:83-113` seals and projects the overview.
  - Carriers in `ai-gateway/providerwire/v4`:
    - unary result and error: `ServeHTTP` (`handler.go:104-110`; line 110 is the error after a selected result fails to map);
    - stream: `runStream` (`stream.go:280-292`), which seals the view at the first part;
    - finish metadata: `processStreamPart` (`stream.go:473`);
    - committed error metadata: `processStreamPart` (`stream.go:438`);
    - terminal-wait errors (closed, deadline, canceled, idle timeout; `execution.Wait`, `ai-gateway/internal/execution/runtime.go:204,253`): `writeStreamTerminalForWait` (`stream.go:547-549`), called from `runStream` (line 287) and `consumeStreamParts` (line 326);
    - adapter-failure terminal error: `handleStreamPartResult` (`stream.go:342-344`).
  - `schema/gateway_execution.json` sets `additionalProperties: false` on the attempt.
- **Provider (already at parity).**
  - `providers/anthropic/options.go:52` (`Fallbacks *FallbackConfig`) and lines 78-171 (decode and validation, `Fallback{Model, MaxTokens, Thinking, OutputConfig, Speed}`).
  - `convert_request.go:2936-2972` (`applyFallbacks`): no-op for empty values; warns and ignores on Vertex (line 2940); default uses the `server-side-fallback-2026-07-01` beta, an explicit chain uses `-06-01` (lines 27-28).
  - `convert_response.go:31-37` and `convert_stream.go:165-170` emit `anthropic.fallback` custom content with `from.model`/`to.model` (`fallback_content.go:16-25`).
  - `ResponseMetadata.ModelID = msg.Model` (`convert_response.go:508`, `convert_stream.go:102`).
- **Upstream.** `@ai-sdk/anthropic@4.0.82` (tag resolves to registered commit `b7f6255a`):
  - `anthropic-language-model-options.ts:344-374`: `fallbacks` is `'default' | {model, max_tokens?, thinking?, output_config?, speed?}[]`, triggered when safety classifiers refuse a turn.
  - `anthropic-language-model.ts:774-777` forwards `'default'` or any non-empty array, including repeated models (for example the same model with different `thinking`); there is no allowlist. Lines 1049-1055 add the betas.
  - Response `modelId: response.model` (line 1799); stream `value.message.model` (line 2959).
  - `@ai-sdk/gateway@4.0.116` has no operator policy for provider server-side fallback; its own `models`/`order` are Gateway routing controls (#316).

### Verified assumptions

- **Vertex.** The Gateway `anthropic` type only constructs `anthropicprovider.New` against the direct API (`nativemodel/model.go:27-37`), so no Gateway candidate reaches the Vertex warn-and-ignore branch. Accepting the policy only on `type: anthropic` therefore covers every provider that honors fallbacks. Providers of type `openai` and `openai-compatible` reject the policy at load.
- **Decider.** `execution.FallbackDecider` (`ai-gateway/internal/execution/translate.go:53-55`) stops only on `ErrTranslation`, and otherwise applies `fallback.DefaultDecider` (`fallback/fallback.go:337-349`). That decider returns `true` for any error that is not an `APICallError`. So `ErrUnsupportedRequest` **advances** to the next candidate.
  - `ClassifyProvider` maps it to invalid request (`classify.go:91-92`).
  - Existing proof: `TestNativeMCP_ActualCandidateConsumption` ("next native consumer validates": primary 503, then the Anthropic refusal gives 400), and `TestNativeOptions_BypassAndOrdinaryControls` with `fallback=true` (both candidates refuse, zero native requests, 400).
  - The refused attempt carries an error summary: without an `APICallError`, `summarize` returns `&Failure{Message: err.Error()}` (`ai-gateway/internal/execution/error.go:37-42`), and `project` stores it on non-selected attempts (`overview.go:100-102`). The message is the fixed `gateway: unsupported request` (`ai-gateway/catalog/catalog.go:11`), so it names no models.
- **Served model.** `ResponseMetadata.ModelID` does **not** reflect a hop. In the upstream fixture `test/conformance/anthropic/upstream/fallback` (byte-identical to `__fixtures__/anthropic-fallback.chunks.txt` at the tag):
  - `message_start.model` is `claude-fable-5` and `response-metadata.modelId` is `claude-fable-5`;
  - the `fallback` block goes from `claude-fable-5` to `claude-opus-4-8`;
  - `usage.iterations` lists a `fallback_message` with model `claude-opus-4-8`.

  `fallback-mid-output` shows the same pattern. The recorded `server-side-fallback-{chain,default}` rows had no hop, and `message.model` is the requested `claude-opus-5`. Only the marker's `to.model` identifies the model that served the response.
- **Map key.** Adding a policy to `Primary` must not change duplicate detection. A slice field would make `Primary` non-comparable, and a pointer field would compare by identity. The duplicate check must key on `{Provider, Model}`.

## Goals / Non-Goals

**Goals:** policy-gated forwarding at the existing native consumption seam, with no provider-module change. BYOK keeps refusing. A truthful served-model field that leaves existing overviews byte-identical.

**Non-Goals:**
- A BYOK policy surface.
- Exposing the policy in discovery.
- Routing design (#316).
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

Programmatic construction: `BuildCatalog` fails with a value-free `gateway service:` error when a descriptor with a policy resolves to a provider type other than `anthropic`. Otherwise the policy would be silently ignored, because `service/catalog.go:57-81` wires a validator only for Anthropic candidates.

Errors follow the existing `config: models.<id>.primary.serverFallbacks…` / `fallback[<i>].serverFallbacks…` style and never echo values. Duplicate detection keys on `{Provider, Model}`. There is no count constant: the 1 MiB catalog file bounds the list, and the per-request check is a map lookup.

### D3. Enforcement at native consumption
`nativeoptions` owns `AnthropicFallbackPolicy{models map[string]struct{}, allowDefault bool}` and a constructor. The validator becomes `Anthropic(policy)`, which returns `func(provider.CallOptions) error`.
- `service/catalog.go` builds one validator per descriptor from its policy. `byok/model.go` passes the zero policy.
- Order inside the validator:
  1. resolve options as today;
  2. refuse container skills (unchanged);
  3. if fallbacks are inactive (null, absent or `[]`), skip steps 4-5 only;
  4. `Default` requires `allowDefault`;
  5. a chain requires every `Model` to be allowlisted and the models to be pairwise distinct;
  6. run the MCP checks (unchanged). They run whether or not fallbacks are active.
- Distinctness caps the server-side generations one request can bill to the operator account at the allowlist size, without a new count constant. Upstream accepts repeated models; the Gateway refuses them.
- Comparison is exact bytes, per AGENTS.md canonical casing: no trimming or case folding. Model IDs are opaque, so near-matches are not grants.
- Other entry fields stay as decoded and are emitted by `applyFallbacks`.
- Wrapping order and per-attempt translation are unchanged (host contract: translation, then native validation).
- *Alternative: a new wrapper or a field on `nativeoptions.Model`.* Rejected. The closure keeps one validator signature for both modes.

### D4. BYOK keeps refusing
The zero policy denies both forms. BYOK is catalog-independent, and no service-owned approval surface exists. Enabling BYOK fallback would need one, which belongs in a separate change. *Alternative: allow BYOK unbounded because the caller pays.* Deferred to nara (see Confirmations).

### D5. Combination with ordered fallback (#121) and routing (#316)
Server-side fallback is one outbound request inside one Gateway attempt. Each candidate checks its own policy; a refusal advances through the existing decider (see Verified assumptions). No pre-validation across candidates is added.
- *Alternative: refuse the whole call if any candidate refuses.* Rejected. It would block a later candidate that the operator deliberately allowed, and it would add a second validation pass.
- *Alternative: stop on a refusal.* Rejected. That would change `FallbackDecider` for every unsupported request.

Routing is only specified as "each selected candidate's policy applies".

### D6. Consumption audit row (gateway-native-provider-options)

| Path | Spelling/scope/precedence | Disposition and paired proof |
| --- | --- | --- |
| `ResolveOption[AnthropicOptions]` → `applyFallbacks` | Exact `anthropic` call namespace. Case-insensitive typed decoding. `null` is a nil pointer; `[]` is a no-op; `"default"` and a chain are exclusive. | Admit allowlisted distinct chains and policy-allowed default. Refuse other models, repeated entries, unallowed default and any BYOK value. Paired: allowed vs disallowed, configured vs BYOK, unary vs stream, translated vs supplied. Custom-part `fallbacks` scopes stay inert (existing "custom scopes cannot enable guarded features" case). |

### D7. Served model identity
`execution.Attempt` gains `ServedModelID string json:"servedModelId,omitempty"`, set only on the selected attempt.
- The runtime records the `to.model` of the last valid `anthropic.fallback` custom part. For unary calls it reads the selected result's content; for streams it reads each custom part as it passes `Stream.Next`. Decoding is the ordinary JSON of the `anthropic` metadata that `fallback_content.go` writes. A marker is invalid, and ignored, when `to.model` is empty or invalid UTF-8. No length bound is added (Non-Goal: new size constants; `config`'s unexported `maxStringBytes` is not importable from `internal/execution`).
- Recording covers every marker the selected output yields, including markers observed after sealing, and does not change the sealed attempts list.
- Every carrier that publishes a selected attempt uses the current served model: unary result and error (`ServeHTTP`, `handler.go:104-110`), stream finish and committed stream error (`processStreamPart`, `stream.go:473`, 438), terminal-wait errors (closed, deadline, canceled, idle timeout) via `writeStreamTerminalForWait` (`stream.go:547-549`) and the adapter-failure terminal error (`handleStreamPartResult`, `stream.go:342-344`). The stream view stays sealed at the first part; only the served model is read later, on the adapter goroutine.
- The schema adds an optional nonempty `servedModelId` on the attempt, forbidden unless the outcome is `selected`.
- This is generic for configured and BYOK overviews. BYOK never hops while it refuses fallbacks.
- Existing #201 rows that replay a hop (`anthropic/upstream/fallback`, `fallback-mid-output`) will now carry `servedModelId` in their overviews. #201 strips `gateway.execution` from finish parts before comparison (`withoutGatewayExecution`, `test/conformance/scenario_compare.go:106-130` at `6cc5480d`), so those rows are unaffected.
- *Alternative: `ResponseMetadata.ModelID`.* Rejected; the evidence shows it is the requested model.
- *Alternative: raw `usage.iterations`.* Rejected; raw usage is provider-raw and unvalidated.
- *Alternative: always emit the served model.* Rejected. It would duplicate the response `modelId` and change every existing overview.
- *Alternative: a host hook in the service package.* Rejected; it adds API for one documented kind.

### D8. Matrix harness (#201)
PR #201 (head `6cc5480d`) builds `models.yaml` in `test/conformance/tools/gateway-runtime.mts:91-100` with one route per row. Row IDs are `${tc.name}/${client}` (`gateway-matrix.mts:44`), stored as `Route.rowID` (`gateway.mts:60`, which also uses the row config's `model`). The plan:
- The harness owns a fixed allowlist keyed by fixture case name (`TestCase.name`), not by the client-suffixed row ID: `anthropic/recorded/server-side-fallback-chain` gets the chain's models, and `anthropic/recorded/server-side-fallback-default` gets `allowDefault: true`. That is two fixture cases (four client rows). The policy is not derived from a row's own request, so a future case that sends fallbacks stays refused until the allowlist names it.
- `Route` gains an optional server-fallback policy, and `catalog()` emits it as `primary.serverFallbacks` only for rows of those two cases.
- A harness unit test asserts both client rows of each case get the policy and a case outside the allowlist gets none.
- Recorded inputs are unchanged.

### D9. Parity
- The provider layer is parity-preserving and unchanged.
- The Gateway refusal was a Gateway-owned support boundary. After this change, the boundary is policy-gated configured mode, BYOK refusal and the marker-only `servedModelId`, recorded in `gateway-anthropic-server-fallbacks` and `gateway-execution-overview`.
- Gateway-added refusals with no upstream counterpart are intentional Gateway support boundaries, recorded in `gateway-anthropic-server-fallbacks`. Upstream `@ai-sdk/anthropic@4.0.82` forwards any array, repeated models included, and has no allowlist. The Gateway adds:
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
- [A refusing candidate advances, possibly to a non-Anthropic candidate that ignores `anthropic.fallbacks`] → Existing namespace behavior: the call succeeds without server-side fallback. Docs say so.
- [Callers control per-entry `speed`, `max_tokens` and `thinking` within an allowed model] → These are forwarded unchanged and can raise cost. Docs say the policy grants models, not entry settings.
- [Model renames break the exact match] → The operator updates the config. The refusal stays fail-closed.
- [`servedModelId` comes from provider output] → It comes from the configured native destination only. A value that is empty or invalid UTF-8 is omitted. No new length bound is added (Non-Goal: new size constants); the value is the provider's own model ID, also published as the custom part.
- [#201 not merged] → The matrix acceptance task is gated. Go and command tests prove the behavior independently.

## Migration Plan

The field is additive and an absent policy keeps today's behavior, so no migration is needed. Rollback reverts the change; configs with `serverFallbacks` then fail strict decoding, so remove the key before rolling back.

## Confirmations for nara

1. **BYOK (D4):** keep refusing server-side fallback for BYOK, including when the caller pays? If not, a service-owned BYOK approval surface is a separate change.
2. **`servedModelId` only on a hop (D7):** the candidate sources were `ResponseMetadata.ModelID` and the marker's `to.model`. This design uses only the marker, because `ResponseMetadata.ModelID` is the requested model even after a hop. Confirm that absent means "the attempt's `modelId` served".
3. **Discovery:** leaving the policy out of discovery rows is deferrable. Exposing it later is additive.
