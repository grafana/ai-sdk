## MODIFIED Requirements

### Requirement: File validation preserves processing order and bounds
Malformed role/data/reference/media-field shapes, typed null, mixed arms, missing required fields, and schema-invalid file members SHALL fail before the execution boundary, resolution, and model invocation. Schema-valid runtime-deferred branches SHALL remain safe unsupported failures. The route request-body bound under gateway-resource-policy SHALL bound file strings and decoding work, including JSON escaping and base64 overhead.

#### Scenario: Invalid request has no effects
- **WHEN** a file request has conflicting arms, a forbidden reference member, a missing mediaType, a file directly in a tool-role content array, or a typed null filename
- **THEN** complete validation SHALL fail with zero execution-boundary, resolver, and model calls

#### Scenario: Encoded request byte boundary
- **WHEN** otherwise supported file requests are below, exactly at, and one byte above the route request-body bound
- **THEN** only the first two SHALL reach mapping and the oversized body SHALL fail with the fixed invalid-request document and zero resolver or model calls

#### Scenario: Supported file request reaches configured candidates
- **WHEN** a mapped unary or streaming request contains supported ordinary files, active scoped options or supported file-bearing tool history and a primary fails eligibly before commitment
- **THEN** the next configured candidate SHALL receive the same mapped values without text-only refusal, arm substitution or option filtering

#### Scenario: File presence survives eligible failover
- **WHEN** a supported request includes selected empty text/data and filenames that are absent, explicitly empty or nonempty
- **THEN** each attempted model SHALL receive the original selected arms and filename presence, not collapsed absence or substituted bytes
