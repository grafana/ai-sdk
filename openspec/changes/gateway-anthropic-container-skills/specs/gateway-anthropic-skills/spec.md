# Spec Delta

## Purpose

Define how the Gateway admits Anthropic container skills (`providerOptions.anthropic.container.skills`): an operator policy on each configured candidate, static request bounds enforced at native consumption, BYOK admission, and the account-scoping boundary for operator-owned skills and containers.

## ADDED Requirements

### Requirement: Candidate skills policy configuration
A configured candidate (route `primary` or `fallback` entry) SHALL accept an optional `skills.allow` list of entries, each with `type` (`anthropic` or `custom`), `skillId` and optional `versions`. An absent policy SHALL admit no skill. The policy SHALL be operator configuration only, SHALL apply to that candidate alone and SHALL NOT appear in discovery, errors or caller-visible output.

#### Scenario: Policy on a direct Anthropic candidate
- **WHEN** an `anthropic` primary declares `skills.allow: [{type: anthropic, skillId: pptx}]`
- **THEN** configuration SHALL load, keep the policy for that candidate only, and leave discovery rows unchanged

#### Scenario: Custom skill entry
- **WHEN** a policy lists `{type: custom, skillId: skill_01X}`
- **THEN** it SHALL match a request custom skill whose `providerReference.anthropic` resolves to `skill_01X`

#### Scenario: Policy is absent
- **WHEN** a candidate omits `skills`
- **THEN** that candidate SHALL refuse every non-empty container skills list, as before this change

### Requirement: Strict skills policy validation
Model catalog validation (the `model_catalog` setting) SHALL fail before secret resolution or provider construction when a policy is on a candidate whose provider type is not `anthropic`, or when `allow` is empty. It SHALL also fail when an entry has an unknown type, an empty, blank, invalid UTF-8 or over-128-byte `skillId` or version, a repeated `(type, skillId)`, a repeated version, or an empty `versions` list. Strict YAML SHALL reject unknown policy fields, and errors SHALL NOT echo values.

#### Scenario: Policy on an unsupported provider type
- **WHEN** an `openai` or `openai-compatible` candidate declares `skills`
- **THEN** startup SHALL fail with a diagnostic naming the candidate path before serving requests

#### Scenario: Malformed allowlist
- **WHEN** a policy has an empty `allow`, an unknown type, a repeated entry, an empty `skillId` or an empty `versions` list
- **THEN** startup SHALL fail with a diagnostic naming the candidate path and without the configured values

#### Scenario: Candidate duplicate detection is unchanged
- **WHEN** two fallback entries share provider and model but carry different policies
- **THEN** startup SHALL reject the repeated candidate as before

### Requirement: Single skills policy per destination
All configured candidates, in any route, that name the same provider instance and backend model SHALL carry an equal skills policy (compared as sets, ignoring order) or all omit it. Otherwise model catalog validation SHALL fail before secret resolution, naming the conflicting candidate paths without values, so routing's destination dedup cannot change which policy applies.

#### Scenario: Shared destination with differing policies
- **WHEN** route `a`'s primary and route `b`'s fallback both name provider `anthropic-main` and model `claude-x`, and only one of them declares `skills`, or both declare different allow lists
- **THEN** startup SHALL fail with a diagnostic naming both candidate paths and no skill IDs

#### Scenario: Shared destination with equal policies
- **WHEN** two routes name the same provider instance and backend model with allow lists that differ only in entry or version order
- **THEN** the catalog SHALL load, and a routed call that skips the repeated destination SHALL apply the same policy whichever placement is kept

### Requirement: Static skill request bounds
Each native Anthropic attempt SHALL validate any non-empty skills list in the `AnthropicOptions` the adapter consumes, before native I/O in both modes. The list SHALL have at most 8 entries. Each type SHALL be checked first and SHALL be `anthropic` or `custom`. Each resolved skill ID SHALL be non-empty valid UTF-8 of at most 128 bytes; an empty version SHALL count as omitted, and any other version SHALL meet the same bound. Resolved `(type, ID)` pairs SHALL be distinct.

#### Scenario: Bounds exceeded or malformed
- **WHEN** a request carries nine skills, an unknown type, an empty or over-long resolved ID, an over-long or invalid UTF-8 version, a repeated skill, or a custom skill without `providerReference.anthropic` (including one set only by `skillId`)
- **THEN** the attempt SHALL fail with the existing unsupported-request error before its native request, without echoing skill or container IDs

#### Scenario: Empty version is omitted
- **WHEN** a skill sends `version: ""`
- **THEN** the bounds SHALL treat it as an omitted version and the adapter SHALL send no `version`

#### Scenario: Empty or absent skills
- **WHEN** `container` is absent or `container.skills` is null or empty
- **THEN** no skills check SHALL run and existing container-ID behavior SHALL be unchanged

### Requirement: Policy-gated configured skills
On a configured candidate, every requested skill SHALL match an allow entry by type and resolved ID. When that entry lists `versions`, the request SHALL carry one of them, matched literally; an omitted or empty request version SHALL match only an entry without `versions`, and `"latest"` SHALL NOT match an omitted version. A request combining skills with `container.id` SHALL be refused. Checks SHALL run on each attempt after per-attempt translation, in unary and streaming modes.

#### Scenario: Allowed Anthropic skill
- **WHEN** a candidate allows `{type: anthropic, skillId: pptx}` and the call sends that skill with the code-execution tool
- **THEN** the native request SHALL carry `container.skills: [{type: anthropic, skill_id: pptx}]` with the required skills betas

#### Scenario: Disallowed skill, version or container reuse
- **WHEN** a call sends an unlisted skill, a version outside the entry's `versions`, or skills together with `container.id`
- **THEN** that candidate SHALL fail with the existing unsupported-request error before its native request

#### Scenario: Mixed candidate policies
- **WHEN** the primary has no policy and the next candidate allows the requested skills
- **THEN** the primary SHALL make no native request, the configured chain SHALL advance to the next candidate (the unsupported-request error is not a provider API error, so the default fallback decider advances), and the policied candidate SHALL serve the request

#### Scenario: No candidate allows
- **WHEN** no candidate of a configured route allows the requested skills
- **THEN** the call SHALL return the fixed invalid-request error and no candidate SHALL make a native request

#### Scenario: Routed chain uses each candidate's own policy
- **WHEN** a configured caller calls an unpolicied route with `gateway.models` naming a route whose candidate, on a different destination, allows the requested skills
- **THEN** the unpolicied candidate SHALL make no native request, the routed chain SHALL advance, and the policied candidate SHALL serve the request under its own policy, granting nothing the caller could not get by calling that route directly

### Requirement: BYOK skills admission
Request-account (BYOK) invocation SHALL NOT consult configured skills policies. It SHALL admit `anthropic` and `custom` skills and `container.id` that satisfy the static bounds, before any account attempt and again on each account after translation, in unary and streaming modes.

#### Scenario: BYOK custom skill with container ID
- **WHEN** an Anthropic BYOK call sends a bounded custom skill and a `container.id`
- **THEN** the native request on the caller's account SHALL carry both unchanged, regardless of any configured policy

#### Scenario: BYOK bounds violation
- **WHEN** an all-Anthropic BYOK chain receives skills that violate the static bounds
- **THEN** it SHALL return the existing unsupported-request error with zero account attempts and no overview

#### Scenario: BYOK bounds violation in a mixed routed chain
- **WHEN** a routed BYOK chain with an Anthropic account and an OpenAI fallback model receives skills that violate the static bounds
- **THEN** the Anthropic attempt SHALL fail before its native request, and the OpenAI candidate SHALL serve the request

### Requirement: Operator-account skill scoping
Configured-mode custom skills SHALL be treated as operator-owned resources shared with every caller of the allowing candidate. The Gateway SHALL NOT offer per-tenant or per-namespace skill scoping. Documentation SHALL state that listed custom skills are shared by all callers of that candidate, and that skill-generated files remain in the operator's account.

#### Scenario: Two tenants call one candidate
- **WHEN** callers from different namespaces call a candidate that allows a custom skill
- **THEN** both SHALL be admitted under the same policy, and neither SHALL be able to select an unlisted skill

### Requirement: Skills evidence boundary
Acceptance SHALL use synthetic native requests through the Go and TypeScript Gateway command tests for policy and bound cases, in both registered clients, plus the direct Go replay of the upstream `anthropic/upstream/code-execution-pptx-skill` fixture. Fixture inputs SHALL NOT change. Deterministic evidence SHALL NOT claim live skill execution, Files API retrieval, container reuse across requests or Gateway conformance-matrix coverage of the pptx row.

#### Scenario: Local skills evidence
- **WHEN** the change is accepted
- **THEN** the Gateway command tests SHALL show an allowed skill reaching the fake Anthropic backend with the skills betas and a disallowed skill refused, for both clients, and the direct Go replay of the pptx fixture SHALL pass against unchanged inputs
