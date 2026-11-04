# Spec Delta

## MODIFIED Requirements

### Requirement: Request-only native consumption guards
Request-only invocation SHALL retain shared consumption-backed guards before attempts. Valid bounded MCP/history, inert markers and unrelated namespaces SHALL survive; invalid consumed state, skills and Anthropic server-side fallback SHALL fail without catalog inventories.

#### Scenario: Request-only consumption policy
- **WHEN** native options and history reach the BYOK model
- **THEN** BYOK invocation SHALL retain gateway-native-provider-options protections at the actual consuming namespace and scope without restoring catalog inventories.
- **AND** Invalid consumed Anthropic MCP configuration/history and container skills SHALL fail before native I/O in both invocation modes. Provider-side fallback controls SHALL fail before native I/O in BYOK invocation; configured invocation SHALL admit them only under gateway-anthropic-server-fallbacks candidate policy, which SHALL NOT apply to BYOK.
- **AND** Valid bounded MCP configuration and configured provider-executed history SHALL remain supported.
- **AND** Harmless ignored fields and unrelated namespaces SHALL NOT be rejected by blanket spelling rules.

#### Scenario: Consumed execution bypass is refused
- **WHEN** an Anthropic BYOK request supplies invalid consumed MCP configuration/history, a container skill or a provider-side fallback control
- **THEN** it SHALL return the fixed invalid-request error without any credential attempt

#### Scenario: Ordinary native fields survive
- **WHEN** native options carry a safe container ID or ignored protected-looking fields in non-consuming scopes
- **THEN** the request SHALL reach the adapter without changing its selected model, account or destination

#### Scenario: Valid MCP configuration and history survive
- **WHEN** an Anthropic BYOK request supplies valid bounded MCP servers and corresponding provider-executed history
- **THEN** the native adapter SHALL receive the MCP configuration and history without consulting configured accounts

### Requirement: Shared native execution protections
Configured and request-only models SHALL share consumption-backed native protection. Valid Anthropic MCP/history and inert markers SHALL survive; unsupported consumed execution controls in the request SHALL fail before any account attempt when every composed destination is Anthropic, and otherwise each Anthropic account's attempt SHALL fail before its native request; those produced by per-attempt translation SHALL fail that account's attempt before its native request, without new field inventories or adapter changes.

#### Scenario: Native consumption policy
- **WHEN** an Anthropic request-only model receives native options and history
- **THEN** Configured and BYOK construction SHALL share the existing consumption-backed native-option checks, applied to each candidate or account after per-attempt option translation; BYOK SHALL also apply them to the request before its first account attempt.
- **AND** Anthropic BYOK invocation SHALL validate consumed MCP configuration and provider-owned history under gateway-anthropic-mcp, and refuse container skills and native server-side fallback before any account attempt when every composed destination is Anthropic, and otherwise on each Anthropic account before its native request, and again on each account after translation, in both unary and streaming modes.
- **AND** BYOK native server-side fallback refusal SHALL hold regardless of any configured candidate's gateway-anthropic-server-fallbacks policy; BYOK SHALL have no server-fallback policy of its own.
- **AND** Valid MCP configuration/history and inert local markers SHALL retain their native semantics.
- **AND** These checks SHALL NOT introduce a provider-field inventory or change the native SDK adapters.

#### Scenario: Native execution control is rejected
- **WHEN** an Anthropic BYOK call supplies invalid consumed MCP configuration/history, container skills or native server-side fallback
- **THEN** the call SHALL return the existing unsupported-request error without native I/O or any account attempt, and no overview SHALL be published

#### Scenario: Translated execution control is rejected
- **WHEN** per-attempt translation adds invalid consumed MCP configuration, container skills or native server-side fallback to an Anthropic BYOK account's options
- **THEN** that account's attempt SHALL fail with the unsupported-request error before its native request

#### Scenario: Valid MCP configuration and history
- **WHEN** an Anthropic BYOK call supplies valid MCP configuration and matching provider-owned history
- **THEN** the native request SHALL preserve them without changing the inference endpoint, model or credential

#### Scenario: Harmless options are preserved
- **WHEN** native options contain an ordinary container ID, inert local MCP markers or protected-looking fields in non-consuming namespaces
- **THEN** the adapter SHALL receive ordinary options without changing the selected model, account or destination

#### Scenario: Configured policy does not reach BYOK
- **WHEN** a configured candidate allows `claude-opus-4-8` with `allowDefault: true`, and an Anthropic BYOK call sends either a chain naming `claude-opus-4-8` or `"default"`, in unary or streaming mode
- **THEN** the BYOK call SHALL return the existing unsupported-request error with zero account attempts and no overview

#### Scenario: Routed fallback-model accounts refuse
- **WHEN** an Anthropic BYOK call sends a chain or `"default"` with `models: ["anthropic/y"]`, or with `models: ["openai/y"]`, in unary or streaming mode
- **THEN** the all-Anthropic chain SHALL return the existing unsupported-request error with zero account attempts and no overview, and in the mixed chain each Anthropic account SHALL fail before its native request, so no native request SHALL carry server-side fallback
