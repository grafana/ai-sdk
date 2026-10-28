# Tasks

## 1. WP1: Anthropic provider skills parity (`providers/anthropic`, breaking)

The PR title carries breaking intent for `providers/anthropic` (`feat!:` or a `BREAKING CHANGE:` footer, `CONTRIBUTING.md:518-527`).

- [ ] 1.1 Add failing request tests in `convert_request_test.go` (`TestBuildParams_ContainerSkills`). They should cover:
  - custom `providerReference.anthropic` mapped beside an anthropic skill;
  - a reference without `anthropic`, which fails with exactly `anthropic: invalid provider options: container skill <i>: no provider reference found for provider anthropic; available providers: openai` (one `anthropic:` prefix) and sends no request;
  - a custom skill with `skillId` and no `providerReference`, which fails with the same message and an empty provider list, and sends no request;
  - a custom skill whose `providerReference.anthropic` is `""` or `null`, which fails with exactly `anthropic: invalid provider options: container skill <i>: empty provider reference for provider anthropic` and sends no request;
  - an unknown skill type, which fails with exactly `anthropic: invalid provider options: container skill <i>: unsupported type "<type>"` and sends no request;
  - an empty `version`, which is omitted from the body;
  - skills without a code-execution tool: all three betas once, plus the `other` warning;
  - skills with `code_execution_20250825` or `_20260120`: no warning;
  - an ID-only container, unchanged.

  Port the cases from upstream 4.0.82 `anthropic-language-model.test.ts` "agent skills" (L5347+). Verify that `cd providers/anthropic && go test -run TestBuildParams_ContainerSkills ./...` fails.
- [ ] 1.2 Add the typed `ContainerSkillType` (`ContainerSkillAnthropic`, `ContainerSkillCustom`), `ContainerSkill.ProviderReference` and an exported `ContainerSkill.ResolveSkillID`, reusing the `resolveAnthropicFileReference` error shape without its `anthropic:` prefix and erroring on an empty or null `anthropic` value (design D6, "Error messages"). Add `validateContainerSkills`, which prefixes the skill index, in `buildParams` after `validateFallbackConfig` (`convert_request.go:215-217`), returning `anthropic: invalid provider options: %w`; keep `applyProviderOptions` error-free and map skills through the resolver there, with the `code-execution-2025-08-25` beta and the warning (design D6). Run `mise run tidy` and commit any `go.mod`/`go.sum` change. Verify that 1.1 passes and that `cd test/conformance && go test -tags conformance -run 'TestConformance/upstream/code-execution-pptx-skill$' -v ./anthropic/` still passes, with output containing `--- PASS: TestConformance/upstream/code-execution-pptx-skill`.
- [ ] 1.3 Update godoc for `Container`/`ContainerSkill` and `docs/providers/anthropic.md` (extend the container options text near L95 with a short skills section): providerReference, the warning, and the fact that code execution is required. Verify with `cd providers/anthropic && go doc . ContainerSkill` and `mise run lint-docs`.
- [ ] 1.4 Verify the provider module with `MODULE=providers/anthropic mise run verify-module`, which downloads, verifies, builds and tests it with its local first-party replacements (`mise.toml:203,207`).

## 2. WP2: Candidate skills policy config

- [ ] 2.1 Add failing cases to `config/file_test.go` (`TestLoadFile_CandidateSkillsPolicy`). They should cover:
  - a valid policy on an `anthropic` primary and on a fallback entry;
  - rejection on `openai` and `openai-compatible` candidates;
  - rejection of an empty `allow`, an unknown type, a repeated entry, an empty, blank, invalid-UTF-8 or over-128-byte ID or version, an empty `versions`, a repeated version and an unknown field;
  - two fallback entries that differ only by policy, still rejected as repeated;
  - diagnostics that name the path without values.

  Verify that `cd ai-gateway && go test ./cmd/grafana-ai-gateway/internal/config/...` fails.
- [ ] 2.2 Add `Primary.Skills`, `SkillsPolicy`, `AllowedSkill` and the typed `SkillType`. Validate them in `File.Validate`, and key duplicate detection on `{Provider, Model}` (design D1). If #562 already did this, reuse it. Verify that 2.1 passes and that the existing config tests are unchanged.
- [ ] 2.3 Assert that configured discovery output excludes the policy (`service` discovery test). Verify with `cd ai-gateway && go test ./cmd/grafana-ai-gateway/internal/service/...`.

## 3. WP3: Native enforcement (configured and BYOK)

- [ ] 3.1 Add failing cases to `nativeoptions/options_test.go` (`TestAnthropic_SkillsPolicy`).
  - In configured mode, these are allowed: a listed anthropic skill, a listed custom skill by `providerReference.anthropic`, a pinned version, and an empty `version` against an entry without `versions`.
  - In configured mode, these are refused: an unlisted skill, a wrong version, an omitted or empty version against a pinned entry (including `versions: ["latest"]`), skills plus `container.id`, and any skill with no policy.
  - Policy representation (design D2): `AllowlistedAnthropicSkills` with nil or empty grants returns the denied zero value and refuses every skill, and no configured constructor yields bounded mode (only `RequestAccountAnthropicPolicy()` admits bounded skills without an allowlist). In `service/catalog_test.go` (with 3.4), a configured policy converted from a nil `*config.SkillsPolicy` or an empty `allow` (bypassing config validation) refuses every skill.
  - In BYOK mode, bounded custom skills plus `container.id` are allowed.
  - In both modes, these are refused: nine skills, an unknown type, an empty or over-128-byte resolved ID, an over-128-byte or invalid-UTF-8 version, a duplicate, a reference without `anthropic`, an empty or `null` `providerReference.anthropic`, and a custom skill with `skillId` but no `providerReference`.
  - Empty, null or absent skills, ID-only containers and MCP checks stay unchanged.

  Verify that `cd ai-gateway && go test ./cmd/grafana-ai-gateway/internal/nativeoptions/...` fails.
- [ ] 3.2 Before implementing enforcement, record the audit row for the skills path (design D3/D4, spec "Container skills consumption audit") in this change's design.md: the consumed path, its decoding and custom-ID resolution, the configured and BYOK dispositions and the paired bypass/harmless cases. Verify that each paired case maps to a case planned in 3.1 or 3.4-3.7, including the two-namespace case in 3.7 for spec "Two tenants call one candidate", and re-check after 3.7 that those cases exist.
- [ ] 3.3 Implement `AnthropicPolicy`, `AnthropicSkillsPolicy` with its unexported mode (zero value deny), `AllowlistedAnthropicSkills`, `RequestAccountAnthropicPolicy()` and `Anthropic(policy)` following design D2/D3, with `maxAnthropicSkills` and `maxAnthropicSkillBytes` as their own constants, merging with #562's plumbing if it has landed. Wire one validator per descriptor in `service/catalog.go:55-61` and the request-account policy in `byok/model.go:21`. Verify that 3.1 passes and that `cd ai-gateway && go build ./...` succeeds.
- [ ] 3.4 Update `service/catalog_test.go:111-113` so the pptx skill is still refused on the unpolicied route. Add:
  - a policied route that reaches the fake backend with `container.skills`;
  - a route whose unpolicied primary refuses and whose policied fallback serves: the primary makes no native request, the chain advances (`ErrUnsupportedRequest` is a plain error, so `execution.FallbackDecider` → `fallback.DefaultDecider` returns true), and the fallback's request carries the skills;
  - a route where no candidate allows the skills: the fixed invalid-request error and zero native requests;
  - a route whose unpolicied primary refuses and whose policied fallback returns 503: a `buildCatalog` test whose fake fallback candidate returns `*provider.APICallError{StatusCode: 503}`, with the call's error classified through `execution.ClassifyProvider` as `CategoryInvalidRequest` (400) and exactly one native request, the fallback's. It lives here because `nativeOptionsRequest` cannot express a refusing primary with a failing policied fallback. This pins the current classification, where any refusal in the joined chain error wins (design Context, "Failure classification").

  Verify with `cd ai-gateway && go test -race ./cmd/grafana-ai-gateway/internal/service/...`.
- [ ] 3.5 Rewrite every existing `{"type":"custom","skillId":...}` Gateway test payload, which design D6 makes malformed. Tests that use skills as the canonical refusal vector keep their assertions; only the payload changes. Admitted cases are separate rows.
  - `service/native_options_test.go:39` `TestBYOKSelection_NativeOptionGuards`: rename the existing "container skills" row to "malformed custom skill"; it stays a refusal and keeps `{"type":"custom","skillId":"external"}` as an intentionally malformed skill (custom without `providerReference.anthropic`). Add an admitted row with `{"type":"custom","providerReference":{"anthropic":"external"}}` and a refused nine-skill row.
  - `service/native_options_test.go:173` ("anthropic custom scopes cannot enable guarded features"): a custom-part scope that must stay inert. Keep the payload and assertions; `container` must still not appear in the native body.
  - `service/native_options_test.go:503` `nativeOptionsRequest`: add a `*config.SkillsPolicy` parameter that the helper sets on both the primary and the fallback candidate, so the `configuredFallback` path (primary 503, then fallback) applies the policy on each candidate. Existing callers pass `nil`.
  - `service/native_options_test.go:276` `TestNativeOptions_BypassAndOrdinaryControls`: keep `skills` denied without a policy, and add an allowed row under a policy for both `configuredFallback` values.
  - `service/selection_test.go:291-306` `TestBYOKSelection_RejectedBeforeAttemptsHasNoOverview`: keep the 400, zero calls and no-overview assertions; replace the now-admissible `{"type":"anthropic","skillId":"skill"}` body with nine distinct skills.
  - `service/translation_test.go:232-262` `TestTranslation_AnthropicNativeValidation`: keep `{"type":"custom","skillId":"external"}` as an intentionally malformed skill, refused in both the configured and BYOK branches with one native request (the valid call). Add a BYOK row where a translated `providerReference.anthropic` custom skill is admitted.
  - `service/translation_test.go:272-309` `TestTranslation_BYOKAnthropicValidation`: both subtests ("request options refused before any account", "translated options refused on each account") keep the malformed `skills` payload at L276, the `ErrUnsupportedRequest` and invalid-request assertions, the attempt outcomes (`nil` and `[failed, failed]`) and zero requests.

  Verify with `cd ai-gateway && go test -race ./cmd/grafana-ai-gateway/internal/service/...`.
- [ ] 3.6 Update `byok/model_test.go` `TestNew_AnthropicNativeOptionGuards` (L626): rename the "container skills" row to "malformed custom skill"; it keeps its refusal with zero calls and its `{"type":"custom","skillId":"external"}` payload as an intentionally malformed skill. Add rows for a nine-skill refusal and an admitted `providerReference.anthropic` custom skill with `container.id` that reaches native I/O, in unary and streaming modes. Verify with `cd ai-gateway && go test ./cmd/grafana-ai-gateway/internal/byok/...`.
- [ ] 3.7 Extend `ai-gateway/test/providerwire-v4/gateway-command.test.ts` ("protects Anthropic execution controls", L564). The existing unpolicied refusal stays. Add cases for both clients:
  - a policied model whose fake Anthropic request has `container.skills` and the three betas;
  - a disallowed skill on that model, which gets the fixed 400;
  - a BYOK custom skill (`providerReference.anthropic` form) with `container.id`, which is forwarded;
  - two callers in different namespaces calling the same policied model with the listed custom skill: both reach the fake Anthropic backend with the same `container.skills`, and an unlisted skill from either namespace gets the fixed 400 (spec "Two tenants call one candidate").

  Verify with `mise run test-ai-gateway-command`.
- [ ] 3.8 Label `maxAnthropicSkills = 8` in its definition comment as a Gateway resource bound, so an offline run does not block. Confirm Anthropic's per-request skill limit from the official docs when available, adjust the value if it differs, and cite the source. Verify that the constant and comment match.
- [ ] 3.9 Update the docs:
  - `ai-gateway/README.md`: the `skills.allow` YAML example, Anthropic-only, and deny by default.
  - `docs/providers/grafana-gateway.md` ("Provider-defined tools"): configured versus BYOK behavior; keep skills policies consistent across a route's Anthropic candidates, because any refusal in the chain makes the call a 400 invalid request, so a fallback outage or a cancellation after a refusal also surfaces as 400; no `container.id` with skills on configured models, so configured-mode skills are single-request and `pause_turn`/multi-turn container reuse is unavailable until F1; `file_id`s not downloadable through the Gateway; allowlist version matching (omitted matches only entries without `versions`, `"latest"` is literal).
  - `docs/best-practices/security.md`: custom skills "shared by all callers of this candidate", and the container-ID risk.
  - `docs/guides/gateway-authentication.md`: BYOK skills note, including that account-scoped `container.id` and custom skill IDs fail at the provider on a later BYOK account.
  - `docs/concepts/gateway-glossary.md`: extend the "Guard; policy; preflight" row with "skills policy".
  - `docs/concepts/gateway-architecture.md`: per-candidate option policy in step 4, if the text no longer matches.

  Verify with `mise run lint-docs`.
- [ ] 3.10 Update godoc for `config.Primary`, `config.SkillsPolicy` and the `nativeoptions` policy types. Verify with `go doc` on each.

## 4. Delivery and archive (option (a))

This change archives on merge together with WP1-WP3, the spec deltas and the docs (design "Work packages and delivery"). Gateway conformance-matrix evidence and issue acceptance #1 belong to the matrix follow-up, delivered with or after #201.

- [ ] 4.1 Run the direct Go replay of the pptx fixture, `cd test/conformance && go test -tags conformance -run 'TestConformance/upstream/code-execution-pptx-skill$' -v ./anthropic/`, against unchanged inputs. Verify that it passes and that its output contains `--- PASS: TestConformance/upstream/code-execution-pptx-skill` (a bare `pptx` run pattern matches no top-level test and passes vacuously), alongside the Go and TS command tests from 3.4-3.7.
- [ ] 4.2 If WP1 or WP2 merges earlier, do so only as its own PR that carries no unarchived OpenSpec change. If WP2 merges before WP3, the `config.Primary.Skills` and `config.SkillsPolicy` godoc must state that the field is inert (parsed and validated, not enforced) until WP3 lands; otherwise merge WP2 together with WP3. Merge WP3 only in the PR that carries the spec deltas and archives this change, because `main`'s `gateway-request-byok` (L36, L41, L46, L300, L305, L309) still requires refusing skills. Verify that `openspec validate gateway-anthropic-container-skills --strict` passes and that the change is archived in that PR.
- [ ] 4.3 In that PR's description, reference #563 without a closing keyword, state that issue acceptance #1 is owned by the matrix follow-up, and link the follow-up. Verify that the description does not close #563.

## 5. Integration checks and records

- [ ] 5.1 Run `mise run fmt`, `mise run vet`, `mise run lint`, `mise run test`, `mise run parity-check`, `MODULE=providers/anthropic mise run verify-module` and `openspec validate gateway-anthropic-container-skills --strict`. Verify that all pass.
- [ ] 5.2 Confirm that `test/conformance/PARITY.md` needs no edit, because no surface status or evidence location changes. Verify that `git diff --stat test/conformance/PARITY.md` is empty.

## Workflow follow-up

- Update #216: drop "custom skill providerReference" and the skills beta/warning items. Add the container wire-form gap (bare ID string, empty container) and the observation that Vertex forwards skills ungated.
- Open follow-up F1 for nara: configured-mode ownership of caller-supplied `container.id` and file IDs, and `pause_turn`/multi-turn container reuse with skills (design D7, Risks).
- Open follow-up F2 for nara (out of scope here): whether to change chain-failure classification so that a refusal no longer outranks a later candidate's outage or a cancellation (design Context "Failure classification", Risks). Until then, task 3.4 pins the current behavior.
- Open the matrix follow-up, delivered with or after #201: the harness emits `primary.skills.allow` derived from each row's config in `test/conformance/tools/gateway-runtime.mts` `catalog()`, with a case in `gateway-runtime.test.mts` (`mise run test-conformance-gateway-harness`), and then both `anthropic/upstream/code-execution-pptx-skill` client rows pass `GRAFANA_VERSION=… mise run test-conformance-gateway` against unchanged fixtures (design D8). It owns issue acceptance #1; #563 stays open until it lands.
- Get nara's confirmation on D5 (BYOK bounded admission) and on archive/merge gating option (a) before implementation starts (design "Work packages and delivery").
- Coordinate with `gateway-anthropic-server-fallbacks` (#562) on D2. The later change rebases its `gateway-request-byok` delta.
- Archive in the PR that carries WP3 and the spec deltas (task 4.2).
