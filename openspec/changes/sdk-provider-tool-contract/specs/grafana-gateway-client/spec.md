## MODIFIED Requirements

### Requirement: Bounded normalized unary consumption
For a successful unary response, the client SHALL require a JSON media type, read no more than the configured unary-response byte limit, accept one complete JSON document, and explicitly map registered text, supported function/provider calls and provider results, finish reason and usage into `provider.GenerateResult`. Execution, dynamic and preliminary markers SHALL normalize absent/false to disabled and preserve true on their registered arms. Supported tool metadata SHALL be mapped independently of server DTOs. The client SHALL reject malformed required fields, null output results, unknown content or finish discriminators, negative or non-JavaScript-safe known usage, trailing JSON and oversized input. It SHALL replace server-supplied request and response: Request.Body SHALL be the locally encoded request and Response.Headers and Response.Body SHALL come from the bounded HTTP response. Warnings SHALL preserve valid server warning fields in order using the registered streaming warning validation, defaulting to a non-nil empty slice when absent or null. Server response identity and private metadata SHALL not be adopted; reviewed per-tool metadata SHALL follow sdk-provider-tools.

#### Scenario: Minimal unary success is consumed
- **WHEN** the server returns valid ordered text content, registered finish reason and valid usage
- **THEN** the client SHALL return those fields plus local request metadata, HTTP response headers/body and empty warnings when none were supplied

#### Scenario: Server supplies client-owned fields
- **WHEN** a successful body includes warnings, request metadata, response ID, model ID, timestamp, headers, body or arbitrary additional provider metadata
- **THEN** the result SHALL preserve valid warnings and reviewed tool-part metadata but ignore other server-owned values in favor of client-owned replacements

#### Scenario: Unary response exceeds its bound
- **WHEN** the response is one byte larger than the configured unary limit or has trailing data
- **THEN** the call SHALL fail without returning a partial result or retaining an unbounded body

#### Scenario: Unary result is not in the WP5 text family
- **WHEN** a successful body contains an output discriminator not owned by the supported text/function/provider-tool families
- **THEN** the client SHALL fail explicitly until that capability extends the closed mapper

#### Scenario: Unary warning is malformed
- **WHEN** a warning has an unknown discriminator or lacks a required field
- **THEN** the call SHALL fail rather than exposing unvalidated warning content

#### Scenario: Provider-owned unary result
- **WHEN** ordered content contains a hosted call and its result
- **THEN** the client SHALL preserve call ownership, IDs, names, input, result and supported markers/metadata without requiring or emitting a result-level providerExecuted wire member

### Requirement: Incremental bounded SSE consumption
A successful streaming setup SHALL require SSE media type and return a `StreamResult` whose request body and response headers are client-owned. One goroutine SHALL own the body, parse incrementally under configured cumulative-byte, complete-event-byte and event-count limits, send mapped parts with context-aware backpressure, close the body and close the output channel exactly once. It SHALL not buffer the full response or an unbounded line/event. The mapper SHALL accept supported text, function-tool and provider-tool calls/results, safe error parts and bounded raw parts needed for registered filtering behavior. Supported execution/dynamic/preliminary markers and reviewed tool metadata SHALL be preserved. Input-start dynamic SHALL retain absent, explicit false and true independently through decoding; it SHALL NOT be eagerly defaulted. A deferred result SHALL NOT require a repeated call in the same response. Every unsupported, malformed or oversized event SHALL emit at most one terminal non-retryable protocol PartError and close. Server lifecycle validation SHALL NOT be imported into the client as a new independent protocol dialect.

#### Scenario: Text stream completes
- **WHEN** the server emits valid start, metadata, sequential text parts, safe errors and finish frames
- **THEN** the client SHALL deliver every accepted part in order, including required empty deltas and finish, then close the channel

#### Scenario: Consumer is slow
- **WHEN** result delivery blocks and the call context is canceled
- **THEN** the stream owner SHALL stop the blocked send, close the response body, close the channel and leave no client-owned goroutine reading the stream

#### Scenario: SSE resource limit is crossed
- **WHEN** cumulative bytes, complete event bytes or event count exceeds its configured limit
- **THEN** the client SHALL stop reading, emit at most one bounded protocol error when delivery remains possible and close all owned resources

#### Scenario: Unsupported response family is received
- **WHEN** the client receives reasoning, file, source, custom, approval or another unsupported later-package stream part
- **THEN** it SHALL produce an explicit protocol error rather than decoding through provider-domain JSON accidentally

#### Scenario: Hosted dynamic call with preview results
- **WHEN** valid tool input, a provider-owned dynamic call, preliminary results and a final result arrive before finish
- **THEN** all parts and enabled markers SHALL be delivered in order, with equivalent absent/false markers normalized, input-start dynamic presence retained and final-event-before-EOF behavior unchanged

#### Scenario: Input-start dynamic is presence-sensitive
- **WHEN** otherwise equivalent input-start events omit dynamic or contain false or true
- **THEN** the client SHALL preserve nil, false and true respectively for subsequent core inference

#### Scenario: Deferred result is consumed
- **WHEN** the current request carries an unresolved provider-owned call in history and the response contains only its result and finish
- **THEN** the client SHALL deliver the success or error result without demanding a repeated call or adding a result-level providerExecuted wire member
