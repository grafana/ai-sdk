## MODIFIED Requirements

### Requirement: Normalized stream start and value-safe warnings
Every committed writable stream SHALL emit exactly one stream-start as its first JSON event. The first provider part SHALL determine whether its initial warnings are mapped; provider stream-start SHALL remain valid only as the first provider part. Unary and streaming warning fields SHALL preserve the registered union, values and order within gateway-caller-response-policy's warning/input/complete-output bounds rather than replace arbitrary strings with fixed backend-hiding prose. Required empty strings SHALL survive; inactive fields SHALL be omitted; optional empty details SHALL use the documented Go presence normalization. Unknown discriminators, invalid UTF-8 or over-limit warnings SHALL cause an empty start followed by at most one fixed terminal internal error. Warning cardinality/aggregate bytes SHALL be checked before allocation/encoding.

When the provider omits start, the handler SHALL emit warnings: [] and process the first part in order. Late/duplicate starts SHALL remain invalid. Built-in Anthropic initial upstream-error preflight SHALL remain before stream exposure, and successful preflight SHALL emit one PartStreamStart with request-conversion warnings before the pre-read event. Finish warnings SHALL remain empty.

#### Scenario: Provider start carries known warnings
- **WHEN** the first provider start carries valid unsupported, compatibility, deprecated and other warnings
- **THEN** exactly one public start SHALL preserve meaningful feature/setting/message/details and order within the reviewed bounds

#### Scenario: Warning prose is caller-visible but not telemetry
- **WHEN** a valid warning names a provider feature or a caller setting
- **THEN** the caller SHALL receive it without generic backend concealment
- **AND** logical logs, metric labels and metadata-only exports SHALL not receive its arbitrary strings

#### Scenario: Warning output is invalid
- **WHEN** required fields, UTF-8, registered type or warning/complete-frame limits fail
- **THEN** no partial warning start SHALL be written and an empty start plus at most one fixed safe terminal error SHALL be attempted

#### Scenario: Provider omits start
- **WHEN** the first provider part is metadata, text, provider error or finish
- **THEN** one non-null empty-warning start SHALL precede normal processing of that first part

#### Scenario: Provider start is late or duplicated
- **WHEN** start appears after an earlier part or start
- **THEN** at most one synthetic terminal safe error SHALL occur without a second public start

#### Scenario: Anthropic warnings follow successful preflight
- **WHEN** Anthropic conversion produces warnings and initial error preflight succeeds
- **THEN** its provider stream SHALL emit the warnings on PartStreamStart before the pre-read event and none on finish

#### Scenario: Anthropic initial API failure remains setup failure
- **WHEN** Anthropic initial preflight returns an API error
- **THEN** DoStream SHALL return it before StreamResult/start exposure and the handler SHALL use the reviewed bounded precommit provider projection when available

### Requirement: Canonical metadata and text block state
After start and within provider-part bounds, the state machine SHALL accept at most one response-metadata part before output, sequential text blocks, independent function-tool events under gateway-streaming-function-tools, ordered non-terminal provider errors before finish, and one authoritative finish. Response metadata SHALL preserve supplied valid response ID, actual modelId and timestamp; absent modelId SHALL remain absent rather than fabricate the resolver's canonical ID. Provider/topology fields and native response transport SHALL remain excluded. Canonical route identity SHALL remain solely a routing/authorization/logical-observation invariant, not a substitute for response model identity.

Text IDs SHALL remain valid UTF-8, nonempty and globally unique within their family; only one text block SHALL be active, deltas/ends SHALL match it, and required empty deltas SHALL survive. Supported text/other stream metadata SHALL use #280's ordinary contract at registered placements with unknown namespaces and nested values under reviewed bounds, without raw filtering. Provider errors SHALL not open, close or change text/metadata state. Existing reasoning/source/tool output-start placement rules SHALL remain effective.

#### Scenario: Actual metadata precedes output
- **WHEN** an alias resolves to a canonical route but provider metadata reports a different actual model and response ID
- **THEN** the public part SHALL preserve the actual model/ID/timestamp while logical telemetry remains canonical

#### Scenario: Metadata omits actual model
- **WHEN** valid response metadata omits modelId
- **THEN** no canonical or backend configuration identity SHALL be fabricated in that member

#### Scenario: Sequential text blocks are valid
- **WHEN** unique non-overlapping text blocks are emitted
- **THEN** events, empty deltas and supported metadata SHALL survive in order

#### Scenario: Text lifecycle is invalid
- **WHEN** an ID is empty, invalid UTF-8, reused, mismatched or opens/ends contrary to active-block state
- **THEN** provider work SHALL be canceled and at most one fixed terminal safe error attempted

#### Scenario: Metadata placement is invalid
- **WHEN** response metadata is duplicated or follows any payload-starting event
- **THEN** at most one fixed terminal safe error SHALL occur without forwarding the invalid part

### Requirement: Finish validation and terminal authority
A finish part SHALL be valid only when no text or tool-input block is active, its warnings are empty, and it contains a non-nil registered finish reason and non-nil usage. Known usage counts SHALL be non-negative JavaScript-safe integers and SHALL use the registered input/output groups; `usage.raw` SHALL contain a supplied valid bounded provider JSON object, or be omitted when Raw is absent; supported finish providerMetadata SHALL be preserved through #280 under its ordinary reviewed bounds and SHALL NOT depend on IncludeRawChunks. A valid finish SHALL be written once as the final public event and SHALL be authoritative. The handler SHALL then cancel provider work, begin bounded asynchronous drain, and return clean EOF immediately without waiting for provider channel closure or emitting `[DONE]`. Provider parts observed after a written finish SHALL be suppressed during drain, treated as provider lifecycle defects for later operational reporting, and SHALL never produce a second public terminal event.

#### Scenario: Finish closes a valid stream
- **WHEN** a valid finish is emitted with no active text or tool-input block
- **THEN** the finish SHALL preserve normalized usage and finish reason, supported finish metadata SHALL survive while native transport and invented topology SHALL be omitted, and the response SHALL end at clean EOF without `[DONE]`

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

### Requirement: Ordered non-terminal provider errors
Each valid pre-finish provider PartError SHALL use gateway-caller-response-policy's reviewed provider projection and be emitted at its original position as the existing error event with message/type/param/code/statusCode/retryable. Supported structured diagnostics SHALL survive under original-input and complete-frame bounds without whole APICallError/Data/body serialization. Native transport, headers, credentials, URLs, requests and causes SHALL remain excluded. Nil, malformed, unreviewed/message-only/internal errors SHALL use the corresponding fixed safe error part; invalid nonzero HTTP status outside 100 through 599 SHALL reduce to internal without inspecting wrapped transport causes.

Provider errors SHALL remain non-terminal and SHALL not alter active blocks, metadata placement, finish authority or later part order. Status-zero transport errors SHALL retain existing safe timeout/upstream classification. Valid reported status-bearing stream errors, including HTTP 200, SHALL retain reviewed diagnostics and the existing retryable member, distinct from HTTP setup client class construction. This change SHALL NOT change core stream termination semantics.

#### Scenario: Provider error is followed by content
- **WHEN** a reviewed provider error occurs before or within a text block followed by valid content and finish
- **THEN** its actionable bounded error and all later events SHALL remain ordered without block/state changes

#### Scenario: Multiple provider errors are ordered
- **WHEN** multiple errors are separated by valid stream parts
- **THEN** each SHALL be independently projected and retained in place without terminating provider consumption

#### Scenario: Provider error includes transport credentials
- **WHEN** a structured provider error also carries credential-bearing URL/header/body/cause or auth-failure prose
- **THEN** only reviewed safe diagnostics SHALL be emitted and auth failures SHALL use fixed actionable prose

#### Scenario: Provider error status is malformed
- **WHEN** APICallError has a nonzero status outside 100 through 599
- **THEN** canonical internal error SHALL be emitted without inspecting a wrapped transport cause

#### Scenario: Provider error has no HTTP status
- **WHEN** status-zero APICallError wraps a timeout or ordinary network/DNS cause
- **THEN** the existing safe timeout or upstream classification SHALL remain
- **AND** a valid status-bearing structured stream error, including status 200, SHALL keep its reviewed status/retry diagnostics
