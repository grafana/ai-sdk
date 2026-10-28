# Spec Delta

## MODIFIED Requirements

### Requirement: Request-only native consumption guards
Request-only invocation SHALL retain shared consumption-backed guards before attempts. Valid bounded MCP/history, inert markers and unrelated namespaces SHALL survive; invalid consumed state, skills and native fallback SHALL fail without catalog inventories.

#### Scenario: Request-only consumption policy
- **WHEN** native options and history reach the BYOK model
- **THEN** BYOK invocation SHALL retain gateway-native-provider-options protections at the actual consuming namespace and scope without restoring catalog inventories.
- **AND** Invalid consumed Anthropic MCP configuration/history, container skills and provider-side fallback controls SHALL fail before native I/O in both invocation modes.
- **AND** Valid bounded MCP configuration and configured provider-executed history SHALL remain supported.
- **AND** Harmless ignored fields and unrelated namespaces SHALL NOT be rejected by blanket spelling rules.

#### Scenario: Consumed execution bypass is refused
- **WHEN** an Anthropic BYOK request supplies invalid consumed MCP configuration/history, a container skill or a provider-side fallback control
- **THEN** it SHALL return the fixed invalid-request error without native I/O on any account

#### Scenario: Ordinary native fields survive
- **WHEN** native options carry a safe container ID or ignored protected-looking fields in non-consuming scopes
- **THEN** the request SHALL reach the adapter without changing its selected model, account or destination

#### Scenario: Valid MCP configuration and history survive
- **WHEN** an Anthropic BYOK request supplies valid bounded MCP servers and corresponding provider-executed history
- **THEN** the native adapter SHALL receive the MCP configuration and history without consulting configured accounts

### Requirement: Shared native execution protections
Configured and request-only models SHALL share consumption-backed native protection. Valid Anthropic MCP/history and inert markers SHALL survive; unsupported consumed execution controls SHALL fail each account attempt before native I/O without new field inventories or adapter changes.

#### Scenario: Native consumption policy
- **WHEN** an Anthropic request-only model receives native options and history
- **THEN** Configured and BYOK construction SHALL share the existing consumption-backed native-option checks, applied to each candidate or account after per-attempt option translation.
- **AND** Anthropic BYOK invocation SHALL validate consumed MCP configuration and provider-owned history under gateway-anthropic-mcp, and refuse container skills and native server-side fallback on each account before its native request in both unary and streaming modes.
- **AND** Valid MCP configuration/history and inert local markers SHALL retain their native semantics.
- **AND** These checks SHALL NOT introduce a provider-field inventory or change the native SDK adapters.

#### Scenario: Native execution control is rejected
- **WHEN** an Anthropic BYOK call supplies invalid consumed MCP configuration/history, container skills or native server-side fallback
- **THEN** the call SHALL return the existing unsupported-request error without native I/O on any account
- **AND** the request's attempt capture SHALL hold one failed attempt per account, and no overview SHALL be published

#### Scenario: Valid MCP configuration and history
- **WHEN** an Anthropic BYOK call supplies valid MCP configuration and matching provider-owned history
- **THEN** the native request SHALL preserve them without changing the inference endpoint, model or credential

#### Scenario: Harmless options are preserved
- **WHEN** native options contain an ordinary container ID, inert local MCP markers or protected-looking fields in non-consuming namespaces
- **THEN** the adapter SHALL receive ordinary options without changing the selected model, account or destination
