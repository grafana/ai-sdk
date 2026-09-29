## MODIFIED Requirements

### Requirement: Finish validation and terminal authority
A finish part SHALL be valid only when no text or tool-input block is active, its warnings are empty, and it contains a non-nil registered finish reason and non-nil usage. Known usage counts SHALL be non-negative JavaScript-safe integers and SHALL use the registered input/output groups; `usage.raw` SHALL contain a supplied valid bounded provider JSON object, or be omitted when Raw is absent; other provider metadata SHALL be omitted. A valid finish SHALL be written once as the final public event and SHALL be authoritative. The handler SHALL then cancel provider work, begin bounded asynchronous drain, and return clean EOF immediately without waiting for provider channel closure or emitting `[DONE]`. Provider parts observed after a written finish SHALL be suppressed during drain, treated as provider lifecycle defects for later operational reporting, and SHALL never produce a second public terminal event.

#### Scenario: Finish closes a valid stream
- **WHEN** a valid finish is emitted with no active text or tool-input block
- **THEN** the finish SHALL preserve normalized usage and finish reason, other provider-private fields SHALL be omitted, and the response SHALL end at clean EOF without `[DONE]`

#### Scenario: Finish is invalid
- **WHEN** finish arrives with an active block, warnings, nil or invalid usage, nil or invalid finish reason, unsafe token counts, or invalid or over-limit supplied raw usage
- **THEN** finish SHALL not be written and the handler SHALL attempt at most one synthetic terminal internal error

#### Scenario: Provider emits after finish
- **WHEN** any provider part is available after a valid finish has been written
- **THEN** finish SHALL remain the final public event and the later part SHALL be suppressed while cancellation and bounded drain complete

#### Scenario: Native raw object survives finish
- **WHEN** a valid finish contains normalized usage and a supplied in-limit raw JSON object including nested provider-native keys or `{}`
- **THEN** its complete `usage.raw` object SHALL be emitted with the normalized counts, and a distinct `type: "raw"` stream part SHALL NOT be synthesized

#### Scenario: No raw value is supplied
- **WHEN** a valid finish has absent provider `Usage.Raw`
- **THEN** the finish SHALL omit `usage.raw` and retain its normalized counts

#### Scenario: Raw usage fails after stream commitment
- **WHEN** finish usage supplies malformed, null, scalar, array, or more than 1,048,576 bytes of raw JSON, or its encoded finish exceeds the configured complete-frame limit
- **THEN** the finish SHALL NOT be written and the handler SHALL attempt at most one fixed complete terminal internal-error SSE frame, without reflecting the offending value or issuing normalized-only finish

## ADDED Requirements

### Requirement: Bounded raw-usage finish representation
The handler SHALL count supplied raw-usage bytes, including whitespace, before JSON validation or encoding. It SHALL reject raw usage exceeding the smaller of 1,048,576 bytes and configured `StreamFrameBytes`, validate one complete JSON object when present, and reject invalid UTF-8 or unpaired escaped UTF-16 surrogates in any raw string key or nested value on the original bytes before Go JSON encoding normalizes them; valid surrogate pairs SHALL be accepted. It SHALL only write a finish after the entire `data: <json>\n\n` frame fits `StreamFrameBytes`. Existing `StreamParts` and complete-frame limits SHALL continue to bound aggregate represented stream output; validation and encoding SHALL use memory bounded by a constant multiple of the configured frame limit.

#### Scenario: Input fits but SSE framing does not
- **WHEN** raw usage meets its input limit but JSON encoding or SSE framing makes the finish exceed `StreamFrameBytes`
- **THEN** no finish bytes SHALL be committed and the handler SHALL attempt at most one fixed bounded terminal error frame

#### Scenario: Invalid raw Unicode fails before finish commitment
- **WHEN** in-limit finish raw usage contains invalid UTF-8 or a lone escaped surrogate in an object key or nested value
- **THEN** the handler SHALL not encode or write finish and SHALL attempt at most one fixed bounded terminal error frame
- **WHEN** in-limit raw usage contains a valid escaped surrogate pair
- **THEN** it SHALL pass Unicode validation, with success still subject to object and complete-frame limits

#### Scenario: Oversized input never reaches parser
- **WHEN** raw usage has one byte more than its input limit, including JSON whitespace
- **THEN** the handler SHALL reject it before parsing or encoding raw JSON
