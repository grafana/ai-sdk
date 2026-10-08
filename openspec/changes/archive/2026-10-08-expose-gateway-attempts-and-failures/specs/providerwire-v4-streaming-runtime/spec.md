## MODIFIED Requirements

### Requirement: Ordered non-terminal provider errors
Each pre-finish provider `PartError` SHALL be independently reduced through the closed safe-error classification and emitted in place with base fields `{"type":"error","error":{"message":string,"type":string,"param":null,"code":string,"statusCode":integer,"retryable":boolean}}`. Optional `error.data` MAY carry protected current-error and execution summaries under gateway-attempt-failure-evidence. A valid provider error SHALL not terminate the stream or alter lifecycle state; later metadata, content, additional provider errors, and finish SHALL remain valid. Nil, malformed, or unclassifiable provider error values SHALL reduce to the canonical internal safe error part. Arbitrary native bodies, headers, causes and metadata SHALL NOT enter the public error event. Protected message/type/code/status summaries and observed configured identity MAY appear only through the governed optional error data; actual credentials and other tenants' state SHALL remain protected.

#### Scenario: Provider error is followed by content
- **WHEN** a provider emits an error before or within a text block and later emits otherwise valid content and finish
- **THEN** the client SHALL receive the safe error and every later event in provider order

#### Scenario: Multiple provider errors are ordered
- **WHEN** a provider emits multiple errors separated by valid stream parts
- **THEN** each error SHALL be independently normalized and retained in its original position without terminating the stream

#### Scenario: Provider error contains hostile detail
- **WHEN** a provider error contains credentials, URLs, bodies, headers, data, causes, backend identity, or arbitrary messages
- **THEN** its base public fields SHALL retain the approved category message, while optional error data SHALL contain only the governed protected summary/overview rather than a native error dump

#### Scenario: Provider error status is malformed
- **WHEN** an `APICallError` carries a non-zero status outside the valid HTTP range 100 through 599
- **THEN** it SHALL reduce to the canonical internal error without inspecting a wrapped transport cause

#### Scenario: Provider error has no HTTP status
- **WHEN** an `APICallError` has status zero and wraps a timeout-capable transport error
- **THEN** it SHALL reduce to the canonical timeout error
- **AND** a status-zero `APICallError` with an ordinary network or DNS cause SHALL reduce to the canonical upstream error
- **AND** a valid status-bearing provider error, including an SSE error reported with HTTP status 200, SHALL retain status-based classification

### Requirement: Complete bounded SSE framing and flushing
Committed responses SHALL use HTTP 200, `Content-Type: text/event-stream`, and `Cache-Control: no-cache, no-transform`; connection-specific keep-alive headers SHALL not be protocol requirements. The handler SHALL flush commitment headers and every fully written event. Unsupported flushing SHALL not fail the stream; every other header or frame flush error or panic SHALL be a writer failure. Every public event SHALL contain only its registered outer fields, with supported providerMetadata treated as opaque object-valued namespaces rather than a namespace/key allowlist and SHALL be framed as exactly `data: <json>\n\n`. Aggregate metadata key/original raw bytes and cardinality SHALL participate with other event values in overflow-safe preflight before UTF-8/JSON validation or metadata allocation. Encoding work and temporary memory SHALL remain bounded by a constant multiple of the configured frame limit, and the complete frame SHALL fit that limit before any of its bytes are written. Synthetic terminal errors SHALL retain their fixed base fields; optional execution attribution SHALL follow gateway-attempt-failure-evidence and preserve the canonical base frame when enrichment cannot fit. The server SHALL never emit SSE `event:` fields or `[DONE]`. A write error, short write, writer panic, or supported flush failure SHALL cancel provider work and end immediately without another write.

#### Scenario: Event is written and flushed
- **WHEN** a valid event fits the complete-frame limit and the writer supports flushing
- **THEN** exactly one complete `data:` frame SHALL be fully written and flushed before the next event

#### Scenario: Flushing is unsupported
- **WHEN** the response writer does not support flushing
- **THEN** the stream SHALL continue without treating that limitation as a failure

#### Scenario: Commitment header flush fails
- **WHEN** flushing commitment headers fails or panics
- **THEN** provider work SHALL be canceled and the handler SHALL end without attempting a stream frame

#### Scenario: Event exceeds its complete-frame limit
- **WHEN** an encoded event is one byte larger than the configured complete-frame limit
- **THEN** no bytes from that oversized event SHALL be written and one bounded terminal internal-error frame SHALL be attempted

#### Scenario: Writer fails
- **WHEN** writing or flushing an event fails, writes short, or panics
- **THEN** provider work SHALL be canceled and no synthetic event or second write SHALL be attempted on that writer

#### Scenario: Invalid metadata cannot become partial event
- **WHEN** registered metadata is malformed JSON, invalid UTF-8, non-object at a namespace, or passes input preflight but pushes the encoded complete frame over its limit
- **THEN** no bytes of that event SHALL be written, existing terminal cancellation/cleanup SHALL apply and no selected fallback candidate SHALL be replayed

## ADDED Requirements

### Requirement: Optional streaming execution overview and current failures

Streaming SHALL use the existing reader/commitment/drain ownership to optionally enrich finish metadata and classified error payloads. First-part selection SHALL NOT claim completion. Current protected native summaries SHALL remain event-local; finish SHALL NOT duplicate them. Existing complete-frame limits, original metadata preflight, error ordering, active-block state, authoritative finish and writer failure behavior SHALL remain unchanged. Optional enrichment failure SHALL preserve the original fitting frame, including native namespaces that cannot be relocated.

#### Scenario: Selected leading error followed by content
- **WHEN** the first provider part is an error followed by valid text and finish
- **THEN** no later candidate SHALL run, both clients SHALL consume all parts in order and finish SHALL contain only the optional compact execution overview rather than stream-error history

#### Scenario: Finish fits only without overview
- **WHEN** original finish encoding fits but enriched finish does not
- **THEN** the original finish SHALL remain authoritative without a new terminal error or read-ahead
