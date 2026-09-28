## MODIFIED Requirements

### Requirement: Canonical metadata and text block state
After the public start and within the configured provider-part count, the state machine SHALL accept at most one response-metadata part before the first text or tool event, zero or more sequential text blocks and independently tracked function-tool input/call/result events governed by gateway-streaming-function-tools, non-terminal provider error parts at any pre-finish point, and exactly one finish. Response metadata SHALL preserve an optional valid response ID and timestamp, SHALL always set `modelId` to the resolver's canonical public ID, and SHALL omit provider identity, backend model ID, response headers, and metadata on the response-metadata event. Only the closed supported text and function-tool event metadata projection SHALL be emitted on eligible parts; other event metadata SHALL be omitted. Each text start ID SHALL be valid UTF-8, non-empty, globally unique within the stream, and SHALL open the only active block. Text deltas and ends SHALL use the active ID; an end SHALL close it. Required empty deltas SHALL be preserved. Provider errors SHALL not open, close, or otherwise change text or metadata state.

#### Scenario: Canonical metadata precedes text
- **WHEN** a provider emits one valid response-metadata part before text or tool events
- **THEN** the public metadata SHALL preserve only allowlisted ID and timestamp values and SHALL use the canonical public model ID

#### Scenario: Approved OpenAI text identity
- **WHEN** OpenAI text-start and text-end have valid `openai.itemId` equal to their already-public text ID alongside unknown metadata
- **THEN** each event SHALL contain only `providerMetadata.openai.itemId`, and text-delta SHALL not gain metadata

#### Scenario: Sequential text blocks are valid
- **WHEN** a provider emits multiple non-overlapping text start/delta/end blocks with unique IDs
- **THEN** all events SHALL be emitted in order and empty delta strings SHALL remain present

#### Scenario: Text lifecycle is invalid
- **WHEN** a text ID is empty, invalid UTF-8, reused, mismatched, opened while another text block is active, or ended without an active matching block
- **THEN** the handler SHALL cancel provider work and attempt at most one synthetic terminal internal error

#### Scenario: Metadata placement is invalid
- **WHEN** response metadata is duplicated or appears after a text or tool event has started
- **THEN** the handler SHALL terminate with at most one synthetic safe error rather than forwarding the invalid metadata

### Requirement: Finish validation and terminal authority
A finish part SHALL be valid only when no text or tool-input block is active, its warnings are empty, and it contains a non-nil registered finish reason and non-nil usage. Known usage counts SHALL be non-negative JavaScript-safe integers and SHALL use the registered input/output groups; raw usage and provider metadata SHALL be omitted from finish (even when eligible metadata is present on earlier content). A valid finish SHALL be written once as the final public event and SHALL be authoritative. The handler SHALL then cancel provider work, begin bounded asynchronous drain, and return clean EOF immediately without waiting for provider channel closure or emitting `[DONE]`. Provider parts observed after a written finish SHALL be suppressed during drain, treated as provider lifecycle defects for later operational reporting, and SHALL never produce a second public terminal event.

#### Scenario: Finish closes a valid stream
- **WHEN** a valid finish is emitted with no active text or tool-input block
- **THEN** the finish SHALL preserve normalized usage and finish reason, provider-private fields SHALL be omitted, and the response SHALL end at clean EOF without `[DONE]`

#### Scenario: Finish is invalid
- **WHEN** finish arrives with an active block, warnings, nil or invalid usage, nil or invalid finish reason, or unsafe token counts
- **THEN** finish SHALL not be written and the handler SHALL attempt at most one synthetic terminal internal error

#### Scenario: Provider emits after finish
- **WHEN** any provider part is available after a valid finish has been written
- **THEN** finish SHALL remain the final public event and the later part SHALL be suppressed while cancellation and bounded drain complete

## ADDED Requirements

### Requirement: Closed supported provider-part metadata projection
Only supported OpenAI `itemId` and Anthropic direct `caller` metadata SHALL pass on eligible content. OpenAI text-start/text-end `itemId` SHALL be a nonempty ASCII identifier matching `[A-Za-z0-9_-]+`, equal to that event's text ID; on basic tool-call/tool-result it SHALL be a nonempty identifier of the same restricted form (not required to equal `toolCallId`). Anthropic basic tool-call/tool-result `caller` SHALL be exactly `{"type":"direct"}`. Metadata SHALL appear under its original provider namespace only when its approved field is present. The server SHALL discard unknown provider-domain namespaces without inspecting their raw values, even if those values are deep, oversized or malformed, and SHALL NOT serialize their bytes. For recognized `openai` and `anthropic` namespaces, the server SHALL check the entire raw namespace against the remaining output-byte budget before parsing; it SHALL then validate well-formed JSON and a shallow shape no deeper than namespace -> object -> caller object. Bounded, well-formed unknown fields inside a recognized namespace SHALL be discarded, but excessive raw namespace bytes, malformed JSON, excessive depth (including an unknown sibling's value), duplicate keys, invalid UTF-8, and unsupported approved-field types or values SHALL fail safely even if only unknown fields would otherwise be emitted. An Anthropic `caller` whose `type` is not `direct` SHALL fail rather than being treated as a future unknown field; nested extra `caller` members SHALL also fail. Other namespaces and fields, including arbitrary backend names, credentials, headers and future metadata, SHALL NOT appear in output. Failure SHALL NOT echo raw values. No provider-part metadata SHALL be copied onto response-metadata, start, text-delta, incremental tool-input events, error or finish. This policy SHALL NOT govern ordinary caller-visible text or basic tool result content.

#### Scenario: Valid field alongside hostile extras
- **WHEN** an eligible tool-call carries `anthropic.caller={"type":"direct"}` and an unknown credential-bearing field or namespace
- **THEN** only the direct caller SHALL appear in the output, with no credential field in error, log, metric or metadata-only observability

#### Scenario: Unknown namespace needs no inspection
- **WHEN** an eligible part carries a valid approved field and an unrelated namespace with deep, oversized or malformed raw JSON
- **THEN** only the approved field SHALL be emitted; the unrelated value SHALL NOT be parsed, sized, serialized or exposed

#### Scenario: Unknown sibling within recognized namespace
- **WHEN** a recognized namespace contains a bounded well-formed shallow unknown field alongside an approved field, or contains only bounded well-formed shallow unknown fields
- **THEN** unknown fields SHALL be discarded, the approved field (if any) SHALL be emitted, and an empty projection SHALL omit `providerMetadata`

#### Scenario: Recognized namespace is unsafe even without approved fields
- **WHEN** a recognized namespace exceeds its pre-parse raw-byte budget, contains malformed JSON, duplicate keys, invalid UTF-8, or exceeds the shallow depth limit via an unknown sibling, even if it contains no approved fields
- **THEN** the event SHALL NOT be partially written, provider work SHALL be canceled and at most one safe terminal error SHALL be attempted without echoing raw values

#### Scenario: Invalid approved field
- **WHEN** an approved field is malformed, oversized, contains a credential-like or backend-identifier string instead of a valid OpenAI item identifier, or an Anthropic caller has an unsupported `type` or an extra nested member
- **THEN** the event SHALL NOT be partially written, provider work SHALL be canceled and at most one safe terminal error SHALL be attempted without echoing the field

#### Scenario: Unsupported metadata locations
- **WHEN** finish, response-metadata, text-delta, tool-input, stream-start or error parts carry otherwise valid metadata
- **THEN** their existing strict public event shapes SHALL remain unchanged
