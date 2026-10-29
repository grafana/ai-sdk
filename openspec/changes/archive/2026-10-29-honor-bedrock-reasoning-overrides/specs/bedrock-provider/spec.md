## MODIFIED Requirements

### Requirement: Converse explicit reasoning configuration merge

Explicit disabled config SHALL suppress custom reasoning before derivation. Otherwise explicit effort or budget SHALL take precedence over portable derivation, with no diagnostic for an overridden value. Missing type and controls SHALL retain capability-based defaults. Raw `budgetTokens:0` SHALL remain explicit; typed zero SHALL mean omission. Request preparation SHALL NOT mutate caller reasoning config, including provider-default calls.

#### Scenario: Converse explicit reasoning configuration merge

- **WHEN** root reasoning supplies a derived budget and raw config explicitly sets zero, or a typed config leaves budget zero
- **THEN** raw zero SHALL override the budget while typed zero SHALL leave it derived; merged disabled thinking SHALL clear derived budget and effort

#### Scenario: Disabled thinking suppresses unused diagnostics

- **WHEN** custom root reasoning including `none`, `minimal`, `xhigh` or `max` is combined with explicit type `disabled`, budget and effort on Anthropic, Nova or OpenAI Converse
- **THEN** the request SHALL omit thinking, budget and effort without warnings from the suppressed reasoning settings, and caller config SHALL remain unchanged

#### Scenario: Explicit effort prevents portable mapping diagnostics

- **WHEN** an adaptive Anthropic, Nova 2 Lite or OpenAI model receives portable `minimal`, `xhigh` or `max` with explicit effort `low`
- **THEN** the request SHALL use `low` without portable mapping warnings, filling absent type with `adaptive` for Anthropic or `enabled` for Nova and retaining the output token limit for Nova low effort

#### Scenario: Budget-only override retains enabled type and explicit zero

- **WHEN** a budget-based Anthropic model receives custom reasoning with explicit budget only, including raw zero
- **THEN** thinking SHALL be enabled with that budget, the output token limit SHALL increase by exactly that budget, and overridden budget derivation SHALL emit no diagnostic

#### Scenario: Namespace precedence is preserved

- **WHEN** explicit reasoning config is supplied under either modern or legacy namespace
- **THEN** both SHALL resolve identically; non-null modern options SHALL take complete precedence and absent/null modern options SHALL fall back to legacy

#### Scenario: Provider-default and repeated or concurrent calls preserve ownership

- **WHEN** caller options are reused by sequential or concurrent calls, including omitted/provider-default reasoning and between-tools effort capping
- **THEN** native requests and warnings SHALL remain deterministic and caller options SHALL remain unchanged
- **AND** provider-default reasoning SHALL retain explicit controls without portable derivation or disabled-field cleanup

### Requirement: Converse none thinking and pinned deviation

Anthropic root reasoning `none` SHALL replace explicit partial config with disabled thinking, except IDs containing `claude-sonnet-5-5`, which SHALL use `between_tools`. Explicit type `disabled` SHALL take precedence over either default. The between-tools behavior is an intentional deviation recorded in `test/conformance/upstream.yaml`: upstream disabled thinking omits `thinking`, allowing default-effort adaptive thinking.

#### Scenario: Converse none thinking and pinned deviation

- **WHEN** root reasoning `none` accompanies partial provider config on Sonnet 5.5 and on another Anthropic model
- **THEN** Sonnet 5.5 SHALL send between-tools thinking; the other model SHALL use disabled thinking with no derived budget or effort

#### Scenario: Explicit disabled overrides the Sonnet 5.5 none default

- **WHEN** root reasoning `none` accompanies explicit type `disabled` on Sonnet 5.5
- **THEN** the request SHALL omit thinking and effort, preserving the caller's explicit choice without applying between-tools defaulting

### Requirement: Converse adaptive effort mapping

When deriving adaptive effort without an explicit effort override, reasoning SHALL map `minimal`/`low` to `low`, `medium` to `medium`, `high` to `high`, and `xhigh` to `max` in `additionalModelRequestFields.output_config.effort`. Changed level names SHALL emit a compatibility warning only when that mapping supplies the effective effort.

#### Scenario: Converse adaptive effort mapping

- **WHEN** custom root reasoning is `minimal`, `low`, `medium`, `high`, or `xhigh` on an adaptive model without explicit effort
- **THEN** effort SHALL be `low`, `low`, `medium`, `high`, or `max` respectively; only changed names SHALL warn
