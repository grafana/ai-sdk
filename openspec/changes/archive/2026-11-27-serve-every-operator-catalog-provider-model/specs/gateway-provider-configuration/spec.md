## ADDED Requirements

### Requirement: Optional operator provider serving settings

An operator `model_catalog` provider SHALL accept optional `serveAllModels` (boolean) and `modelBlocklist` (sequence of backend model ID strings). An omitted or false switch SHALL mean listed models only. An omitted or empty blocklist SHALL block no IDs; a blocklist SHALL be accepted while the switch is off. Startup credential and endpoint checks SHALL still apply.

#### Scenario: Existing catalog
- **WHEN** an existing valid operator catalog omits both fields
- **THEN** it SHALL remain valid and its providers SHALL serve listed models only

#### Scenario: Empty model map
- **WHEN** an operator catalog declares `models: {}` and a valid provider opts in
- **THEN** startup SHALL accept the empty map
- **AND** startup SHALL refuse an empty map without an opted-in provider, or a missing or null map

#### Scenario: Instance name cannot qualify a selector
- **WHEN** a provider opts in with a name that contains a slash or does not meet blocklist ID validation
- **THEN** startup SHALL refuse the opt-in

### Requirement: Validate operator provider blocklists

Each `modelBlocklist` SHALL use the stack provider blocklist rules for ID validity, count, exact matching and listed-target conflicts, and SHALL also apply to Anthropic `serverFallbacks` on the same provider instance. Configuration loading SHALL reject invalid fields and conflicts before resolving secrets, regardless of the switch. Errors SHALL identify the offending fields without echoing blocklist entries or credentials.

#### Scenario: Wrong field type or invalid entries
- **WHEN** either setting has the wrong type, or a blocklist exceeds 128 entries, contains an exact duplicate, or contains an invalid ID
- **THEN** startup SHALL fail; blocklist IDs SHALL meet the shared public-ID grammar and 320-byte bound

#### Scenario: Conflicting primary, Gateway fallback or explicit Anthropic fallback
- **WHEN** a backend ID is both blocked and used as a listed primary or Gateway fallback, or allowed by either candidate's Anthropic `serverFallbacks.models` on the same provider instance
- **THEN** startup SHALL fail even if `serveAllModels` is off
- **AND** an otherwise valid target with that ID on another provider instance SHALL NOT conflict

#### Scenario: Vendor chooses the fallback
- **WHEN** an Anthropic candidate enables `allowDefault` on a provider with a nonempty blocklist
- **THEN** startup SHALL fail because the Gateway cannot check which model Anthropic will choose
