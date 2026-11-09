# Spec Delta

## MODIFIED Requirements

### Requirement: Consumption-backed guard weakening audit
Before weakening global guards, the implementation SHALL record each actual consumed override path, namespace/scope, recognized spelling, native precedence, disposition and paired bypass/harmless regression case. Policy-gated relaxations SHALL also record each mode's disposition; tightenings SHALL also record the prior disposition and its source. This audit SHALL NOT introduce a new provider-field inventory or blanket all-provider scope validator.

#### Scenario: Consumption-backed guard weakening audit
- **WHEN** a change weakens a global option guard for an actual native consumed path
- **THEN** the audit SHALL record namespace/scope/spelling, native precedence, disposition and paired bypass/harmless cases without adding a provider inventory or all-provider scope validator

#### Scenario: Policy-gated relaxation
- **WHEN** a change admits a previously refused consumed path only under a configured candidate policy
- **THEN** the audit SHALL record the policy owner, the deny-by-default behavior and the configured and BYOK dispositions, with paired allowed/disallowed cases in both modes

#### Scenario: Guard tightening
- **WHEN** a change refuses a consumed path that an earlier change admitted
- **THEN** the audit SHALL record the earlier disposition and its source, the new configured and BYOK dispositions, and paired cases showing the refusal and the still-admitted values

## ADDED Requirements

### Requirement: Container consumption audit
Admitting Anthropic container skills and refusing configured container IDs SHALL record the audit for the consumed `anthropic` call-namespace `container` path (`skills` and `id`), covering case-insensitive typed decoding and custom-skill ID resolution. The dispositions SHALL be: skills policy-gated in configured mode and bounded in BYOK; `container.id` refused in configured mode with or without skills, and bounded in BYOK. Each disposition SHALL have paired bypass/harmless cases.

#### Scenario: Container ID tightening is recorded
- **WHEN** configured candidates start refusing a container ID without skills
- **THEN** the audit SHALL record it as a tightening of the `2026-10-02-forward-native-provider-options` disposition that admitted a container ID without skills

#### Scenario: Container path is recorded
- **WHEN** the skills refusal is replaced by the gateway-anthropic-skills policy and configured container IDs are refused
- **THEN** the audit SHALL record the consumed path, its dispositions per mode, the container-ID tightening and its prior source, and paired cases including a configured ID-only container refused, a BYOK ID-only container admitted, empty or null containers remaining no-ops and custom-part `container` scopes remaining inert
