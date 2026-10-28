## ADDED Requirements

### Requirement: Sequential response segments
A committed stream SHALL consist of one or more response segments. The public start SHALL open the first segment, and every accepted response-metadata part SHALL open a new one. Response-metadata SHALL be accepted only while no text, reasoning or tool-input block is open and no tool preview series lacks its final result, the same condition a finish requires. Repeated response-metadata and finish parts SHALL be written in provider order.

#### Scenario: Message-shaped segments
- **WHEN** a provider emits metadata, content and a valid finish, then a raw part, then metadata, content and a valid finish again, and then closes its stream
- **THEN** every part SHALL be written in order, including both metadata parts and both finish parts, and the response SHALL end at clean EOF

#### Scenario: Response-shaped segments
- **WHEN** a provider emits metadata, reasoning and a complete tool call, then metadata, a complete tool call, then metadata, text and one valid finish, and then closes its stream
- **THEN** every part SHALL be written in order without a terminal error, and the response SHALL end at clean EOF

#### Scenario: Metadata while a block is open
- **WHEN** response-metadata arrives while a text, reasoning or tool-input block is open, or while a tool preview series lacks its final result
- **THEN** the metadata SHALL NOT be written, provider work SHALL be canceled and at most one synthetic terminal internal error SHALL be attempted

#### Scenario: Overlapping message start
- **WHEN** a second message's response-metadata arrives before the previous message's text block has ended
- **THEN** the handler SHALL fail safely as for metadata while a block is open, without closing the open block or writing a finish

#### Scenario: Consecutive metadata parts
- **WHEN** two valid response-metadata parts arrive with nothing open between them
- **THEN** both SHALL be written, each opening a segment

### Requirement: Segment-scoped and stream-scoped identifiers
Text-start and reasoning-start IDs SHALL be unique within their response segment and MAY repeat in a later segment. Tool-input, tool-call and approval IDs SHALL remain unique for the whole stream and SHALL never reopen. A complete provider-executed call written in one segment SHALL remain eligible for its result in a later segment of the same stream.

#### Scenario: Text ID repeats in a later segment
- **WHEN** a segment ends a text block with ID `0` and a later segment opens a text block with ID `0`
- **THEN** both blocks SHALL be written with their native ID unchanged

#### Scenario: Text or reasoning ID repeats within a segment
- **WHEN** a text-start or reasoning-start ID is reused within the same segment
- **THEN** the reused start SHALL NOT be written and at most one synthetic terminal internal error SHALL be attempted

#### Scenario: Tool ID reopens in a later segment
- **WHEN** a later segment starts tool input or a tool call with an ID already used in an earlier segment
- **THEN** the part SHALL NOT be written and at most one synthetic terminal internal error SHALL be attempted

#### Scenario: Provider result crosses segments
- **WHEN** a complete provider-executed call is written in the first segment and its matching final result arrives in a later segment
- **THEN** the result SHALL be written in place and the stream SHALL continue

### Requirement: Parts after a finish
After a valid finish, only response-metadata, raw parts under gateway-raw-output, provider error parts or provider close SHALL follow. Any other part, including a second finish, SHALL NOT be written. The written finish SHALL remain in place, provider work SHALL be canceled and at most one synthetic terminal internal error SHALL follow it. A provider error part after a finish SHALL be written in place and SHALL NOT change lifecycle state.

#### Scenario: Content follows a finish without metadata
- **WHEN** a text, reasoning, tool, source, custom or approval part follows a valid finish without an intervening response-metadata part
- **THEN** the part SHALL NOT be written and one synthetic terminal internal error SHALL follow the finish

#### Scenario: Second finish without metadata
- **WHEN** a second finish follows a valid finish without an intervening response-metadata part
- **THEN** the second finish SHALL NOT be written and one synthetic terminal internal error SHALL follow the first finish

#### Scenario: Provider error follows a finish
- **WHEN** a valid finish is followed by a provider error part and the provider then closes its stream
- **THEN** the error SHALL be written in place with its normal classification and the response SHALL end at clean EOF without a synthetic error

#### Scenario: Raw part follows a finish
- **WHEN** a raw part follows a valid finish and precedes the next response-metadata part
- **THEN** the raw part SHALL be written when requested and dropped otherwise, and the next segment SHALL be accepted

### Requirement: Stream completion on provider close
The response SHALL end at provider close, at a terminal adapter or runtime failure, at idle timeout, request deadline or cancellation, or at writer failure, not at a finish. Lifecycle parts SHALL be every part except raw parts and provider error parts. Provider close SHALL be a clean EOF without `[DONE]` only when the last lifecycle part is a valid finish; otherwise the handler SHALL attempt one synthetic premature-close internal error.

#### Scenario: Close after a finish
- **WHEN** the provider closes its stream after a valid finish that is the last lifecycle part
- **THEN** the response SHALL end at clean EOF without `[DONE]` or another event

#### Scenario: Close without a finish
- **WHEN** the provider closes its stream before any valid finish
- **THEN** the handler SHALL attempt one synthetic internal error and SHALL NOT synthesize a finish or block end

#### Scenario: Close after a new segment opens
- **WHEN** the provider closes its stream after a valid finish followed by response-metadata
- **THEN** the handler SHALL attempt one synthetic internal error after the metadata and SHALL NOT synthesize a finish

### Requirement: Timeout and cancellation after a finish
Idle timeout, request deadline expiry and request cancellation SHALL keep their normal terminal behavior after a written finish; only provider close SHALL depend on the last lifecycle part. The handler SHALL cancel provider work before encoding the terminal event and write it after the finish: a timeout error for idle timeout or deadline expiry, or a cancellation error only while the writer is still usable. The finish SHALL NOT be retracted.

#### Scenario: Idle after a finish
- **WHEN** a provider writes a valid finish and then neither emits nor closes until the idle timeout
- **THEN** provider work SHALL be canceled before one timeout error event is written after the finish, the response SHALL end within the idle duration plus bounded handler latency, and cleanup SHALL remain single-owner and bounded

#### Scenario: Stall between segments
- **WHEN** a provider writes a valid finish for one message, has further messages pending and then stalls until the idle timeout or the request deadline
- **THEN** the finish SHALL remain written and one timeout error event SHALL follow it, so the response SHALL NOT end at clean EOF

#### Scenario: Cancellation after a finish
- **WHEN** the request is canceled after a valid finish that is the last lifecycle part
- **THEN** provider work SHALL be canceled and at most one cancellation error event SHALL be attempted after the finish, only if the writer is still usable

#### Scenario: Idle after a segment opens
- **WHEN** a valid finish is followed by response-metadata and the idle timeout then expires
- **THEN** provider work SHALL be canceled before one timeout error event is written

### Requirement: Per-finish usage and execution metadata
Each valid finish SHALL forward its own normalized usage, supplied `usage.raw` and providerMetadata unchanged, including native safeguard results. The runtime SHALL NOT sum, merge, carry over or rewrite usage or metadata across finish parts. When an execution overview is available, every written finish SHALL carry it under `providerMetadata.gateway.execution`, encoded with that finish, without buffering or read-ahead.

#### Scenario: Intermediate and final usage
- **WHEN** two segments end with finish parts that report different normalized usage, raw usage and provider metadata
- **THEN** each finish SHALL carry exactly its own values in its own position

#### Scenario: Every finish is enriched
- **WHEN** a configured stream with an available execution overview writes several finish parts
- **THEN** each finish SHALL carry the same compact overview, and any native `gateway` namespace on each finish SHALL be relocated under it

### Requirement: Segments share stream-global bounds and ownership
Response segments SHALL NOT add or reset any bound. The idle timeout SHALL restart after each represented part in any segment, the request deadline SHALL cover all segments, and no part, segment, byte or frame count SHALL be imposed. One cleanup owner SHALL cancel before any terminal write and drain only after a terminal outcome. Later segments SHALL NOT re-enter candidate selection, fallback or replay.

#### Scenario: Many segments within the deadlines
- **WHEN** a provider emits many valid segments, each within the idle duration, before the request deadline and then closes
- **THEN** every part of every segment SHALL be written in order and the response SHALL end at clean EOF

#### Scenario: Continuously ready provider with segments
- **WHEN** a provider keeps emitting valid segments without closing until the request deadline expires
- **THEN** provider work SHALL be canceled at the deadline, at most one applicable terminal outcome SHALL be produced, and the drain SHALL stop within the drain duration

#### Scenario: Later segment fails on a fallback route
- **WHEN** a configured fallback candidate has been selected and a later segment of its stream fails validation
- **THEN** the existing terminal failure SHALL apply and no other candidate SHALL be invoked

### Requirement: Sequential segment evidence
Automated evidence SHALL include focused raw-wire and schema tests for the segment rules. It SHALL replay the committed `expected-provider-parts.jsonl` of each registered sequential fixture through the real handler, read-only. Both registered clients SHALL reproduce each fixture's unchanged `expected.jsonl` and `expected-requests.jsonl` through the real adapters and handler. Mock-stream evidence SHALL NOT be described as live-provider acceptance.

#### Scenario: Committed provider parts replay through the handler
- **WHEN** the committed provider parts of the seven registered sequential fixtures are served through the handler with raw chunks requested
- **THEN** every part SHALL be written in order with only documented ProviderWire projections, and each response SHALL end at clean EOF

#### Scenario: Both clients reproduce the direct goldens
- **WHEN** both registered clients call the seven fixtures through the real adapters and handler
- **THEN** their UI chunks SHALL equal the unchanged `expected.jsonl` and the native requests SHALL equal the unchanged `expected-requests.jsonl`

#### Scenario: Fixture inputs remain authentic
- **WHEN** the sequential evidence is produced
- **THEN** no `input*.chunks.txt`, `expected-provider-parts.jsonl`, `expected.jsonl` or `expected-requests.jsonl` file SHALL be modified, split or skipped

## MODIFIED Requirements

### Requirement: Native response metadata and text block state
Streaming SHALL preserve represented native identity and sequential text-block state, validate required/optional fields before output and retain terminal/commitment authority. Route identity SHALL not fabricate native fields or alter independent configured/BYOK observation.

#### Scenario: Native response metadata and text block state policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** After public start, the state machine SHALL accept response-metadata and finish parts under the sequential response segment rules, sequential text blocks, independent tool events under gateway-streaming-function-tools, supported reasoning/sources/custom under their capabilities and non-terminal provider errors. Response metadata SHALL copy representable native ResponseID, ModelID and Timestamp at registered id, modelId and timestamp fields. Nonempty native strings SHALL remain unchanged; optional empty Go strings and zero timestamps SHALL be omitted as documented unrepresentable-presence adaptations. A nonzero timestamp SHALL encode a validated UTC RFC3339Nano instant. A provider metadata part with no identity SHALL contain only its type. The runtime SHALL NOT synthesize missing metadata or substitute requested/canonical route identity. Provider identity, headers and opaque metadata remain outside this scoped identity change.
- **AND** Configured canonical identity SHALL remain authoritative for configured resolution and operator logical metrics, separately from returned identity. BYOK selection and observation SHALL instead follow the request-scoped identity and bounded-cardinality rules in gateway-text-observability, without catalog lookup or response-derived logical identity. Each text-start ID SHALL remain nonempty, valid UTF-8 and unique within its response segment, opening the only active text block. Deltas/ends SHALL use its active ID and required empty deltas SHALL survive. Provider errors SHALL not change text/segment state. Reasoning, source, custom and tool output SHALL NOT prevent a later response-metadata part once no block is open. New native identity strings and timestamps SHALL be validated before any write.

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
- **WHEN** a provider emits multiple non-overlapping text start/delta/end blocks with IDs unique within their segment
- **THEN** all events SHALL be emitted in order and empty delta strings SHALL remain present

#### Scenario: Text lifecycle is invalid
- **WHEN** a text ID is empty, invalid UTF-8, reused within its segment, mismatched, opened while another text block is active, or ended without an active matching block
- **THEN** the handler SHALL cancel provider work and attempt at most one synthetic terminal internal error

#### Scenario: Metadata placement is invalid
- **WHEN** metadata arrives while a text, reasoning or tool-input block is open or a tool preview series lacks its final result
- **THEN** it SHALL not be forwarded and the existing safe terminal behavior SHALL apply

#### Scenario: Metadata follows closed output
- **WHEN** metadata arrives after text, tool, reasoning, source or custom output with no block open
- **THEN** it SHALL be forwarded and open a new response segment

#### Scenario: Metadata representation exceeds bounds
- **WHEN** native identity contains invalid UTF-8 or an invalid timestamp
- **THEN** no metadata bytes SHALL be written and at most one safe terminal error SHALL be attempted
- **WHEN** valid native identity strings encode to a frame above the former 1 MiB limit
- **THEN** the complete metadata event SHALL be written

#### Scenario: Text metadata changes at end
- **WHEN** text-start supplies an object, text-delta omits metadata and text-end supplies an empty or replacement object
- **THEN** those exact metadata positions and presence states SHALL reach both clients in order, with no deep merge or relocation

### Requirement: Canonical telemetry and sequential text lifecycle

Canonical identity SHALL remain authoritative for resolution and operator logical metrics, separately from returned identity. Each text-start ID SHALL remain nonempty, valid UTF-8 and unique within its response segment, opening the only active text block. Deltas/ends SHALL use its active ID and required empty deltas SHALL survive. Provider errors SHALL not change text/segment state. Reasoning, source, custom and tool output SHALL follow the sequential response segment rules.

#### Scenario: Canonical telemetry and sequential text lifecycle
- **WHEN** a provider error interleaves with a text block whose ID differs from native model identity
- **THEN** canonical telemetry SHALL stay separate, the active nonempty UTF-8 text ID SHALL remain unique within its segment, and errors SHALL NOT change text/segment state

### Requirement: Finish validation and terminal authority

A finish part SHALL be valid only when no text, reasoning or tool-input block is active, no tool preview series lacks its final result, the current segment has not already finished, its warnings are empty, and it contains a non-nil registered finish reason and non-nil usage. A written finish SHALL never be retracted or followed by a synthetic finish.

#### Scenario: Finish closes a valid stream
- **WHEN** a valid finish is emitted with no active text, reasoning or tool-input block
- **THEN** the finish SHALL preserve normalized usage, finish reason and supplied ordinary providerMetadata, unrelated transport fields SHALL remain omitted, the handler SHALL keep reading the provider stream under the sequential response segment rules, and the response SHALL end at clean EOF without `[DONE]` when the provider then closes

#### Scenario: Finish is invalid
- **WHEN** finish arrives with an active block, an unfinished tool preview series, after a finish without intervening response-metadata, with warnings, nil or invalid usage, nil or invalid finish reason, unsafe token counts, invalid supplied raw usage, or invalid finish metadata
- **THEN** finish SHALL not be written and the handler SHALL attempt at most one synthetic terminal internal error

#### Scenario: Provider emits after finish
- **WHEN** any provider part is available after a valid finish has been written
- **THEN** response-metadata, raw and provider error parts SHALL be processed under their rules, any other part SHALL fail safely under the parts-after-a-finish rule, and the written finish SHALL remain in place

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

Known usage counts SHALL be non-negative JavaScript-safe integers and SHALL use the registered input/output groups; `usage.raw` SHALL contain a supplied valid provider JSON object, or be omitted when Raw is absent; finish providerMetadata SHALL be preserved as opaque object-valued namespaces under gateway-provider-metadata. Each valid finish SHALL be written once in place and SHALL remain authoritative for its segment.

#### Scenario: Finish usage metadata and final public authority
- **WHEN** a valid finish contains normalized counts, a raw object and opaque metadata
- **THEN** registered usage groups/safe counts and raw/metadata presence SHALL survive, with the finish written once in place and never rewritten by a later finish
- **AND** when the provider closes with that finish as the last lifecycle part, it SHALL be the final lifecycle event, with no synthetic error after it; provider error and raw parts MAY follow it under their rules

### Requirement: Ordered non-terminal provider errors

Each provider `PartError`, before or after a finish, SHALL be independently reduced through the closed safe-error classification and emitted in place with base fields `{"type":"error","error":{"message":string,"type":string,"param":null,"code":string,"statusCode":integer,"retryable":boolean}}`.

#### Scenario: Ordered non-terminal provider errors policy
- **WHEN** the runtime encodes the corresponding response or stream event
- **THEN** Optional `error.data` MAY carry native current-error and execution summaries under gateway-attempt-failure-evidence. A valid provider error SHALL not terminate the stream or alter lifecycle state; later metadata, content, additional provider errors, and finish SHALL remain valid under the sequential response segment rules.

#### Scenario: Provider error is followed by content
- **WHEN** a provider emits an error before or within a text block and later emits otherwise valid content and finish
- **THEN** the client SHALL receive the safe error and every later event in provider order

#### Scenario: Provider error follows a finish
- **WHEN** a provider emits an error after a valid finish and then either closes or emits response-metadata and another segment
- **THEN** the client SHALL receive the safe error in place, and the stream SHALL end at clean EOF or continue with the new segment

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

### Requirement: Synthetic terminal adapter errors

Unsupported families, lifecycle violations, invalid output, premature close, mapping or framing failure, timeout and cancellation SHALL keep bounded terminal errors, also after a written finish. A provider close SHALL be premature unless the last lifecycle part is a valid finish. Reasoning, source, custom and raw parts SHALL follow their own capabilities. Ordinary generated files, approvals and unsupported tool behavior SHALL remain unsupported.

#### Scenario: Reasoning lifecycle violation
- **WHEN** reasoning ends without a matching active start, reuses an ended reasoning ID within its segment, or remains open at finish
- **THEN** the handler SHALL produce at most one safe terminal error and no synthetic finish

#### Scenario: Unsupported stream family appears
- **WHEN** the text runtime receives provider-executed/dynamic/preliminary tool behavior, ordinary generated file, approval, or another unsupported part
- **THEN** it SHALL emit at most one terminal internal error rather than serializing the provider-domain part

#### Scenario: Custom stream part is supported
- **WHEN** the runtime receives a well-formed custom part within a response segment before its finish
- **THEN** it SHALL emit the custom event and continue the stream rather than emitting a terminal error

#### Scenario: Concurrent reasoning
- **WHEN** two reasoning blocks and a text block sharing one raw ID interleave
- **THEN** family-specific active state SHALL preserve each block independently and IDs SHALL not be rewritten

### Requirement: Adapter failure does not manufacture lifecycle completion

Failure SHALL not synthesize block closures or a finish, nor emit after a terminal outcome or writer failure.

#### Scenario: Adapter failure does not manufacture lifecycle completion
- **WHEN** an unsupported part arrives while a text block is open
- **THEN** failure SHALL NOT synthesize text-end or finish, nor write after a terminal outcome or writer failure

### Requirement: Cancellation and timeouts
The request context's deadline SHALL cover selection, setup, attempts and streaming without resets, and the handler SHALL add no total deadline of its own. Idle timeout SHALL follow represented provider parts; cancellation and timeout SHALL retain terminal authority, bounded cleanup and writer-failure behavior, including after a written finish.

#### Scenario: Streaming deadline and terminal policy
- **WHEN** selection or streaming encounters cancellation, timeout or competing terminal conditions
- **THEN** The request context's deadline SHALL cover selection/construction, logical `DoStream` setup, all credential attempts and stream consumption.
- **AND** Logical invocation, credential attempts and stream establishment SHALL NOT reset or extend the request deadline.
- **AND** Expiry of the request context's deadline SHALL be a timeout, and cancellation of the request context SHALL be a cancellation.
- **AND** Selection cancellation and late-result cleanup SHALL follow the shared bounded invocation contract.
- **AND** After stream commitment, the configured idle timeout SHALL restart after each accepted provider part is successfully represented, including a consumed start and a written provider error.
- **AND** Request cancellation, request deadline expiry, or idle timeout SHALL cancel selection/provider work before the corresponding safe terminal response or event is written.
- **AND** A valid finish written before another terminal outcome SHALL remain written and SHALL NOT be retracted; a later request cancellation, request deadline expiry or idle timeout SHALL still produce its corresponding safe terminal event after it.
- **AND** When provider output, cancellation, and timeout become ready concurrently before terminal output, any applicable safe bounded outcome MAY win; the protocol does not define scheduler-level precedence.
- **AND** Writer failure SHALL terminate immediately without another write.

#### Scenario: Terminal conditions race
- **WHEN** provider output, request cancellation, the request deadline or the idle timeout become ready concurrently before terminal output
- **THEN** the handler SHALL produce at most one applicable bounded terminal outcome and SHALL not leak or clean up the stream twice

#### Scenario: Selection exhausts streaming time
- **WHEN** the request deadline expires during delayed or non-cooperative selection
- **THEN** handler latency SHALL remain bounded with a pre-commit JSON failure, zero logical model invocations and bounded cleanup of late selection results

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
- **WHEN** a valid finish is the last lifecycle part written before cancellation or timeout terminates the stream
- **THEN** the finish SHALL remain written and SHALL NOT be retracted or followed by a synthetic finish
- **AND** provider work SHALL be canceled before the terminal event is encoded, and the timeout error, or the cancellation error while the writer is still usable, SHALL be attempted after the finish as that terminal event

#### Scenario: Accepted activity resets idle time
- **WHEN** accepted provider parts arrive within each idle interval before the request deadline
- **THEN** the idle timeout SHALL restart after each represented part and SHALL not expire solely because total stream age exceeds one idle interval

#### Scenario: Stream without a request deadline
- **WHEN** the request context carries no deadline and the provider keeps emitting parts within the idle duration
- **THEN** the stream SHALL continue until provider close, caller cancellation or an idle timeout

### Requirement: Streaming terminal authority and scheduler race boundary

A written valid finish SHALL never be retracted; a later idle timeout, deadline or cancellation SHALL produce its own terminal outcome after it. When provider output, cancellation, and timeout become ready concurrently before terminal output, any applicable safe bounded outcome MAY win; the protocol does not define scheduler-level precedence. Writer failure SHALL terminate immediately without another write.

#### Scenario: Streaming terminal authority and scheduler race boundary
- **WHEN** provider output and timeout become ready concurrently before terminal output
- **THEN** any applicable safe bounded outcome MAY win without defined scheduler precedence; a previously written valid finish SHALL remain written and writer failure SHALL prohibit another write

### Requirement: Native-value regressions preserve terminal authority

Native warning/source/identity changes SHALL preserve existing stream-start normalization, the sequential response segment rules, non-terminal provider errors, idle reset, per-segment finish authority, cancellation, writer failure and bounded cleanup rules. Raw/schema and exact-pinned TS/independent Go consumption tests SHALL establish these properties without relying on permissive client parsing.

#### Scenario: Sources interleave and finish remains final
- **WHEN** repeated and equal-cross-variant native source IDs interleave with active content and provider errors before finish, followed by a later source without intervening response-metadata
- **THEN** accepted values SHALL retain order without closing active blocks, the finish SHALL remain written, and the later source SHALL NOT be written but SHALL be followed by one synthetic terminal internal error

#### Scenario: Cancellation or writer failure occurs
- **WHEN** processing native-value events is canceled or a full-frame write/flush fails
- **THEN** existing cancellation and single-owner bounded cleanup SHALL apply with no second write after writer failure or a terminal outcome

## REMOVED Requirements

### Requirement: Immediate EOF and post-finish provider suppression
**Reason**: A valid finish ends a response segment, not the stream. One `DoStream` result can carry several native responses or messages, and the registered direct oracle forwards all of them. Cancelling at the first finish truncated five registered Anthropic fixtures, and suppressing later parts hid that data loss.
**Migration**: The stream now ends under "Stream completion on provider close". Parts after a finish follow "Parts after a finish", and an idle timeout, deadline or cancellation after a finish follows "Timeout and cancellation after a finish", which keeps the terminal error after the written finish. Clients need no change: both registered clients already read until EOF.
