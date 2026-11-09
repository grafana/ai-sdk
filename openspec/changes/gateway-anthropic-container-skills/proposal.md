# Proposal

## Why

Gateway callers get `400 invalid request` for any Anthropic `providerOptions.anthropic.container.skills`. `nativeoptions.Anthropic` refuses skills before native I/O in both invocation modes (`nativeoptions/options.go:47-50`, `gateway-request-byok` spec L36, L41, L45-47). As a result, a request shaped like the upstream `anthropic/upstream/code-execution-pptx-skill` fixture cannot reach the backend through the Gateway (#563).

Custom skills and containers are resources of the operator's Anthropic account. The refusal can only be lifted if an operator decides what to share and the Gateway prevents callers from reaching resources they were not granted.

## What Changes

- **Anthropic provider parity (`providers/anthropic`, breaking):**
  - `ContainerSkill.Type` becomes a typed `ContainerSkillType` (`anthropic`, `custom`); unknown types fail request building, matching upstream's discriminated union.
  - Custom skills resolve `skill_id` only from upstream's `providerReference.anthropic`. The legacy custom `skillId` input is dropped: a custom skill without `providerReference.anthropic` fails request building.
  - Container skills always add the `code-execution-2025-08-25` beta.
  - A request with skills but no `code_execution_20250825`/`_20260120` tool gets upstream's `other` warning: "code execution tool is required when using skills".
- **Candidate skills policy (ai-gateway config):**
  - Adds an optional per-candidate `skills.allow` list on a route's `primary` and on each `fallback` entry. Each entry has a `type` (`anthropic` or `custom`), a `skillId` and optional `versions`.
  - The policy is valid only on `anthropic` provider candidates and is checked when the `model_catalog` setting is parsed (`config.ParseModelCatalog`).
  - Every candidate, in any route, that names the same provider instance and backend model carries an equal policy or none, so request routing's destination dedup cannot change which policy applies.
  - Absent policy means deny, which is today's behavior.
- **Native enforcement (`nativeoptions`):**
  - Static skill bounds apply in both modes: at most 20 skills (Anthropic's documented per-request limit), known type first, then a non-empty resolved ID of at most 128 bytes, an optional version (empty means omitted) of at most 128 bytes, and distinct entries. A custom skill without `providerReference.anthropic` is malformed.
  - Configured candidates also enforce their allowlist. Routed chains (`gateway.models`/`order`/`only`, #596) apply each candidate's own policy per attempt.
  - **Configured candidates refuse every caller-supplied `container.id`**, with or without skills. This tightens today's admission of an ID-only container. Container reuse (`pause_turn` continuation, multi-turn) is therefore unavailable on configured models; response `providerMetadata.anthropic.container` is still returned unchanged.
  - Refusal uses the existing `catalog.ErrUnsupportedRequest` before native I/O, without echoing IDs.
- **BYOK (request-account) mode:**
  - Admits `anthropic` and `custom` skills under the static bounds, and a `container.id` of at most 128 bytes of valid UTF-8, with or without skills. These are the key holder's own resources.
  - BYOK never consults configured policy.
- **Aggregate chain-failure classification (`ai-gateway/internal/execution`):** for a failure that joins several attempt failures, a call-level cancellation or deadline wins first; otherwise the most recent non-refusal attempt failure decides; invalid request only when every attempt refused. This applies to every Gateway chain (configured fallback, routed, BYOK, all providers) and visibly changes classification for existing mixed chains. The public `fallback` module is unchanged.
- **Shared plumbing with #562:** a single candidate-scoped Anthropic policy is passed into `nativeoptions.Anthropic(policy)`. Whichever of this change and `gateway-anthropic-server-fallbacks` lands first introduces that plumbing; the other extends it.
- **Records and docs:** records the guard-weakening audit for skills, the guard tightening for configured container IDs, and the account-scoping decision. Caller-supplied file references on operator accounts are out of scope and move to a separate cross-provider issue.
- This change closes #563.

## Capabilities

### New Capabilities

- `gateway-anthropic-skills`: covers the operator policy for Anthropic container skills per configured candidate, its per-destination consistency under request routing, the request bounds, configured refusal of caller container IDs, BYOK admission, account scoping and evidence boundaries.

### Modified Capabilities

- `gateway-request-byok`: skills are no longer refused in either mode; they follow `gateway-anthropic-skills`, which bounds them in BYOK and policy-gates them in configured mode. Container IDs survive only in BYOK; configured candidates refuse them.
- `gateway-native-provider-options`: the guard audit covers policy-gated relaxations and tightenings, and records the container path.
- `gateway-provider-configuration`: candidate native-option policies are configured state that BYOK never consults.
- `gateway-execution-runtime`: "Closed failure classification" gains the aggregate-chain rule.
- `providerwire-v4-unary-runtime`: "Unsupported-functionality rejection classification" treats the rejection as a refusal in chain aggregates, so a rejection next to an unknown error is internal.
- `complex-server-tools`: adds the Anthropic container-skills request mapping (`providerReference`, required betas and the missing-code-execution warning) next to the code-execution tool definitions it depends on.

## Impact

- **Code:**
  - `providers/anthropic`: `options.go:221-232` and `convert_request.go:218-220,3139-3158`.
  - `ai-gateway/pkg/internal`: `config/file.go:51-55,99-209`, `nativeoptions/options.go:15-81`, `service/catalog.go:76-82` and `byok/model.go:67-69,78`, plus tests.
  - `ai-gateway/internal/execution/classify.go:102-133` (`ClassifyProvider`), plus `classify_test.go`.
- **API and behavior:**
  - Breaking for `providers/anthropic` Go callers: typed `ContainerSkillType`, new `ContainerSkill.ProviderReference` and a skill-ID resolver; custom skills set only through `skillId` now fail. The PR title carries breaking intent (`feat!:` or a `BREAKING CHANGE:` footer, `CONTRIBUTING.md:518-527`).
  - A new strict-YAML candidate field. Existing configs load unchanged.
  - Visible Gateway changes for existing callers: configured Anthropic candidates refuse a caller `container.id`, and aggregate chain failures classify by the new rule. The PR description states both.
- **Modules:** one PR may carry the provider and Gateway changes. The Gateway builds from same-revision SDK source through the `ai-gateway/go.mod` replace directives (`CONTRIBUTING.md:567-569`), and libraries release together from one release PR (`CONTRIBUTING.md:536-540`); there is no manual publication order or tagging. Run `mise run tidy`, commit any `go.mod`/`go.sum` changes, and validate with `MODULE=providers/anthropic mise run verify-module`.
- **Merge gating:** this change archives on merge together with WP1-WP4, the spec deltas and the docs, and the PR closes #563. The evidence is local: the Go and TS command tests plus the direct Go replay of the pptx fixture. WP1 and WP2 may merge earlier only as their own PRs without an unarchived OpenSpec change; WP3 and WP4 merge only with the spec deltas and the archive (design "Work packages and delivery").
- **Baseline:** `@ai-sdk/anthropic` 4.0.94 (`test/conformance/upstream.yaml:8`); skills behavior is unchanged since 4.0.82. No pins or recorded fixtures change.
- **Related issues:**
  - Overlaps #216 ("custom skill providerReference"). Update #216 to drop the skills items.
  - #562 shares the policy plumbing (D2) and the routed-dedup consistency rule (D9), and benefits identically from the classification rule (D10).
  - A new cross-provider issue owns caller-supplied file references on operator accounts (Anthropic `file_id`, OpenAI file IDs, openai-compatible `file.file_id` from #598).
