# providerwire-v4-streaming-runtime Specification

## Purpose

Define the production strict, bounded ProviderWire V4 streaming text runtime and its compatibility evidence.

## Requirements

### Requirement: Constructed streaming limits
The strict ProviderWire V4 handler SHALL use the stream idle and post-cancellation drain durations of the execution runtime it is constructed from, which gateway-execution-runtime requires to be positive. It SHALL NOT take a total model duration, a provider stream-part count or a complete SSE frame byte limit. Complete frames SHALL consist of the `data: ` prefix, JSON payload, and terminating `\n\n`.

#### Scenario: Valid streaming limits
- **WHEN** a handler is constructed from a runtime with positive idle and drain durations
- **THEN** handler construction SHALL succeed and runtime streaming behavior SHALL be fixed by those values

#### Scenario: Invalid streaming limits
- **WHEN** a streaming duration is zero or negative
- **THEN** runtime construction SHALL fail before any handler serves a request

### Requirement: Shared strict streaming request pipeline
Unary and streaming SHALL share bounded validation/mapping and one authenticated host selection. Supported streaming SHALL invoke DoStream once, never DoGenerate, without repeated selection or catalog access for BYOK; failures before invocation SHALL remain safe non-2xx JSON.

#### Scenario: Streaming request pipeline policy
- **WHEN** a validated request selects unary or streaming execution
- **THEN** The handler SHALL accept `ai-language-model-streaming` only when its single exact value is `true` or `false`.
- **AND** It SHALL route `false` to the existing unary path and `true` to the streaming path only after the same bounded body read, standard Go JSON and complete request-schema validation, supported request mapping and exactly one authenticated host selection.
- **AND** Selection SHALL return a non-nil V4 model and a non-empty valid-UTF-8 logical identity appropriate to its account-access policy.
- **AND** Configured access SHALL resolve the catalog exactly once and retain canonical identity; BYOK SHALL select from request data with zero catalog calls and use the request-scoped observation contract.
- **AND** A supported streaming request SHALL invoke the selected logical model's `DoStream` exactly once and SHALL NOT invoke `DoGenerate`.
- **AND** The selected logical model MAY own bounded credential attempts under gateway-request-byok; these SHALL NOT cause repeated host selection or bypass existing logical SSE commitment and single-owner cleanup.
- **AND** Any failure before stream invocation SHALL select a bounded privacy-safe non-2xx JSON document and SHALL produce no SSE commitment.

#### Scenario: Supported streaming envelope executes once
- **WHEN** a valid configured-access request uses streaming value `true` and passes mapping and selection
- **THEN** host selection, configured resolution and logical `DoStream` SHALL each run once, and `DoGenerate` SHALL not run

#### Scenario: BYOK streaming envelope executes without a catalog
- **WHEN** a valid BYOK request uses streaming value `true` and passes mapping and request-only selection
- **THEN** host selection and logical `DoStream` SHALL each run once, with zero catalog calls and zero `DoGenerate` calls
- **AND** any eligible credential attempts SHALL remain inside the selected logical model's bounded execution

#### Scenario: Streaming request fails before invocation
- **WHEN** a streaming request fails envelope, body, standard JSON, schema, mapping or selection processing
- **THEN** selection or model invocation SHALL not run after an earlier failure and the response SHALL remain a bounded privacy-safe non-2xx JSON error rather than SSE

#### Scenario: Invalid streaming selector
- **WHEN** the streaming header is missing, empty, repeated, or has a value other than exact `true` or `false`
- **THEN** envelope validation SHALL fail before body mapping, selection or model invocation

### Requirement: Streaming invocation and pre-invocation failure mode

A supported streaming request SHALL invoke `DoStream` exactly once and SHALL NOT invoke `DoGenerate`. Any failure before stream invocation SHALL select a fixed non-2xx JSON document and SHALL produce no SSE commitment.

#### Scenario: Streaming invocation and pre-invocation failure mode
- **WHEN** a streaming request passes mapping and resolution or fails before invocation
- **THEN** a supported request SHALL call DoStream once and not DoGenerate; any earlier failure SHALL remain fixed non-2xx JSON without SSE commitment

### Requirement: Single-owner stream setup and logical commitment

The request context's deadline SHALL cover `DoStream` setup and all later stream consumption. Setup SHALL run synchronously through the execution runtime with panic recovery. Every returned stream SHALL have one cleanup owner, the runtime: it SHALL cancel and drain exactly once a stream the handler consumed, a stream returned with an error, and a stream returned after the request context ended.

#### Scenario: Stream setup fails before commitment
- **WHEN** `DoStream` returns an error, panics, returns `nil, nil`, or returns a result with a nil stream channel
- **THEN** the handler SHALL return the corresponding non-2xx safe JSON response and SHALL not commit SSE

#### Scenario: Result and error include a stream
- **WHEN** `DoStream` returns both an error and a non-nil stream channel
- **THEN** the outcome SHALL remain pre-commit, the runtime SHALL cancel and start asynchronous bounded drain exactly once, and the handler SHALL return non-2xx JSON without waiting for drain duration

#### Scenario: Setup completion races termination
- **WHEN** `DoStream` returns a valid stream after the request context was canceled or its deadline expired
- **THEN** the handler SHALL return the pre-commit cancellation or timeout JSON failure, and the runtime SHALL cancel and drain the stream exactly once

#### Scenario: Non-nil stream commits streaming mode
- **WHEN** the handler receives a non-nil result and stream channel without error while the request context is live
- **THEN** the response mode SHALL irrevocably become HTTP 200 SSE before the first provider part is interpreted, and the runtime SHALL own cleanup

#### Scenario: Initial provider part never arrives
- **WHEN** setup establishes a stream but no first provider part arrives before cancellation or timeout
- **THEN** the handler SHALL remain committed to SSE and SHALL attempt one empty start followed by one corresponding terminal error frame when the writer remains usable

### Requirement: Stream setup validity and irrevocable commitment

A setup error, panic, `nil, nil` return, nil result, nil stream channel, or result-plus-error SHALL remain a bounded non-2xx safe JSON failure before commitment. Every non-nil invalid or late channel SHALL be canceled and asynchronously bounded-drained without delaying the response. A valid received result with a non-nil stream channel SHALL be the irrevocable logical SSE commitment boundary.

#### Scenario: Invalid received stream drains without delaying JSON
- **WHEN** DoStream returns both an error and a non-nil channel
- **THEN** the invalid channel SHALL be canceled and asynchronously bounded-drained without delaying the safe JSON response

### Requirement: Post-commit SSE failure and private setup metadata

After the valid-result logical SSE commitment boundary every failure SHALL remain in SSE when the writer is usable; provider request metadata, response headers, and other `StreamResult` metadata SHALL not cross the public boundary.

#### Scenario: Post-commit SSE failure and private setup metadata
- **WHEN** a committed stream fails while its writer is usable and StreamResult has native headers
- **THEN** failure SHALL remain SSE and private setup metadata/headers SHALL NOT cross the public boundary

### Requirement: Normalized stream start and value-safe warnings

Every committed writable stream SHALL emit exactly one public stream-start as its first JSON event. The handler SHALL read the first provider part before selecting it: a provider stream-start is valid only as the first provider part and SHALL be consumed with its warnings mapped through the same explicit registered warning union used by unary output.

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
- **WHEN** the start contains an unknown warning type or invalid UTF-8
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

### Requirement: Stream-start warning union active fields and presence

Unsupported/compatibility SHALL preserve required feature and optional nonempty details; deprecated SHALL preserve required setting/message; other SHALL preserve required message. Required strings SHALL remain present even when empty, order/multiplicity SHALL survive and inactive fields SHALL be omitted. Optional empty Go details SHALL normalize to omission as a documented representation adaptation, not a privacy policy.

#### Scenario: Stream-start warning union active fields and presence
- **WHEN** start warnings include unsupported, compatibility, deprecated and other variants with required empty strings
- **THEN** active fields/order/multiplicity SHALL survive, inactive fields SHALL be omitted and optional empty details SHALL normalize to omission

### Requirement: Uncensored warning values and invalid-start fallback

Native backend names and ordinary application strings SHALL NOT be replaced with generic prose or heuristically censored. Unknown warning discriminators or invalid represented strings SHALL cause one empty public start followed by at most one synthetic terminal internal error when writable.

#### Scenario: Uncensored warning values and invalid-start fallback
- **WHEN** a valid warning includes a backend-looking string, or a start has an unknown warning type
- **THEN** the valid string SHALL remain unchanged and the invalid start SHALL yield one empty start and at most one safe terminal internal error

### Requirement: Duplicate and late provider-start refusal

Duplicate/late starts SHALL remain invalid.

#### Scenario: Duplicate and late provider-start refusal
- **WHEN** a second provider stream-start appears after text
- **THEN** it SHALL remain invalid without producing another public start

### Requirement: Native response metadata and text block state
Streaming SHALL preserve represented native identity and sequential text-block state, validate required/optional fields before output and retain terminal/commitment authority. Route identity SHALL not fabricate native fields or alter independent configured/BYOK observation.

#### Scenario: Native response metadata and text block state policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** After public start, the state machine SHALL accept at most one response-metadata part before payload output, sequential text blocks, independent tool events under gateway-streaming-function-tools, supported reasoning/sources/custom under their capabilities, non-terminal provider errors and exactly one finish. Response metadata SHALL copy representable native ResponseID, ModelID and Timestamp at registered id, modelId and timestamp fields. Nonempty native strings SHALL remain unchanged; optional empty Go strings and zero timestamps SHALL be omitted as documented unrepresentable-presence adaptations. A nonzero timestamp SHALL encode a validated UTC RFC3339Nano instant. A provider metadata part with no identity SHALL contain only its type. The runtime SHALL NOT synthesize missing metadata or substitute requested/canonical route identity. Provider identity, headers and opaque metadata remain outside this scoped identity change.
- **AND** Configured canonical identity SHALL remain authoritative for configured resolution and operator logical metrics, separately from returned identity. BYOK selection and observation SHALL instead follow the request-scoped identity and bounded-cardinality rules in gateway-text-observability, without catalog lookup or response-derived logical identity. Each text-start ID SHALL remain nonempty, valid UTF-8 and globally unique within its stream, opening the only active text block. Deltas/ends SHALL use its active ID and required empty deltas SHALL survive. Provider errors SHALL not change text/metadata state. Existing reasoning/source/tool first-output placement rules SHALL remain effective. New native identity strings and timestamps SHALL be validated before any write.

#### Scenario: Native metadata precedes output
- **WHEN** a configured provider emits native response identity different from requested alias and canonical route before payload output
- **THEN** the event SHALL preserve the native values without changing route resolution or canonical operator metrics

#### Scenario: BYOK native identity differs from request identity
- **WHEN** a BYOK provider emits native response identity different from the requested provider/model
- **THEN** the event SHALL preserve representable native values while logical observation retains its request-scoped identity and fixed model metric class
- **AND** neither identity SHALL trigger catalog lookup

#### Scenario: Metadata has partial or no native identity
- **WHEN** the provider emits partial identity or an identity-free metadata part
- **THEN** only supplied representable fields SHALL appear, with no canonical fallback
- **AND** no metadata part SHALL be manufactured for a provider that emitted none

#### Scenario: Native model ID is not a public route
- **WHEN** modelId contains valid native Unicode, spaces or punctuation outside public route syntax
- **THEN** the native modelId SHALL remain unchanged

Each text-start, text-delta and text-end SHALL preserve supported opaque providerMetadata at its original event position under gateway-provider-metadata, including absent versus explicit empty presence. Content metadata SHALL NOT be moved into response-metadata.

#### Scenario: Sequential text blocks are valid
- **WHEN** a provider emits multiple non-overlapping text start/delta/end blocks with unique IDs
- **THEN** all events SHALL be emitted in order and empty delta strings SHALL remain present

#### Scenario: Text lifecycle is invalid
- **WHEN** a text ID is empty, invalid UTF-8, reused, mismatched, opened while another text block is active, or ended without an active matching block
- **THEN** the handler SHALL cancel provider work and attempt at most one synthetic terminal internal error

#### Scenario: Metadata placement is invalid
- **WHEN** metadata is duplicated or arrives after text, tool, reasoning, source or custom output starts
- **THEN** it SHALL not be forwarded and the existing safe terminal behavior SHALL apply

#### Scenario: Metadata representation exceeds bounds
- **WHEN** native identity contains invalid UTF-8 or an invalid timestamp
- **THEN** no metadata bytes SHALL be written and at most one safe terminal error SHALL be attempted
- **WHEN** valid native identity strings encode to a frame above the former 1 MiB limit
- **THEN** the complete metadata event SHALL be written

#### Scenario: Text metadata changes at end
- **WHEN** text-start supplies an object, text-delta omits metadata and text-end supplies an empty or replacement object
- **THEN** those exact metadata positions and presence states SHALL reach both clients in order, with no deep merge or relocation

### Requirement: Native stream identity representation and omitted presence

Nonempty native strings SHALL remain unchanged; optional empty Go strings and zero timestamps SHALL be omitted as documented unrepresentable-presence adaptations. A nonzero timestamp SHALL encode a validated UTC RFC3339Nano instant. A provider metadata part with no identity SHALL contain only its type. The runtime SHALL NOT synthesize missing metadata or substitute requested/canonical route identity. Provider identity, headers and opaque metadata remain outside this scoped identity change.

#### Scenario: Native stream identity representation and omitted presence
- **WHEN** provider metadata has empty optional strings, zero timestamp or no identity
- **THEN** only representable native fields SHALL appear, without fabricated metadata, route substitution or added transport/provider identity

### Requirement: Canonical telemetry and sequential text lifecycle

Canonical identity SHALL remain authoritative for resolution and operator logical metrics, separately from returned identity. Each text-start ID SHALL remain nonempty, valid UTF-8 and globally unique within its stream, opening the only active text block. Deltas/ends SHALL use its active ID and required empty deltas SHALL survive. Provider errors SHALL not change text/metadata state. Existing reasoning/source/tool first-output placement rules SHALL remain effective.

#### Scenario: Canonical telemetry and sequential text lifecycle
- **WHEN** a provider error interleaves with a text block whose ID differs from native model identity
- **THEN** canonical telemetry SHALL stay separate, the active nonempty UTF-8 text ID SHALL remain unique, and errors SHALL NOT change text/metadata state

### Requirement: Finish validation and terminal authority

A finish part SHALL be valid only when no text or tool-input block is active, its warnings are empty, and it contains a non-nil registered finish reason and non-nil usage.

#### Scenario: Finish closes a valid stream
- **WHEN** a valid finish is emitted with no active text or tool-input block
- **THEN** the finish SHALL preserve normalized usage, finish reason and supplied ordinary providerMetadata, unrelated transport fields SHALL remain omitted, and the response SHALL end at clean EOF without `[DONE]`

#### Scenario: Finish is invalid
- **WHEN** finish arrives with an active block, warnings, nil or invalid usage, nil or invalid finish reason, unsafe token counts, invalid supplied raw usage, or invalid finish metadata
- **THEN** finish SHALL not be written and the handler SHALL attempt at most one synthetic terminal internal error

#### Scenario: Provider emits after finish
- **WHEN** any provider part is available after a valid finish has been written
- **THEN** finish SHALL remain the final public event and the later part SHALL be suppressed while cancellation and bounded drain complete

#### Scenario: Native raw object survives finish
- **WHEN** a valid finish contains normalized usage and a supplied raw JSON object of any size, including nested provider-native keys or `{}`
- **THEN** its complete `usage.raw` object SHALL be emitted with the normalized counts, and a distinct `type: "raw"` stream part SHALL NOT be synthesized

#### Scenario: No raw value is supplied
- **WHEN** a valid finish has absent provider `Usage.Raw`
- **THEN** the finish SHALL omit `usage.raw` and retain its normalized counts

#### Scenario: Raw usage fails after stream commitment
- **WHEN** finish usage supplies malformed, null, scalar, array or invalid-UTF-8 raw JSON
- **THEN** the finish SHALL NOT be written and the handler SHALL attempt at most one fixed complete terminal internal-error SSE frame, without reflecting the offending value or issuing normalized-only finish

#### Scenario: Finish metadata is ordinary output
- **WHEN** valid finish metadata contains unknown namespaces or an explicit empty object and IncludeRawChunks is false
- **THEN** both clients SHALL receive that metadata on finish before clean EOF without any synthesized raw event

### Requirement: Finish usage metadata and final public authority

Known usage counts SHALL be non-negative JavaScript-safe integers and SHALL use the registered input/output groups; `usage.raw` SHALL contain a supplied valid provider JSON object, or be omitted when Raw is absent; finish providerMetadata SHALL be preserved as opaque object-valued namespaces under gateway-provider-metadata. A valid finish SHALL be written once as the final public event and SHALL be authoritative.

#### Scenario: Finish usage metadata and final public authority
- **WHEN** a valid finish contains normalized counts, a raw object and opaque metadata
- **THEN** registered usage groups/safe counts and raw/metadata presence SHALL survive, with finish written once as final authority

### Requirement: Immediate EOF and post-finish provider suppression

The handler SHALL then cancel provider work, begin bounded asynchronous drain, and return clean EOF immediately without waiting for provider channel closure or emitting `[DONE]`. Provider parts observed after a written finish SHALL be suppressed during drain, treated as provider lifecycle defects for later operational reporting, and SHALL never produce a second public terminal event.

#### Scenario: Immediate EOF and post-finish provider suppression
- **WHEN** a provider emits another part after valid finish without closing its channel
- **THEN** the handler SHALL cancel, asynchronously bounded-drain and return clean EOF immediately without DONE or another public event, classifying later parts as lifecycle defects

### Requirement: Ordered non-terminal provider errors

Each pre-finish provider `PartError` SHALL be independently reduced through the closed safe-error classification and emitted in place with base fields `{"type":"error","error":{"message":string,"type":string,"param":null,"code":string,"statusCode":integer,"retryable":boolean}}`.

#### Scenario: Ordered non-terminal provider errors policy
- **WHEN** the runtime encodes the corresponding response or stream event
- **THEN** Optional `error.data` MAY carry native current-error and execution summaries under gateway-attempt-failure-evidence. A valid provider error SHALL not terminate the stream or alter lifecycle state; later metadata, content, additional provider errors, and finish SHALL remain valid.

#### Scenario: Provider error is followed by content
- **WHEN** a provider emits an error before or within a text block and later emits otherwise valid content and finish
- **THEN** the client SHALL receive the safe error and every later event in provider order

#### Scenario: Multiple provider errors are ordered
- **WHEN** a provider emits multiple errors separated by valid stream parts
- **THEN** each error SHALL be independently normalized and retained in its original position without terminating the stream

#### Scenario: Provider error contains hostile detail
- **WHEN** a provider error contains credentials, URLs, bodies, headers, data, causes, backend identity, or arbitrary messages
- **THEN** its base public fields SHALL retain the approved category message, while optional error data SHALL contain only the governed native summary/overview rather than a native error dump

#### Scenario: Provider error status is malformed
- **WHEN** an `APICallError` carries a non-zero status outside the valid HTTP range 100 through 599
- **THEN** it SHALL reduce to the canonical internal error without inspecting a wrapped transport cause

#### Scenario: Provider error has no HTTP status
- **WHEN** an `APICallError` has status zero and wraps a timeout-capable transport error
- **THEN** it SHALL reduce to the canonical timeout error
- **AND** a status-zero `APICallError` with an ordinary network or DNS cause SHALL reduce to the canonical upstream error
- **AND** a valid status-bearing provider error, including an SSE error reported with HTTP status 200, SHALL retain status-based classification

### Requirement: Provider error canonical fallback and privacy exclusions

Nil, malformed, or unclassifiable provider error values SHALL reduce to the canonical internal safe error part. Arbitrary native bodies, headers, causes and metadata SHALL NOT enter the public error event. Native message/type/code/status summaries and observed configured identity MAY appear through governed optional error data. Summary protection SHALL follow gateway-attempt-failure-evidence.

#### Scenario: Provider error canonical fallback and privacy exclusions
- **WHEN** a provider PartError is nil or carries arbitrary backend details
- **THEN** nil/unclassifiable values SHALL reduce to the internal safe error; only governed native current-error and execution summaries MAY accompany the base fields, with no arbitrary native transport data
- **AND** summary strings SHALL redact exact configured API-key matches; other scalar echoes, account authorization and request isolation SHALL remain intact

### Requirement: Synthetic terminal adapter errors

Before valid finish, unsupported families, lifecycle violations, invalid output, premature close, mapping or framing failure, timeout and cancellation SHALL retain existing bounded terminal error behavior. Reasoning start/delta/end and atomic reasoning-file events SHALL be supported under gateway-reasoning-content, URL/document sources SHALL be supported under gateway-sources, and atomic custom events SHALL be supported under gateway-custom-content. Raw parts SHALL follow gateway-raw-output. Ordinary generated files, approvals and unsupported tool behavior SHALL remain unsupported.

#### Scenario: Reasoning lifecycle violation
- **WHEN** reasoning ends without a matching active start, reuses an ended reasoning ID, or remains open at finish
- **THEN** the handler SHALL produce at most one safe terminal error and no synthetic finish

#### Scenario: Unsupported stream family appears
- **WHEN** the text runtime receives provider-executed/dynamic/preliminary tool behavior, ordinary generated file, approval, or another unsupported part
- **THEN** it SHALL emit at most one terminal internal error rather than serializing the provider-domain part

#### Scenario: Custom stream part is supported
- **WHEN** the runtime receives a well-formed custom part between stream start and finish
- **THEN** it SHALL emit the custom event and continue the stream rather than emitting a terminal error

#### Scenario: Concurrent reasoning
- **WHEN** two reasoning blocks and a text block sharing one raw ID interleave
- **THEN** family-specific active state SHALL preserve each block independently and IDs SHALL not be rewritten

### Requirement: Adapter failure does not manufacture lifecycle completion

Failure SHALL not synthesize block closures or a finish, nor emit after terminal authority or writer failure.

#### Scenario: Adapter failure does not manufacture lifecycle completion
- **WHEN** an unsupported part arrives while a text block is open
- **THEN** failure SHALL NOT synthesize text-end or finish, nor write after terminal authority or writer failure

### Requirement: Complete bounded SSE framing and flushing

Committed responses SHALL use HTTP 200, `Content-Type: text/event-stream`, and `Cache-Control: no-cache, no-transform`; connection-specific keep-alive headers SHALL not be protocol requirements. The handler SHALL flush commitment headers and every fully written event. Unsupported flushing SHALL not fail the stream; every other header or frame flush error or panic SHALL be a writer failure.

#### Scenario: Event is written and flushed
- **WHEN** a valid event is completely encoded and the writer supports flushing
- **THEN** exactly one complete `data:` frame SHALL be fully written and flushed before the next event

#### Scenario: Flushing is unsupported
- **WHEN** the response writer does not support flushing
- **THEN** the stream SHALL continue without treating that limitation as a failure

#### Scenario: Commitment header flush fails
- **WHEN** flushing commitment headers fails or panics
- **THEN** provider work SHALL be canceled and the handler SHALL end without attempting a stream frame

#### Scenario: Event exceeds its complete-frame limit
- **WHEN** a valid encoded event exceeds the former 1 MiB complete-frame limit
- **THEN** the whole frame SHALL be written and flushed, and the stream SHALL continue

#### Scenario: Writer fails
- **WHEN** writing or flushing an event fails, writes short, or panics
- **THEN** provider work SHALL be canceled and no synthetic event or second write SHALL be attempted on that writer

#### Scenario: Invalid metadata cannot become partial event
- **WHEN** registered metadata is malformed JSON, invalid UTF-8, or non-object at a namespace
- **THEN** no bytes of that event SHALL be written, existing terminal cancellation/cleanup SHALL apply and no selected fallback candidate SHALL be replayed

### Requirement: Complete SSE writes fixed terminal frames and writer failure

Each frame SHALL be completely encoded before any of its bytes are written.

#### Scenario: Complete SSE writes fixed terminal frames and writer failure policy
- **WHEN** the runtime encodes the corresponding response or stream event
- **THEN** Synthetic terminal errors SHALL retain their fixed base fields; optional execution attribution SHALL follow gateway-attempt-failure-evidence and SHALL fall back to the canonical base frame only when the enriched frame cannot be encoded. The server SHALL never emit SSE `event:` fields or `[DONE]`. A write error, short write, writer panic, or supported flush failure SHALL cancel provider work and end immediately without another write.

#### Scenario: Complete SSE writes fixed terminal frames and writer failure
- **WHEN** a full-frame write is short or flushing panics
- **THEN** provider work SHALL be canceled with no subsequent write, while complete-frame encoding before write and the prohibition of event fields/DONE SHALL remain effective

### Requirement: Cancellation and timeouts
The request context's deadline SHALL cover selection, setup, attempts and streaming without resets, and the handler SHALL add no total deadline of its own. Idle timeout SHALL follow represented provider parts; cancellation and timeout SHALL retain terminal authority, bounded cleanup and writer-failure behavior.

#### Scenario: Streaming deadline and terminal policy
- **WHEN** selection or streaming encounters cancellation, timeout or competing terminal conditions
- **THEN** The request context's deadline SHALL cover selection/construction, logical `DoStream` setup, all credential attempts and stream consumption.
- **AND** Logical invocation, credential attempts and stream establishment SHALL NOT reset or extend the request deadline.
- **AND** Expiry of the request context's deadline SHALL be a timeout, and cancellation of the request context SHALL be a cancellation.
- **AND** Selection and setup SHALL run synchronously under the request context; a selection or stream returned after it ended SHALL be discarded under gateway-execution-runtime.
- **AND** After stream commitment, the configured idle timeout SHALL restart after each accepted provider part is successfully represented, including a consumed start and a written provider error.
- **AND** Request cancellation, request deadline expiry, or idle timeout SHALL cancel selection/provider work before the corresponding safe terminal response or event is written.
- **AND** A valid finish written before another terminal outcome SHALL remain authoritative.
- **AND** When provider output, cancellation, and timeout become ready concurrently before terminal output, any applicable safe bounded outcome MAY win; the protocol does not define scheduler-level precedence.
- **AND** Writer failure SHALL terminate immediately without another write.

#### Scenario: Terminal conditions race
- **WHEN** provider output, request cancellation, the request deadline or the idle timeout become ready concurrently before terminal output
- **THEN** the handler SHALL produce at most one applicable bounded terminal outcome and SHALL not leak or clean up the stream twice

#### Scenario: Selection exhausts streaming time
- **WHEN** selection returns only after the request deadline expired
- **THEN** the handler SHALL return a pre-commit timeout JSON failure with zero logical model invocations

#### Scenario: Attempts inherit the remaining budget
- **WHEN** selection consumes part of the time before the request deadline and the chosen model then tries multiple eligible credentials
- **THEN** every attempt and subsequent stream consumption SHALL use only the remaining time without resetting it

#### Scenario: Total timeout cancels provider before output
- **WHEN** the request deadline expires during setup or stream consumption
- **THEN** the model context SHALL be canceled before the timeout response or event is encoded or written, and that response or event SHALL use the timeout category

#### Scenario: Idle timeout cancels provider before output
- **WHEN** the idle timeout expires after stream establishment
- **THEN** the model context SHALL be canceled before the idle-timeout event is encoded or written

#### Scenario: Provider finish arrives before termination
- **WHEN** a valid finish is written before cancellation or timeout terminates the stream
- **THEN** finish SHALL be the authoritative final event

#### Scenario: Accepted activity resets idle time
- **WHEN** accepted provider parts arrive within each idle interval before the request deadline
- **THEN** the idle timeout SHALL restart after each represented part and SHALL not expire solely because total stream age exceeds one idle interval

#### Scenario: Stream without a request deadline
- **WHEN** the request context carries no deadline and the provider keeps emitting parts within the idle duration
- **THEN** the stream SHALL continue until finish, caller cancellation or an idle timeout

### Requirement: Streaming terminal authority and scheduler race boundary

A valid finish written before another terminal outcome SHALL remain authoritative. When provider output, cancellation, and timeout become ready concurrently before terminal output, any applicable safe bounded outcome MAY win; the protocol does not define scheduler-level precedence. Writer failure SHALL terminate immediately without another write.

#### Scenario: Streaming terminal authority and scheduler race boundary
- **WHEN** provider output and timeout become ready concurrently before terminal output
- **THEN** any applicable safe bounded outcome MAY win without defined scheduler precedence; a previously written valid finish SHALL remain authoritative and writer failure SHALL prohibit another write

### Requirement: Bounded stream ownership and drain

Every returned stream SHALL have one cleanup owner, the execution runtime. Terminal handling SHALL cancel provider work and start one asynchronous drain that reads only until channel close or the configured drain duration.

#### Scenario: Provider closes after cancellation
- **WHEN** terminal handling cancels provider work and the provider closes its stream within the drain duration
- **THEN** the bounded drain SHALL consume the remaining channel values and exit

#### Scenario: Provider ignores cancellation
- **WHEN** a provider channel does not close before the drain duration
- **THEN** handler latency and the drain lifetime SHALL remain bounded even though provider-owned work may remain defective

#### Scenario: Continuously ready drain reaches a hard bound
- **WHEN** a canceled provider channel remains permanently ready with values
- **THEN** the drain SHALL stop within the configured duration rather than consuming indefinitely

#### Scenario: Setup returns a late stream
- **WHEN** `DoStream` returns a non-nil stream after the request context ended
- **THEN** the runtime SHALL cancel and bounded-drain that channel exactly once instead of abandoning it unread

### Requirement: Bounded drain latency and provider lifecycle responsibility

Draining SHALL not extend handler latency and SHALL remain bounded even when the provider channel is continuously ready. Providers SHALL remain responsible for observing cancellation, returning from setup and closing their channels; a provider call that never returns holds its own request, and a producer that remains blocked after bounded drain SHALL be classified as a provider lifecycle defect rather than a bounded Gateway resource.

#### Scenario: Bounded drain latency and provider lifecycle responsibility
- **WHEN** a canceled provider continuously produces parts or never returns
- **THEN** Gateway drain latency SHALL remain bounded, a setup call that never returns SHALL hold only its own request, and non-cooperative provider-owned work SHALL be classified as a lifecycle defect

### Requirement: Streaming contract and cross-language evidence

Automated evidence SHALL prove that committed streaming requests execute through the production handler, raw HTTP output satisfies the strict lifecycle and privacy rules, large outputs are written whole, and the registered Gateway client consumes normal, provider-error, timeout, cancellation, finish, and clean-EOF outcomes. Provider lifecycle and transport-failure evidence SHALL not rely on invented provider conformance input.

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
- **THEN** local DTO schemas, state-machine tests, raw HTTP assertions, privacy tests, and large-output tests SHALL remain authoritative for server correctness

### Requirement: Streaming parity-map scope and deferred-family gaps

The parity map SHALL identify the achieved strict streaming text scope and retain explicit gaps for every deferred stream family.

#### Scenario: Streaming parity-map scope and deferred-family gaps
- **WHEN** strict streaming text evidence is recorded in the parity map
- **THEN** the achieved scope SHALL be named and every deferred stream-family gap SHALL remain explicit

### Requirement: Native-value regressions preserve terminal authority

Native warning/source/identity changes SHALL preserve existing stream-start normalization, metadata placement, non-terminal provider errors, idle reset, authoritative finish, cancellation, writer failure and bounded cleanup rules. Raw/schema and exact-pinned TS/independent Go consumption tests SHALL establish these properties without relying on permissive client parsing.

#### Scenario: Sources interleave and finish remains final
- **WHEN** repeated and equal-cross-variant native source IDs interleave with active content and provider errors before finish, followed by later source/metadata parts
- **THEN** accepted values SHALL retain order without closing active blocks, and finish SHALL suppress every later public part

#### Scenario: Cancellation or writer failure occurs
- **WHEN** processing native-value events is canceled or a full-frame write/flush fails
- **THEN** existing cancellation and single-owner bounded cleanup SHALL apply with no second write after writer failure or authoritative terminal output

### Requirement: Optional streaming execution overview and current failures

Streaming SHALL use the existing reader/commitment/drain ownership to optionally enrich finish metadata and classified error payloads.

#### Scenario: Optional streaming execution overview and current failures policy
- **WHEN** the runtime encodes the corresponding response or stream event
- **THEN** First-part selection SHALL NOT claim completion. Current native summaries SHALL remain event-local and eligible without catalog provenance; finish SHALL NOT duplicate them. Original metadata validation, error ordering, active-block state, authoritative finish and writer failure behavior SHALL remain unchanged. The enriched finish or error frame SHALL be encoded once; an error frame SHALL fall back to its fixed base frame only when that encoding fails, and a native `gateway` namespace SHALL always be relocated under the overview.

#### Scenario: Selected leading error followed by content
- **WHEN** the first provider part is an error followed by valid text and finish
- **THEN** no later candidate SHALL run, both clients SHALL consume all parts in order and finish SHALL contain only the optional compact execution overview rather than stream-error history

#### Scenario: Finish fits only without overview
- **WHEN** a valid finish would fit the former 1 MiB frame limit only without its overview
- **THEN** the enriched finish SHALL be written whole and remain authoritative, without a new terminal error or read-ahead

### Requirement: Stream output validity without size accounting
Before writing an event, the runtime SHALL validate the part's represented strings as UTF-8, its discriminators, identity timestamp, metadata namespaces, raw usage shape, token ranges and lifecycle state. It SHALL NOT count provider parts, preflight sizes or cardinality, or limit complete frame bytes; time and size are owned by gateway-resource-policy.

#### Scenario: Many provider parts
- **WHEN** a provider emits more than 100,000 valid parts before a valid finish
- **THEN** every part SHALL be represented in order and the finish SHALL be written

#### Scenario: Large frames
- **WHEN** a valid text delta, tool input, tool result, source, reasoning file, custom part, raw event or finish encodes to a complete frame above 1 MiB
- **THEN** that complete frame SHALL be written and the stream SHALL continue

#### Scenario: Many start warnings
- **WHEN** the provider start carries more valid warnings than a 1 MiB frame could hold
- **THEN** one public start SHALL carry every warning in order

#### Scenario: Invalid output still fails
- **WHEN** an event of any size contains invalid UTF-8, an unknown discriminator, a non-object metadata namespace, an invalid timestamp or a lifecycle violation
- **THEN** no bytes of that event SHALL be written and the handler SHALL attempt at most one synthetic terminal internal error

### Requirement: Raw-usage finish validity
When finish usage supplies Raw, the runtime SHALL require one complete JSON object, SHALL reject invalid UTF-8 in any key or nested value on the original bytes, and SHALL preserve valid JSON escapes, including lone and paired UTF-16 surrogate escapes. It SHALL NOT impose a byte cap on raw usage.

#### Scenario: Raw usage above the former cap
- **WHEN** finish raw usage is a valid object above 1,048,576 bytes, including JSON whitespace
- **THEN** the finish SHALL carry it unchanged in `usage.raw`

#### Scenario: Raw UTF-8 and JSON escapes at finish
- **WHEN** finish raw usage contains invalid UTF-8 in an object key or nested value
- **THEN** the handler SHALL not encode or write finish and SHALL attempt at most one fixed terminal error frame
- **WHEN** raw usage contains valid JSON with lone or paired escaped surrogates
- **THEN** the handler SHALL preserve the raw JSON escapes

### Requirement: Start validation omitted-start normalization and Anthropic pre-read ordering

Start warnings SHALL be validated for registered type and UTF-8, then encoded once into the start frame. When the provider omits start, warnings SHALL be an empty array and the first part SHALL then be processed. Built-in Anthropic SHALL retain initial upstream-event error preflight, emit its start with conversion warnings before processing the pre-read event, and attach no warnings to finish.

#### Scenario: Start validation omitted-start normalization and Anthropic pre-read ordering
- **WHEN** Anthropic conversion produces warnings before a successfully pre-read event
- **THEN** its start SHALL precede handling that event and finish SHALL have no warnings, with omitted-start normalization intact

### Requirement: Registered SSE outer fields and opaque metadata

Every public event SHALL contain only its registered outer fields, with supported providerMetadata treated as opaque object-valued namespaces rather than a namespace/key allowlist, and SHALL be framed as exactly `data: <json>\n\n`. Namespace names and values SHALL be validated as UTF-8 and object-shaped before the event is encoded.

#### Scenario: Registered SSE outer fields and opaque metadata
- **WHEN** a source frame carries unknown namespace objects with large original raw bytes
- **THEN** registered outer fields and the opaque metadata SHALL be preserved unchanged, using exact data framing

### Requirement: Single SSE frame encoding
Each finish and error frame SHALL be encoded once together with its optional execution metadata and current native summary. When an error frame's encoding fails, the handler SHALL write the fixed base error frame for the same category. When a finish frame's encoding fails, the handler SHALL attempt one terminal internal error. Frames SHALL NOT be size-limited or truncated.

#### Scenario: Large enriched error frame
- **WHEN** a provider error carries a current native summary and an execution overview whose frame exceeds 64 KiB
- **THEN** the complete frame with both SHALL be written in order and the stream SHALL continue

#### Scenario: Optional error data cannot be encoded
- **WHEN** encoding an error frame with its optional data fails
- **THEN** the fixed base frame with the same classification SHALL be written
