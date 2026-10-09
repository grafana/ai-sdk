# gateway-reasoning-content Specification

## Purpose
Define bounded unary and streaming reasoning transport through ProviderWire, preserving continuation metadata and independent reasoning lifecycles.
## Requirements
### Requirement: Bounded reasoning transport
The service SHALL accept assistant reasoning text and reasoning files with exactly one data or URL arm, preserve scoped options, and return ordered reasoning content through unary and streaming ProviderWire. Required empty strings SHALL remain present. Unknown content families SHALL remain unsupported.

#### Scenario: Valid reasoning-only paid response
- **WHEN** a provider returns only reasoning
- **THEN** both registered clients SHALL consume success without a retry or another provider invocation

### Requirement: Continuation metadata replacement
The service and independent Go client SHALL preserve bounded opaque metadata with arbitrary object-valued namespaces under gateway-provider-metadata, including absence versus an empty object and nested null encrypted content, and retain metadata at its original event position. Namespace/key/type inventories SHALL NOT select continuation fields or discard unknown metadata. Core assembly SHALL replace the previous object with the latest non-nullish object rather than recursively merge it.

#### Scenario: Signature arrives on end
- **WHEN** an end event carries a signature or final encrypted content
- **THEN** the next assistant history and native provider request SHALL retain that final value

#### Scenario: Empty object replaces without deep merge
- **WHEN** reasoning metadata initially contains native continuation values and a later registered event supplies an empty object or a disjoint replacement object
- **THEN** both clients' assembled reasoning SHALL use the later complete object, while an omitted later object SHALL retain the previous metadata

### Requirement: Reasoning continuation telemetry exclusion
Continuation values SHALL never enter metadata-only telemetry.

#### Scenario: Reasoning continuation telemetry exclusion
- **WHEN** a reasoning end event carries final encrypted content or a signature
- **THEN** the caller SHALL retain supported continuation values but metadata-only telemetry SHALL exclude them

### Requirement: Concurrent reasoning lifecycle
The service SHALL track separate active reasoning IDs independently from text and tools, preserve raw IDs and ordering, and reject invalid lifecycle before writing the invalid event. Existing bounded execution and privacy rules SHALL apply.

#### Scenario: Overlapping blocks
- **WHEN** two reasoning blocks overlap with text using an equal ID and an atomic reasoning file
- **THEN** each block SHALL close independently and the events SHALL retain provider order
