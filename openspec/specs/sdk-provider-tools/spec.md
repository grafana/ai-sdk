# sdk-provider-tools Specification

## Purpose

Define provider-tool validation and bounded Go Gateway client decoding independently of Gateway service activation.

## Requirements

### Requirement: Direct tool validation before backend I/O
Direct native-provider entry points and the Go Gateway client SHALL reject representably populated inactive tool fields before native I/O. Provider tools SHALL NOT carry function-only fields; function tools SHALL NOT carry provider ID/args. Nil Go provider args SHALL normalize to an empty object; non-nil raw argument values SHALL be valid JSON. Structurally valid unsupported IDs SHALL retain native unsupported-feature handling.

#### Scenario: Mixed direct tool variant
- **WHEN** a provider tool has non-nil strict, input schema, input examples or provider options, or nonempty description
- **THEN** generate and stream setup SHALL fail before HTTP

#### Scenario: Nil Go args
- **WHEN** a provider tool has a valid ID/name and nil args
- **THEN** conversion SHALL use empty-object semantics and the Go Gateway request SHALL contain `args: {}`

### Requirement: Independent bounded opaque client metadata
The Go Gateway client SHALL decode tool metadata through its independent object-valued namespace decoder under gateway-provider-metadata without importing server DTOs. It SHALL preserve unknown namespaces, keys, nested values and omission versus explicit empty objects at supported tool-content and tool-event scopes. It SHALL reject malformed metadata structure under existing protocol bounds, without imposing semantic field inventories or projecting MCP/caller fields. Semantic ownership and correlation validation SHALL remain separate from opaque transport. Response-level identity SHALL remain client-owned. Returning provider metadata SHALL NOT authorize telemetry capture. Client decoding SHALL be independent of which capabilities the Gateway service currently accepts.

#### Scenario: MCP metadata with extensions
- **WHEN** a tool response contains Anthropic MCP type/serverName together with caller and unknown namespace or extension fields
- **THEN** the client SHALL preserve the complete object-valued metadata without deriving routing, execution or capture authority from it

#### Scenario: Empty tool metadata is distinct from omission
- **WHEN** a tool content part or input/call/result event contains explicit empty metadata
- **THEN** the client SHALL preserve the empty object separately from an omitted metadata field

#### Scenario: Deferred result without a repeated call
- **WHEN** a successful unary or SSE response contains only a supported tool result
- **THEN** the client SHALL decode it without importing server correlation state

### Requirement: Client decoding does not activate Gateway capabilities
The Gateway service SHALL retain its unsupported provider-tool request capabilities until separately enabled. Actual native Anthropic MCP configuration and continuation SHALL remain unsupported; foreign options and opaque returned metadata SHALL NOT activate MCP or become grounds for blanket rejection. Existing consumers SHALL compile against the updated SDK types in candidate-source checks without introducing a dependency from Apache modules to Gateway code. Declared internal module pins SHALL resolve to revisions already merged into canonical main; standalone builds SHALL be checked separately before module or image publication.

#### Scenario: Existing service rejection is preserved
- **WHEN** unsupported provider-tool request capabilities or actual native Anthropic MCP consumption reach the existing Gateway runtime
- **THEN** the service SHALL safely reject the unsupported capability before invocation
