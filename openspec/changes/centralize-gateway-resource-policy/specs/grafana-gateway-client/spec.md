# Spec Delta

## MODIFIED Requirements

### Requirement: Bounded unary document and required payload consumption

Required payload checks and opaque metadata structure SHALL remain unchanged. For successful unary responses, the client SHALL require JSON media type, read within its per-response limit, accept one complete valid document and map its supported registered content, including provider calls and results, finishReason, usage, warnings and ordinary result/content providerMetadata into provider.GenerateResult.

#### Scenario: Bounded unary document and required payload consumption
- **WHEN** a successful response has valid ordered content and ordinary metadata at the configured response byte limit
- **THEN** the client SHALL consume one complete JSON document into GenerateResult with required payload, metadata and transport bounds intact

### Requirement: Successful unary transport read failures retain retryability

When a successful HTTP 200 `DoGenerate` response body fails to read because of transport I/O after headers, the Grafana client SHALL return a retryable `*provider.APICallError` with HTTP status 200, a bounded locally worded primary message, and a discoverable underlying read cause, without returning a partial result or making another model request. Wrong media type, malformed JSON, malformed consumed fields, and response byte-limit violations SHALL remain non-retryable protocol failures.

#### Scenario: HTTP 200 response body is interrupted after headers
- **WHEN** the real HTTP handler sends JSON headers and a partial, below-limit body with a declared longer `Content-Length`, then closes the connection before the body completes
- **THEN** `DoGenerate` SHALL return no result and one retryable `*provider.APICallError` with status 200, a locally bounded primary message that does not disclose the response body, and the underlying body-read failure discoverable through its cause chain, after exactly one model request

#### Scenario: Successful unary body cannot be accepted for protocol reasons
- **WHEN** a successful unary response has a wrong media type, malformed JSON, invalid consumed result fields, or exceeds the configured response byte limit, including when an over-limit partial body and a transport read error occur together
- **THEN** `DoGenerate` SHALL return no partial result and a non-retryable bounded protocol error after exactly one model request; the response byte limit SHALL take precedence over the read error, except that context cancellation or deadline expiry SHALL retain highest precedence

#### Scenario: Unary read is canceled
- **WHEN** the call context is canceled or reaches its deadline while a successful unary response body is being read
- **THEN** `DoGenerate` SHALL preserve context error identity through `errors.Is`, SHALL NOT return a retryable transport error, and SHALL NOT replay the request

#### Scenario: Stream parts precede a transport failure
- **WHEN** an established `DoStream` response delivers one or more parts and then its transport fails
- **THEN** the client SHALL NOT issue another HTTP request or replay any previously delivered part

### Requirement: SSE protocol failure without server lifecycle dialect

Every unsupported or malformed event, and any stream that exceeds the response byte limit, SHALL emit at most one terminal non-retryable protocol PartError and close. Server lifecycle validation SHALL NOT be imported into the client as a new independent protocol dialect.

#### Scenario: SSE protocol failure without server lifecycle dialect
- **WHEN** an event has an unknown discriminator or the stream exceeds its configured response byte limit
- **THEN** the client SHALL emit at most one terminal non-retryable protocol PartError and close without importing server lifecycle validation

### Requirement: Optional bounded provider raw usage consumption

On a successful unary result and a streaming finish, the Grafana client SHALL preserve a supplied `usage.raw` as a `provider.Usage.Raw` JSON object, including nested provider-native fields and `{}`; absent raw SHALL remain absent. The client SHALL reject present null, scalar, array, malformed or incomplete JSON. It SHALL NOT apply a raw-usage-specific size cap; the per-response limit bounds the containing document.

#### Scenario: Unary and finish have present or absent raw
- **WHEN** a bounded valid unary result or finish contains nested raw usage, `{}`, or no raw member
- **THEN** the resulting Go usage SHALL preserve the supplied JSON object semantics, retain an empty object when supplied, or leave `Raw` absent while preserving validated normalized counts

#### Scenario: Raw usage larger than the former cap
- **WHEN** a unary result or finish contains a valid `usage.raw` object larger than 1,048,576 bytes within the response limit
- **THEN** the client SHALL retain it in `provider.Usage.Raw`

#### Scenario: Hostile unary response contains invalid raw
- **WHEN** a successful HTTP 200 unary response contains malformed, null or non-object `usage.raw`
- **THEN** the client SHALL return a bounded non-retryable protocol error without any partial result or raw data in its error text

#### Scenario: Hostile streaming finish contains invalid raw
- **WHEN** a streaming finish contains malformed, null or non-object `usage.raw`
- **THEN** the client SHALL emit at most one bounded terminal non-retryable protocol `PartError` and close, without delivering the invalid finish

#### Scenario: UTF-8 and JSON escapes in both paths
- **WHEN** bounded unary or streaming finish JSON contains invalid UTF-8 in `usage.raw`, including a nested key or value
- **THEN** the client SHALL reject the response with the path's bounded protocol error, without retaining normalized replacement characters in raw usage
- **WHEN** `usage.raw` contains valid JSON with lone or paired escaped surrogates
- **THEN** both paths SHALL accept the object under the same shape rules and preserve its raw JSON escapes

#### Scenario: Raw part filtering does not erase usage
- **WHEN** a finish includes `usage.raw` and `IncludeRawChunks` is false
- **THEN** the finish SHALL retain its raw usage even when independent `type: "raw"` stream parts are filtered

### Requirement: Raw usage escape preservation and bounded intermediates
Valid JSON with lone or paired escaped surrogates SHALL be accepted, with raw-object escapes preserved in `provider.Usage.Raw`. The per-response byte limit SHALL still apply, and intermediate usage copies SHALL remain bounded by the containing response or stream.

#### Scenario: Raw usage escape preservation and bounded intermediates
- **WHEN** usage.raw contains a lone escaped UTF-16 surrogate in a valid object
- **THEN** the escape SHALL be accepted and retained within the per-response bound

### Requirement: Closed Gateway error classification

Every non-2xx model or discovery response SHALL be read within that route's configured response limit and mapped only from the registered public error envelope into the closed categories authentication, forbidden, invalid request, model not found, rate limit, failed dependency, and internal server. The resulting `GatewayError` SHALL expose category, public code, public message, HTTP status, and status-derived retryability and SHALL unwrap to a bounded `*provider.APICallError`.

#### Scenario: Registered error is returned
- **WHEN** the server returns one of the registered error type/status/code documents
- **THEN** `errors.As` SHALL find the matching `GatewayError` category and underlying `*provider.APICallError`, and retryability SHALL match the Gateway version registered in test/conformance/upstream.yaml, independently of nested native retry facts

#### Scenario: Error members use noncanonical property casing
- **WHEN** an otherwise valid HTTP error envelope or committed error payload uses case-insensitive Go matches for its member names
- **THEN** typed error decoding SHALL accept those members using standard encoding/json behavior
- **AND** retained HTTP documents and selected SSE data SHALL preserve their original JSON bytes without changing classification or retryability rules

#### Scenario: Error body is malformed or oversized
- **WHEN** a non-2xx body is malformed, has a wrong media type, or exceeds the route's response limit
- **THEN** the client SHALL return a bounded local `*provider.APICallError` without accepting a partial category or exposing the full body in its message

#### Scenario: Large classified error envelope
- **WHEN** a model-route error envelope with execution evidence exceeds the former 64 KiB error limit but fits the response limit
- **THEN** the client SHALL classify it and retain its complete data

#### Scenario: Transport fails
- **WHEN** HTTP transport fails without context cancellation
- **THEN** the client SHALL return a retryable `*provider.APICallError` that retains the transport cause

#### Scenario: Stream error event is received
- **WHEN** a valid registered error event appears in SSE
- **THEN** it SHALL become an ordered `PartError` with the equivalent bounded `APICallError`, preserving the Go provider contract without ending later valid parts by itself
- **AND** supplied valid error.data SHALL be retained exactly in APICallError.Data, independently of mapped status/retryability, without manufacturing data when absent
- **AND** APICallError.ResponseBody SHALL remain empty rather than manufacturing an HTTP response document from the SSE event

#### Scenario: All-failed data survives classification
- **WHEN** a valid registered non-2xx envelope contains attributed attempts and heterogeneous native errors
- **THEN** errors.As SHALL expose the existing GatewayError and API-call cause with complete envelope Data, while outer status/code/category and hop retryability remain unchanged

#### Scenario: Consumer middleware inspects stream data
- **WHEN** a consumer wrapper reads an attributed PartError followed by valid text and finish
- **THEN** it SHALL observe the retained data and original event order without enabling operator capture or changing core result/error policy

### Requirement: Exact-pinned differential and black-box evidence

Tests SHALL compare Go and the exact Gateway version registered in test/conformance/upstream.yaml for semantic method/path/headers/body, supported result normalization, errors/retryability, cancellation, discovery, DONE, raw filtering, timestamp conversion and EOF. Native-value cases SHALL cover all warning variants/order/required empties, URL/document IDs/display/order and optional native stream identity.

#### Scenario: Equivalent text calls are compared
- **WHEN** the differential suite issues representable unary and streaming text/scalar calls through both clients
- **THEN** their semantic requests and normalized observable results SHALL agree except for documented parity-preserving Go adaptations

#### Scenario: Registered baseline changes
- **WHEN** the Gateway or provider package pin changes
- **THEN** baseline validation SHALL fail until differential evidence and every observed divergence are reviewed and updated together

#### Scenario: Client bounds are tested
- **WHEN** fake endpoints exercise exact-limit and one-byte-over-limit discovery, unary, error, and cumulative SSE inputs plus cancellation under blocked delivery
- **THEN** tests SHALL prove bounded allocation/read behavior, body closure, channel closure, and absence of retained client goroutines

#### Scenario: Authenticated command is exercised
- **WHEN** the repository integration suite runs the protocol test host with test-signed platform tokens and provider fakes serving supported native values
- **THEN** both clients SHALL preserve contracted values while configured credentials and cross-tenant state remain absent
- **AND** the Go client SHALL complete discovery, unary text, streaming text, cancellation and registered errors, and pinned TS configured discovery SHALL run as a self-hosted service account
- **AND** canonical operator identity and metadata-only capture SHALL remain independent of returned native values and authorized configured discovery

#### Scenario: Configured discovery consumers are compared
- **WHEN** Go ListModels, pinned TS getAvailableModels and the shipped TS helper inspect canonical model rows with configured alias metadata
- **THEN** normalized public fields SHALL remain compatible, Go and the helper SHALL retain matching configured facts, stock TS SHALL still strip the extension and provider inference counts SHALL remain zero

#### Scenario: Server-accepted large output
- **WHEN** the protocol test host serves unary, error and streaming outputs that exceed the removed server and client sublimits but stay within the default outbound bound
- **THEN** both the registered TypeScript client and the Go client with default limits SHALL consume them

### Requirement: Incremental bounded SSE consumption
A successful streaming setup SHALL require SSE media type and return a `StreamResult` whose request body and response headers are client-owned. One goroutine SHALL own the body, parse incrementally under the configured cumulative per-response byte limit, send mapped parts with context-aware backpressure, close the body, and close the output channel exactly once. It SHALL not buffer the full response, and no single line or event SHALL grow beyond the cumulative limit.

#### Scenario: Text stream completes
- **WHEN** the server emits valid start, metadata, sequential text parts, safe errors, and finish frames
- **THEN** the client SHALL deliver every accepted part in order, including required empty deltas and finish, then close the channel

#### Scenario: Consumer is slow
- **WHEN** result delivery blocks and the call context is canceled
- **THEN** the stream owner SHALL stop the blocked send, close the response body, close the channel, and leave no client-owned goroutine reading the stream

#### Scenario: SSE resource limit is crossed
- **WHEN** cumulative stream bytes exceed the configured response byte limit
- **THEN** the client SHALL stop reading, emit at most one bounded protocol error when delivery remains possible, and close all owned resources

#### Scenario: Large event and many events
- **WHEN** a stream contains one event larger than the former 1 MiB event limit or more than 100,000 events within the response limit
- **THEN** the client SHALL deliver every part in order

#### Scenario: Unsupported response family is received
- **WHEN** the client receives another unsupported later-package stream part
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

The Grafana client SHALL independently decode providerMetadata on supported text/reasoning start/delta/end, reasoning-file, source, tool-input start/delta/end, function call, basic result, custom and finish parts. It SHALL preserve namespace objects, nested values, original event order and absent versus empty presence without IncludeRawChunks. Accepted events and retained copies SHALL remain bounded by the cumulative per-response byte limit.

#### Scenario: Unknown namespace survives all supported placements
- **WHEN** valid events carry future object-valued namespaces with nested null/false/zero/empty values and explicit empty metadata at supported positions
- **THEN** the independent Go client SHALL preserve those values and positions under existing limits without importing server code

#### Scenario: Malformed metadata on finish
- **WHEN** a bounded finish contains a null namespace or malformed metadata shape
- **THEN** the client SHALL emit at most one bounded protocol PartError and close without delivering the invalid finish or metadata-free success

## ADDED Requirements

### Requirement: Independent client response limits
The client `Limits` SHALL contain `DiscoveryBytes` (default 4,194,304) for discovery responses and `ResponseBytes` (default 67,108,864) for model-route unary success, error and cumulative SSE bodies. Both SHALL be positive and safely representable. The defaults SHALL accept every output a default-configured Gateway emits from in-limit upstream responses. The bound exists to protect client memory independently of the server, and it is configurable.

#### Scenario: Default limits
- **WHEN** a caller reads `DefaultLimits()`
- **THEN** it SHALL return a fresh value with `DiscoveryBytes` 4,194,304 and `ResponseBytes` 67,108,864, and no event-size, event-count or error-specific field

#### Scenario: Invalid limits
- **WHEN** a caller supplies a zero, negative or unsafe value for either limit
- **THEN** construction SHALL fail before authentication or network I/O

### Requirement: Original document validity before decoding
The client SHALL validate each complete unary or error document and each complete SSE event for UTF-8 and JSON syntax before Go decoding can normalize invalid bytes. This validation SHALL NOT impose a size limit beyond the per-response bound.

#### Scenario: Invalid UTF-8 in an otherwise bounded document
- **WHEN** a unary document or SSE event contains invalid UTF-8 inside a JSON string
- **THEN** the client SHALL fail it as a protocol error without retaining U+FFFD replacements

## REMOVED Requirements

### Requirement: Retained raw-object and original-document limits
**Reason**: The 1,048,576-byte retained raw-object cap, `UnaryBytes` and `StreamEventBytes` are removed in favor of one per-response bound coherent with server policy.
**Migration**: Configure `Limits.ResponseBytes`; raw usage is validated for shape and UTF-8 only; pre-decode validity moves to "Original document validity before decoding".
