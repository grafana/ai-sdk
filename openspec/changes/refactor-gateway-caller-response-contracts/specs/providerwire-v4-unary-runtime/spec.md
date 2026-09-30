## MODIFIED Requirements

### Requirement: Fixed privacy-safe errors
Gateway-internal, host, resolver, panic, invalid-request and transport-only failures SHALL retain fixed safe status/message/type/code/param documents, including fixed host authentication and permission errors. Reviewed provider API failures SHALL instead use gateway-caller-response-policy's bounded selected-adapter projection and registered status/category/retry policy. Generic causes, provider transport credentials, URLs, headers, request bodies and whole native bodies/data SHALL never be serialized. Unknown or invalid categories SHALL fall back to fixed internal error.

#### Scenario: Provider API failure
- **WHEN** DoGenerate returns a reviewed structured provider API failure
- **THEN** the handler SHALL expose only its bounded actionable projection, not reduce all valid provider messages/codes to generic prose

#### Scenario: Internal or transport failure
- **WHEN** DoGenerate returns a transport-only or arbitrary internal error, panics or lacks a reviewed provider source
- **THEN** the handler SHALL retain fixed safe diagnostics without serializing the cause

#### Scenario: Unknown model
- **WHEN** catalog resolution reports an unknown public model
- **THEN** the handler SHALL return the fixed model-not-found document

#### Scenario: Client classification
- **WHEN** both clients consume reviewed provider or fixed Gateway errors
- **THEN** category/status/retry behavior SHALL match the exact registered public client, with code/param availability described accurately

### Requirement: Minimal unary success response
A successful response SHALL contain ordered supported text, client-executed function-tool-call, URL/document source, reasoning and reasoning-file content, registered finishReason, validated usage, preserved warnings and supported providerMetadata. #280 SHALL own providerMetadata preservation across content/results with unknown object-valued namespaces, nested JSON values and absent/empty distinctions under reviewed bounds, not gated by IncludeRawChunks. Source identity/display SHALL follow gateway-sources. Required empty text/reasoning and selected empty file data SHALL survive. Existing non-negative JavaScript-safe usage validation and usage.raw object/absent/1 MiB contract SHALL remain unchanged.

Warnings SHALL be a non-null array preserving registered fields/order under gateway-caller-response-policy bounds and documented optional-empty Go normalization. Registered response id/modelId/timestamp SHALL preserve supplied actual provider values when present and valid; canonical route identity SHALL NOT substitute for them. Native request data, provider response headers/body and invented provider/topology members SHALL remain omitted. Clients SHALL own Gateway request/response transport: pinned typed unary identity is overwritten, but the registered response fields remain available in the bounded raw response body. Valid output SHALL not fail solely for carrying supported metadata/warnings/actual identity. Invalid output SHALL fail safely before HTTP 200.

#### Scenario: Reasoning-only paid success
- **WHEN** a provider returns a valid reasoning-only result
- **THEN** the Gateway SHALL return success rather than a retryable adaptation error
- **AND** default high-level retries SHALL not invoke the provider again for that valid result

#### Scenario: Valid text result
- **WHEN** the model returns text, a registered finish reason and valid usage with supported warnings/metadata
- **THEN** the handler SHALL preserve those values and only registered supported top-level members

#### Scenario: Unsupported provider result
- **WHEN** the model returns unsupported content/execution markers, unknown finish reason, invalid usage, nil result without error or panics
- **THEN** the handler SHALL return fixed internal error before committing HTTP 200, without stripping enabled execution markers into success

#### Scenario: Caller response differs from transport
- **WHEN** the result contains supported metadata, warnings and actual response identity plus native headers/body
- **THEN** supported caller data SHALL survive while native transport SHALL be omitted
- **AND** pinned clients SHALL combine unary warnings and retain their own transport, with native identity visible only in raw response body

#### Scenario: Provider raw usage is present or absent
- **WHEN** provider usage supplies a valid in-limit nested object, empty object or no Raw bytes
- **THEN** usage.raw SHALL respectively contain that object, contain {} or be absent without changing normalized counts

#### Scenario: Supplied raw usage is invalid
- **WHEN** nonempty Raw is malformed, null, array/scalar or over its existing input limit
- **THEN** fixed internal error SHALL occur before HTTP success without reflecting it or returning normalized-only success

### Requirement: Bounded preflight and standard success encoding

Before encoding, the handler SHALL reject content cardinality or aggregate content, metadata, warning and response-identity bytes, raw-finish string bytes, and raw-usage input bytes that cannot fit the configured unary budget using overflow-safe accounting. It SHALL count raw-usage bytes before parsing or marshaling and reject raw usage longer than 1,048,576 bytes or the configured unary response limit, including whitespace. It SHALL then validate that any present raw is a single JSON object with valid UTF-8 on original bytes. Standard JSON encoding SHALL preserve valid JSON escape sequences, including lone and paired UTF-16 surrogate escapes, in the raw object. Validation SHALL occur only after the size preflight so it remains bounded. The complete explicit private DTO SHALL then be encoded with standard Go JSON, rejected when the final bytes exceed the configured limit, and committed only after successful encoding and the final size check. Provider-domain JSON marshalers SHALL NOT control the response. Standard encoding MAY allocate a bounded constant multiple of the configured limit for worst-case escaping.

#### Scenario: Preflight rejects oversized provider values
- **WHEN** content count or aggregate raw string bytes (including metadata, warnings, response identity and supplied raw usage) exceed the unary budget, or raw usage exceeds 1,048,576 bytes
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

### Requirement: Compatibility evidence

The runtime SHALL replay every committed ProviderWire request golden without modifying it. Cross-language integration tests SHALL call the production handler through the exact registered `@ai-sdk/gateway` version and verify supported unary success, streaming text through clean EOF, representative errors, and cancellation. Raw Go tests SHALL remain authoritative for exact documents, privacy, sequencing, lifecycle, and byte bounds.

#### Scenario: Registered client success
- **WHEN** the pinned Gateway client sends a supported unary request
- **THEN** it SHALL consume content, finish reason, and usage from the production handler and combine server and client warnings and supply its own request/response transport fields, while tests explicitly assert the typed unary response-identity overwrite boundary

#### Scenario: Streaming request
- **WHEN** the registered client sends a supported streaming text request
- **THEN** the handler SHALL invoke `DoStream` once and the client SHALL consume the strict stream through clean EOF
