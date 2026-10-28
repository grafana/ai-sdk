# Spec Delta

## MODIFIED Requirements

### Requirement: Consumption-backed guard weakening audit
Before weakening global guards, the implementation SHALL record each actual consumed override path, namespace/scope, recognized spelling, native precedence, disposition and paired bypass/harmless regression case. When a refusal is replaced by an operator policy, the audit SHALL also record the policy owner and default, and the disposition in each invocation mode. This audit SHALL NOT introduce a new provider-field inventory or blanket all-provider scope validator.

#### Scenario: Consumption-backed guard weakening audit
- **WHEN** a change weakens a global option guard for an actual native consumed path
- **THEN** the audit SHALL record namespace/scope/spelling, native precedence, disposition and paired bypass/harmless cases without adding a provider inventory or all-provider scope validator

#### Scenario: Policy-gated relaxation
- **WHEN** a change admits a previously refused consumed path only under a configured candidate policy
- **THEN** the audit SHALL record the policy owner, the deny-by-default behavior and the configured and BYOK dispositions, with paired allowed/disallowed cases in both modes

## ADDED Requirements

### Requirement: Container skills consumption audit
Admitting Anthropic container skills SHALL record the audit for the consumed `anthropic` call-namespace `container` path (`skills` and `id`). The record SHALL cover how case-insensitive typed decoding is resolved and how custom-skill IDs are resolved. It SHALL state the dispositions: policy-gated with `container.id` refused in configured mode, bounded in BYOK. Each disposition SHALL have paired bypass/harmless cases.

#### Scenario: Skills path is recorded
- **WHEN** the skills refusal is replaced by the gateway-anthropic-skills policy
- **THEN** the audit SHALL record the consumed path, its dispositions per mode, and paired cases including container-ID-only requests remaining unchanged and custom-part `container` scopes remaining inert
