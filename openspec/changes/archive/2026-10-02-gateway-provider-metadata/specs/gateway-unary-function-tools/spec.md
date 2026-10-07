## MODIFIED Requirements

### Requirement: Bounded private unary tool calls
Unary output SHALL preserve ordered text and client-executed function tool calls using explicit private DTOs with toolCallId, toolName and string input. Provider output containing providerExecuted true or dynamic true SHALL be rejected with the existing fixed internal-error document before HTTP 200. False or absent markers MAY normalize to disabled. The encoder SHALL NOT strip an enabled execution marker and forward the call as client-executed. Provider-tool results and enabled preliminary behavior SHALL remain outside this supported unary output union. New strings/cardinality SHALL participate in preflight and final encoding bounds. Supported tool-call providerMetadata SHALL retain opaque native namespace objects and represented presence under gateway-provider-metadata's aggregate unary bounds. This SHALL NOT add backend identity/transport fields or enable unsupported execution markers.

#### Scenario: Enabled execution marker cannot become a client call
- **WHEN** a provider unary result contains a tool call with providerExecuted true or dynamic true, including alongside valid text
- **THEN** the entire response SHALL be the fixed internal-error document before HTTP 200, with no partial success content
- **AND** both registered Vercel and Go client scenarios SHALL observe an error and execute zero local tools

#### Scenario: Disabled markers preserve client ownership
- **WHEN** a supported function call has absent or false providerExecuted and dynamic markers
- **THEN** it SHALL remain client-executed and its disabled markers MAY be omitted without changing ownership

#### Scenario: Tool call consumed by both clients
- **WHEN** a provider produces a supported function call with a tool-calls finish
- **THEN** both clients SHALL receive semantically equivalent call IDs, names, inputs, supported call metadata, usage and finish through the real handler

#### Scenario: Oversized tool input
- **WHEN** a provider output cannot fit the configured unary budget
- **THEN** no partial HTTP 200 SHALL be committed and the fixed safe error SHALL be returned

#### Scenario: Actual returned caller continues natively
- **WHEN** each client's first native Anthropic tool-call response supplies caller metadata on a supported local function call and its actual assembled response history is reused
- **THEN** the second native assistant tool_use request SHALL contain that caller without the test injecting history metadata, and the Gateway SHALL execute no local function
