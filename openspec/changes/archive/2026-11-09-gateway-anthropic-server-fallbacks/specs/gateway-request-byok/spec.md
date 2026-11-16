# Spec Delta

## MODIFIED Requirements

### Requirement: Request-only native consumption guards
Request-only invocation SHALL retain shared consumption-backed guards before attempts. Valid bounded MCP/history, inert markers, unrelated namespaces and Anthropic server-side fallback SHALL survive; invalid consumed state and skills SHALL fail without catalog inventories.

#### Scenario: Request-only consumption policy
- **WHEN** native options and history reach the BYOK model
- **THEN** BYOK invocation SHALL retain gateway-native-provider-options protections at the actual consuming namespace and scope without restoring catalog inventories.
- **AND** Invalid consumed Anthropic MCP configuration/history and container skills SHALL fail before native I/O in both invocation modes.
- **AND** BYOK invocation SHALL forward Anthropic server-side fallback without a policy; configured invocation SHALL admit it only under gateway-anthropic-server-fallbacks candidate policy, which SHALL NOT apply to BYOK.
- **AND** Valid bounded MCP configuration and configured provider-executed history SHALL remain supported.
- **AND** Harmless ignored fields and unrelated namespaces SHALL NOT be rejected by blanket spelling rules.

#### Scenario: Consumed execution bypass is refused
- **WHEN** an Anthropic BYOK request supplies invalid consumed MCP configuration/history or a container skill
- **THEN** it SHALL return the fixed invalid-request error without any credential attempt

#### Scenario: Ordinary native fields survive
- **WHEN** native options carry a safe container ID or ignored protected-looking fields in non-consuming scopes
- **THEN** the request SHALL reach the adapter without changing its selected model, account or destination

#### Scenario: Valid MCP configuration and history survive
- **WHEN** an Anthropic BYOK request supplies valid bounded MCP servers and corresponding provider-executed history
- **THEN** the native adapter SHALL receive the MCP configuration and history without consulting configured accounts

### Requirement: Unsupported controls fail explicitly
Unsupported credentials, mappings and controls SHALL fail with non-secret diagnostics before I/O. Native options and headers SHALL NOT override the selected account, model or destination. Anthropic server-side fallback inside the selected account's native request is not an override; a hop SHALL be reported only as `servedModelId` under gateway-execution-overview.

#### Scenario: Unsupported-control policy
- **WHEN** BYOK input contains provider, mapping or routing controls
- **THEN** Unsupported provider selection, meaningful modelMappings, Azure/Vertex/Bedrock/compatible credential families and requested unsupported Gateway controls SHALL fail before provider I/O with actionable diagnostics. Unrelated additive account fields SHALL NOT cause failure. models, order and only SHALL follow gateway-request-routing; every other Gateway control, including providerTimeouts, SHALL remain unsupported.
- **AND** Nested gateway namespaces SHALL NOT be interpreted as credential controls.
- **AND** Native options or body headers SHALL NOT override credential source, destination or the selected model. Anthropic server-side fallback executed by the provider within the selected account and destination SHALL NOT count as such an override.

#### Scenario: Unsupported credential family or mapping
- **WHEN** a request supplies a known but unsupported credential family or modelMappings
- **THEN** it SHALL receive a capability-specific rejection rather than silently ignore the form or use another account

#### Scenario: Canonical credential control
- **WHEN** a request supplies BYOK or another case variant, alone or alongside canonical byok
- **THEN** it SHALL decode as the byok control under ordinary Go decoding, with the last duplicate member winning
- **AND** the call-level gateway namespace, not the member name, SHALL remain the capture boundary: the runtime SHALL remove it before any model, middleware or telemetry

#### Scenario: Routing controls accompany valid BYOK
- **WHEN** valid credentials accompany gateway.order, gateway.only or gateway.models
- **THEN** the request SHALL be routed under gateway-request-routing

#### Scenario: Unsupported control accompanies valid BYOK
- **WHEN** valid credentials accompany gateway.sort, gateway.providerTimeouts or another unsupported control
- **THEN** the request SHALL fail before I/O without suggesting that the control was applied

#### Scenario: Server-side fallback is not an override
- **WHEN** an Anthropic BYOK call to `anthropic/claude-fable-5` sends `fallbacks: [{model: claude-opus-4-8}]` and the provider reports a hop to `claude-opus-4-8`
- **THEN** the native request SHALL use the selected account, destination and model `claude-fable-5`, and the selected attempt SHALL carry `modelId` `claude-fable-5` and `servedModelId` `claude-opus-4-8`

### Requirement: Shared native execution protections
Configured and request-only models SHALL share consumption-backed native protection. Valid Anthropic MCP/history and inert markers SHALL survive; unsupported consumed execution controls in the request SHALL fail before any account attempt in an all-Anthropic chain, otherwise before each Anthropic account's request, and those produced by per-attempt translation SHALL fail that account's attempt before its native request, without new field inventories or adapter changes.

#### Scenario: Native consumption policy
- **WHEN** an Anthropic request-only model receives native options and history
- **THEN** Configured and BYOK construction SHALL share the existing consumption-backed native-option checks, applied to each candidate or account after per-attempt option translation; BYOK SHALL also apply them to the request before its first account attempt.
- **AND** Anthropic BYOK invocation SHALL validate consumed MCP configuration and provider-owned history under gateway-anthropic-mcp, and refuse container skills before any account attempt when every composed destination is Anthropic, and otherwise on each Anthropic account before its native request, and again on each account after translation, in both unary and streaming modes.
- **AND** Anthropic BYOK invocation SHALL forward native server-side fallback, an explicit chain (including repeated models) or `"default"`, from the request or from per-attempt translation, to each Anthropic account's native request without a policy, in both unary and streaming modes.
- **AND** Configured candidate policies under gateway-anthropic-server-fallbacks SHALL NOT apply to BYOK, and no client control SHALL grant or apply one.
- **AND** Valid MCP configuration/history and inert local markers SHALL retain their native semantics.
- **AND** These checks SHALL NOT introduce a provider-field inventory or change the native SDK adapters.

#### Scenario: Native execution control is rejected
- **WHEN** an Anthropic BYOK call supplies invalid consumed MCP configuration/history or container skills
- **THEN** the call SHALL return the existing unsupported-request error without native I/O or any account attempt, and no overview SHALL be published

#### Scenario: Translated execution control is rejected
- **WHEN** per-attempt translation adds invalid consumed MCP configuration or container skills to an Anthropic BYOK account's options
- **THEN** that account's attempt SHALL fail with the unsupported-request error before its native request

#### Scenario: Valid MCP configuration and history
- **WHEN** an Anthropic BYOK call supplies valid MCP configuration and matching provider-owned history
- **THEN** the native request SHALL preserve them without changing the inference endpoint, model or credential

#### Scenario: Harmless options are preserved
- **WHEN** native options contain an ordinary container ID, inert local MCP markers or protected-looking fields in non-consuming namespaces
- **THEN** the adapter SHALL receive ordinary options without changing the selected model, account or destination

#### Scenario: Server-side fallback is forwarded
- **WHEN** an Anthropic BYOK call sends a chain, including one that repeats a model with different `thinking`, or sends `"default"`, in unary or streaming mode
- **THEN** the selected account's native Anthropic request SHALL carry the fallbacks unchanged, with the explicit server-side fallback beta for a chain or the default beta for `"default"`, at the selected model and destination

#### Scenario: Configured policy does not apply to BYOK
- **WHEN** a configured candidate allows only `claude-opus-4-8` without `allowDefault`, and an Anthropic BYOK call sends a chain naming another model, or `"default"`
- **THEN** the BYOK native request SHALL carry the fallbacks unchanged, as without that configured policy

#### Scenario: Routed accounts forward server-side fallback
- **WHEN** an Anthropic BYOK call sends a chain or `"default"` with `models: ["anthropic/y"]`, or with `models: ["openai/y"]`, in unary or streaming mode
- **THEN** each attempted Anthropic account's native request SHALL carry the fallbacks unchanged with the matching beta, and an OpenAI account's request SHALL carry no server-side fallback

#### Scenario: Translation adds server-side fallback
- **WHEN** per-attempt translation adds fallbacks to an Anthropic BYOK account's options
- **THEN** that account's native request SHALL carry the translated fallbacks with the matching beta
