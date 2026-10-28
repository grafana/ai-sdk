## REMOVED Requirements

### Requirement: Independent unary handler

**Reason:** The unary-only boundary and its streaming-refusal scenario are
replaced by the independent Chat handler requirement, which preserves unary
and route checks while adding streaming.

**Migration:** Existing unary callers keep their behavior. Requests with
`stream:true` now receive the represented streaming contract.

#### Scenario: Unary-only boundary is replaced
- **WHEN** the streaming capability is composed on the unary change
- **THEN** the replacement handler requirement preserves unary behavior and
  replaces the former streaming-refusal scenario.

## ADDED Requirements

### Requirement: Independent Chat handler

The AGPL Chat package SHALL construct an HTTP handler with a non-nil execution.Runtime and translate supported unary and streaming requests to the provider
domain. It SHALL accept relative POST `/v1/chat/completions` with JSON content;
`stream:true` SHALL select streaming. Handler construction SHALL NOT register a production route or supply
authentication; the host owns registration and access.

#### Scenario: Unary request through the official Go client
- **WHEN** the official OpenAI Go client calls the mounted test handler with a
  valid text request and configured model alias
- **THEN** it receives one Chat completion choice with assistant content,
  valid identity and represented usage.

#### Scenario: Streaming through the official Go client
- **WHEN** the official OpenAI Go client requests streaming with usage enabled
- **THEN** it consumes Chat chunks, one finish and one usage-only chunk without
  changing the unary contract.

#### Scenario: Handler route boundary
- **WHEN** a request uses an unsupported method, path, query, encoded path,
  content encoding or non-JSON content type
- **THEN** the handler refuses it before model invocation, with an OpenAI error
  envelope and an Allow header for the method refusal.

### Requirement: Streaming Chat representation

The handler SHALL emit SSE data frames and flush represented text and function
progress without buffering the entire response. It SHALL open with an
assistant-role chunk, emit a valid finish, optionally emit usage with empty
choices, and terminate successful output with `[DONE]`. It SHALL NOT replay
the model call after output is written. Failure SHALL omit `[DONE]` and use a
fixed Chat error where the writer remains usable.

#### Scenario: Text and optional usage
- **WHEN** the provider supplies valid text followed by finish and closure
- **THEN** the adapter emits text chunks, finish, a usage-only chunk when
  requested, and `[DONE]` in that order.

#### Scenario: Premature termination
- **WHEN** the provider stream closes before a valid finish
- **THEN** the adapter emits a fixed upstream error where writable and does not
  emit `[DONE]`.

#### Scenario: Additional native segment is refused
- **WHEN** another part, metadata, start or finish follows the first finish
  instead of channel closure
- **THEN** Chat refuses the unsupported continuation without a successful
  finish or `[DONE]`; it does not silently truncate or replay the response.

### Requirement: Streaming native identity and diagnostics

The adapter SHALL use available native identity before its first role/content
chunk and fill absent fields with unary fallbacks. Public id/model/created
SHALL remain stable after opening. Later native identity SHALL appear in
`grafana.native_response`; supported stream-start warnings SHALL appear in
`grafana.warnings` without making otherwise valid output fail.

#### Scenario: Early and late identity
- **WHEN** response metadata arrives before or after the first content
- **THEN** early metadata supplies public identity, while late metadata uses
  the extension without changing the identity of later chunks.

### Requirement: Streaming function and structured-output rules

The adapter SHALL assign each function one stable tool index, emit its initial
ID/name/type once and forward argument fragments once. Final arguments SHALL
match the represented stream and the declared tool. Chat-specific tool-choice,
parallelism, JSON argument and strict structured-output rules SHALL remain
enforced. Unsupported text-encoded function input SHALL be rejected.

#### Scenario: Fragmented and empty arguments
- **WHEN** a function arrives as start, fragments, end and matching final call
- **THEN** fragments are emitted once; an empty fragment sequence finalized as
  `{}` emits that normalization once.

#### Scenario: Contradicting final call
- **WHEN** the final function input differs from its streamed fragments
- **THEN** the adapter fails safely without a successful terminator.

### Requirement: Streaming runtime ownership

The handler SHALL use Call.Stream, Stream.Next and Stream.Close for setup,
wait outcomes, cancellation and drain. It SHALL NOT implement a private setup
worker, idle timer or drain loop. Runtime idle accounting SHALL follow native
part activity, including metadata; Chat SHALL retain framing, commitment and
its representation state. #483 shared lifecycle validation remains pending
and SHALL NOT be claimed by this change.

#### Scenario: Runtime activity owns idle
- **WHEN** native metadata or reasoning parts arrive within each runtime idle
  interval but produce no Chat text
- **THEN** the runtime activity policy applies without a second Chat-visible
  progress timer; the host total deadline remains effective.

#### Scenario: Cleanup through the runtime
- **WHEN** streaming setup fails or the handler ends after acquiring a stream
- **THEN** runtime setup handling and Stream.Close own provider cancellation
  and bounded drain without a second consumer in Chat.

#### Scenario: No duplicate transport bounds
- **WHEN** valid output contains a large frame or many parts
- **THEN** Chat restores no frame, total-byte or part-count limits and uses
  shared runtime waits and host context.
