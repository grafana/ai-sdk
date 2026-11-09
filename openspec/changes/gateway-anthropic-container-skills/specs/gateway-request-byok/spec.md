# Spec Delta

## MODIFIED Requirements

### Requirement: Request-only native consumption guards
Request-only invocation SHALL retain shared consumption-backed guards before attempts. Valid bounded MCP/history, bounded container skills, bounded container IDs, inert markers and unrelated namespaces SHALL survive; invalid consumed state, out-of-bounds skills or container IDs and native fallback SHALL fail without catalog inventories.

#### Scenario: Request-only consumption policy
- **WHEN** native options and history reach the BYOK model
- **THEN** BYOK invocation SHALL retain gateway-native-provider-options protections at the actual consuming namespace and scope without restoring catalog inventories.
- **AND** Invalid consumed Anthropic MCP configuration/history and provider-side fallback controls SHALL fail before native I/O in both invocation modes.
- **AND** Container skills and container IDs SHALL follow gateway-anthropic-skills: BYOK SHALL apply only its static request bounds, and configured candidates SHALL also apply their own skills policy and refuse every caller container ID.
- **AND** Valid bounded MCP configuration, bounded container skills and configured provider-executed history SHALL remain supported.
- **AND** Harmless ignored fields and unrelated namespaces SHALL NOT be rejected by blanket spelling rules.

#### Scenario: Consumed execution bypass is refused
- **WHEN** an Anthropic BYOK request supplies invalid consumed MCP configuration/history, container skills or a container ID outside the gateway-anthropic-skills static bounds or a provider-side fallback control
- **THEN** it SHALL return the fixed invalid-request error without any credential attempt

#### Scenario: Ordinary native fields survive
- **WHEN** BYOK native options carry a bounded container ID or ignored protected-looking fields in non-consuming scopes
- **THEN** the request SHALL reach the adapter without changing its selected model, account or destination

#### Scenario: Configured container ID is refused
- **WHEN** a configured-account Anthropic candidate receives a non-empty container ID, with or without container skills
- **THEN** that candidate SHALL fail with the fixed invalid-request error before its native request, under gateway-anthropic-skills

#### Scenario: Valid MCP configuration and history survive
- **WHEN** an Anthropic BYOK request supplies valid bounded MCP servers and corresponding provider-executed history
- **THEN** the native adapter SHALL receive the MCP configuration and history without consulting configured accounts

#### Scenario: Bounded container skills survive
- **WHEN** an Anthropic BYOK request supplies bounded `anthropic` or `custom` container skills, with or without a bounded container ID
- **THEN** the native adapter SHALL receive them without consulting configured skills policies or accounts

### Requirement: Shared native execution protections
Configured and request-only models SHALL share consumption-backed native protection. Valid Anthropic MCP/history, bounded container skills and inert markers SHALL survive; bounded container IDs SHALL survive only on request-only accounts, and configured candidates SHALL refuse them. Unsupported consumed execution controls in the request SHALL fail before any account attempt, and those produced by per-attempt translation SHALL fail that account's attempt before its native request, without new field inventories or adapter changes.

#### Scenario: Native consumption policy
- **WHEN** an Anthropic request-only model receives native options and history
- **THEN** Configured and BYOK construction SHALL share the existing consumption-backed native-option checks, applied to each candidate or account after per-attempt option translation; BYOK SHALL also apply them to the request before its first account attempt.
- **AND** Anthropic BYOK invocation SHALL validate consumed MCP configuration and provider-owned history under gateway-anthropic-mcp, validate container skills and container IDs against the gateway-anthropic-skills static bounds and refuse native server-side fallback before any account attempt, and again on each account after translation, in both unary and streaming modes.
- **AND** Valid MCP configuration/history, bounded container skills and inert local markers SHALL retain their native semantics.
- **AND** These checks SHALL NOT introduce a provider-field inventory or change the native SDK adapters.

#### Scenario: Native execution control is rejected
- **WHEN** an Anthropic BYOK call supplies invalid consumed MCP configuration/history, out-of-bounds container skills or container ID, or native server-side fallback
- **THEN** the call SHALL return the existing unsupported-request error without native I/O or any account attempt, and no overview SHALL be published

#### Scenario: Translated execution control is rejected
- **WHEN** per-attempt translation adds invalid consumed MCP configuration, out-of-bounds container skills or container ID, or native server-side fallback to an Anthropic BYOK account's options
- **THEN** that account's attempt SHALL fail with the unsupported-request error before its native request

#### Scenario: Valid MCP configuration and history
- **WHEN** an Anthropic BYOK call supplies valid MCP configuration and matching provider-owned history
- **THEN** the native request SHALL preserve them without changing the inference endpoint, model or credential

#### Scenario: Harmless options are preserved
- **WHEN** BYOK native options contain a bounded container ID, inert local MCP markers or protected-looking fields in non-consuming namespaces
- **THEN** the adapter SHALL receive ordinary options without changing the selected model, account or destination

#### Scenario: Configured candidate refuses a container ID
- **WHEN** a configured Anthropic candidate's options, as supplied or after per-attempt translation, contain a non-empty container ID
- **THEN** that candidate's attempt SHALL fail with the unsupported-request error before its native request, without echoing the ID
