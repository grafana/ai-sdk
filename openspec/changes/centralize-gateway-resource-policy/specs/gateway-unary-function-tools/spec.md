# Spec Delta

## MODIFIED Requirements

### Requirement: Bounded private unary tool calls

Unary output SHALL preserve ordered text, client-executed and provider-executed calls, and provider tool results using explicit private DTOs. Calls SHALL retain toolCallId, toolName, string input and enabled providerExecuted/dynamic markers. Results SHALL retain toolCallId, toolName, non-null JSON result, error status and registered dynamic/preliminary markers.

#### Scenario: Enabled execution marker cannot become a client call
- **WHEN** a supported provider unary result contains a tool call with providerExecuted true or dynamic true, including alongside valid text
- **THEN** enabled markers SHALL survive to both clients
- **AND** a provider-executed call SHALL execute zero local tools

#### Scenario: Disabled markers preserve client ownership
- **WHEN** a supported function call has absent or false providerExecuted and dynamic markers
- **THEN** it SHALL remain client-executed and disabled markers SHALL normalize without changing ownership

#### Scenario: Tool call consumed by both clients
- **WHEN** a provider produces a supported function call with a tool-calls finish
- **THEN** both clients SHALL receive semantically equivalent call IDs, names, inputs, supported call metadata, usage and finish through the real handler

#### Scenario: Oversized tool input
- **WHEN** a provider returns valid tool input larger than the former unary budget
- **THEN** the complete call SHALL be returned without a size-based failure

#### Scenario: Actual returned caller continues natively
- **WHEN** each client's first native Anthropic tool-call response supplies caller metadata on a supported local function call and its actual assembled response history is reused
- **THEN** the second native assistant tool_use request SHALL contain that caller without the test injecting history metadata, and the Gateway SHALL execute no local function

#### Scenario: Provider result is consumed
- **WHEN** a unary response contains a provider call and matching non-null JSON result with supported metadata
- **THEN** both clients SHALL preserve ordered content, result semantics and supported metadata without a result-level providerExecuted wire field

#### Scenario: Deferred unary completion
- **WHEN** a request includes an unresolved provider-owned call in assistant history and the unary output contains its matching success or error result without repeating the call
- **THEN** both clients SHALL receive that result, while unknown, mismatched or already-completed historical matches SHALL fail before HTTP 200

#### Scenario: Unsupported content mixed with valid calls
- **WHEN** a unary result contains supported calls plus a deferred output family
- **THEN** the complete response SHALL fail safely before HTTP 200 without partial success

### Requirement: Unary tool metadata identity and output bounds

Tool metadata SHALL retain opaque namespace objects and omitted/empty presence under gateway-provider-metadata. This SHALL NOT promote provider metadata into routing authority or replace catalog identity. Added strings and result/metadata values SHALL be validated before encoding without adapter byte or cardinality budgets; unsupported or invalid output SHALL fail before HTTP 200.

#### Scenario: Unary tool metadata identity and output bounds
- **WHEN** a supported call contains an unknown metadata object and an invalid UTF-8 input
- **THEN** metadata SHALL remain opaque without routing authority, and the invalid complete response SHALL fail before HTTP 200

### Requirement: Unary function tools remain direct and stateless

The Gateway SHALL NOT execute application functions or persist tool-loop state. Direct and configured-fallback routes SHALL accept the existing supported unary function definitions, choices and history/results through the same strict mapper. Configured candidates SHALL receive the same mapped request without tool/choice/history-specific admission guards, namespace translation or candidate intersections.

#### Scenario: Two-call unary continuation
- **WHEN** an application executes a returned tool locally and sends the call plus result in a second unary request through a direct or configured-fallback route
- **THEN** the real handler SHALL process two independent requests and the application SHALL receive final text
- **AND** the deterministic function SHALL execute once for its returned call with no Gateway executor or persisted workflow state

#### Scenario: Fallback route receives tool history
- **WHEN** a supported unary tool continuation targets a fallback-configured route and the primary fails eligibly before success
- **THEN** the next configured candidate SHALL receive the same supported definitions, history/results, choices and scoped options without omission or reinterpretation

#### Scenario: Fallback route receives text-only automatic choice
- **WHEN** an otherwise supported text request contains automatic choice with no tool name and absent or empty tools
- **THEN** the original automatic choice SHALL reach each attempted physical candidate unchanged

#### Scenario: All supported choices remain eligible
- **WHEN** a schema-valid supported unary request supplies auto, none, required or named function choice
- **THEN** a configured-fallback route SHALL NOT reject it merely because of the choice and every attempted candidate SHALL receive the original choice

#### Scenario: Selected unary call is not replayed
- **WHEN** a candidate returns a successful unary function call and the strict response encoder rejects an accompanying unsupported or invalid output
- **THEN** the existing safe response failure SHALL apply without invoking a later candidate or executing a consumer function in the Gateway
