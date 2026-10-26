## MODIFIED Requirements

### Requirement: File validation preserves processing order and bounds
Malformed role/data/reference/media-field shapes, typed null, mixed arms, missing required fields, and schema-invalid file members SHALL fail before the execution boundary, resolution, and model invocation. Schema-valid runtime-deferred branches SHALL remain safe unsupported failures. The platform request size under gateway-resource-policy SHALL bound file strings and decoding work, including JSON escaping and base64 overhead; the Gateway SHALL add no file or request byte bound of its own.

#### Scenario: Invalid request has no effects
- **WHEN** a file request has conflicting arms, a forbidden reference member, a missing mediaType, a file directly in a tool-role content array, or a typed null filename
- **THEN** complete validation SHALL fail with zero execution-boundary, resolver, and model calls

#### Scenario: Encoded request byte boundary
- **WHEN** an otherwise supported file request is above 1 MiB and within the host route message size, or above the host route message size
- **THEN** the first SHALL reach mapping and the configured candidates, and the second SHALL be rejected by the host with zero resolver or model calls

#### Scenario: Supported file request reaches configured candidates
- **WHEN** a mapped unary or streaming request contains supported ordinary files, active scoped options or supported file-bearing tool history and a primary fails eligibly before commitment
- **THEN** the next configured candidate SHALL receive the same mapped values without text-only refusal, arm substitution or option filtering

#### Scenario: File presence survives eligible failover
- **WHEN** a supported request includes selected empty text/data and filenames that are absent, explicitly empty or nonempty
- **THEN** each attempted model SHALL receive the original selected arms and filename presence, not collapsed absence or substituted bytes

