## MODIFIED Requirements

### Requirement: Contract evidence boundary

The ProviderWire V4 workspace SHALL describe one HTTP dialect compatible with the exact registered public client. It SHALL NOT claim compatibility with every request accepted by Vercel's private Gateway service. Client-consumption probes SHALL NOT serve as unary, stream, or error response authority for the Go server. The unary runtime SHALL instead use private DTOs, test-time response schemas, raw HTTP assertions, privacy and bounds tests, and semantic replay of the committed client requests.

#### Scenario: Client overwrites or permissively accepts fields
- **WHEN** the registered client masks a server field or accepts arbitrary response JSON
- **THEN** the contract documentation and tests SHALL identify the limitation
- **AND** Go server correctness SHALL require independent DTO, test-time schema, raw HTTP, privacy, sequencing, and bounds evidence

#### Scenario: Production unary replay is established
- **WHEN** the strict unary runtime is complete
- **THEN** each committed request emitted by the registered client SHALL replay to its expected unary result
- **AND** streaming records SHALL fail unary envelope validation without model resolution
- **AND** unary records SHALL reach complete schema validation and either supported execution or a safe unsupported-family response
- **AND** dedicated supported scalar and focused one-capability requests SHALL cover behavior that multi-capability goldens cannot isolate
- **AND** a pinned registered client SHALL complete a supported minimal unary text call against the real Go handler

#### Scenario: Streaming remains deferred
- **WHEN** this unary runtime change is complete
- **THEN** strict streaming commitment, event state, SSE framing, and clean-EOF behavior SHALL remain unimplemented by the Go handler
- **AND** the phase 2 streaming client probes SHALL remain consumption evidence only
