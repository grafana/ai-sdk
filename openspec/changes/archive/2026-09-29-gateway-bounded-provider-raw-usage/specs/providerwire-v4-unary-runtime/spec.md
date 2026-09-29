## MODIFIED Requirements

### Requirement: Minimal unary success response

A successful unary response SHALL contain only ordered supported text and function-tool-call `content`, `finishReason`, and `usage`. The handler SHALL accept only registered finish reasons and non-negative usage counts no greater than JavaScript's maximum safe integer. Provider warnings, request data, response IDs, timestamps, model IDs, provider identity, headers, bodies, provider metadata, and content metadata SHALL be omitted. `usage.raw` SHALL be omitted when provider `Usage.Raw` is absent, and SHALL contain the provider JSON object unchanged in meaning when present, valid, and bounded; no other new fields SHALL be emitted. The registered Gateway client owns unary `warnings`, `request`, and `response`; raw response-body details outside this minimal contract are not guaranteed.

#### Scenario: Valid text result
- **WHEN** the model returns text, a registered finish reason, and valid usage
- **THEN** the handler SHALL preserve those values and emit no other top-level members

#### Scenario: Unsupported provider result
- **WHEN** the model returns content outside the supported text/function-tool-call subset, an unknown finish reason, invalid usage, `nil, nil`, or panics
- **THEN** the handler SHALL return the fixed internal-error document before committing HTTP 200

#### Scenario: Provider-private fields
- **WHEN** the model result contains warnings, response metadata, backend identity, or provider metadata
- **THEN** none of those values SHALL appear outside the explicitly allowed provider `usage.raw` object in the unary response document

#### Scenario: Provider raw usage is present or absent
- **WHEN** provider usage contains a valid in-limit object including provider-native nested values, an empty object, or no Raw bytes
- **THEN** the unary response SHALL respectively include the object under `usage.raw` with its native keys, include `{}`, or omit the `raw` member without changing normalized counts

#### Scenario: Supplied raw usage is invalid
- **WHEN** provider `Usage.Raw` is nonempty but malformed JSON, JSON null, an array or scalar, or exceeds its input byte limit
- **THEN** the handler SHALL emit the fixed internal-error document before HTTP success is committed, without serializing the raw data or returning normalized-only success

### Requirement: Bounded preflight and standard success encoding

Before encoding, the handler SHALL reject content cardinality or aggregate content, raw-finish string bytes, and raw-usage input bytes that cannot fit the configured unary budget using overflow-safe accounting. It SHALL count raw-usage bytes before parsing or marshaling and reject raw usage longer than 1,048,576 bytes or the configured unary response limit, including whitespace. It SHALL then validate that any present raw is a single JSON object with valid UTF-8 and no unpaired escaped UTF-16 surrogates, including in nested string keys and values, on original bytes before Go JSON encoding can normalize invalid Unicode; valid high/low pairs SHALL be accepted. Unicode scanning SHALL occur only after the size preflight so it remains bounded. The complete minimal private DTO SHALL then be encoded with standard Go JSON, rejected when the final bytes exceed the configured limit, and committed only after successful encoding and the final size check. Provider-domain JSON marshalers SHALL NOT control the response. Standard encoding MAY allocate a bounded constant multiple of the configured limit for worst-case escaping.

#### Scenario: Preflight rejects oversized provider values
- **WHEN** content count or aggregate raw string bytes (including supplied raw usage) exceed the unary budget, or raw usage exceeds 1,048,576 bytes
- **THEN** the result SHALL fail before UTF-8 scanning or JSON encoding

#### Scenario: Escaping crosses the final boundary
- **WHEN** raw bytes pass preflight but standard JSON escaping makes the encoded response exceed the limit
- **THEN** the handler SHALL return the fixed internal error before committing HTTP 200

#### Scenario: Invalid raw Unicode fails before normalization
- **WHEN** in-limit provider raw usage contains invalid UTF-8 or a lone escaped surrogate in an object key or nested value
- **THEN** the handler SHALL reject it before JSON encoding and return the fixed internal error without committing HTTP 200
- **WHEN** in-limit raw usage contains a valid escaped surrogate pair
- **THEN** it SHALL pass Unicode validation, with success still subject to object and complete-response limits

#### Scenario: Response byte boundary
- **WHEN** the encoded response is below, exactly at, or above the configured limit
- **THEN** only complete in-limit documents SHALL receive HTTP 200

#### Scenario: Raw usage exactly meets its input cap
- **WHEN** raw usage bytes, including JSON whitespace, are at or one byte above the smaller of 1,048,576 bytes and the configured unary response limit
- **THEN** only the at-limit value SHALL reach JSON validation, and success SHALL still require the final complete response to fit the unary limit
