# Design

## Context

See proposal.md for the motivation. This design is written against `main` `4fdb70af` and baseline `@ai-sdk/anthropic` 4.0.94. Gateway paths below are relative to `ai-gateway/pkg/internal/` unless stated otherwise.

- **The refusal.** `nativeoptions.Anthropic` (`nativeoptions/options.go:42-81`) resolves `AnthropicOptions` the same way the adapter does. At L47-50 it refuses non-empty `Container.Skills` and active fallbacks; MCP bounds follow at L51 and L83-131. The validator signature is `func(provider.CallOptions) error` with no policy input. A `container.id` without skills is admitted in both modes, as the archived `2026-10-02-forward-native-provider-options` design decided (`design.md:116`, "allow ... container ID without skills").
- **Configured wiring.** `service/catalog.go:64` builds descriptors from `Primary` plus `Fallback`. L81 sets `nativeoptions.Anthropic` for every `anthropic` candidate; the `openai-compatible` validator is already a closure (L97-99). L110-113 wraps the native model, then `nativeoptions.Model`, then `execution.Translate`, so validation sees options after translation. L127 keeps those candidate instances per route for request routing.
- **BYOK wiring.** `byok/model.go` uses the same validator on each Anthropic account (L78) and, when every candidate is Anthropic, on the whole chain (L67-69), so a refused request on an all-Anthropic chain makes zero attempts. In a mixed routed chain the Anthropic account's attempt fails before its native I/O and the chain advances (`service/routed_test.go:507`).
- **Config.** The catalog is the `model_catalog` plugin setting (`config/settings.go:48`), parsed by `config.ParseModelCatalog` (`config/file.go:68`) with strict YAML (`KnownFields`, L80) and `File.Validate` (L99). `gateway.New` parses it (`gateway/gateway.go:87`) before resolving secrets (L94), so catalog errors fail startup before any secret is read. `config/file.go:52-55` defines `Primary{Provider, Model}`. L174-185 uses `map[Primary]struct{}` for duplicate detection within a route. The only provider types are `anthropic`, `openai` and `openai-compatible` (L107-134); there is no Vertex type.
- **Provider.**
  - `ContainerSkill{Type string, SkillID, Version string,omitempty}` lives at `options.go:228-232`; the adapter treats an empty version as absent (`convert_request.go:3149`). `applyProviderOptions` (`convert_request.go:3139-3158`; no error return, L3065) maps it, sends `container.id` only when non-empty (L3141), and adds the `skills-2025-10-02` and `files-api-2025-04-14` betas. The `code-execution-2025-08-25` beta comes only from the `code_execution_20250825` tool (L2627); `code_execution_20260120` adds no beta (L2629-2632).
  - The Vertex model (`model.go:74`) goes through the same `applyProviderOptions` and forwards skills without any capability gate.
  - Responses surface `providerMetadata.anthropic.container` (`provider_metadata.go:255-281`).
- **Upstream 4.0.94** (`git show @ai-sdk/anthropic@4.0.94:packages/anthropic/src/...`). Skills behavior is unchanged since 4.0.82 (the options schema, request mapping, betas/warning and the "agent skills" tests are identical apart from indentation).
  - `anthropic-language-model-options.ts:279-304` defines skills as a discriminated union: `anthropic{skillId}` or `custom{providerReference}`. There is no count limit.
  - `anthropic-language-model.ts:868-890` resolves `providerReference.anthropic` through `resolveProviderReference` (`@ai-sdk/provider-utils` 5.0.72), which throws `NoSuchProviderReferenceError` when it is missing.
  - L1076-1099 always adds `code-execution-2025-08-25`, `skills-2025-10-02` and `files-api-2025-04-14`, and warns when no code-execution 20250825/20260120 tool is present.
- **Anthropic request limit.** Anthropic's "Using Agent Skills with the API" guide (`platform.claude.com/docs/en/build-with-claude/skills-guide`, "Limits and constraints" → "Request limits") documents "Maximum Skills for each request: 20".
- **Fixture.** The pptx fixture requests `{type: anthropic, skillId: pptx}` with `code_execution_20250825`. It makes no `container.id` request and ends with `end_turn`. The direct Go replay already passes on `main`, including its provider-parts subtest: `cd test/conformance && go test -tags conformance -run 'TestConformance/upstream/code-execution-pptx-skill$' -v ./anthropic/` prints `--- PASS: TestConformance/upstream/code-execution-pptx-skill`. A bare `pptx` run pattern matches no top-level test and passes vacuously, so it is not evidence.
  - The generated `.pptx` appears only as a `file_id` inside a `bash_code_execution_output` tool result, not as a generated-media `file` part. `gateway-generated-media` is therefore not exercised; the code-execution continuation path (`gateway-provider-tools` L114) is, and it sends no `container.id`.
- **Request routing (#596).** `gateway.models`/`order`/`only` compose one chain across routes (`service/routed.go:21-72`) from the startup-constructed candidates, each with its own validator (L57-60). Routing uses only destinations the caller can select directly (`gateway-request-routing` "Authorized destinations only"), and each routed attempt runs its own translation and native validation ("Candidate-native request semantics across routed attempts"). A candidate whose provider instance and backend model already appear earlier is skipped ("Repeated models and candidates"; key `ProviderInstance + "\x00" + ModelID`, `service/routed.go:47`, `routing/compose.go:57-64`), regardless of the candidate's policy.
- **Sibling change.** `gateway-anthropic-server-fallbacks` (#562, not on main) plans `Primary.ServerFallbacks`, `nativeoptions.Anthropic(policy)` returning a closure, one validator per descriptor, and a zero policy for BYOK.
- **Fallback advancement.** `catalog.ErrUnsupportedRequest` is a plain `errors.New` (`ai-gateway/catalog/catalog.go:11`), not a `*provider.APICallError`. `execution.FallbackDecider` (`ai-gateway/internal/execution/translate.go:53-55`) delegates to `fallback.DefaultDecider`, which returns `true` for it (`fallback/fallback.go:331-349`: godoc L331-336, function L337-349). A refusing candidate therefore makes no native request and the chain advances.
- **Failure classification today.** When more than one candidate fails, `failedAttemptsError` returns `errors.Join` of every failure, newest first, each wrapped with its provider and model (`fallback/fallback.go:315-328`). When the call context ends between attempts, `DoGenerate` and `DoStream` join `ctx.Err()` after those failures (`fallback/fallback.go:118`, L146). `execution.ClassifyProvider` (`ai-gateway/internal/execution/classify.go:102`) checks `errors.Is(err, catalog.ErrUnsupportedRequest)` (L114) before cancellation, timeout and API status, so any refusal anywhere in the chain classifies the whole call as invalid request. An unpolicied primary that refuses followed by a policied fallback that returns 503 yields 400, and so does a cancellation after a refusal. `provider.ErrUnsupportedFunctionality` is checked last, after API status and transport (L129-131), so a chain that joins it with an unknown error also yields 400. `ClassifyProvider` also backs `ClassifySelection` (L97), ProviderWire (`ai-gateway/providerwire/v4/errors.go:74`) and `internal/testadapter` (`adapter.go:58-78`).

## Goals / Non-Goals

**Goals:**
- Policy-gated skills at the existing native consumption seam, applied per candidate.
- Configured candidates refuse every caller-supplied `container.id`.
- BYOK bounded admission of skills and container IDs.
- Provider parity for the skills request shape.
- A chain failure reports the real failure rather than an earlier refusal.
- One shared policy plumbing with #562.

**Non-Goals:**
- Per-tenant scoping.
- Caller-supplied file references (Anthropic `file_id`, OpenAI file IDs, openai-compatible `file.file_id`) on operator accounts. `gateway-file-inputs` is unchanged; a separate cross-provider issue owns them.
- A Files API download path.
- Vertex skills behavior.
- Other #216 request controls.
- The bare-string container-ID wire form.
- Changes to the public `fallback` module.

## Decisions

### D1. Policy lives on the configured candidate

`Primary` gains `Skills *SkillsPolicy` (YAML key `skills`). The struct is `SkillsPolicy{Allow []AllowedSkill}`, with `AllowedSkill{Type SkillType; SkillID string yaml:"skillId"; Versions []string}`, and `SkillType` is a typed string with constants for `anthropic` and `custom`.

Duplicate detection at `file.go:174-185` must key on `{Provider, Model}`, because a pointer field would compare by identity. #562 needs the same change.

Validation (spec "Strict skills policy validation"):
- Only `anthropic` provider types may carry a policy. Vertex has no Gateway type, so no Vertex rule is needed. A future Vertex type must decide separately whether it honors skills.
- `allow` and any `versions` list must be non-empty; entry types must be `anthropic` or `custom`.
- Skill IDs and versions must be non-empty and not blank (not only whitespace), valid UTF-8, and at most 128 bytes.
- `(type, skillId)` entries and the versions within an entry must be distinct.
- Errors use the `config: models.<id>.primary.skills…` and `fallback[<i>].skills…` style.
- Candidates naming the same provider instance and backend model carry an equal policy across routes (D9).

*Alternatives:*
- A per-provider-instance allowlist was rejected: it would grant skills to every route on that account, including routes the operator never meant to grant them to.
- A global plugin setting was rejected as too coarse.

### D2. Shared candidate-policy plumbing with #562

There is one mechanism:
- `nativeoptions.Anthropic(policy AnthropicPolicy) func(provider.CallOptions) error`.
- `AnthropicPolicy` has one field per feature, `Fallbacks` (#562) and `Skills` (this change), plus an unexported container-ID mode.
- `service/catalog.go` builds a single validator per descriptor from that descriptor's `Primary` fields.
- `byok/model.go` passes `RequestAccountAnthropicPolicy()` at both validator sites (L68 chain, L78 per account), including routed fallback-model accounts. That policy keeps fallbacks denied, sets skills to bounded mode and sets container IDs to bounded mode.

The zero `AnthropicPolicy` denies skills and fallbacks and refuses container IDs, so configured candidates without a policy refuse everything this change gates.

Nothing can fail open:
- The skills field is `AnthropicSkillsPolicy{mode anthropicSkillsMode; allow []AnthropicSkillGrant}` with unexported fields and an explicit mode:
  - `anthropicSkillsDenied` is the zero value and refuses every non-empty skills list.
  - `anthropicSkillsBounded` (request-account) applies only the static bounds. Outside package `nativeoptions`, only `RequestAccountAnthropicPolicy()` can construct it.
  - `anthropicSkillsAllowlisted` (configured) applies bounds plus the allowlist. `AllowlistedAnthropicSkills(grants)` constructs it only for a non-empty `grants`; nil or empty grants return the denied zero value.
- The container-ID mode's zero value refuses any non-empty `container.id`; only `RequestAccountAnthropicPolicy()` sets the bounded mode. No configured constructor reaches it.

`service/catalog.go` converts `*config.SkillsPolicy` through `AllowlistedAnthropicSkills`, so a nil policy or an empty `allow` yields deny even if config validation were bypassed. The configured path has no route to either bounded mode. #562's `Fallbacks` field follows the same rule: the zero value denies and only the BYOK constructor sets BYOK behavior.

Whichever change lands first introduces `Anthropic(policy)`. If #562 lands first with `Anthropic(AnthropicFallbackPolicy)`, this change wraps that parameter into `AnthropicPolicy{Fallbacks, Skills}` instead of adding a second constructor or wrapper.

Both changes MODIFY the same `gateway-request-byok` requirements, so the later one rebases its delta onto the archived text.

### D3. Enforcement order inside `Anthropic(policy)`

1. Resolve options. Return the existing refusal on a decode error.
2. Run the fallback policy (#562 or the current refusal).
3. If `Container != nil && Container.ID != ""`, run the container-ID check (D8): configured mode refuses; request-account mode requires valid UTF-8 of at most `maxAnthropicContainerIDBytes = 128` bytes. An empty ID counts as omitted, because the adapter sends none.
4. If `Container != nil && len(Skills) > 0`, run the skills checks:
   - **Static bounds:** at most `maxAnthropicSkills = 20` skills; the type is checked first (`anthropic` or `custom`), then the ID is resolved through the provider's exported resolver so the Gateway and adapter share one rule (a custom skill without `providerReference.anthropic` is malformed); the resolved ID is non-empty UTF-8 of at most `maxAnthropicSkillBytes = 128` bytes; an empty version is treated as omitted, because `Version` is `omitempty` and the adapter drops it, and a non-empty version is valid UTF-8 of at most 128 bytes; resolved `(type, ID)` pairs are distinct.
   - **Configured mode:** each skill is allowlisted. Version matching is exact: an entry without `versions` admits only an omitted (or empty) request version; an entry with `versions` admits only a listed value. `"latest"` is matched literally, so `versions: ["latest"]` does not admit an omitted version.
   - **Request-account mode:** stops after the bounds.
5. Run the MCP checks unchanged.

Every refusal is `catalog.ErrUnsupportedRequest` with no values.

The skill count bound is a **Gateway resource bound** set to Anthropic's documented per-request limit of 20 (Context, "Anthropic request limit"); upstream has no limit. Its definition comment cites the skills guide. `maxAnthropicSkillBytes` and `maxAnthropicContainerIDBytes` are their own constants, so they do not move with `maxMCPNameBytes`.

### D4. Account scoping

- **`anthropic` skills** are Anthropic-owned. Configured mode admits them only when listed.
- **`custom` skills** live in the operator's account, and callers hold no operator credential that could create them, so every custom skill there is operator-owned. Listing one shares it with **all callers of that candidate**. The validator has no tenant, and namespace scoping would widen the plumbing, so there is none.
- **Generated files** stay in the operator's Files API. Configured callers see only `file_id`s and have no Gateway path to download them, which is a documented support boundary.
- **Container IDs** on the operator account could reach another caller's container and its skill-generated files, so configured mode refuses every caller-supplied `container.id` (D8).

### D5. BYOK admits bounded skills and container IDs

BYOK runs on the caller's account, so skills, containers and files belong to the key holder. BYOK may not consult configured state (`gateway-provider-configuration` L8-14). Admission therefore mirrors bounded MCP ("valid bounded MCP ... SHALL remain supported", `gateway-request-byok` L42): BYOK admits `anthropic` and `custom` skills under the static bounds and a `container.id` under the container-ID bound (D3), with or without skills.

This differs from #562: native fallback selects models outside the catalog, while skills and containers do not. The alternative was to keep custom skills refused in BYOK; it was rejected because it addresses no identified threat.

Ordered BYOK credentials, and routed BYOK fallback models, may span accounts (`byok/model.go:47-61`). OpenAI candidates in a mixed routed chain ignore `anthropic` options. A `container.id` or custom skill ID belongs to one account, so a later account attempt fails at the provider. The Gateway neither rewrites nor refuses it, and the docs say so.

### D6. Provider parity work package (`providers/anthropic`)

- Change `ContainerSkill.Type` to a typed `ContainerSkillType` with `ContainerSkillAnthropic` and `ContainerSkillCustom`.
- Add `ProviderReference map[string]string` to `ContainerSkill`.
- Add an exported `ContainerSkill.ResolveSkillID() (string, error)`:
  - `anthropic` returns `SkillID`;
  - `custom` returns `ProviderReference["anthropic"]`; a missing `providerReference`, or one without an `anthropic` entry, returns the `resolveAnthropicFileReference` error shape (`convert_request.go:919`). The legacy custom `skillId` input is dropped, matching upstream, where `custom` requires `providerReference`;
  - `custom` with an empty `anthropic` value returns an error. `map[string]string` decodes a JSON `null` to `""`, so `{"anthropic": null}` and `{"anthropic": ""}` both take this path instead of resolving to an empty ID;
  - any other type returns an error, matching upstream's discriminated union.
- **Error messages.** `ResolveSkillID` returns unprefixed messages, unlike `resolveAnthropicFileID` (`convert_request.go:916-922`), which prefixes `anthropic:`. `validateContainerSkills` adds the skill index, and `buildParams` adds the single `anthropic:` prefix. The exact messages are:
  - missing reference: `anthropic: invalid provider options: container skill <i>: no provider reference found for provider anthropic; available providers: <sorted, comma-separated keys>`. When `providerReference` is absent or has no keys, the message ends at `available providers:` followed by the single space the shared `%s` format leaves (the shape `resolveAnthropicFileID` already pins in `convert_request_test.go:2533`); tests compare the whole string, so that space is part of the pinned message;
  - empty or null reference: `anthropic: invalid provider options: container skill <i>: empty provider reference for provider anthropic`;
  - unknown type: `anthropic: invalid provider options: container skill <i>: unsupported type "<type>"`.
- **Error site:** `applyProviderOptions` returns no error (`convert_request.go:3065`), so its signature stays. A new `validateContainerSkills(anthropicOpts.Container)` runs in `buildParams` beside the other option errors, after `validateFallbackConfig` (`convert_request.go:218-220`), and fails with `anthropic: invalid provider options: %w`. `applyProviderOptions` then maps skills with `ResolveSkillID`, which cannot fail after validation.
- `applyProviderOptions` adds `code-execution-2025-08-25` when skills are present and emits the warning (`provider.WarnOther`).
- Gateway code does not refuse a request that lacks the code-execution tool, matching upstream, which only warns.
- **Breaking change:** Go callers that set `Type` as a bare string or send custom skills by `skillId` must change. The PR title carries breaking intent (`feat!:` or a `BREAKING CHANGE:` footer, `CONTRIBUTING.md:518-527`); the modules are alpha.
- Existing Gateway tests that send `{"type":"custom","skillId":"external"}` now carry a malformed skill; tasks 3.5-3.6 assign each one its form.

### D7. `pause_turn` and container reuse

- Within one Gateway request, no container reuse is needed. The Gateway is stateless and makes one native call per attempt, and the pptx row is a single request that sends no `container.id`.
- Across requests, continuing a `pause_turn` or a multi-turn session on the same container requires sending `container.id`, which configured mode refuses with or without skills (D8).
- User-visible result: on configured models every request starts a fresh container, so `pause_turn` continuation and multi-turn refinement on the same container are unavailable. The response still returns `providerMetadata.anthropic.container` unchanged (`provider_metadata.go:255-281`); the caller cannot send its ID back. BYOK keeps container reuse. The docs say this.

### D8. Configured accounts refuse container IDs (guard tightening)

A caller-supplied `container.id` on the operator account could attach to another caller's container, including files that skills generated there. Configured mode therefore refuses every non-empty `providerOptions.anthropic.container.id`, with or without skills, before native I/O, with the fixed invalid-request error and no echo. There is no opt-in.

- The check runs on the options the adapter consumes after per-attempt translation, so a translation-produced ID is refused too. In a heterogeneous configured chain only the Anthropic candidate refuses; the chain advances as in D3.
- This tightens the archived disposition that admitted a container ID without skills (`2026-10-02-forward-native-provider-options` `design.md:116`). The audit records it as a tightening (spec "Container consumption audit").
- Response metadata is unchanged (D7).
- File references on operator accounts have a similar exposure across providers. They are out of scope; a separate cross-provider issue owns them.

*Alternative:* refuse `container.id` only together with skills. Rejected because an ID-only request reaches the same container and its files.

### D9. Skills policy under request routing

- **Per-attempt policy, no new mechanism.** A routed chain reuses the startup-constructed candidates, so every routed attempt runs its own candidate's `Anthropic(policy)` validator after its own translation (spec "Candidate-native request semantics across routed attempts"). A refusing routed candidate makes no native request and the chain advances, as in D3.
- **No escalation.** Routing admits only destinations the caller can already select directly (spec "Authorized destinations only"). A caller who routes from an unpolicied route to a policied one gains nothing it could not get by calling the policied route directly, and `order`/`only` only reorder or filter candidates that each keep their own policy.
- **Single policy per destination (catalog validation rule).** Routed dedup skips a later candidate with the same provider instance and backend model and ignores its policy. If routes A and B both name `{anthropic-main, claude-x}` with different `skills`, a call to A with `models: [B]` would use A's placement, and a call to B with `models: [A]` would use B's, so the effective policy would depend on placement order. Catalog validation therefore requires every candidate (primary or fallback, in any route) that names the same provider instance and backend model to carry an equal skills policy, or for all of them to omit it. Policies are compared as sets: entries by `(type, skillId)`, each with the same set of `versions`, in any order. Otherwise `ParseModelCatalog` fails with a bounded error naming both candidate paths, for example `config: models.<a>.fallback[<i>].skills differs from models.<b>.primary.skills for the same provider and model`, without skill IDs. The cross-route pass runs after the existing per-route checks (`config/file.go:174-185`), so a same-route repeat keeps its "repeats a candidate" diagnostic. Models are checked in sorted ID order, so the error is deterministic. The rule holds for catalogs loaded through `gateway.New`; `buildCatalog` relies on a validated `File`. Operators who need different policies for one backend model declare a second provider instance.
- *Alternative:* the first placement wins. This was rejected because the policy that applies would depend on the caller's `models`/`order` choice, and it is hard to document and to test.
- **#562** has the same issue for its candidate fallbacks policy. The rule should cover the whole candidate Anthropic policy, so whichever change lands first introduces the check and the other extends the compared fields.

### D10. Aggregate chain-failure classification: the real failure wins

Without this, a policied fallback's outage or a cancellation after a refusing candidate surfaces as 400 (Context, "Failure classification today"). `execution.ClassifyProvider` gains an aggregate rule for chain failures, in the shapes the fallback module produces.

- **Refusal.** An attempt failure that the single-error rules classify as invalid request through `catalog.ErrUnsupportedRequest` or `provider.ErrUnsupportedFunctionality`. Both are pre-I/O "this candidate cannot take the request" errors. `ClassifyProvider` already classifies `ErrUnsupportedFunctionality` as invalid request, checked after API status and transport (`classify.go:129-131`), so an API error that wraps it stays an API error. The aggregate rule generalizes that precedent and treats both sentinels identically.
- **Aggregate.** A chain failure that joins two or more attempt failures, or one or more attempt failures with the call-level context error. The fallback module returns a single failure unwrapped (`fallback/fallback.go:319-321`), joins several failures as newest-first `fallback: provider %q model %q: %w` wrappers (L322-328), and, when the call context ends between attempts, joins `ctx.Err()` after them in a further top-level join (L118, L146). A refusal followed by a call cancellation is therefore `errors.Join(refusal, ctx.Err())`.
- **Call-level context error.** The last element of the top-level join, when it is identical to `context.Canceled` or `context.DeadlineExceeded`. An attempt failure that merely wraps `context.Canceled` or `context.DeadlineExceeded` is not call-level.

The aggregate rule:

1. A call-level cancellation or expired deadline wins first: cancellation or timeout.
2. Otherwise the most recent attempt failure that is not a refusal decides, through the existing single-error rules. The joined attempts are newest first.
3. Invalid request only when every attempt refused.

A failure that is not an aggregate keeps today's rules, so a lone refusal stays invalid request. Detection is structural, so a single provider error that itself joins a refusal with `context.Canceled` (the "cancellation wins" row of `TestUnsupportedFunctionalityClassification`) still classifies as cancellation. The change lives in `ai-gateway/internal/execution/classify.go` (`ClassifyProvider`, L102; the refusal check at L114), so `ClassifySelection`, ProviderWire (`providerwire/v4/errors.go:74`) and `internal/testadapter` follow it. The public `fallback` module is unchanged.

- **Scope.** It applies to every Gateway chain: configured fallback, routed, BYOK, and every provider. Existing chains change visibly, and the PR description lists each change:
  - an `ErrUnsupportedRequest` refusal next to another attempt failure now reports that failure, not invalid request: a 503 is overload, a 429 rate limit and an unknown error internal;
  - a refusal followed by a call cancellation or expired deadline is now cancellation or timeout;
  - in a chain without refusals the newest attempt now decides over an earlier attempt's error matched first today: an attempt-level `DeadlineExceeded` or `context.Canceled` followed by a newer 503 is now overload, not timeout or cancellation (`classify.go:116-119` matches the context error first today);
  - `TestUnsupportedFunctionalityClassification` (`providerwire/v4/runtime_test.go:1613-1648`): the rows "rejection then unknown error" and "unknown error then rejection" change from invalid request to internal, because the unknown error is the newest non-refusal in both. The rows "rejection then 503" and "503 then rejection" stay overload, and the other rows are unchanged.
- **Spec consistency.** `gateway-execution-runtime` gains "Aggregate chain-failure classification" beside "Closed failure classification", whose text is unchanged. `providerwire-v4-unary-runtime` "Unsupported-functionality rejection classification" (L520) is modified too: its body would otherwise keep a rejection next to an unknown error at invalid request, so it defers aggregates to the runtime rule and gains a "Rejection next to an unknown error" scenario; its other scenarios are unchanged. `gateway-ordered-text-fallback` "Fallback chain is exhausted" and `gateway-attempt-failure-evidence` ("unchanged classification") only state that discovery and evidence enrichment do not change classification; they defer to the runtime rule and do not contradict it. `gateway-request-byok` "Candidates are exhausted" likewise defers to "existing safe classification", and its exhausted-attempts clause (L344) keeps ProviderWire classification without a blanket exhaustion status.
- **#562** benefits identically: a refused native fallback followed by a later candidate's failure reports that failure.

*Alternatives:* keep "any refusal wins" (rejected: it hides real outages and cancellations behind 400) or pick the most severe failure (rejected: severity has no closed order, and the newest failure is what the chain ended on).

## Container consumption audit

Recorded before the enforcement change (spec "Container consumption audit"). Prior disposition: the `2026-10-02-forward-native-provider-options` audit (`design.md:116`) refused non-empty container skills and allowed "container ID without skills" in both modes.

| Consumed path | Decoding and precedence | Disposition | Paired cases |
| --- | --- | --- | --- |
| `anthropic` call namespace `container.skills` (`ResolveOption[AnthropicOptions]`, `validateContainerSkills`, `applyProviderOptions`) | Exact `anthropic` namespace; typed JSON decoding with case-insensitive field names; the skill ID resolves through `ContainerSkill.ResolveSkillID` (`skillId` for `anthropic`, `providerReference.anthropic` for `custom`), the same rule the adapter sends. | **Relaxed under policy.** Owner: the configured candidate's `skills.allow`; default deny (zero `AnthropicPolicy`). Configured: static bounds plus allowlist and exact version match. BYOK: static bounds only (`RequestAccountAnthropicPolicy`). | Allowed vs unlisted, wrong-version and no-policy skills in configured mode; bounded vs 21-skill, malformed and duplicate skills in both modes (`nativeoptions` 3.1, `service` 3.4-3.5, `byok` 3.6, command 3.7); empty, null or absent skills stay no-ops. |
| `anthropic` call namespace `container.id` (same path; sent only when non-empty, `convert_request.go:3141`) | As above; an empty ID is omitted by the adapter, so it is a no-op. | **Tightened.** Configured: refused with or without skills. BYOK: admitted when valid UTF-8 of at most 128 bytes. | Configured ID-only and ID-plus-skills refused, including after translation and in a heterogeneous chain; BYOK ID-only admitted at 128 bytes, refused at 129 bytes; empty ID and null container admitted in both modes. |
| Custom-part `container` scopes (`native_options_test.go:173`) | Not read by the adapter. | Inert, unchanged. | `container` never reaches the native body. |

The two-tenant case (spec "Two tenants call one candidate") holds by construction, because the validator takes no tenant input; command task 3.7 covers it, and the routed cases in 3.5 cover per-candidate policy under routing.

## Work packages and delivery

1. **WP1, `providers/anthropic` parity:** tests first (request snapshots in `convert_request_test.go`). Breaking for the provider module.
2. **WP2, config:** add `Primary.Skills`, its validation and the D9 per-destination consistency check, inert until WP3.
3. **WP3, native enforcement:** `Anthropic(policy)` with the skills and container-ID checks, catalog and BYOK wiring, existing tests updated, and docs and spec records.
4. **WP4, aggregate classification (D10):** tests first in `classify_test.go`, then `ClassifyProvider`, then service-level chain cases.

**Modules and release.** One PR may carry WP1-4. The Gateway builds from same-revision SDK source through the `ai-gateway/go.mod` replace directives (`CONTRIBUTING.md:567-569`), and libraries release together from one release PR (`CONTRIBUTING.md:536-540`). There is no manual publication order, tag or publish step. Run `mise run tidy` and commit any `go.mod`/`go.sum` change. Release intent comes from the PR title: breaking for `providers/anthropic` (WP1), `feat` for the Gateway.

**Archive and merge gating.** A PR that touches `openspec/` must archive its change before merge (`CONTRIBUTING.md:328-329,497`). This change therefore archives on merge together with WP1-WP4, the spec deltas and the docs. The evidence is local: the Go and TS command tests plus the direct Go replay of `anthropic/upstream/code-execution-pptx-skill`. The PR closes #563 (`Closes #563`).
- **WP1** (provider parity; no `main` requirement conflicts with it) and **WP2** (inert config) may merge earlier, but only as their own PRs that carry no unarchived OpenSpec change.
- **WP3 and WP4** must merge only together with the spec deltas and the archive. `main`'s `gateway-request-byok` still requires refusing container skills (`openspec/specs/gateway-request-byok/spec.md` L36, L41, L46, L309, L314, L318) and admitting ordinary container IDs (L50, L326), `gateway-execution-runtime` "Closed failure classification" (L135) does not define the aggregate rule, and `providerwire-v4-unary-runtime` "Unsupported-functionality rejection classification" (L520) keeps a rejection next to an unknown error at invalid request, so either package without the archived deltas would contradict the main specs.

## Divergence classification

| Difference vs upstream 4.0.94 | Classification | Record |
| --- | --- | --- |
| Missing `providerReference` | Implementation bug (parity gap) | Fixed in WP1, and dropped from #216 |
| Missing skills code-execution beta and warning | Implementation bug | Fixed in WP1 |
| Custom skill accepted by `skillId`; untyped skill `type` | Implementation bug | Fixed in WP1 (dropped, typed, unknown types rejected) |
| Gateway allowlist, 20-skill bound, distinctness, configured refusal of caller `container.id`, BYOK container-ID byte bound | Gateway-owned support boundary | `gateway-anthropic-skills` spec, not knownGaps |
| Container without skills is sent as an object `{id}` rather than upstream's bare ID string; serialization of an empty container is unverified | Coverage gap, out of scope | Note added to #216 |
| Vertex forwards skills ungated | Coverage gap, not a Gateway type | Note added to #216 |
| Empty or `null` `providerReference.anthropic` is rejected; upstream `resolveProviderReference` forwards `""` (it returns `reference[provider]` when `!= null`) and fails `null` in its `record<string,string>` schema | Intentional deviation: Go's `map[string]string` cannot distinguish `null` from `""`, and an empty skill ID is never valid | `complex-server-tools` capability spec ("Empty anthropic reference"), not knownGaps |

D10 is Gateway-owned classification with no upstream counterpart. `PARITY.md` is unchanged: the AI Gateway row stays `mixed` and no surface or evidence location moves.

## Risks / Trade-offs

- **Configured models lose container reuse.** Every caller `container.id` is refused, so `pause_turn` continuation and multi-turn work on one container need BYOK. → Docs state it; the response still reports the container.
- **Caller-supplied file references on operator accounts** (Anthropic `file_id`, OpenAI file IDs, openai-compatible `file.file_id` from #598) have a similar cross-caller exposure and stay admitted. → Separate cross-provider issue; this change does not alter file references.
- **Custom skills are shared by every caller of a candidate.** → Explicit allowlist, docs wording, and no wildcard.
- **Classification changes for existing chains (D10).** Mixed configured, routed and BYOK chains that ended with a refusal next to another failure now report that failure or the cancellation instead of 400, and the newest attempt now decides in chains without refusals. Clients that keyed on 400 see overload, rate limit, timeout, cancellation or internal. → PR description and docs state it; a chain where every candidate refused still returns 400.
- **Anthropic may change its documented skill limit.** → One constant whose comment cites the source.
- **Merge conflict with #562** in `options.go`, `catalog.go`, `file.go` and the byok spec. → D2 defines the merge shape; D9 the shared consistency check.
- **The consistency rule can reject catalogs that share a backend model across routes with different policies.** Existing catalogs have no policy, so they load unchanged. → The error names both paths, and docs describe the second-instance workaround.

## Migration Plan

Configs without `skills` load unchanged. Two Gateway behaviors change for existing callers: configured Anthropic candidates now refuse a caller `container.id`, and aggregate chain failures classify by D10. The provider change is breaking for Go callers that send custom skills by `skillId` or set an untyped `Type`. Rollback means reverting the change; WP1 can stay because it is provider parity.

## Validation

- `mise run fmt`, `mise run lint`, `mise run vet`, `mise run test`.
- `cd providers/anthropic && go test ./...`
- `mise run tidy` (commit any `go.mod`/`go.sum` change) and `MODULE=providers/anthropic mise run verify-module`
- `cd ai-gateway && go test -race ./internal/execution/... ./pkg/internal/...`
- `mise run test-ai-gateway-command`
- `mise run parity-check`
- `openspec validate gateway-anthropic-container-skills --strict`
- `cd test/conformance && go test -tags conformance -run 'TestConformance/upstream/code-execution-pptx-skill$' -v ./anthropic/` (direct Go replay of the pptx fixture). The output must contain `--- PASS: TestConformance/upstream/code-execution-pptx-skill`; a run without that line is not evidence.

## Open Questions

None.
