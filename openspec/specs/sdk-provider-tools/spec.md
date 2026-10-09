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

The Go Gateway client SHALL decode tool metadata with its independent object-valued namespace decoder under gateway-provider-metadata, not server DTOs. At supported tool-content/event scopes it SHALL retain unknown namespaces/keys/nested values and omission versus empty objects. It SHALL reject malformed structure under existing protocol bounds, without semantic inventories or MCP/caller projection.

#### Scenario: MCP metadata with extensions
- **WHEN** a tool response contains Anthropic MCP type/serverName together with caller and unknown namespace or extension fields
- **THEN** the client SHALL preserve the complete object-valued metadata without deriving routing, execution or capture authority from it

#### Scenario: Empty tool metadata is distinct from omission
- **WHEN** a tool content part or input/call/result event contains explicit empty metadata
- **THEN** the client SHALL preserve the empty object separately from an omitted metadata field

#### Scenario: Deferred result without a repeated call
- **WHEN** a successful unary or SSE response contains only a supported tool result
- **THEN** the client SHALL decode it without importing server correlation state

### Requirement: Opaque client metadata conveys no authority

Semantic ownership/correlation checks SHALL remain separate from opaque transport. Response identity SHALL remain client-owned. Returned provider metadata SHALL NOT authorize telemetry capture. Client decoding SHALL be independent of Gateway service capability acceptance.

#### Scenario: Service support does not control opaque decoding
- **WHEN** a response has valid opaque metadata for a capability not accepted by the service
- **THEN** client decoding SHALL remain independent and SHALL NOT derive ownership, correlation or telemetry capture authority from it.

### Requirement: Client decoding does not activate Gateway capabilities

Gateway support SHALL depend on request mapping and selected native adapter, not client decoding. Foreign options/opaque returned metadata SHALL NOT independently activate MCP or justify blanket rejection. Existing consumers SHALL compile against updated SDK types in candidate-source checks without Apache-module dependencies on Gateway code.

#### Scenario: Existing service rejection is preserved
- **WHEN** an unsupported request capability reaches the Gateway runtime
- **THEN** the service SHALL safely reject the unsupported capability before invocation

### Requirement: Internal pins and standalone publication checks

Declared internal module pins SHALL resolve to revisions already merged into canonical main. Standalone builds SHALL be checked separately before module or image publication.

#### Scenario: Publication uses merged pins and standalone checks
- **WHEN** a module or image is prepared for publication
- **THEN** internal pins SHALL reference canonical-main merged revisions and standalone builds SHALL be checked separately.
