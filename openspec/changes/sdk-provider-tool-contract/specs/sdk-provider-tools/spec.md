## ADDED Requirements

### Requirement: Direct tool validation before backend I/O
Direct native-provider entry points and the Go Gateway client SHALL reject representably populated inactive tool fields before native I/O. Provider tools SHALL NOT carry function-only fields; function tools SHALL NOT carry provider ID/args. Nil Go provider args SHALL normalize to an empty object; non-nil raw argument values SHALL be valid JSON. Structurally valid unsupported IDs SHALL retain native unsupported-feature handling.

#### Scenario: Mixed direct tool variant
- **WHEN** a provider tool has non-nil strict, input schema, input examples or provider options, or nonempty description
- **THEN** generate and stream setup SHALL fail before HTTP

#### Scenario: Nil Go args
- **WHEN** a provider tool has a valid ID/name and nil args
- **THEN** conversion SHALL use empty-object semantics and the Go Gateway request SHALL contain `args: {}`

### Requirement: Independent bounded client metadata projection
The Go Gateway client SHALL decode tool metadata independently of server DTOs. It SHALL retain Anthropic MCP type/serverName, closed Anthropic caller type/toolId, and OpenAI/Azure itemId, namespace and closed caller type/callerId shapes. Unknown/private fields SHALL be omitted; malformed supported shapes SHALL fail. MCP metadata SHALL exclude caller fields. Response-level identity SHALL remain client-owned. These readers SHALL NOT claim that the current Gateway supports every decoded family.

#### Scenario: MCP metadata with private fields
- **WHEN** a tool response contains Anthropic MCP type/serverName together with caller and arbitrary private fields
- **THEN** the client SHALL retain only type/serverName under the MCP namespace shape

#### Scenario: Deferred result without a repeated call
- **WHEN** a successful unary or SSE response contains only a supported tool result
- **THEN** the client SHALL decode it without importing server correlation state

### Requirement: SDK readiness does not activate Gateway capabilities
Existing Gateway consumers SHALL migrate to the new boolean fields and published immutable dependencies without enabling provider tools, provider-owned results or nonempty root provider options. Apache modules SHALL NOT import Gateway code. Workspace and isolated module checks SHALL pass without local Gateway replacements.

#### Scenario: Existing service rejection is preserved
- **WHEN** provider definitions, enabled provider output markers or MCP root options reach the existing Gateway runtime
- **THEN** the existing safe rejection behavior SHALL remain until the corresponding service capability change lands
