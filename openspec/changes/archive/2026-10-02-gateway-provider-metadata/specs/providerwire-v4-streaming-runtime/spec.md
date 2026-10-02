## MODIFIED Requirements

### Requirement: Canonical metadata and text block state
After the public start and within the configured provider-part count, the state machine SHALL accept at most one response-metadata part before the first text or tool event, zero or more sequential text blocks and independently tracked function-tool input/call/result events governed by gateway-streaming-function-tools, non-terminal provider error parts at any pre-finish point, and exactly one finish. Response identity/scalar values SHALL follow the separately owned #320/PR #325 contract; this metadata-only change SHALL NOT override integrated native response identity behavior. The current base substitutes the canonical modelId and omits backend identity, but that is baseline context rather than renewed policy. Response headers and unregistered providerMetadata SHALL NOT be added to the response-metadata part. Copied requirement blocks and tests SHALL be reconciled on rebase/merge without restoring canonical-only identity or taking over #320. Each text start ID SHALL be valid UTF-8, non-empty, globally unique within the stream, and SHALL open the only active block. Text deltas and ends SHALL use the active ID; an end SHALL close it. Required empty deltas SHALL be preserved. Each text-start, text-delta and text-end SHALL retain supplied bounded opaque providerMetadata at its original event position under gateway-provider-metadata; absent metadata SHALL remain absent and an explicit empty object SHALL remain present. This content metadata SHALL NOT be moved into response-metadata. Provider errors SHALL not open, close, or otherwise change text or metadata state.

#### Scenario: Canonical metadata precedes text
- **WHEN** a provider emits one valid response-metadata part before text or tool events
- **THEN** its placement SHALL remain valid and response identity SHALL follow the separately owned scalar contract, preserving #320/PR #325 native identity behavior when integrated rather than restoring the current base's canonical substitution

#### Scenario: Sequential text blocks are valid
- **WHEN** a provider emits multiple non-overlapping text start/delta/end blocks with unique IDs
- **THEN** all events SHALL be emitted in order and empty delta strings SHALL remain present

#### Scenario: Text lifecycle is invalid
- **WHEN** a text ID is empty, invalid UTF-8, reused, mismatched, opened while another text block is active, or ended without an active matching block
- **THEN** the handler SHALL cancel provider work and attempt at most one synthetic terminal internal error

#### Scenario: Metadata placement is invalid
- **WHEN** response metadata is duplicated or appears after a text or tool event has started
- **THEN** the handler SHALL terminate with at most one synthetic safe error rather than forwarding the invalid metadata

#### Scenario: Text metadata changes at end
- **WHEN** text-start supplies an object, text-delta omits metadata and text-end supplies an empty or replacement object
- **THEN** those exact metadata positions and presence states SHALL reach both clients in order, with no deep merge or relocation

### Requirement: Finish validation and terminal authority
A finish part SHALL be valid only when no text or tool-input block is active, its warnings are empty, and it contains a non-nil registered finish reason and non-nil usage. Known usage counts SHALL be non-negative JavaScript-safe integers and SHALL use the registered input/output groups; `usage.raw` SHALL contain a supplied valid bounded provider JSON object, or be omitted when Raw is absent; finish providerMetadata SHALL be preserved as bounded opaque object-valued namespaces under gateway-provider-metadata. A valid finish SHALL be written once as the final public event and SHALL be authoritative. The handler SHALL then cancel provider work, begin bounded asynchronous drain, and return clean EOF immediately without waiting for provider channel closure or emitting `[DONE]`. Provider parts observed after a written finish SHALL be suppressed during drain, treated as provider lifecycle defects for later operational reporting, and SHALL never produce a second public terminal event.

#### Scenario: Finish closes a valid stream
- **WHEN** a valid finish is emitted with no active text or tool-input block
- **THEN** the finish SHALL preserve normalized usage, finish reason and supplied ordinary providerMetadata, unrelated transport fields SHALL remain omitted, and the response SHALL end at clean EOF without `[DONE]`

#### Scenario: Finish is invalid
- **WHEN** finish arrives with an active block, warnings, nil or invalid usage, nil or invalid finish reason, unsafe token counts, invalid or over-limit supplied raw usage, or invalid or over-limit finish metadata
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

#### Scenario: Finish metadata is ordinary output
- **WHEN** valid finish metadata contains unknown namespaces or an explicit empty object and IncludeRawChunks is false
- **THEN** both clients SHALL receive that metadata on finish before clean EOF without any synthesized raw event

### Requirement: Complete bounded SSE framing and flushing
Committed responses SHALL use HTTP 200, `Content-Type: text/event-stream`, and `Cache-Control: no-cache, no-transform`; connection-specific keep-alive headers SHALL not be protocol requirements. The handler SHALL flush commitment headers and every fully written event. Unsupported flushing SHALL not fail the stream; every other header or frame flush error or panic SHALL be a writer failure. Every public event SHALL contain only its registered outer fields, with supported providerMetadata treated as opaque object-valued namespaces rather than a namespace/key allowlist and SHALL be framed as exactly `data: <json>\n\n`. Aggregate metadata key/original raw bytes and cardinality SHALL participate with other event values in overflow-safe preflight before UTF-8/JSON validation or metadata allocation. Encoding work and temporary memory SHALL remain bounded by a constant multiple of the configured frame limit, and the complete frame SHALL fit that limit before any of its bytes are written. Synthetic terminal errors SHALL use fixed complete frames. The server SHALL never emit SSE `event:` fields or `[DONE]`. A write error, short write, writer panic, or supported flush failure SHALL cancel provider work and end immediately without another write.

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
