# providerwire-v4-streaming-runtime Specification

## Purpose

Define the production strict, bounded ProviderWire V4 streaming text runtime and its compatibility evidence.
## Requirements
### Requirement: Constructed streaming limits
The strict ProviderWire V4 handler SHALL require positive limits for provider stream-part count, complete SSE frame bytes, stream idle duration, and post-cancellation drain duration in addition to the existing request, unary, and total model-duration limits. Construction SHALL reject a frame limit that cannot contain the canonical empty `stream-start` frame and every fixed stream-error frame. Complete frame size SHALL include the `data: ` prefix, JSON payload, and terminating `\n\n`.

#### Scenario: Valid streaming limits
- **WHEN** a caller supplies positive part-count, safe frame, idle, and drain limits whose frame limit contains the canonical empty start and every fixed stream-error frame
- **THEN** handler construction SHALL succeed and runtime streaming behavior SHALL be fixed by those values

#### Scenario: Invalid streaming limits
- **WHEN** a streaming limit is zero or negative or the frame limit cannot contain the canonical empty start or any fixed stream-error frame
- **THEN** construction SHALL fail before the handler serves a request

### Requirement: Shared strict streaming request pipeline
The handler SHALL accept `ai-language-model-streaming` only when its single exact value is `true` or `false`. It SHALL route `false` to the existing unary path and `true` to the streaming path only after the same bounded body read, standard Go JSON and complete request-schema validation, explicit supported text/function-tool subset mapping, exact-once catalog resolution, and validation of a non-empty valid-UTF-8 canonical ID with a non-nil V4 model. A supported streaming request SHALL invoke `DoStream` exactly once and SHALL NOT invoke `DoGenerate`. Any failure before stream invocation SHALL select a fixed non-2xx JSON document and SHALL produce no SSE commitment.

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
The total model duration SHALL cover `DoStream` setup and all later stream consumption. Setup SHALL execute with panic recovery so request cancellation or timeout can end handler latency even if `DoStream` does not return. Every returned stream SHALL have one cleanup owner: an outcome received by the handler belongs to the handler, while setup work SHALL clean up a stream returned after the request has ended. A setup error, panic, `nil, nil` return, nil result, nil stream channel, or result-plus-error SHALL remain a bounded non-2xx safe JSON failure before commitment. Every non-nil invalid or late channel SHALL be canceled and asynchronously bounded-drained without delaying the response. A valid received result with a non-nil stream channel SHALL be the irrevocable logical SSE commitment boundary. After that boundary every failure SHALL remain in SSE when the writer is usable; provider request metadata, response headers, and other `StreamResult` metadata SHALL not cross the public boundary.

#### Scenario: Stream setup fails before commitment
- **WHEN** `DoStream` returns an error, panics, returns `nil, nil`, or returns a result with a nil stream channel
- **THEN** the handler SHALL return the corresponding bounded non-2xx safe JSON response and SHALL not commit SSE

#### Scenario: Result and error include a stream
- **WHEN** `DoStream` returns both an error and a non-nil stream channel
- **THEN** the outcome SHALL remain pre-commit, its single owner SHALL cancel and start asynchronous bounded drain exactly once, and the handler SHALL return bounded non-2xx JSON without waiting for drain duration

#### Scenario: Setup completion races termination
- **WHEN** a valid setup outcome and request cancellation or timeout become ready concurrently
- **THEN** either the handler SHALL receive and own the stream or the setup worker SHALL retain and clean up the late stream, with no unowned channel or duplicate cleanup

#### Scenario: Non-nil stream commits streaming mode
- **WHEN** the handler receives a non-nil result and stream channel without error
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
Every committed writable stream SHALL emit exactly one public stream-start as its first JSON event. The handler SHALL read the first provider part before selecting it: a provider stream-start is valid only as the first provider part and SHALL be consumed with its warnings mapped through the same explicit registered warning union used by unary output. Unsupported/compatibility SHALL preserve required feature and optional nonempty details; deprecated SHALL preserve required setting/message; other SHALL preserve required message. Required strings SHALL remain present even when empty, order/multiplicity SHALL survive and inactive fields SHALL be omitted. Optional empty Go details SHALL normalize to omission as a documented representation adaptation, not a privacy policy. Native backend names and ordinary application strings SHALL NOT be replaced with generic prose or heuristically censored.

Unknown warning discriminators, invalid represented strings or over-budget starts SHALL cause one empty public start followed by at most one synthetic terminal internal error when writable. Cardinality and aggregate active string bytes SHALL be bounded before mapped allocation and UTF-8 validation; standard JSON encoding and complete-frame limits SHALL still govern the final write. When the provider omits start, warnings SHALL be an empty array and the first part SHALL then be processed. Built-in Anthropic SHALL retain initial upstream-event error preflight, emit its start with conversion warnings before processing the pre-read event, and attach no warnings to finish. Duplicate/late starts SHALL remain invalid.

#### Scenario: Provider start carries known warnings
- **WHEN** the first provider part carries all registered warning variants with distinct native strings
- **THEN** exactly one public start SHALL preserve their active fields and order

#### Scenario: Required empties and optional details
- **WHEN** feature, setting or message is empty and details is absent/empty
- **THEN** required fields SHALL remain present and optional details SHALL be omitted

#### Scenario: Native warning strings resemble private identifiers
- **WHEN** a valid warning contains a backend model name, URL or token-looking application string not sourced from a credential-bearing structure
- **THEN** its registered value SHALL survive unchanged without enabling operator payload capture

#### Scenario: Warning cannot be represented safely
- **WHEN** the start contains an unknown warning type, invalid UTF-8, excessive warning count, aggregate string bytes or escaping-expanded frame size
- **THEN** no partial provider start SHALL be written and the handler SHALL emit one empty start and at most one safe terminal error when writable

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

### Requirement: Native response metadata and text block state

After public start and within the provider-part limit, the state machine SHALL accept at most one response-metadata part before payload output, sequential text blocks, independent tool events under gateway-streaming-function-tools, supported reasoning/sources under their capabilities, non-terminal provider errors and exactly one finish. Response metadata SHALL copy representable native ResponseID, ModelID and Timestamp at registered id, modelId and timestamp fields. Nonempty native strings SHALL remain unchanged; optional empty Go strings and zero timestamps SHALL be omitted as documented unrepresentable-presence adaptations. A nonzero timestamp SHALL encode a validated UTC RFC3339Nano instant. A provider metadata part with no identity SHALL contain only its type. The runtime SHALL NOT synthesize missing metadata or substitute requested/canonical route identity. Provider identity, headers and opaque metadata remain outside this scoped identity change.

Canonical identity SHALL remain authoritative for resolution and operator logical metrics, separately from returned identity. Each text-start ID SHALL remain nonempty, valid UTF-8 and globally unique within its stream, opening the only active text block. Deltas/ends SHALL use its active ID and required empty deltas SHALL survive. Provider errors SHALL not change text/metadata state. Existing reasoning/source/tool first-output placement rules SHALL remain effective. New native identity strings and timestamp bytes SHALL participate in preflight and complete-frame bounds before any write.

#### Scenario: Native metadata precedes output
- **WHEN** a provider emits native response identity different from requested alias and canonical route before payload output
- **THEN** the event SHALL preserve the native values without changing route resolution or canonical operator metrics

#### Scenario: Metadata has partial or no native identity
- **WHEN** the provider emits partial identity or an identity-free metadata part
- **THEN** only supplied representable fields SHALL appear, with no canonical fallback
- **AND** no metadata part SHALL be manufactured for a provider that emitted none

#### Scenario: Native model ID is not a public route
- **WHEN** modelId contains valid native Unicode, spaces or punctuation outside public route syntax
- **THEN** the native modelId SHALL remain unchanged within the complete-frame budget

Each text-start, text-delta and text-end SHALL preserve supported opaque providerMetadata at its original event position under gateway-provider-metadata, including absent versus explicit empty presence. Content metadata SHALL NOT be moved into response-metadata.

#### Scenario: Sequential text blocks are valid
- **WHEN** a provider emits multiple non-overlapping text start/delta/end blocks with unique IDs
- **THEN** all events SHALL be emitted in order and empty delta strings SHALL remain present

#### Scenario: Text lifecycle is invalid
- **WHEN** a text ID is empty, invalid UTF-8, reused, mismatched, opened while another text block is active, or ended without an active matching block
- **THEN** the handler SHALL cancel provider work and attempt at most one synthetic terminal internal error

#### Scenario: Metadata placement is invalid
- **WHEN** metadata is duplicated or arrives after text, tool, reasoning or source output starts
- **THEN** it SHALL not be forwarded and the existing safe terminal behavior SHALL apply

#### Scenario: Metadata representation exceeds bounds
- **WHEN** native identity contains invalid UTF-8, an invalid timestamp or yields an over-limit complete frame
- **THEN** no metadata bytes SHALL be written and at most one safe terminal error SHALL be attempted

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

### Requirement: Bounded raw-usage finish representation
The handler SHALL count supplied raw-usage bytes, including whitespace, before JSON validation or encoding. It SHALL reject raw usage exceeding the smaller of 1,048,576 bytes and configured `StreamFrameBytes`, validate one complete JSON object when present, and reject invalid UTF-8 in any raw string key or nested value on the original bytes. Valid JSON escapes, including lone and paired UTF-16 surrogate escapes, SHALL be preserved in the raw object. It SHALL only write a finish after the entire `data: <json>\n\n` frame fits `StreamFrameBytes`. Existing `StreamParts` and complete-frame limits SHALL continue to bound aggregate represented stream output; validation and encoding SHALL use memory bounded by a constant multiple of the configured frame limit.

#### Scenario: Input fits but SSE framing does not
- **WHEN** raw usage meets its input limit but JSON encoding or SSE framing makes the finish exceed `StreamFrameBytes`
- **THEN** no finish bytes SHALL be committed and the handler SHALL attempt at most one fixed bounded terminal error frame

#### Scenario: Raw UTF-8 and JSON escapes at finish
- **WHEN** in-limit finish raw usage contains invalid UTF-8 in an object key or nested value
- **THEN** the handler SHALL not encode or write finish and SHALL attempt at most one fixed bounded terminal error frame
- **WHEN** in-limit raw usage contains valid JSON with lone or paired escaped surrogates
- **THEN** the handler SHALL preserve the raw JSON escapes, with success still subject to object and complete-frame limits

#### Scenario: Oversized input never reaches parser
- **WHEN** raw usage has one byte more than its input limit, including JSON whitespace
- **THEN** the handler SHALL reject it before parsing or encoding raw JSON

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

Before valid finish, unsupported families, lifecycle violations, invalid output, premature close, mapping or framing failure, timeout and cancellation SHALL retain existing bounded terminal error behavior. Reasoning start/delta/end and atomic reasoning-file events SHALL be supported under gateway-reasoning-content, and URL/document sources SHALL be supported under gateway-sources. Raw parts SHALL follow gateway-raw-output. Ordinary generated files, custom output, approvals and unsupported tool behavior SHALL remain unsupported. Failure SHALL not synthesize block closures or a finish, nor emit after terminal authority or writer failure.

#### Scenario: Reasoning lifecycle violation
- **WHEN** reasoning ends without a matching active start, reuses an ended reasoning ID, or remains open at finish
- **THEN** the handler SHALL produce at most one safe terminal error and no synthetic finish

#### Scenario: Unsupported stream family appears
- **WHEN** the text runtime receives provider-executed/dynamic/preliminary tool behavior, ordinary generated file, custom, approval, or another unsupported part
- **THEN** it SHALL emit at most one terminal internal error rather than serializing the provider-domain part

#### Scenario: Concurrent reasoning
- **WHEN** two reasoning blocks and a text block sharing one raw ID interleave
- **THEN** family-specific active state SHALL preserve each block independently and IDs SHALL not be rewritten

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

### Requirement: Cancellation and timeouts
The configured total timeout SHALL begin immediately before `DoStream` invocation and cover setup and consumption. After stream commitment, the configured idle timeout SHALL restart after each accepted provider part is successfully represented, including a consumed start and a written provider error. Request cancellation, total timeout, or idle timeout SHALL cancel provider work before the corresponding safe terminal response or event is written. A valid finish written before another terminal outcome SHALL remain authoritative. When provider output, cancellation, and timeout become ready concurrently before terminal output, any applicable safe bounded outcome MAY win; the protocol does not define scheduler-level precedence. Writer failure SHALL terminate immediately without another write.

#### Scenario: Terminal conditions race
- **WHEN** provider output, request cancellation, or a configured timeout become ready concurrently before terminal output
- **THEN** the handler SHALL produce at most one applicable bounded terminal outcome and SHALL not leak or clean up the stream twice

#### Scenario: Total timeout cancels provider before output
- **WHEN** the total timeout expires during setup or stream consumption
- **THEN** the model context SHALL be canceled before the timeout response or event is encoded or written

#### Scenario: Idle timeout cancels provider before output
- **WHEN** the idle timeout expires after stream establishment
- **THEN** the model context SHALL be canceled before the idle-timeout event is encoded or written

#### Scenario: Provider finish arrives before termination
- **WHEN** a valid finish is written before cancellation or timeout terminates the stream
- **THEN** finish SHALL be the authoritative final event

#### Scenario: Accepted activity resets idle time
- **WHEN** accepted provider parts arrive within each idle interval while total duration remains available
- **THEN** the idle timeout SHALL restart after each represented part and SHALL not expire solely because total stream age exceeds one idle interval

### Requirement: Bounded stream ownership and drain
Every returned stream SHALL have one receiver responsible for cleanup. A stream transferred from setup belongs to the handler; when model-context termination wins before transfer, the setup worker SHALL clean up any stream returned later. Terminal handling SHALL cancel provider work and start an asynchronous drain that reads only until channel close, the first part above the request's `StreamParts` count, or the configured drain duration. Draining SHALL not extend handler latency and SHALL remain bounded even when the provider channel is continuously ready. Providers SHALL remain responsible for observing cancellation and closing their channels; a provider call that never returns or a producer that remains blocked after bounded drain SHALL be classified as a provider lifecycle defect rather than a bounded Gateway resource.

#### Scenario: Provider closes after cancellation
- **WHEN** terminal handling cancels provider work and the provider closes its stream within the drain duration
- **THEN** the bounded drain SHALL consume the remaining channel values and exit

#### Scenario: Provider ignores cancellation
- **WHEN** a provider channel does not close before the drain duration
- **THEN** handler latency and the handler-owned drain lifetime SHALL remain bounded even though provider-owned work may remain defective

#### Scenario: Continuously ready drain reaches a hard bound
- **WHEN** a canceled provider channel remains permanently ready with values
- **THEN** the drain SHALL stop within the configured duration or at the first excess provider part rather than consuming indefinitely

#### Scenario: Setup returns a late stream
- **WHEN** handler cancellation or timeout wins and `DoStream` later returns a non-nil stream
- **THEN** the late setup owner SHALL cancel and bounded-drain that channel exactly once instead of abandoning it unread

### Requirement: Streaming contract and cross-language evidence
Automated evidence SHALL prove that committed streaming requests execute through the production handler, raw HTTP output satisfies the strict lifecycle, privacy, and resource bounds, and the registered Gateway client consumes normal, provider-error, timeout, cancellation, finish, and clean-EOF outcomes. Provider lifecycle and transport-failure evidence SHALL not rely on invented provider conformance input. The parity map SHALL identify the achieved strict streaming text scope and retain explicit gaps for every deferred stream family.

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

### Requirement: Native-value regressions preserve terminal authority

Native warning/source/identity changes SHALL preserve existing stream-start normalization, metadata placement, non-terminal provider errors, idle reset, authoritative finish, cancellation, writer failure and bounded cleanup rules. Raw/schema and exact-pinned TS/independent Go consumption tests SHALL establish these properties without relying on permissive client parsing.

#### Scenario: Sources interleave and finish remains final
- **WHEN** repeated and equal-cross-variant native source IDs interleave with active content and provider errors before finish, followed by later source/metadata parts
- **THEN** accepted values SHALL retain order without closing active blocks, and finish SHALL suppress every later public part

#### Scenario: Cancellation or writer failure occurs
- **WHEN** processing native-value events is canceled or a full-frame write/flush fails
- **THEN** existing cancellation and single-owner bounded cleanup SHALL apply with no second write after writer failure or authoritative terminal output
