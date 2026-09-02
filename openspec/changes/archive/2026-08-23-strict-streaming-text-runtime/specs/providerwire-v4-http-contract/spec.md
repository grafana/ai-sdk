## MODIFIED Requirements

### Requirement: Contract evidence boundary

The exact registered public `@ai-sdk/gateway` client SHALL be authoritative for its observable request emission and response consumption. The ProviderWire V4 workspace SHALL describe one strict HTTP dialect compatible with that client and SHALL NOT claim compatibility with every request accepted by Vercel's private Gateway service. Private protocol DTOs and test-time schemas SHALL own server-side shapes the client does not observe, while raw HTTP, privacy, and bounds tests SHALL own unobserved server safety properties.

#### Scenario: Observable client behavior is authoritative
- **WHEN** request emission or response consumption is observable through the registered client
- **THEN** contract evidence SHALL treat that behavior as authoritative for the strict ProviderWire V4 dialect
- **AND** it SHALL NOT infer acceptance by Vercel's private Gateway service

#### Scenario: Unobserved server shape and safety remain independent
- **WHEN** the registered client masks a field, permissively accepts arbitrary response JSON, or cannot observe a server safety property
- **THEN** private protocol DTOs and test-time schemas SHALL define the unobserved server shape
- **AND** raw HTTP, privacy, and bounds tests SHALL define the unobserved server safety requirement
- **AND** those authorities SHALL NOT contradict observable registered-client behavior

#### Scenario: Production unary and streaming replay is established
- **WHEN** the strict streaming text runtime is complete
- **THEN** each committed request emitted by the registered client SHALL replay to its expected result
- **AND** supported unary records SHALL execute through `DoGenerate`
- **AND** supported streaming records SHALL execute through `DoStream`, bounded SSE framing, terminal finish, and clean EOF
- **AND** other records SHALL reach complete schema validation and either supported execution or a safe unsupported-family response
- **AND** dedicated supported scalar and focused one-capability requests SHALL cover behavior that multi-capability goldens cannot isolate
- **AND** a pinned registered client SHALL complete supported minimal unary and streaming text calls against the real Go handler

#### Scenario: Streaming response authority is local
- **WHEN** the pinned client consumes normalized start, metadata, text, provider errors, finish, and clean EOF
- **THEN** that result SHALL prove observable client compatibility
- **AND** the test-only stream-event schema, explicit encoder fixtures, raw SSE bytes, state-machine tests, privacy tests, and boundary tests SHALL remain authoritative for unobserved server behavior

#### Scenario: Later stream families remain deferred
- **WHEN** the strict streaming text runtime is complete
- **THEN** reasoning, tools, approvals, files, sources, custom content, raw output, and every other later stream family SHALL remain explicit unsupported capabilities or safe terminal adapter failures according to their request or response boundary
- **AND** the repository SHALL NOT claim complete LanguageModelV4 stream execution coverage
