## MODIFIED Requirements

### Requirement: Contract evidence boundary
The ProviderWire V4 workspace SHALL describe one strict HTTP dialect compatible with the exact registered public client. It SHALL NOT claim compatibility with every request accepted by Vercel's private Gateway service. Client-consumption probes SHALL NOT serve as unary, stream, or error response authority for the Go server. The strict runtime SHALL instead use private DTOs, local response and event schemas, raw HTTP assertions, privacy and bounds tests, state-machine tests, and semantic replay of the committed client requests.

#### Scenario: Client overwrites or permissively accepts fields
- **WHEN** the registered client masks a server field, accepts arbitrary response JSON, filters raw events, tolerates `[DONE]`, or converts response-metadata timestamps
- **THEN** the contract documentation and tests SHALL identify the limitation
- **AND** Go server correctness SHALL require independent DTO, schema, raw HTTP, privacy, sequencing, lifecycle, framing, and bounds evidence

#### Scenario: Production unary and streaming replay is established
- **WHEN** the strict streaming text runtime is complete
- **THEN** each committed request emitted by the registered client SHALL replay to its expected phase 4 stage
- **AND** supported unary records SHALL execute through `DoGenerate`
- **AND** supported streaming records SHALL execute through `DoStream`, bounded SSE framing, terminal finish, and clean EOF
- **AND** other unary records SHALL reach schema validation and either supported execution or their deterministic first unsupported capability
- **AND** dedicated supported scalar and focused one-capability requests SHALL provide evidence that multi-capability goldens cannot provide
- **AND** a pinned registered client SHALL complete supported unary and streaming text calls against the real Go handler

#### Scenario: Streaming response authority is local
- **WHEN** the pinned client consumes normalized start, metadata, text, provider errors, finish, and clean EOF
- **THEN** that result SHALL prove client compatibility only
- **AND** embedded event schemas, explicit encoders, raw SSE bytes, state-machine tests, privacy tests, and boundary tests SHALL remain authoritative for the server-emitted stream

#### Scenario: Later stream families remain deferred
- **WHEN** the strict streaming text runtime is complete
- **THEN** reasoning, tools, approvals, files, sources, custom content, raw output, and every other later stream family SHALL remain explicit unsupported capabilities or safe terminal adapter failures according to their request or response boundary
- **AND** the repository SHALL NOT claim complete LanguageModelV4 stream execution coverage
