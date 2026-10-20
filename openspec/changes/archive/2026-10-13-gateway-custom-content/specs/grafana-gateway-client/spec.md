# Spec Delta

## MODIFIED Requirements

### Requirement: Incremental bounded SSE consumption

A successful streaming setup SHALL require SSE media type and return a `StreamResult` whose request body and response headers are client-owned. One goroutine SHALL own the body, parse incrementally under configured cumulative-byte, complete-event-byte, and event-count limits, send mapped parts with context-aware backpressure, close the body, and close the output channel exactly once. It SHALL not buffer the full response or an unbounded line/event.

#### Scenario: Text stream completes
- **WHEN** the server emits valid start, metadata, sequential text parts, safe errors, and finish frames
- **THEN** the client SHALL deliver every accepted part in order, including required empty deltas and finish, then close the channel

#### Scenario: Consumer is slow
- **WHEN** result delivery blocks and the call context is canceled
- **THEN** the stream owner SHALL stop the blocked send, close the response body, close the channel, and leave no client-owned goroutine reading the stream

#### Scenario: SSE resource limit is crossed
- **WHEN** cumulative bytes, complete event bytes, or event count exceeds its configured limit
- **THEN** the client SHALL stop reading, emit at most one bounded protocol error when delivery remains possible, and close all owned resources

#### Scenario: Unsupported response family is received
- **WHEN** the client receives file, approval or another unsupported later-package stream part
- **THEN** it SHALL produce an explicit protocol error rather than decoding through provider-domain JSON accidentally

#### Scenario: Hosted dynamic call with preview results
- **WHEN** valid tool input, a provider-owned dynamic call, preliminary results and a final result arrive before finish
- **THEN** all parts and enabled markers SHALL be delivered in order, with false Dynamic/Preliminary pointer presence and input-start dynamic presence retained and final-event-before-EOF behavior unchanged

#### Scenario: Input-start dynamic is presence-sensitive
- **WHEN** otherwise equivalent input-start events omit dynamic or contain false or true
- **THEN** the client SHALL preserve nil, false and true respectively for subsequent core inference

#### Scenario: Deferred result is consumed
- **WHEN** the current request carries an unresolved provider-owned call in history and the response contains only its result and finish
- **THEN** the client SHALL deliver the success or error result without demanding a repeated call or adding a result-level providerExecuted wire member

### Requirement: Independent opaque stream metadata consumption

The Grafana client SHALL independently decode providerMetadata on supported text/reasoning start/delta/end, reasoning-file, source, tool-input start/delta/end, function call, basic result, custom and finish parts. It SHALL preserve namespace objects, nested values, original event order and absent versus empty presence without IncludeRawChunks. Original accepted events, retained copies and metadata cardinality SHALL remain bounded by the existing event, cumulative byte and count limits.

#### Scenario: Unknown namespace survives all supported placements
- **WHEN** valid events carry future object-valued namespaces with nested null/false/zero/empty values and explicit empty metadata at supported positions
- **THEN** the independent Go client SHALL preserve those values and positions under existing limits without importing server code

#### Scenario: Malformed metadata on finish
- **WHEN** a bounded finish contains a null namespace or malformed metadata shape
- **THEN** the client SHALL emit at most one bounded protocol PartError and close without delivering the invalid finish or metadata-free success

## ADDED Requirements

### Requirement: Custom content consumption
The client SHALL decode unary `custom` content into provider custom content and stream `custom` events into custom stream parts, retaining kind, order and opaque providerMetadata presence. A missing or non-string kind SHALL fail through the existing bounded protocol-error paths. Unrelated fields SHALL be ignored.

#### Scenario: Unary custom content
- **WHEN** a unary success contains text followed by custom content `openai.compaction` with metadata
- **THEN** the result SHALL contain both parts in order with the kind and metadata unchanged

#### Scenario: Streamed custom event
- **WHEN** a stream contains a custom event between text deltas, with and without metadata
- **THEN** the client SHALL deliver a custom part in order with the kind preserved and metadata absent or retained as supplied

#### Scenario: Malformed custom kind
- **WHEN** unary or streamed custom content has a missing or non-string kind
- **THEN** unary decoding SHALL fail explicitly and streaming SHALL emit at most one bounded protocol PartError

### Requirement: Custom request projection is unchanged
The client SHALL continue projecting assistant custom parts with dotted kinds and custom tool-result entries with their scoped options, matching the registered client's request bodies.

#### Scenario: Custom request bodies match
- **WHEN** equivalent custom history and custom tool-result content are sent through both clients
- **THEN** their semantic request bodies SHALL be equal
