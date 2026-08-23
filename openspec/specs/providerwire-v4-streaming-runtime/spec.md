# providerwire-v4-streaming-runtime Specification

## Purpose

Define the production strict, bounded ProviderWire V4 streaming text runtime and its compatibility evidence.

## Requirements

### Requirement: Constructed streaming limits
The strict ProviderWire V4 handler SHALL require positive limits for provider stream-part count, complete SSE frame bytes, stream idle duration, and post-cancellation drain duration in addition to the existing request, unary, and total model-duration limits. Construction SHALL reject a frame limit that cannot safely use `limit+1` or cannot contain the canonical empty `stream-start` frame and every fixed stream-error frame. Complete frame size SHALL include the `data: ` prefix, JSON payload, and terminating `\n\n`.

#### Scenario: Valid streaming limits
- **WHEN** a caller supplies positive part-count, safe frame, idle, and drain limits whose frame limit contains the canonical empty start and every fixed stream-error frame
- **THEN** handler construction SHALL succeed and runtime streaming behavior SHALL be fixed by those values

#### Scenario: Invalid streaming limits
- **WHEN** a streaming limit is zero or negative, the frame limit cannot safely use `limit+1`, or the frame limit cannot contain the canonical empty start or any fixed stream-error frame
- **THEN** construction SHALL fail before the handler serves a request

### Requirement: Shared strict streaming request pipeline
The handler SHALL accept `ai-language-model-streaming` only when its single exact value is `true` or `false`. It SHALL route `false` to the existing unary path and `true` to the streaming path only after the same bounded body read, standard Go JSON and complete request-schema validation, explicit text-subset mapping, exact-once catalog resolution, and validation of a non-empty valid-UTF-8 canonical ID with a non-nil V4 model. A supported streaming request SHALL invoke `DoStream` exactly once and SHALL NOT invoke `DoGenerate`. Any failure before stream invocation SHALL select a fixed non-2xx JSON document and SHALL produce no SSE commitment.

#### Scenario: Supported streaming envelope executes once
- **WHEN** a valid text request uses streaming value `true` and passes mapping and resolution
- **THEN** resolution and `DoStream` SHALL each run once, and `DoGenerate` SHALL not run

#### Scenario: Streaming request fails before invocation
- **WHEN** a streaming request fails envelope, body, standard JSON, schema, mapping, or resolution processing
- **THEN** resolution or model invocation SHALL not run after an earlier failure and the response SHALL remain a fixed non-2xx JSON error rather than SSE

#### Scenario: Invalid streaming selector
- **WHEN** the streaming header is missing, empty, repeated, or has a value other than exact `true` or `false`
- **THEN** envelope validation SHALL fail before body mapping, resolution, or model invocation

### Requirement: Single-owner stream setup and logical commitment
The total model-duration clock SHALL cover `DoStream` setup and all later stream consumption. Setup SHALL execute with panic recovery and an explicit atomic pending, handler-owned, or abandoned handoff. After outcome readiness, one precedence snapshot SHALL be the ownership linearization point: caller cancellation and protocol-clock total expiry observable in that snapshot SHALL cause atomic abandonment, while a successful handler claim based on that snapshot SHALL establish ownership and make conditions observable afterward post-claim outcomes. An abandoned setup worker SHALL exclusively own cancellation and asynchronous bounded drain for any channel returned later; a claiming handler SHALL exclusively own all cleanup. A claimed setup error, panic, `nil, nil` return, nil result, nil stream channel, or result-plus-error SHALL remain a bounded non-2xx safe JSON failure before commitment. Every non-nil invalid channel present in a claimed outcome SHALL be canceled and start the same asynchronous bounded drain exactly once before JSON return, without waiting for drain completion. A non-nil channel returned after abandonment SHALL start cleanup immediately when the late outcome becomes available, even though the handler may already have returned. A claimed non-nil result with a non-nil stream channel and no error SHALL be the irrevocable logical SSE commitment boundary. After that boundary every failure SHALL remain in SSE when the writer is usable; provider request metadata, response headers, and other `StreamResult` metadata SHALL not cross the public boundary.

#### Scenario: Stream setup fails before commitment
- **WHEN** `DoStream` returns an error, panics, returns `nil, nil`, or returns a result with a nil stream channel
- **THEN** the handler SHALL return the corresponding bounded non-2xx safe JSON response and SHALL not commit SSE

#### Scenario: Result and error include a stream
- **WHEN** `DoStream` returns both an error and a non-nil stream channel
- **THEN** the outcome SHALL remain pre-commit, its single owner SHALL cancel and start asynchronous bounded drain exactly once, and the handler SHALL return bounded non-2xx JSON without waiting for drain duration

#### Scenario: Ready conditions are snapshotted before ownership
- **WHEN** outcome readiness and cancellation and/or total expiry are all made observable before the ownership decision is invoked
- **THEN** one precedence snapshot SHALL observe those conditions, abandonment SHALL win, the setup worker SHALL own cleanup, and the handler SHALL not commit SSE

#### Scenario: Condition appears after the ownership snapshot
- **WHEN** a valid outcome is ready, the precedence snapshot observes no cancellation or total expiry, the handler successfully claims ownership, and a condition becomes observable afterward
- **THEN** ownership SHALL remain with the handler and that condition SHALL be processed as a post-claim SSE outcome

#### Scenario: Non-nil stream commits streaming mode
- **WHEN** the ownership snapshot observes no cancellation or total expiry and the handler claims a non-nil result and stream channel without error
- **THEN** the response mode SHALL irrevocably become HTTP 200 SSE before the first provider part is interpreted and the handler SHALL own cleanup

#### Scenario: Initial provider part never arrives
- **WHEN** setup establishes a stream but no first provider part arrives before cancellation or timeout
- **THEN** the handler SHALL remain committed to SSE and SHALL attempt one empty start followed by one corresponding terminal error frame when the writer remains usable

### Requirement: Bounded provider-part cardinality
The runtime SHALL use one request-scoped counter for every value received from the provider stream before interpreting it, including provider start, metadata, text, errors, finish, and values consumed during terminal drain. It SHALL accept at most the configured `StreamParts` count. The first excess part SHALL not mutate lifecycle state, grow the retained text-ID set, or produce its provider-derived output; before finish it SHALL cause at most one synthetic terminal internal error, and after an authoritative terminal event it SHALL end drain without another public event. Warning cardinality SHALL be checked against the maximum warnings that can fit the complete start-frame budget before allocating the mapped warning slice.

#### Scenario: Provider parts are below or at the limit
- **WHEN** a valid stream contains fewer than or exactly `StreamParts` provider values including finish
- **THEN** every value SHALL be processed normally

#### Scenario: First excess provider part is rejected
- **WHEN** a provider produces `StreamParts + 1` values before terminal handling
- **THEN** the first excess value SHALL be consumed only for counting, SHALL not affect state or output, and SHALL cause one terminal internal error when writable

#### Scenario: Continuously ready provider floods parts
- **WHEN** a provider channel remains continuously ready with small sequential text blocks
- **THEN** processing and retained text-ID cardinality SHALL stop at `StreamParts` without depending on total-timeout scheduling

#### Scenario: Warning count cannot fit the start budget
- **WHEN** a provider start carries more warnings than the complete frame can minimally represent
- **THEN** mapping SHALL reject the warning list before allocating a same-sized output slice

### Requirement: Normalized stream start and value-safe warnings
Every committed writable stream SHALL emit exactly one public `stream-start` as its first JSON event. The handler SHALL read the first provider part before choosing that event: a provider `stream-start` is valid only as the first provider part and SHALL be consumed while its warnings are mapped through a streaming-specific value-safe mapper. Unary output SHALL omit provider warnings. The streaming mapper SHALL never copy arbitrary provider `Feature`, `Setting`, `Message`, or `Details` strings. It SHALL map `unsupported` to `feature: "model capability"` and `details: "a requested model capability is unsupported"`; `compatibility` to `feature: "model compatibility"` and `details: "a requested setting was adjusted for model compatibility"`; `deprecated` to `setting: "model setting"` and `message: "a requested model setting is deprecated"`; and `other` to `message: "the model reported a warning"`. It SHALL include no provider or model identity in warning prose. Unknown warning discriminators, invalid canonical identity, or oversized warning starts SHALL cause an empty public start followed by at most one synthetic terminal internal error. When the provider omits start, the handler SHALL emit `warnings: []` and process the first provider part. Built-in Anthropic streaming SHALL preserve its initial upstream-event error preflight, then emit one `PartStreamStart` carrying request-conversion warnings before handling or emitting the pre-read first event, and SHALL no longer attach warnings to `PartFinish`.

#### Scenario: Provider start carries known warnings
- **WHEN** the first provider part is `stream-start` with known registered warnings
- **THEN** the client SHALL receive exactly one start containing only approved identifiers and fixed prose with no arbitrary provider string or metadata

#### Scenario: Warning contains hostile private values
- **WHEN** a provider warning contains a credential, URL, private backend model ID, body, header, or arbitrary prose in any warning string field
- **THEN** none of those values SHALL appear publicly and the warning SHALL be generically normalized or fail safely

#### Scenario: Provider omits start
- **WHEN** the first provider part is metadata, text, provider error, or finish
- **THEN** the client SHALL first receive exactly one start with a non-nil empty warnings array and the first provider part SHALL then be processed in order

#### Scenario: Provider start is late or duplicated
- **WHEN** a provider start appears after any earlier provider part or after an initial provider start
- **THEN** the handler SHALL terminate the lifecycle with at most one synthetic safe error and SHALL not emit a second start

#### Scenario: Anthropic warnings follow successful preflight
- **WHEN** Anthropic request conversion produces warnings and initial upstream-event preflight succeeds
- **THEN** its provider stream SHALL emit those warnings on `PartStreamStart` before handling or emitting the pre-read event and its finish part SHALL carry no warnings

#### Scenario: Anthropic initial API failure remains setup failure
- **WHEN** Anthropic initial upstream-event preflight returns an API error
- **THEN** `DoStream` SHALL return that error before any `StreamResult` or provider start is exposed

### Requirement: Canonical metadata and text block state
After the public start and within the configured provider-part count, the text-only state machine SHALL accept at most one response-metadata part before the first text block, zero or more sequential text blocks, non-terminal provider error parts at any pre-finish point, and exactly one finish. Response metadata SHALL preserve an optional valid response ID and timestamp, SHALL always set `modelId` to the resolver's canonical public ID, and SHALL omit provider identity, backend model ID, response headers, and provider metadata. Each text start ID SHALL be valid UTF-8, non-empty, globally unique within the stream, and SHALL open the only active block. Text deltas and ends SHALL use the active ID; an end SHALL close it. Required empty deltas SHALL be preserved. Provider errors SHALL not open, close, or otherwise change text or metadata state.

#### Scenario: Canonical metadata precedes text
- **WHEN** a provider emits one valid response-metadata part before text
- **THEN** the public metadata SHALL preserve only allowlisted ID and timestamp values and SHALL use the canonical public model ID

#### Scenario: Sequential text blocks are valid
- **WHEN** a provider emits multiple non-overlapping text start/delta/end blocks with unique IDs
- **THEN** all events SHALL be emitted in order and empty delta strings SHALL remain present

#### Scenario: Text lifecycle is invalid
- **WHEN** a text ID is empty, invalid UTF-8, reused, mismatched, opened while another block is active, or ended without an active matching block
- **THEN** the handler SHALL cancel provider work and attempt at most one synthetic terminal internal error

#### Scenario: Metadata placement is invalid
- **WHEN** response metadata is duplicated or appears after a text block has started
- **THEN** the handler SHALL terminate with at most one synthetic safe error rather than forwarding the invalid metadata

### Requirement: Finish validation and terminal authority
A finish part SHALL be valid only when no text block is active, its warnings are empty, and it contains a non-nil registered finish reason and non-nil usage. Known usage counts SHALL be non-negative JavaScript-safe integers and SHALL use the registered input/output groups; raw usage and provider metadata SHALL be omitted. A valid finish SHALL be written once as the final public event and SHALL be authoritative. The handler SHALL then cancel provider work, begin bounded asynchronous drain, and return clean EOF immediately without waiting for provider channel closure or emitting `[DONE]`. Provider parts observed after a written finish SHALL be suppressed during drain, treated as provider lifecycle defects for later operational reporting, and SHALL never produce a second public terminal event.

#### Scenario: Finish closes a valid stream
- **WHEN** a valid finish is emitted with no active text block
- **THEN** the finish SHALL preserve normalized usage and finish reason, provider-private fields SHALL be omitted, and the response SHALL end at clean EOF without `[DONE]`

#### Scenario: Finish is invalid
- **WHEN** finish arrives with an active block, warnings, nil or invalid usage, nil or invalid finish reason, or unsafe token counts
- **THEN** finish SHALL not be written and the handler SHALL attempt at most one synthetic terminal internal error

#### Scenario: Provider emits after finish
- **WHEN** any provider part is available after a valid finish has been written
- **THEN** finish SHALL remain the final public event and the later part SHALL be suppressed while cancellation and bounded drain complete

### Requirement: Ordered non-terminal provider errors
Each pre-finish provider `PartError` SHALL be independently reduced through the closed safe-error classification and emitted in place as `{"type":"error","error":{"message":string,"type":string,"param":null,"code":string,"statusCode":integer,"retryable":boolean}}`. A valid provider error SHALL not terminate the stream or alter lifecycle state; later metadata, content, additional provider errors, and finish SHALL remain valid. Nil, malformed, or unclassifiable provider error values SHALL reduce to the canonical internal safe error part. No provider message, URL, body, header, data, cause, provider identity, backend model ID, or arbitrary metadata SHALL enter the public event.

#### Scenario: Provider error is followed by content
- **WHEN** a provider emits an error before or within a text block and later emits otherwise valid content and finish
- **THEN** the client SHALL receive the safe error and every later event in provider order

#### Scenario: Multiple provider errors are ordered
- **WHEN** a provider emits multiple errors separated by valid stream parts
- **THEN** each error SHALL be independently normalized and retained in its original position without terminating the stream

#### Scenario: Provider error contains hostile detail
- **WHEN** a provider error contains credentials, URLs, bodies, headers, data, causes, backend identity, or arbitrary messages
- **THEN** its public event SHALL contain only the closed safe fields and approved category message

#### Scenario: Provider error status is malformed
- **WHEN** an `APICallError` carries a non-zero status outside the valid HTTP range 100 through 599
- **THEN** it SHALL reduce to the canonical internal error without inspecting a wrapped transport cause

#### Scenario: Provider error has no HTTP status
- **WHEN** an `APICallError` has status zero and wraps a timeout-capable transport error
- **THEN** it SHALL reduce to the canonical timeout error
- **AND** a status-zero `APICallError` with an ordinary network or DNS cause SHALL reduce to the canonical upstream error
- **AND** a valid status-bearing provider error, including an SSE error reported with HTTP status 200, SHALL retain status-based classification

### Requirement: Synthetic terminal adapter errors
Before a valid finish is written, an unsupported stream family, lifecycle violation, invalid provider output, premature channel close, mapping failure, frame overflow, total timeout, idle timeout, or caller cancellation SHALL cancel provider work and produce at most one synthetic terminal safe error when the writer remains usable. Lifecycle, mapping, framing, premature-EOF, and invalid-output failures SHALL use the canonical internal category; cancellation and timeout SHALL retain their closed categories. A terminal error SHALL not close active blocks synthetically or emit a synthetic finish. If a terminal event has already been written or the writer has failed, no further event SHALL be attempted.

#### Scenario: Provider channel closes before finish
- **WHEN** the provider stream closes without a valid finish
- **THEN** the handler SHALL attempt one terminal internal error and SHALL not emit a finish or `[DONE]`

#### Scenario: Unsupported stream family appears
- **WHEN** the text runtime receives reasoning, tool, file, source, custom, raw, approval, or another unsupported part
- **THEN** it SHALL emit at most one terminal internal error rather than serializing the provider-domain part

#### Scenario: Provider errors precede an adapter failure
- **WHEN** one or more non-terminal provider errors are written before a later lifecycle failure
- **THEN** those provider errors SHALL remain in order and at most one additional synthetic terminal error SHALL end the stream

### Requirement: Complete bounded SSE framing and flushing
Committed responses SHALL use HTTP 200, `Content-Type: text/event-stream`, and `Cache-Control: no-cache, no-transform`; connection-specific keep-alive headers SHALL not be protocol requirements. The handler SHALL flush commitment headers and every fully written event through `http.NewResponseController(w).Flush()` using identical classification. Direct or wrapped unsupported-flush errors matching `http.ErrNotSupported` through `errors.Is` SHALL mean no flush capability and SHALL not fail the stream; every other header or frame flush error or panic SHALL be a writer failure. Every public event SHALL be mapped to a private DTO, encoded incrementally without first accumulating an oversized value, and framed as exactly `data: <json>\n\n` before full write. Synthetic terminal errors SHALL select fixed precomputed complete frames. The complete frame SHALL fit the configured frame limit. The closed draft 2020-12 stream-event schema SHALL validate fixtures and raw output in tests and SHALL NOT be compiled or evaluated on the production write path. The server SHALL never emit SSE `event:` fields or `[DONE]`. A write error, short write, writer panic, or supported flush failure SHALL cancel provider work and end immediately without another write.

#### Scenario: Event is written and flushed
- **WHEN** a valid event fits the complete-frame limit and the writer supports flushing
- **THEN** exactly one complete `data:` frame SHALL be fully written and flushed before the next event

#### Scenario: Unsupported flush is direct or wrapped
- **WHEN** commitment or frame flushing returns `http.ErrNotSupported` directly or through a wrapped error
- **THEN** `errors.Is` classification SHALL treat the writer as having no flush capability and SHALL continue without failure

#### Scenario: Commitment header flush fails
- **WHEN** commitment headers are selected and response-controller flush returns any non-unsupported error or panics
- **THEN** provider work SHALL be canceled and the handler SHALL end without attempting a stream frame

#### Scenario: Event exceeds its complete-frame limit
- **WHEN** incremental encoding would make an event one byte larger than the configured complete-frame limit
- **THEN** no bytes from that oversized event SHALL be written and one bounded terminal internal-error frame SHALL be attempted

#### Scenario: Writer fails
- **WHEN** writing or flushing an event fails, writes short, or panics
- **THEN** provider work SHALL be canceled and no synthetic event or second write SHALL be attempted on that writer

### Requirement: Deterministic cancellation and timeout precedence
The total deadline SHALL begin immediately before `DoStream` invocation and SHALL cover setup and consumption. The idle deadline SHALL begin at successful setup and SHALL reset after each accepted provider part is successfully represented, including a consumed start and a written provider error. A pure precedence arbiter SHALL evaluate observable cancellation, explicit current time, and explicit total and idle deadlines from one protocol-local controllable clock. The streaming model context SHALL derive from the request through explicit cancellation and SHALL NOT use an independent real-time `context.WithTimeout`; the same protocol-clock timers used for public arbitration SHALL cancel provider context before a total- or idle-timeout event is encoded or written. Timer channels SHALL be wake-ups only. Deadline equality SHALL count as expired. Before a terminal event, the handler SHALL recheck conditions after setup and each receive with this precedence: caller cancellation, total deadline, idle deadline, then a newly observed provider failure or finish. A provider finish observed while none of those conditions is yet observable SHALL be authoritative after it is written. Writer failure SHALL terminate immediately without error-frame precedence processing.

#### Scenario: Caller cancellation and timeout coincide
- **WHEN** caller cancellation and a configured deadline are both observable before a terminal event
- **THEN** cancellation SHALL win and produce the non-retryable cancellation category when writable

#### Scenario: Total and idle deadlines coincide
- **WHEN** both total and idle deadlines are expired before a terminal event
- **THEN** total timeout SHALL win

#### Scenario: Total timeout cancels provider before output
- **WHEN** the controllable total timer expires during setup or stream consumption
- **THEN** the model context SHALL be canceled before the timeout response or event is encoded or written

#### Scenario: Idle timeout cancels provider before output
- **WHEN** the controllable idle timer expires after stream establishment
- **THEN** the model context SHALL be canceled before the idle-timeout event is encoded or written

#### Scenario: Provider finish arrives before deadlines
- **WHEN** a valid finish is received before cancellation or either deadline is observable
- **THEN** finish SHALL be written as the authoritative final event

#### Scenario: Accepted activity resets idle time
- **WHEN** accepted provider parts arrive within each idle interval while total duration remains available
- **THEN** the idle deadline SHALL reset after each represented part and SHALL not expire solely because total stream age exceeds one idle interval

### Requirement: Bounded stream ownership and drain
Every terminal handler path after stream establishment SHALL have exactly one setup-handoff owner cancel the provider context and start a drain that reads only until channel close, the first part above the request's `StreamParts` count, or an absolute configured drain deadline. The drain SHALL check part budget and deadline expiry, with equality expired, before and after every receive; its timer SHALL be a wake-up only. Draining SHALL not extend handler latency and SHALL not create an unbounded handler-owned goroutine even when the provider channel is continuously ready. If a claimed setup outcome contains an error and a non-nil stream, its handler owner SHALL cancel and start the same asynchronous bounded drain exactly once before pre-commit JSON return; handler latency SHALL not wait for drain completion. If an abandoned setup returns a non-nil stream later, its worker owner SHALL start cleanup immediately when that outcome becomes available. Providers SHALL remain responsible for observing cancellation and closing their channels; a provider call that never returns or a producer that remains blocked after bounded drain SHALL be classified as a provider lifecycle defect rather than a bounded Gateway resource.

#### Scenario: Provider closes after cancellation
- **WHEN** terminal handling cancels provider work and the provider closes its stream within the drain duration
- **THEN** the bounded drain SHALL consume the remaining channel values and exit

#### Scenario: Provider ignores cancellation
- **WHEN** a provider channel does not close before the drain duration
- **THEN** handler latency and the handler-owned drain lifetime SHALL remain bounded even though provider-owned work may remain defective

#### Scenario: Continuously ready drain reaches a hard bound
- **WHEN** a canceled provider channel remains permanently ready with values
- **THEN** the drain SHALL stop at the first expired absolute deadline or excess provider part rather than repeatedly selecting channel receives

#### Scenario: Setup returns a late stream
- **WHEN** handler cancellation or timeout wins and `DoStream` later returns a non-nil stream
- **THEN** the late setup owner SHALL cancel and bounded-drain that channel exactly once instead of abandoning it unread

### Requirement: Streaming contract and cross-language evidence
Go tests SHALL replay the committed streaming golden and the streaming record in the sequence golden through the production handler as supported text execution while preserving the existing unary golden outcomes. Raw HTTP tests SHALL assert exact SSE headers and bytes, canonical identity, safe warning values and errors, privacy, event schemas, state transitions, setup ownership, precedence, below/at/above part and frame limits, and continuously ready floods. Pinned `@ai-sdk/gateway@4.0.52` evidence SHALL separate an abort scenario that observes server-side provider-context cancellation from un-aborted normal, provider-error, adapter-timeout, finish, and clean-EOF consumption scenarios. Provider lifecycle and transport-failure cases SHALL use focused unit or provider-independent integration tests rather than invented provider conformance input. The parity map SHALL identify the achieved strict streaming text scope and retain explicit gaps for every deferred stream family.

#### Scenario: Committed streaming requests execute
- **WHEN** the phase 2 streaming golden records are replayed through the phase 4 handler
- **THEN** each supported text record SHALL reach `DoStream` once with exact mapped options and canonical resolution

#### Scenario: Registered client consumes production SSE without abort
- **WHEN** the pinned Gateway client remains connected to a normal, provider-error, or adapter-timeout stream from the real Go handler
- **THEN** it SHALL consume events in order through the applicable terminal behavior without requiring `[DONE]`

#### Scenario: Registered client abort cancels established provider work
- **WHEN** a recording model returns a non-nil silent channel, the pinned Gateway client awaits `doStream()` resolution after HTTP commitment, and the client then aborts
- **THEN** the established server-side recording provider SHALL observe its model context cancellation

#### Scenario: Raw authority catches permissive client behavior
- **WHEN** the pinned client permissively accepts or normalizes an event
- **THEN** local DTO schemas, state-machine tests, raw HTTP assertions, privacy tests, and frame-bound tests SHALL remain authoritative for server correctness
