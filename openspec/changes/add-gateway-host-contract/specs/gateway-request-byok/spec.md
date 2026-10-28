# Spec Delta

## MODIFIED Requirements

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
