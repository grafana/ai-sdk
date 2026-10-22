# gateway-anthropic-messages Specification

## Purpose

Define the supported Anthropic Messages API subset exposed by the Gateway over shared authentication, model resolution, provider execution, and telemetry.

## Requirements

### Requirement: Authenticated Anthropic Messages endpoint

The Gateway SHALL expose `POST /v1/messages` on the private listener through its configured request authenticator and model catalog, requiring configured-account access. Cloud BYOK and operational listeners SHALL NOT expose this endpoint. Anthropic SDK bearer authentication and `x-api-key` authentication SHALL use the same configured Gateway verifier and SHALL NOT be forwarded as provider credentials. Conflicting credential sources SHALL fail before body processing or model execution.

#### Scenario: Authenticated request

- **WHEN** an official Anthropic SDK sends a valid Gateway credential and a valid Messages request
- **THEN** the shared Gateway identity SHALL be used for model resolution and provider execution
- **AND** the incoming credential SHALL NOT replace configured upstream provider credentials

#### Scenario: Listener separation

- **WHEN** a client sends a Messages request to the Cloud BYOK or operational listener
- **THEN** that listener SHALL return not found without accessing the configured catalog or invoking a provider

#### Scenario: Invalid authentication

- **WHEN** credentials are missing, invalid, duplicated, or conflicting
- **THEN** the Gateway SHALL return a fixed Anthropic-shaped authentication error before model resolution or provider invocation

### Requirement: Text and client-tool request mapping

The endpoint SHALL map system text, ordered user/assistant text turns, function tools, tool choice, assistant tool-use blocks, and user tool results into canonical provider calls. It SHALL preserve supported generation options and tool metadata. Unrepresentable requested semantics SHALL produce a fixed unsupported-request error instead of silent loss. Decoding SHALL use ordinary Go JSON behavior without closed-object validation or namespace/key normalization.

#### Scenario: Client tool continuation

- **WHEN** a request contains previous assistant `tool_use` blocks and corresponding user `tool_result` blocks
- **THEN** the adapter SHALL preserve their IDs, names, inputs, outputs, and message order in the canonical provider prompt
- **AND** the Gateway SHALL leave tool execution to the client

#### Scenario: Unsupported requested semantics

- **WHEN** a request uses a content block or capability outside the documented subset
- **THEN** the Gateway SHALL return a fixed unsupported-request error without invoking a provider

#### Scenario: Native tool examples and empty results

- **WHEN** a client supplies native tool input examples or a tool result with omitted content
- **THEN** the mapper SHALL preserve each complete example object and represent the omitted result as empty text

### Requirement: Unary response semantics

For non-streaming requests, the Gateway SHALL return an Anthropic message response with assistant text and client tool-use content blocks, truthful provider stop reason, and provider-reported input/output usage. It SHALL NOT infer nonzero usage when the provider does not supply it.

#### Scenario: Tool call response

- **WHEN** provider execution returns one or more client function calls
- **THEN** the response SHALL represent them as Anthropic `tool_use` blocks with their IDs, names, and JSON inputs
- **AND** the stop reason SHALL be `tool_use`

#### Scenario: Precise tool input

- **WHEN** the provider returns a valid tool input object containing JSON numbers
- **THEN** the unary response SHALL preserve the raw numeric values without converting them through floating-point values

#### Scenario: Cached input usage

- **WHEN** the provider reports uncached and cached input counts
- **THEN** Anthropic `input_tokens` SHALL contain the uncached count and cache counts SHALL be reported separately

#### Scenario: Provider usage unavailable

- **WHEN** provider execution supplies no token usage
- **THEN** returned token counts SHALL be zero rather than fabricated estimates

### Requirement: Incremental Messages SSE lifecycle

For streaming requests, the Gateway SHALL emit valid Anthropic SSE events incrementally, including message start, content block lifecycle, message delta, and message stop on successful completion. It SHALL map text and partial tool JSON deltas while preserving order. Provider errors after stream commitment SHALL produce an error event without a successful message stop. Request cancellation SHALL cancel provider execution and SHALL NOT synthesize successful completion.

#### Scenario: Incremental text stream

- **WHEN** provider text parts arrive over time
- **THEN** corresponding Anthropic content deltas SHALL be flushed as they arrive
- **AND** successful completion SHALL emit message delta and message stop in protocol order

#### Scenario: Stream metadata and final usage

- **WHEN** a stream-start marker precedes response metadata or usage is supplied only at completion
- **THEN** the adapter SHALL preserve the available response identity before content and include final cumulative input, output, and cache counts in the message delta

#### Scenario: Error after commitment

- **WHEN** provider execution fails after response headers or SSE data are committed
- **THEN** the stream SHALL report an Anthropic error event
- **AND** it SHALL NOT report normal completion or retry another candidate

### Requirement: Fixed protocol errors and bounded ownership

The adapter SHALL use fixed Anthropic-shaped public errors that do not disclose provider internals. Request structural checks SHALL belong to ordinary JSON decoding or the exact mapping information consumed by the adapter. The adapter SHALL rely on shared Gateway resource policies when they exist and SHALL NOT add adapter-local byte, cardinality, or output-size budgets. Documentation SHALL identify any relevant resource limit that the shared Gateway policy does not currently enforce.

#### Scenario: Malformed or unmappable request

- **WHEN** JSON is malformed or required consumed mapping data is unusable
- **THEN** the Gateway SHALL return a fixed invalid-request error before provider invocation

#### Scenario: Resource policy boundary

- **WHEN** a shared Gateway request or upstream response resource limit applies
- **THEN** the shared boundary SHALL enforce it using the Gateway's existing failure handling
- **AND** any missing shared limit relevant to this endpoint SHALL be documented

### Requirement: Official-client protocol conformance evidence

The Gateway SHALL have protocol conformance checks using pinned official Anthropic Go and TypeScript clients against the real local command. Replayed provider inputs SHALL retain their provenance. Synthetic boundary cases SHALL be identified separately.

#### Scenario: Recorded and imported fixture replay

- **WHEN** each official SDK consumes the same existing provider fixture directly and through the Gateway
- **THEN** the supported message content, identity, stop reason and token usage SHALL agree
- **AND** Gateway SSE events SHALL obey the message/content lifecycle

#### Scenario: Synthetic boundary behavior

- **WHEN** a controlled provider delays completion or fails before or after response commitment
- **THEN** tests SHALL verify incremental delivery and fixed errors without falsely successful completion
