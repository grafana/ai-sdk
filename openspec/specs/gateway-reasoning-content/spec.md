# gateway-reasoning-content Specification

## Purpose
TBD - created by archiving change gateway-reasoning-content. Update Purpose after archive.
## Requirements
### Requirement: Bounded reasoning transport
The service SHALL accept assistant reasoning text and reasoning files with exactly one data or URL arm, preserve scoped options, and return ordered reasoning content through unary and streaming ProviderWire. Required empty strings SHALL remain present. Unknown content families SHALL remain unsupported.

#### Scenario: Valid reasoning-only paid response
- **WHEN** a provider returns only reasoning
- **THEN** both registered clients SHALL consume success without a retry or another provider invocation

### Requirement: Continuation metadata replacement
The service SHALL project only the namespace/key/type table in the design, preserve absence versus an empty object and null encrypted content, and retain metadata at its original event position. Core assembly SHALL replace the previous object with the latest non-nullish object rather than recursively merge it. Continuation values SHALL never enter metadata-only telemetry.

#### Scenario: Signature arrives on end
- **WHEN** an end event carries a signature or final encrypted content
- **THEN** the next assistant history and native provider request SHALL retain that final value

### Requirement: Concurrent reasoning lifecycle
The service SHALL track separate active reasoning IDs independently from text and tools, preserve raw IDs and ordering, and reject invalid lifecycle before writing the invalid event. Existing bounded execution and privacy rules SHALL apply.

#### Scenario: Overlapping blocks
- **WHEN** two reasoning blocks overlap with text using an equal ID and an atomic reasoning file
- **THEN** each block SHALL close independently and the events SHALL retain provider order
