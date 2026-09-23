## MODIFIED Requirements

### Requirement: Unary function history and selected results
The mapper SHALL support assistant tool calls and tool-role results with text, json, error-text, error-json or text-only content output. It SHALL also support provider-executed assistant calls and registered assistant-side basic tool results under gateway-provider-tools. It SHALL preserve required empty text, empty content arrays and selected JSON null. It SHALL reject inactive fields and leave approvals, execution-denied, file/custom content and non-empty nested result output/content options unsupported. Tool-call/result part provider options SHALL follow gateway-provider-tools continuation rules.

#### Scenario: Empty selected result
- **WHEN** continuation includes a text result with empty value or a JSON result with null
- **THEN** the selected result arm and value SHALL reach the provider without omission or substitution

#### Scenario: Deferred result family
- **WHEN** a schema-valid result contains an execution-denied or file-content arm
- **THEN** it SHALL fail as a fixed unsupported capability before resolution or invocation

#### Scenario: Provider result in assistant history
- **WHEN** continuation contains a provider-executed call and matching basic result inline in assistant content
- **THEN** the mapper SHALL preserve their order and ownership without converting the result into a client-owned tool message

### Requirement: Bounded private unary tool calls
Unary output SHALL preserve ordered text, client-executed and provider-executed calls, and provider tool results using explicit private DTOs. Calls SHALL retain toolCallId, toolName, string input and enabled providerExecuted/dynamic markers. Results SHALL retain toolCallId, toolName, non-null JSON result, error status and registered dynamic/preliminary markers. Results SHALL correlate with current-response calls or eligible unresolved provider calls in supplied history under gateway-provider-tools; a repeated call in the current response SHALL NOT be required. A complete provider-owned call SHALL be allowed to return without a result. Disabled markers SHALL normalize consistently. The encoder SHALL NOT strip an enabled execution marker and forward the call as client-executed. Tool metadata SHALL follow the reviewed gateway-provider-tools projection; arbitrary metadata and backend identity SHALL not leak. All added strings, result/metadata bytes and cardinality SHALL participate in preflight and final encoding bounds; unsupported or invalid output SHALL fail before HTTP 200.

#### Scenario: Enabled execution marker cannot become a client call
- **WHEN** a supported provider unary result contains a tool call with providerExecuted true or dynamic true, including alongside valid text
- **THEN** enabled markers SHALL survive to both clients
- **AND** a provider-executed call SHALL execute zero local tools

#### Scenario: Disabled markers preserve client ownership
- **WHEN** a supported function call has absent or false providerExecuted and dynamic markers
- **THEN** it SHALL remain client-executed and disabled markers SHALL normalize without changing ownership

#### Scenario: Tool call consumed by both clients
- **WHEN** a provider produces a supported function call with a tool-calls finish
- **THEN** both clients SHALL receive semantically equivalent call IDs, names, inputs, usage and finish through the real handler

#### Scenario: Oversized tool input
- **WHEN** a provider output cannot fit the configured unary budget
- **THEN** no partial HTTP 200 SHALL be committed and the fixed safe error SHALL be returned

#### Scenario: Provider result is consumed
- **WHEN** a unary response contains a provider call and matching non-null JSON result with supported metadata
- **THEN** both clients SHALL preserve ordered content, result semantics and supported metadata without a result-level providerExecuted wire field

#### Scenario: Deferred unary completion
- **WHEN** a request includes an unresolved provider-owned call in assistant history and the unary output contains its matching success or error result without repeating the call
- **THEN** both clients SHALL receive that result, while unknown, mismatched or already-completed historical matches SHALL fail before HTTP 200

#### Scenario: Unsupported content mixed with valid calls
- **WHEN** a unary result contains supported calls plus a deferred output family
- **THEN** the complete response SHALL fail safely before HTTP 200 without partial success
