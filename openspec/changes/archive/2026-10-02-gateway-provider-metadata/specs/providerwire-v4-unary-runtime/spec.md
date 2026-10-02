## MODIFIED Requirements

### Requirement: Minimal unary success response

A successful response SHALL include ordered supported text, function-tool-call,
source, reasoning and reasoning-file content, finishReason, usage and optional providerMetadata, alongside warning/scalar fields governed by their separately owned contract. The handler SHALL
accept only registered finish reasons and non-negative usage counts no greater
than JavaScript's maximum safe integer. Result and every supported content variant SHALL preserve supplied providerMetadata through gateway-provider-metadata's bounded opaque object transport, without namespace/key projection. Absence SHALL remain omitted and an explicit empty object SHALL remain present. Provider warnings, request data, response IDs, timestamps, model IDs, provider identity, headers and bodies SHALL retain their separately owned projection; this requirement SHALL NOT introduce native diagnostics or scalar identity changes.
`usage.raw` SHALL be omitted when provider `Usage.Raw` is absent and SHALL
contain the provider JSON object unchanged in meaning when present, valid and
bounded. The registered Gateway client combines supplied server warnings with client warnings and replaces request/response with Gateway-hop information; it does not overwrite server warnings merely because it replaces transport. Warning/scalar restoration remains #320/PR #325; metadata deltas, schemas and tests SHALL preserve its integrated behavior on rebase/merge rather than restore the current base's suppression. Native transport diagnostics remain separately owned.
Required empty reasoning text and selected empty inline file data SHALL be
retained. Invalid output SHALL fail safely before HTTP 200.

#### Scenario: Reasoning-only paid success
- **WHEN** a provider returns a valid reasoning-only result
- **THEN** the Gateway SHALL return success rather than a retryable adaptation error
- **AND** default high-level retries SHALL not invoke the provider again for that valid result

#### Scenario: Valid text result
- **WHEN** the model returns text, a registered finish reason, and valid usage
- **THEN** the handler SHALL preserve those values and include providerMetadata only when supplied, without enabling unrelated top-level transport fields

#### Scenario: Unsupported provider result
- **WHEN** the model returns content outside the supported text/function-tool-call/source/reasoning/reasoning-file subset, an unknown finish reason, invalid usage, `nil, nil`, or panics
- **THEN** the handler SHALL return the fixed internal-error document before committing HTTP 200

#### Scenario: Ordinary metadata is not transport
- **WHEN** a model result contains supported result/content providerMetadata alongside unrelated request or response transport fields
- **THEN** the metadata SHALL survive in its registered scope without adding those transport fields or consulting operator capture flags

#### Scenario: Provider raw usage is present or absent
- **WHEN** provider usage contains a valid in-limit object including provider-native nested values, an empty object, or no Raw bytes
- **THEN** the unary response SHALL respectively include the object under `usage.raw` with its native keys, include `{}`, or omit the `raw` member without changing normalized counts

#### Scenario: Supplied raw usage is invalid
- **WHEN** provider `Usage.Raw` is nonempty but malformed JSON, JSON null, an array or scalar, or exceeds its input byte limit
- **THEN** the handler SHALL emit the fixed internal-error document before HTTP success is committed, without serializing the raw data or returning normalized-only success

### Requirement: Bounded preflight and standard success encoding

Before encoding, the handler SHALL reject content and metadata cardinality or aggregate content, original result/content metadata key/value bytes, raw-finish string bytes, and raw-usage input bytes that cannot fit the configured unary budget using overflow-safe accounting. It SHALL count raw-usage bytes before parsing or marshaling and reject raw usage longer than 1,048,576 bytes or the configured unary response limit, including whitespace. It SHALL then validate that any present raw is a single JSON object with valid UTF-8 on original bytes. Standard JSON encoding SHALL preserve valid JSON escape sequences, including lone and paired UTF-16 surrogate escapes, in the raw object. Metadata SHALL use one aggregate unary budget across the result and all content parts, count original namespace raw bytes including whitespace before JSON parsing or allocation, and validate namespace names and raw JSON objects for UTF-8 and JSON syntax without key projection. Validation SHALL occur only after the size preflight so it remains bounded. The complete minimal private DTO SHALL then be encoded with standard Go JSON, rejected when the final bytes exceed the configured limit, and committed only after successful encoding and the final size check. Provider-domain JSON marshalers SHALL NOT control the response. Standard encoding MAY allocate a bounded constant multiple of the configured limit for worst-case escaping.

#### Scenario: Preflight rejects oversized provider values
- **WHEN** content count or aggregate raw string bytes (including supplied raw usage) exceed the unary budget, or raw usage exceeds 1,048,576 bytes
- **THEN** the result SHALL fail before UTF-8 scanning or JSON encoding

#### Scenario: Escaping crosses the final boundary
- **WHEN** raw bytes pass preflight but standard JSON escaping makes the encoded response exceed the limit
- **THEN** the handler SHALL return the fixed internal error before committing HTTP 200

#### Scenario: Raw UTF-8 and JSON escapes
- **WHEN** in-limit provider raw usage contains invalid UTF-8 in an object key or nested value
- **THEN** the handler SHALL reject it before JSON encoding and return the fixed internal error without committing HTTP 200
- **WHEN** in-limit raw usage contains valid JSON with lone or paired escaped surrogates
- **THEN** the handler SHALL preserve the raw JSON escapes, with success still subject to object and complete-response limits

#### Scenario: Response byte boundary
- **WHEN** the encoded response is below, exactly at, or above the configured limit
- **THEN** only complete in-limit documents SHALL receive HTTP 200

#### Scenario: Raw usage exactly meets its input cap
- **WHEN** raw usage bytes, including JSON whitespace, are at or one byte above the smaller of 1,048,576 bytes and the configured unary response limit
- **THEN** only the at-limit value SHALL reach JSON validation, and success SHALL still require the final complete response to fit the unary limit

#### Scenario: Combined metadata exceeds budget
- **WHEN** individually small metadata objects across multiple content parts and the result collectively exceed the unary budget
- **THEN** the complete result SHALL fail before JSON parsing or encoding rather than returning partial or metadata-free success
