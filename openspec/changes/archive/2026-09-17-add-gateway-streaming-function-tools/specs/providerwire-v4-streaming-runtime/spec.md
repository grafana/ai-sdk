## MODIFIED Requirements

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

### Requirement: Canonical metadata and text block state
After the public start and within the configured provider-part count, the state machine SHALL accept at most one response-metadata part before the first text or tool event, zero or more sequential text blocks and independently tracked function-tool input/call/result events governed by gateway-streaming-function-tools, non-terminal provider error parts at any pre-finish point, and exactly one finish. Response metadata SHALL preserve an optional valid response ID and timestamp, SHALL always set `modelId` to the resolver's canonical public ID, and SHALL omit provider identity, backend model ID, response headers, and provider metadata. Each text start ID SHALL be valid UTF-8, non-empty, globally unique within the stream, and SHALL open the only active block. Text deltas and ends SHALL use the active ID; an end SHALL close it. Required empty deltas SHALL be preserved. Provider errors SHALL not open, close, or otherwise change text or metadata state.

#### Scenario: Canonical metadata precedes text
- **WHEN** a provider emits one valid response-metadata part before text or tool events
- **THEN** the public metadata SHALL preserve only allowlisted ID and timestamp values and SHALL use the canonical public model ID

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
A finish part SHALL be valid only when no text or tool-input block is active, its warnings are empty, and it contains a non-nil registered finish reason and non-nil usage. Known usage counts SHALL be non-negative JavaScript-safe integers and SHALL use the registered input/output groups; raw usage and provider metadata SHALL be omitted. A valid finish SHALL be written once as the final public event and SHALL be authoritative. The handler SHALL then cancel provider work, begin bounded asynchronous drain, and return clean EOF immediately without waiting for provider channel closure or emitting `[DONE]`. Provider parts observed after a written finish SHALL be suppressed during drain, treated as provider lifecycle defects for later operational reporting, and SHALL never produce a second public terminal event.

#### Scenario: Finish closes a valid stream
- **WHEN** a valid finish is emitted with no active text or tool-input block
- **THEN** the finish SHALL preserve normalized usage and finish reason, provider-private fields SHALL be omitted, and the response SHALL end at clean EOF without `[DONE]`

#### Scenario: Finish is invalid
- **WHEN** finish arrives with an active block, warnings, nil or invalid usage, nil or invalid finish reason, or unsafe token counts
- **THEN** finish SHALL not be written and the handler SHALL attempt at most one synthetic terminal internal error

#### Scenario: Provider emits after finish
- **WHEN** any provider part is available after a valid finish has been written
- **THEN** finish SHALL remain the final public event and the later part SHALL be suppressed while cancellation and bounded drain complete

### Requirement: Synthetic terminal adapter errors
Before a valid finish is written, an unsupported stream family, lifecycle violation, invalid provider output, premature channel close, mapping failure, frame overflow, total timeout, idle timeout, or caller cancellation SHALL cancel provider work and produce at most one synthetic terminal safe error when the writer remains usable. Lifecycle, mapping, framing, premature-EOF, and invalid-output failures SHALL use the canonical internal category; cancellation and timeout SHALL retain their closed categories. A terminal error SHALL not close active blocks synthetically or emit a synthetic finish. If a terminal event has already been written or the writer has failed, no further event SHALL be attempted.

#### Scenario: Provider channel closes before finish
- **WHEN** the provider stream closes without a valid finish
- **THEN** the handler SHALL attempt one terminal internal error and SHALL not emit a finish or `[DONE]`

#### Scenario: Unsupported stream family appears
- **WHEN** the text runtime receives reasoning, provider-executed/dynamic/preliminary tool behavior, file, source, custom, raw, approval, or another unsupported part
- **THEN** it SHALL emit at most one terminal internal error rather than serializing the provider-domain part

#### Scenario: Provider errors precede an adapter failure
- **WHEN** one or more non-terminal provider errors are written before a later lifecycle failure
- **THEN** those provider errors SHALL remain in order and at most one additional synthetic terminal error SHALL end the stream
