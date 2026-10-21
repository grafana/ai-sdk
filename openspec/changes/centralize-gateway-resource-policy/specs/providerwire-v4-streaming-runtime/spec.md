# Spec Delta

## MODIFIED Requirements

### Requirement: Constructed streaming limits
The strict ProviderWire V4 handler SHALL require positive stream idle duration and post-cancellation drain duration in addition to the total model duration. It SHALL NOT accept provider stream-part count or SSE frame byte limits. Complete frames SHALL consist of the `data: ` prefix, JSON payload, and terminating `\n\n`.

#### Scenario: Valid streaming limits
- **WHEN** a caller supplies positive idle and drain durations
- **THEN** handler construction SHALL succeed and runtime streaming behavior SHALL be fixed by those values

#### Scenario: Invalid streaming limits
- **WHEN** a streaming duration is zero or negative
- **THEN** construction SHALL fail before the handler serves a request

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

#### Scenario: Many or long warnings
- **WHEN** a valid provider start carries more or longer warnings than the former frame budget allowed
- **THEN** the public start SHALL carry every warning in order

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

### Requirement: Uncensored warning values and invalid-start fallback

Native backend names and ordinary application strings SHALL NOT be replaced with generic prose or heuristically censored. Unknown warning discriminators or invalid represented strings SHALL cause one empty public start followed by at most one synthetic terminal internal error when writable.

#### Scenario: Uncensored warning values and invalid-start fallback
- **WHEN** a valid warning includes a backend-looking string, or a start has an unknown warning type
- **THEN** the valid string SHALL remain unchanged and the invalid start SHALL yield one empty start and at most one safe terminal internal error

### Requirement: Start preflight omitted-start normalization and Anthropic pre-read ordering

Warnings SHALL be validated before the start is written, and standard JSON encoding SHALL govern the single write. When the provider omits start, warnings SHALL be an empty array and the first part SHALL then be processed. Built-in Anthropic SHALL retain initial upstream-event error preflight, emit its start with conversion warnings before processing the pre-read event, and attach no warnings to finish.

#### Scenario: Start preflight omitted-start normalization and Anthropic pre-read ordering
- **WHEN** Anthropic conversion produces warnings before a successfully pre-read event
- **THEN** its start SHALL precede handling that event and finish SHALL have no warnings, with omitted-start normalization intact

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
- **WHEN** native identity contains invalid UTF-8 or an invalid timestamp, or encodes larger than the former complete-frame budget
- **THEN** invalid identity SHALL write no metadata bytes and attempt at most one safe terminal error, while valid large identity SHALL be written complete

#### Scenario: Text metadata changes at end
- **WHEN** text-start supplies an object, text-delta omits metadata and text-end supplies an empty or replacement object
- **THEN** those exact metadata positions and presence states SHALL reach both clients in order, with no deep merge or relocation

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
- **WHEN** a valid finish contains normalized usage and a supplied raw JSON object including nested provider-native keys or `{}`
- **THEN** its complete `usage.raw` object SHALL be emitted with the normalized counts, and a distinct `type: "raw"` stream part SHALL NOT be synthesized

#### Scenario: No raw value is supplied
- **WHEN** a valid finish has absent provider `Usage.Raw`
- **THEN** the finish SHALL omit `usage.raw` and retain its normalized counts

#### Scenario: Raw usage fails after stream commitment
- **WHEN** finish usage supplies malformed, invalid UTF-8, null, scalar or array raw JSON
- **THEN** the finish SHALL NOT be written and the handler SHALL attempt at most one fixed complete terminal internal-error SSE frame, without reflecting the offending value or issuing normalized-only finish

#### Scenario: Finish metadata is ordinary output
- **WHEN** valid finish metadata contains unknown namespaces or an explicit empty object and IncludeRawChunks is false
- **THEN** both clients SHALL receive that metadata on finish before clean EOF without any synthesized raw event

### Requirement: Complete bounded SSE framing and flushing

Committed responses SHALL use HTTP 200, `Content-Type: text/event-stream`, and `Cache-Control: no-cache, no-transform`; connection-specific keep-alive headers SHALL not be protocol requirements. The handler SHALL flush commitment headers and every fully written event. Unsupported flushing SHALL not fail the stream; every other header or frame flush error or panic SHALL be a writer failure.

#### Scenario: Event is written and flushed
- **WHEN** a valid event is encoded and the writer supports flushing
- **THEN** exactly one complete `data:` frame SHALL be fully written and flushed before the next event

#### Scenario: Flushing is unsupported
- **WHEN** the response writer does not support flushing
- **THEN** the stream SHALL continue without treating that limitation as a failure

#### Scenario: Commitment header flush fails
- **WHEN** flushing commitment headers fails or panics
- **THEN** provider work SHALL be canceled and the handler SHALL end without attempting a stream frame

#### Scenario: Event exceeds its complete-frame limit
- **WHEN** a valid event encodes larger than the former complete-frame budget
- **THEN** its complete frame SHALL be written and flushed without a size-based terminal error

#### Scenario: Writer fails
- **WHEN** writing or flushing an event fails, writes short, or panics
- **THEN** provider work SHALL be canceled and no synthetic event or second write SHALL be attempted on that writer

#### Scenario: Invalid metadata cannot become partial event
- **WHEN** registered metadata is malformed JSON, invalid UTF-8 or non-object at a namespace
- **THEN** no bytes of that event SHALL be written, existing terminal cancellation/cleanup SHALL apply and no selected fallback candidate SHALL be replayed

### Requirement: Complete SSE writes fixed terminal frames and writer failure

Each event SHALL be completely encoded before any of its bytes are written.

#### Scenario: Complete SSE writes fixed terminal frames and writer failure policy
- **WHEN** the runtime encodes the corresponding response or stream event
- **THEN** Synthetic terminal errors SHALL retain their fixed base fields; optional execution attribution SHALL follow gateway-attempt-failure-evidence and SHALL be encoded with the base error without fit-based omission, falling back to the canonical base frame only when optional data cannot be encoded. The server SHALL never emit SSE `event:` fields or `[DONE]`. A write error, short write, writer panic, or supported flush failure SHALL cancel provider work and end immediately without another write.

#### Scenario: Complete SSE writes fixed terminal frames and writer failure
- **WHEN** a full-frame write is short or flushing panics
- **THEN** provider work SHALL be canceled with no subsequent write, while complete-event encoding and prohibition of event fields/DONE SHALL remain effective

### Requirement: Bounded stream ownership and drain

Every returned stream SHALL have one receiver responsible for cleanup. A stream transferred from setup belongs to the handler; when model-context termination wins before transfer, the setup worker SHALL clean up any stream returned later. Terminal handling SHALL cancel provider work and start an asynchronous drain that reads only until channel close or the configured drain duration.

#### Scenario: Provider closes after cancellation
- **WHEN** terminal handling cancels provider work and the provider closes its stream within the drain duration
- **THEN** the bounded drain SHALL consume the remaining channel values and exit

#### Scenario: Provider ignores cancellation
- **WHEN** a provider channel does not close before the drain duration
- **THEN** handler latency and the handler-owned drain lifetime SHALL remain bounded even though provider-owned work may remain defective

#### Scenario: Continuously ready drain reaches a hard bound
- **WHEN** a canceled provider channel remains permanently ready with values
- **THEN** the drain SHALL stop within the configured duration rather than consuming indefinitely

#### Scenario: Setup returns a late stream
- **WHEN** handler cancellation or timeout wins and `DoStream` later returns a non-nil stream
- **THEN** the late setup owner SHALL cancel and bounded-drain that channel exactly once instead of abandoning it unread

### Requirement: Optional streaming execution overview and current failures

Streaming SHALL use the existing reader/commitment/drain ownership to optionally enrich finish metadata and classified error payloads.

#### Scenario: Optional streaming execution overview and current failures policy
- **WHEN** the runtime encodes the corresponding response or stream event
- **THEN** First-part selection SHALL NOT claim completion. Current native summaries SHALL remain event-local and eligible without catalog provenance; finish SHALL NOT duplicate them. Original metadata validation, error ordering, active-block state, authoritative finish and writer failure behavior SHALL remain unchanged. A valid finish SHALL be encoded once with its enrichment; only an encoding failure of optional data SHALL preserve the original frame, and size SHALL NOT cause omission.

#### Scenario: Selected leading error followed by content
- **WHEN** the first provider part is an error followed by valid text and finish
- **THEN** no later candidate SHALL run, both clients SHALL consume all parts in order and finish SHALL contain only the optional compact execution overview rather than stream-error history

#### Scenario: Finish fits only without overview
- **WHEN** a valid finish with large native metadata would have fit the former frame budget only without its configured overview
- **THEN** one finish frame SHALL carry both without a new terminal error or read-ahead

## ADDED Requirements

### Requirement: Registered SSE outer fields and opaque metadata
Every public event SHALL contain only its registered outer fields, with supported providerMetadata treated as opaque object-valued namespaces rather than a namespace/key allowlist. It SHALL be framed as exactly `data: <json>\n\n`. Metadata SHALL be validated for namespace object shape and UTF-8 before encoding, without byte or cardinality accounting.

#### Scenario: Unknown large namespace
- **WHEN** a source or text frame carries unknown namespace objects with large original raw bytes
- **THEN** registered outer fields and the opaque metadata SHALL be preserved using exact data framing

### Requirement: Finish raw usage validity
When finish supplies raw usage, the handler SHALL validate it on the original bytes as one valid UTF-8 JSON object and SHALL preserve valid JSON escapes, including lone and paired UTF-16 surrogate escapes. It SHALL NOT apply a raw-usage byte cap.

#### Scenario: Raw escapes at finish
- **WHEN** finish raw usage contains valid JSON with lone or paired escaped surrogates
- **THEN** the finish SHALL preserve the raw JSON escapes

#### Scenario: Raw usage larger than the former cap
- **WHEN** finish raw usage is a valid object larger than 1,048,576 bytes
- **THEN** the finish SHALL carry it unchanged in meaning

## REMOVED Requirements

### Requirement: Bounded provider-part cardinality
**Reason**: A per-request provider part counter is adapter-local resource machinery. Gateway-constructed providers derive parts from upstream responses bounded by the shared outbound transport, and the total, idle and drain durations bound processing time.
**Migration**: Provider output volume is bounded by the shared provider response bound and the model duration under gateway-resource-policy; drain is bounded by the drain duration.

### Requirement: Excess provider parts and warning allocation preflight
**Reason**: Excess-part handling and warning-cardinality preflight depended on the removed part counter and frame budget.
**Migration**: Warnings are validated for type and UTF-8 only; see "Normalized stream start and value-safe warnings".

### Requirement: Native stream identity complete-frame preflight
**Reason**: Complete-frame byte preflight is removed.
**Migration**: Identity validity is covered by "Native response metadata and text block state".

### Requirement: Bounded raw-usage finish representation
**Reason**: The independent raw-usage cap and frame-budget checks are removed.
**Migration**: Use "Finish raw usage validity".

### Requirement: Complete raw-usage finish frame and bounded encoding
**Reason**: `StreamFrameBytes` and `StreamParts` no longer exist.
**Migration**: Finish frames are encoded once under "Complete SSE writes fixed terminal frames and writer failure".

### Requirement: Registered SSE outer fields opaque metadata and aggregate preflight
**Reason**: Aggregate metadata preflight is removed.
**Migration**: Use "Registered SSE outer fields and opaque metadata".
