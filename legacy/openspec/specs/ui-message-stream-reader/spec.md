# Ui Message Stream Reader Specification

## Purpose
Define progressive and blocking helpers for consuming UI message chunk streams without changing the UI chunk wire protocol.

## Requirements

### Requirement: StreamUIMessage emits upstream write-point snapshots
`StreamUIMessage` SHALL expose the signature `func StreamUIMessage(stream <-chan UIMessageChunk, opts ...UIMessageReaderOption) <-chan UIMessage`. It SHALL consume `UIMessageChunk` values in input order and emit isolated `UIMessage` snapshots only for the state update write points that match upstream `ai@7.0.37` `readUIMessageStream` behavior as represented by Go `UIMessage` and `Part` types. It SHALL NOT emit a synthetic final snapshot solely because the input channel closes.

#### Scenario: Progressive text snapshots
- **WHEN** `StreamUIMessage` receives a start chunk followed by text start, text delta, another text delta, and text end chunks for the same text part
- **THEN** the output channel yields snapshots at upstream write points showing the assistant message evolving from an empty text part to the accumulated text and then the completed text part
- **AND** each later text snapshot preserves the same message ID and includes all prior text deltas in order

#### Scenario: Progressive reasoning snapshots
- **WHEN** `StreamUIMessage` receives reasoning start, reasoning delta, and reasoning end chunks
- **THEN** the output channel yields snapshots showing a `ReasoningPart` being created, updated with accumulated reasoning text, and completed without losing provider metadata

#### Scenario: Progressive tool snapshots update one part
- **WHEN** `StreamUIMessage` receives tool input start, tool input delta, tool input available, approval request, approval response, output available, output denied, or output error chunks for a tool call
- **THEN** the output channel yields snapshots at upstream write points that update one tool part for that tool call ID instead of appending duplicate tool parts for each lifecycle transition
- **AND** static tool calls use `ToolInvocationPart` while dynamic tool calls use `DynamicToolUIPart`

#### Scenario: Partial tool input uses valid RawMessage values
- **WHEN** `StreamUIMessage` receives tool-input-delta chunks whose accumulated input text is complete JSON or can be repaired using upstream-compatible partial JSON repair
- **THEN** the emitted tool part snapshot includes `Input` as valid `json.RawMessage` produced from the parsed JSON value
- **AND** marshaling the `UIMessage` snapshot succeeds

#### Scenario: Unparseable partial tool input omits Input
- **WHEN** `StreamUIMessage` receives a tool-input-delta chunk whose accumulated input text cannot be parsed or repaired as JSON
- **THEN** the emitted tool part remains in `input-streaming` state
- **AND** the tool part's `Input` is nil or omitted instead of containing invalid `json.RawMessage`
- **AND** marshaling the `UIMessage` snapshot succeeds

#### Scenario: Progressive non-text parts
- **WHEN** `StreamUIMessage` receives file, reasoning file, source URL, source document, step start, non-transient data, or message metadata chunks
- **THEN** the output channel yields snapshots that include the corresponding Go message part or metadata update whenever upstream `ai@7.0.37` would write a message snapshot for that chunk type

#### Scenario: Transient data is not assembled
- **WHEN** `StreamUIMessage` receives a data chunk with `Transient` set to true
- **THEN** the transient data is not appended to the message parts
- **AND** no snapshot is emitted solely for that transient data chunk

#### Scenario: Error chunk does not emit or terminate progressive output
- **WHEN** `StreamUIMessage` receives a `ChunkError` chunk between otherwise valid chunks
- **THEN** no snapshot is emitted for the `ChunkError` chunk
- **AND** the reader continues consuming subsequent chunks
- **AND** subsequent valid chunks can still produce snapshots

#### Scenario: Invalid chunk order closes progressive output
- **WHEN** `StreamUIMessage` receives a stateful chunk that references a missing active part or missing tool call, such as text delta before text start or tool output before tool input
- **THEN** no snapshot is emitted for the invalid chunk
- **AND** the output channel closes because the streaming API has no error return channel

#### Scenario: Empty stream emits no snapshots
- **WHEN** the input chunk channel closes without any chunks
- **THEN** the output message channel closes without yielding a `UIMessage`

#### Scenario: Non-writing stream emits no synthetic final snapshot
- **WHEN** the input chunk channel contains only chunks that do not produce upstream write-point snapshots
- **THEN** the output message channel closes without yielding a synthetic final `UIMessage`

#### Scenario: Output closes after input closes
- **WHEN** the input chunk channel closes without a malformed state transition
- **THEN** the output message channel closes after all upstream write-point snapshots have been sent
- **AND** if at least one snapshot was emitted, the last received message snapshot is the latest assembled state produced by those write points

### Requirement: StreamUIMessage snapshots are immutable to consumers
Each value sent by `StreamUIMessage` SHALL be an isolated snapshot of the current message state. Mutating a previously received `UIMessage`, its `Parts`, raw JSON payloads, provider metadata, or nested approval values SHALL NOT mutate later snapshots or the reader's internal state.

#### Scenario: Earlier snapshot mutation does not affect later snapshot
- **WHEN** a caller receives a text snapshot from `StreamUIMessage` and mutates its parts before the next chunk is processed
- **THEN** the next snapshot reflects only the stream chunks and not the caller's mutation

#### Scenario: Raw JSON and metadata are copied
- **WHEN** a snapshot contains tool input, tool output, data payload, message metadata, provider metadata, or approval metadata
- **THEN** those JSON and map values are copied so caller mutation of one snapshot cannot alias another snapshot

### Requirement: AssembleUIMessage blocks and returns the final message
`AssembleUIMessage` SHALL expose the signature `func AssembleUIMessage(stream <-chan UIMessageChunk, opts ...UIMessageReaderOption) (UIMessage, error)`. It SHALL consume the chunk channel until it closes, apply the same strict message state transitions as `StreamUIMessage`, and return the final assembled message after all chunks have been applied. This final state MAY include state mutations that occur after the last progressive write-point snapshot.

#### Scenario: Blocking assembly matches the last snapshot when no later non-writing mutations occur
- **WHEN** a chunk stream would make `StreamUIMessage` yield one or more snapshots
- **AND** no later non-writing state-mutating chunks occur after the final progressive write-point snapshot
- **THEN** `AssembleUIMessage` returns a message equal to the final state represented by the last snapshot that `StreamUIMessage` would have emitted for the same chunks and reader options
- **AND** the returned error is nil

#### Scenario: Blocking assembly includes later non-writing state mutations
- **WHEN** a chunk stream makes `StreamUIMessage` emit a text snapshot and then receives a non-writing state-mutating chunk such as `ChunkStartStep` before the channel closes
- **THEN** `StreamUIMessage` does not emit a synthetic final snapshot for the `ChunkStartStep` chunk or for channel close
- **AND** `AssembleUIMessage` returns the final assembled assistant message including the corresponding `StepStartPart`
- **AND** the returned error is nil

#### Scenario: Blocking assembly handles no-snapshot stream
- **WHEN** a chunk stream closes after zero or more chunks that do not produce progressive write-point snapshots
- **THEN** `AssembleUIMessage` returns the final assembled assistant message state for those chunks
- **AND** the returned error is nil

#### Scenario: Empty stream returns generated assistant message
- **WHEN** `AssembleUIMessage` receives an input channel that closes without any chunks
- **THEN** it returns an assistant `UIMessage` with a generated ID and no parts
- **AND** the returned error is nil

#### Scenario: Stream error chunk returns error
- **WHEN** `AssembleUIMessage` receives a `ChunkError` chunk
- **THEN** it returns a non-nil error describing the stream error
- **AND** it does not silently report the stream as successfully assembled

#### Scenario: Invalid chunk order returns error
- **WHEN** `AssembleUIMessage` receives a stateful chunk that references a missing active part or missing tool call, such as text delta before text start or tool output before tool input
- **THEN** it returns a non-nil error describing the invalid chunk sequence

### Requirement: ReadUIMessageStream is replaced
`ReadUIMessageStream` SHALL be removed from the public root `aisdk` API. Callers that need progressive updates SHALL use `StreamUIMessage`. Callers that need one final assembled message SHALL use `AssembleUIMessage`.

#### Scenario: Old helper is not exported
- **WHEN** code in this repository refers to `ReadUIMessageStream` after the implementation
- **THEN** it fails to compile until migrated to `StreamUIMessage` or `AssembleUIMessage`

#### Scenario: User-facing docs show only replacement helpers
- **WHEN** user-facing docs describe consuming a `UIMessageChunk` stream
- **THEN** they show `StreamUIMessage` for progressive consumption or `AssembleUIMessage` for blocking final assembly
- **AND** they do not document `ReadUIMessageStream` as a supported or deprecated API

### Requirement: UI chunk wire format is unchanged
The reader split SHALL NOT change `UIMessageChunk` JSON serialization, SSE event formatting, or `WriteUIMessageStream` behavior.

#### Scenario: Serialized chunk output is unchanged
- **WHEN** existing tests serialize UI message chunks or format SSE events
- **THEN** the serialized chunk JSON and SSE framing are unchanged by the new reader helpers

#### Scenario: Frontend hook compatibility is preserved
- **WHEN** upstream frontend hooks consume Go-produced UI message streams
- **THEN** the wire protocol remains compatible with the registered `@ai-sdk/react` baseline
