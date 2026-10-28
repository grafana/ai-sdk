# gateway-openai-chat-completions-adapter Specification

## Purpose
Define the represented OpenAI Chat Completions codec over shared Gateway execution.
#483 shared lifecycle validation remains pending before readiness.

## Requirements

### Requirement: Explicit unary request subset

The adapter SHALL validate a closed schema before typed mapping. It SHALL
represent text, client-owned functions and continuations, structured response
formats, reasoning effort and supported sampling settings. Unsupported fields,
multiple choices, storage true and invalid settings SHALL fail before model
resolution. Duplicate keys SHALL use their last value; incorrectly cased keys
SHALL fail.

#### Scenario: Duplicate and incorrectly cased fields
- **WHEN** a request repeats the model key or includes `Model` instead
- **THEN** the repeated exact key uses its last value and the incorrectly cased
  key fails validation.

#### Scenario: Function continuation
- **WHEN** history contains an assistant function call followed by its matching
  tool result
- **THEN** the adapter maps the result to that function's name and call ID;
  an orphan result is refused.

#### Scenario: Structured response name and restricted schema
- **WHEN** a supported JSON schema response format supplies its required name
- **THEN** the mapped provider response format preserves the name's presence;
  external references or unsupported schema vocabulary are refused.

### Requirement: Candidate-local Chat defaults

The adapter SHALL supply an execution.Translator per call, copying options and
mapping Chat storage, strictness and explicit parallel-tool settings for the
host-provided native target. Responses and compatible attempts SHALL receive
storage false; omitted/null Chat strict SHALL mean false. The host SHALL
apply translation separately to physical attempts, re-check policy and native
options, and preserve calls without a translator.

#### Scenario: Backend-specific parallel settings
- **WHEN** parallel tool calls are explicitly disabled
- **THEN** translation supplies Responses `parallelToolCalls:false`, Anthropic
  `disableParallelToolUse:true`, or compatible `parallel_tool_calls:false`
  under the target namespace without mutating source options.

#### Scenario: Another protocol uses the model
- **WHEN** a call supplies no Chat translator
- **THEN** the host preserves its options without applying Chat defaults.

### Requirement: Valid unary results and diagnostics

Unary output SHALL contain one choice, a represented finish reason and truthful
optional usage. Available native identity SHALL replace generated fallbacks.
Supported warnings SHALL remain successful `grafana.warnings` diagnostics.
Unsupported content or inconsistent tool output SHALL fail safely. Strict
structured output SHALL validate on successful stop, not length or
content-filter partial output.

#### Scenario: Native identity is absent
- **WHEN** the result omits native response ID, model ID or timestamp
- **THEN** the adapter supplies each missing field from a generated completion
  ID, the canonical catalog model ID or current time respectively.

#### Scenario: Native identity and warnings
- **WHEN** a valid result provides native response metadata and an unsupported
  setting warning
- **THEN** the response retains that identity and warning without turning the
  otherwise successful result into an error.

#### Scenario: Strict structured output
- **WHEN** a successful stop returns invalid JSON or violates an explicitly
  strict response schema
- **THEN** the adapter returns a fixed upstream error instead of a successful
  completion; partial output ending for length is not required to validate.

### Requirement: Host-owned resources and safe failures

The adapter SHALL obtain a Call from execution.Runtime and use Call.Generate
for unary execution. Selection, policy, execution, cancellation and failure
classification SHALL belong to the shared runtime; Chat SHALL map its closed
categories to fixed OpenAI documents. The adapter SHALL add no private
selection/invocation worker, drain, total deadline or transport-byte cap and
SHALL NOT copy raw provider errors, bodies or headers into its errors.

#### Scenario: No adapter-imposed size or duration
- **WHEN** the test host supplies a request and response above the old one MiB
  bound and supplies no deadline
- **THEN** the adapter adds no size cap or deadline and delegates execution to
  the same runtime contract as other adapters.

#### Scenario: Request deadline during generation
- **WHEN** the request deadline expires during cooperative generation
- **THEN** the provider receives the host context and the runtime outcome maps
  to the fixed timeout or cancellation category without resetting a deadline.

#### Scenario: Safe upstream failure
- **WHEN** the runtime classifies a provider failure containing private text
- **THEN** Chat maps its category to a fixed document without copying that text.

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

### Requirement: Response writers without socket deadlines

The handler SHALL flush SSE through the response controller and accept a
writer that does not support socket write deadlines. Where supported, write
deadlines SHALL honor the host request deadline when present and react
to cancellation. Unsupported write deadlines SHALL NOT alone reject a valid
stream; host transport cancellation remains required for plugin writers.

#### Scenario: Flush-capable writer without deadline support
- **WHEN** the response writer supports flushing but reports unsupported write
  deadlines
- **THEN** a valid stream is flushed and completes with `[DONE]`.

#### Scenario: Supported socket deadlines
- **WHEN** the writer supports socket deadlines
- **THEN** the handler retains host deadline and cancellation behavior without adding or
  clearing a host timeout.

### Requirement: Represented Anthropic start metadata

The adapter SHALL discard a custom part whose kind is exactly
`anthropic.message_start` without emitting its raw payload or treating it as
represented progress. Canonical response metadata SHALL remain the source of
native identity. Other custom kinds SHALL continue to fail safely without a
successful stream terminator.

#### Scenario: Anthropic start metadata beside canonical identity
- **WHEN** canonical response metadata and an `anthropic.message_start` custom
  part precede valid text and finish
- **THEN** the adapter preserves native identity, emits the text and completes
  with `[DONE]` without exposing the custom payload.

#### Scenario: Unknown custom kind
- **WHEN** a part uses `anthropic.other` or another provider's custom kind
- **THEN** the adapter emits a fixed error where writable and omits `[DONE]`.
