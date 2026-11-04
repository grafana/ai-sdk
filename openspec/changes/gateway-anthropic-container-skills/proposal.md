# Proposal

## Why

Gateway callers get `400 invalid request` for any Anthropic `providerOptions.anthropic.container.skills`. `nativeoptions.Anthropic` refuses skills before native I/O in both invocation modes (`nativeoptions/options.go:47-50`, `gateway-request-byok` spec L36, L41, L45-47). As a result, both client rows of the upstream `anthropic/upstream/code-execution-pptx-skill` fixture fail in the #201 Gateway matrix with zero backend calls (#563).

Custom skills and container IDs are resources of the operator's Anthropic account. The refusal can only be lifted if an operator decides what to share and the Gateway prevents callers from reaching resources they were not granted.

## What Changes

- **Anthropic provider parity (`providers/anthropic`, breaking):**
  - `ContainerSkill.Type` becomes a typed `ContainerSkillType` (`anthropic`, `custom`); unknown types fail request building, matching upstream's discriminated union.
  - Custom skills resolve `skill_id` only from upstream's `providerReference.anthropic`. The legacy custom `skillId` input is dropped: a custom skill without `providerReference.anthropic` fails request building.
  - Container skills always add the `code-execution-2025-08-25` beta.
  - A request with skills but no `code_execution_20250825`/`_20260120` tool gets upstream's `other` warning: "code execution tool is required when using skills".
- **Candidate skills policy (ai-gateway config):**
  - Adds an optional per-candidate `skills.allow` list on a route's `primary` and on each `fallback` entry. Each entry has a `type` (`anthropic` or `custom`), a `skillId` and optional `versions`.
  - The policy is valid only on `anthropic` provider candidates and is checked when the `model_catalog` setting is parsed (`config.ParseModelCatalog`).
  - Every candidate, in any route, that names the same provider instance and backend model carries an equal policy or none, so request routing's destination dedup cannot change which policy applies *(confirm with nara)*.
  - Absent policy means deny, which is today's behavior.
- **Native enforcement (`nativeoptions`):**
  - Static bounds apply in both modes: at most 8 skills, known type first, then a non-empty resolved ID of at most 128 bytes, an optional version (empty means omitted) of at most 128 bytes, and distinct entries. A custom skill without `providerReference.anthropic` is malformed.
  - Configured candidates also enforce their allowlist and refuse `container.id` combined with skills. Routed chains (`gateway.models`/`order`/`only`, #596) apply each candidate's own policy per attempt.
  - Refusal uses the existing `catalog.ErrUnsupportedRequest` before native I/O, without echoing IDs.
- **BYOK (request-account) mode:**
  - Admits `anthropic` and `custom` skills and `container.id` subject only to the static bounds. These are the key holder's own resources.
  - BYOK never consults configured policy.
- **Shared plumbing with #562:** a single candidate-scoped Anthropic policy is passed into `nativeoptions.Anthropic(policy)`. Whichever of this change and `gateway-anthropic-server-fallbacks` lands first introduces that plumbing; the other extends it.
- **Records and docs:**
  - Records the guard-weakening audit, the account-scoping decision and the `container.id` follow-up risk.
  - Records a matrix follow-up, delivered with or after #201: the harness emits an allowing policy for the pptx row, and that follow-up owns issue acceptance #1.

## Capabilities

### New Capabilities

- `gateway-anthropic-skills`: covers the operator policy for Anthropic container skills per configured candidate, its per-destination consistency under request routing, the request bounds, BYOK admission, account scoping and evidence boundaries.

### Modified Capabilities

- `gateway-request-byok`: skills are no longer refused in either mode. Instead they follow `gateway-anthropic-skills`, which bounds them in BYOK and policy-gates them in configured mode.
- `gateway-native-provider-options`: the guard-weakening audit covers policy-gated relaxations and records the skills path.
- `gateway-provider-configuration`: candidate native-option policies are configured state that BYOK never consults.
- `complex-server-tools`: adds the Anthropic container-skills request mapping (`providerReference`, required betas and the missing-code-execution warning) next to the code-execution tool definitions it depends on.

## Impact

- **Code:**
  - `providers/anthropic`: `options.go:221-232` and `convert_request.go:218-220,3139-3158`.
  - `ai-gateway/pkg/internal`: `config/file.go:51-55,99-209`, `nativeoptions/options.go:42-81`, `service/catalog.go:76-82` and `byok/model.go:67-69,78`, plus tests.
- **API:**
  - Breaking for `providers/anthropic` Go callers: typed `ContainerSkillType`, new `ContainerSkill.ProviderReference` and a skill-ID resolver; custom skills set only through `skillId` now fail. The PR title carries breaking intent (`feat!:` or a `BREAKING CHANGE:` footer, `CONTRIBUTING.md:518-527`).
  - A new strict-YAML candidate field. Existing configs load unchanged.
- **Modules:** one PR may carry the provider and Gateway changes. The Gateway builds from same-revision SDK source through the `ai-gateway/go.mod` replace directives (`CONTRIBUTING.md:567-569`), and libraries release together from one release PR (`CONTRIBUTING.md:536-540`); there is no manual publication order or tagging. Run `mise run tidy`, commit any `go.mod`/`go.sum` changes, and validate with `MODULE=providers/anthropic mise run verify-module`.
- **Merge gating (option (a)):** this change archives on merge together with WP1-WP3, the spec deltas and the docs. Its evidence is the Go and TS command tests plus the direct Go replay of the pptx fixture, so it does not wait for #201. WP1 and WP2 may merge earlier only as their own PRs without an unarchived OpenSpec change; WP3 merges only with the spec deltas and the archive. A follow-up, delivered with or after #201, updates `test/conformance/tools/gateway-runtime.mts` and owns issue acceptance #1. #563 stays open until it lands, so this PR must not close #563 (design "Work packages and delivery").
- **Baseline:** `@ai-sdk/anthropic` 4.0.94 (`test/conformance/upstream.yaml:8`); skills behavior is unchanged since 4.0.82. No pins or recorded fixtures change.
- **Related issues:**
  - Overlaps #216 ("custom skill providerReference"). Update #216 to drop the skills items.
  - The matrix follow-up depends on #201. This change does not.
  - #562 faces the same routed-dedup issue for its candidate fallbacks policy; the consistency rule should cover both (design D9).
