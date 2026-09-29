## MODIFIED Requirements

### Requirement: Bounded normalized unary consumption
For a successful unary response, the client SHALL require a JSON media type, read no more than the configured unary-response byte limit, accept one complete JSON document, and map only registered text/basic function-tool-call content (including eligible approved content metadata), finish reason, and usage into provider.GenerateResult. It SHALL reject malformed required fields, unknown content or finish discriminators, negative or non-JavaScript-safe known usage, trailing JSON, and oversized input. The client SHALL replace server-supplied request and response: Request.Body SHALL be the locally encoded request, and Response.Headers and Response.Body SHALL come from the bounded HTTP response. Warnings SHALL preserve valid server warning fields in order, defaulting to a non-nil empty slice when absent or null. Warning decoding SHALL use the same registered types and validation as streaming. Server response identity and top-level provider metadata SHALL not be adopted. Approved OpenAI itemId and Anthropic direct caller SHALL be validated and mapped into their respective GenerateContentPart.ProviderMetadata; unknown keys inside received content `providerMetadata` (including namespaces, namespace fields and nested `caller` members), malformed approved fields and unsupported recognized values SHALL fail the entire result. Existing treatment of unknown fields outside `providerMetadata` SHALL remain unchanged.

#### Scenario: Minimal unary success is consumed
- **WHEN** the server returns valid ordered text content, registered finish reason, and valid usage
- **THEN** the client SHALL return those fields plus local request metadata, HTTP response headers/body, and empty warnings when none were supplied

#### Scenario: Server supplies client-owned fields
- **WHEN** a successful body includes warnings, request metadata, response ID, model ID, timestamp, headers, body, or top-level provider metadata
- **THEN** the result SHALL preserve valid warnings but ignore the other server-owned values in favor of the registered client-owned replacements

#### Scenario: Approved unary metadata
- **WHEN** a supported content part contains only approved bounded `providerMetadata`
- **THEN** the approved metadata SHALL be mapped onto that same part, without adopting top-level provider metadata or server response identity

#### Scenario: Unexpected unary wire metadata key
- **WHEN** received content `providerMetadata` contains an unknown namespace or field, or a nested extra `caller` member
- **THEN** the client SHALL reject the result rather than silently drop any metadata key

#### Scenario: Unary response exceeds its bound
- **WHEN** the response is one byte larger than the configured unary limit or has trailing data
- **THEN** the call SHALL fail without returning a partial result or retaining an unbounded body

#### Scenario: Unary result is not in the WP5 text family
- **WHEN** a successful body contains an output discriminator not owned by the text client
- **THEN** the client SHALL fail explicitly until the capability's later work package extends the closed mapper

#### Scenario: Unary warning is malformed
- **WHEN** a warning has an unknown discriminator or lacks a required field
- **THEN** the call SHALL fail rather than exposing unvalidated warning content

### Requirement: Incremental bounded SSE consumption
A successful streaming setup SHALL require SSE media type and return a `StreamResult` whose request body and response headers are client-owned. One goroutine SHALL own the body, parse incrementally under configured cumulative-byte, complete-event-byte, and event-count limits, send mapped parts with context-aware backpressure, close the body, and close the output channel exactly once. It SHALL not buffer the full response or an unbounded line/event. The mapper SHALL retain the supported basic function-tool input/call/result parts alongside the WP5 text stream family, safe error parts, and bounded raw parts needed for pinned filtering parity; every unsupported, malformed (including unknown keys inside received `providerMetadata`), or oversized event SHALL emit at most one terminal non-retryable protocol `PartError` and close. Existing handling of unknown event fields outside `providerMetadata` SHALL remain unchanged.

#### Scenario: Text stream completes
- **WHEN** the server emits valid start, metadata, sequential text parts, safe errors, and finish frames
- **THEN** the client SHALL deliver every accepted part in order, including required empty deltas and finish, then close the channel

#### Scenario: Eligible metadata survives bounded SSE decoding
- **WHEN** a stream contains bounded approved OpenAI itemId on text start/end or Anthropic direct caller on basic function call/result with no other received metadata keys
- **THEN** the client SHALL map approved per-part metadata into StreamPart.ProviderMetadata, in order, without adopting response-metadata/finish

#### Scenario: Unexpected streaming wire metadata key
- **WHEN** received per-part `providerMetadata` contains an unknown namespace or field, or a nested extra `caller` member
- **THEN** the client SHALL emit at most one terminal protocol error and close, without silently dropping the key

#### Scenario: Consumer is slow
- **WHEN** result delivery blocks and the call context is canceled
- **THEN** the stream owner SHALL stop the blocked send, close the response body, close the channel, and leave no client-owned goroutine reading the stream

#### Scenario: SSE resource limit is crossed
- **WHEN** cumulative bytes, complete event bytes, or event count exceeds its configured limit
- **THEN** the client SHALL stop reading, emit at most one bounded protocol error when delivery remains possible, and close all owned resources

#### Scenario: Unsupported response family is received
- **WHEN** the WP5 text client receives a reasoning, provider-executed or dynamic tool, file, source, custom, approval, or other later-package stream part (not a supported basic function-tool input/call/result part)
- **THEN** it SHALL produce an explicit protocol error rather than decoding through provider-domain JSON accidentally
