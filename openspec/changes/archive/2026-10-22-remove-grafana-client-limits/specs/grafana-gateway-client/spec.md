# Spec Delta

## ADDED Requirements

### Requirement: Server-owned resource bounds and buffered-read backstop

The client SHALL expose no configurable response-processing limits. The Gateway SHALL own resource bounds, and callers SHALL bound work through their `http.Client` and call context. Every buffered read SHALL share one fixed 2 GiB backstop. This covers unary success, non-2xx error bodies including stream setup failures, and discovery. Exceeding the backstop SHALL fail without truncation. This is a parity-preserving match of the registered upstream download limit.

#### Scenario: Limit configuration is absent
- **WHEN** exported declarations and constructor configuration fields are enumerated
- **THEN** no limit type, default-limit function or per-config limit field SHALL exist

#### Scenario: Buffered body crosses the backstop
- **WHEN** a unary, error or discovery body is exactly at the backstop or one byte above it
- **THEN** the body at the backstop SHALL be accepted, and the body above it SHALL fail with a non-retryable bounded protocol error that returns no partial or truncated result

#### Scenario: Caller bounds a request
- **WHEN** a caller supplies an `http.Client` with its own transport or timeout, or a call context with a deadline
- **THEN** the client SHALL apply those bounds to discovery, unary and streaming calls
- **AND** a context deadline SHALL remain discoverable with `errors.Is` for discovery, unary calls and stream setup, while an established stream SHALL end without a client-generated error

#### Scenario: Guidance describes server-owned bounds
- **WHEN** a reader consults the Grafana provider godoc or the Grafana Gateway guide
- **THEN** it SHALL state that the Gateway owns resource bounds and that callers bound requests through `http.Client` and `context`, without client limit-tuning guidance

### Requirement: Incremental SSE consumption

A successful streaming setup SHALL require SSE media type and return a `StreamResult` whose request body and response headers are client-owned. One goroutine SHALL own the body, parse incrementally without client byte or event-count caps, send mapped parts with context-aware backpressure, close the body, and close the output channel exactly once. It SHALL NOT buffer the full response beyond the event being parsed. A body read failure SHALL be a retryable transport error.

#### Scenario: Text stream completes
- **WHEN** the server emits valid start, metadata, sequential text parts, safe errors, and finish frames
- **THEN** the client SHALL deliver every accepted part in order, including required empty deltas and finish, then close the channel

#### Scenario: Consumer is slow
- **WHEN** result delivery blocks and the call context is canceled
- **THEN** the stream owner SHALL stop the blocked send, close the response body, close the channel, and leave no client-owned goroutine reading the stream

#### Scenario: Large stream is accepted
- **WHEN** a stream contains a single event larger than 1 MiB, more than 100,000 events, or more than 64 MiB in total
- **THEN** the client SHALL deliver every part in order and close the channel without a client-generated protocol error

#### Scenario: Stream body read fails
- **WHEN** reading an established stream body fails without context cancellation
- **THEN** the client SHALL emit at most one retryable `PartError` with status 200 that retains the read cause, and close all owned resources

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

### Requirement: Pre-decode raw usage document validation

The retained raw object SHALL be the representation after JSON decoding/compaction removes insignificant whitespace around and inside the value. The client SHALL validate the original complete unary response or stream event for UTF-8 and JSON syntax before Go decoding can normalize invalid bytes.

#### Scenario: Pre-decode raw usage document validation
- **WHEN** usage.raw contains substantial JSON whitespace in an otherwise valid document
- **THEN** the retained raw object SHALL be compacted without a client size cap, while pre-decode UTF-8/syntax validation of the original document SHALL still apply

### Requirement: Raw usage escape preservation

Valid JSON with lone or paired escaped surrogates SHALL be accepted, with raw-object escapes preserved in `provider.Usage.Raw`.

#### Scenario: Raw usage escape preservation
- **WHEN** usage.raw contains a lone escaped UTF-16 surrogate in a valid object
- **THEN** the escape SHALL be accepted and retained unchanged

## MODIFIED Requirements

### Requirement: Bounded normalized unary consumption

For text/reasoning blocks and tool input/call/result variants, the client SHALL decode only fields consumed by the selected variant and SHALL ignore unrelated variant fields rather than enforce the server's strict output union. Ordinary typed decoding SHALL reject unrepresentable consumed values. Optional null tool flags SHALL normalize to Go absence/zero; explicit false Dynamic and Preliminary values SHALL retain pointer presence.

#### Scenario: Minimal unary success is consumed
- **WHEN** the server returns valid ordered text content, registered finish reason, and valid usage
- **THEN** the client SHALL return those fields plus local request metadata, HTTP response headers/body, and empty warnings when none were supplied

#### Scenario: Server supplies client-owned fields
- **WHEN** a body contains native nested response identity, transport fields and ordered warnings
- **THEN** the client SHALL preserve warnings, ordinary result/content providerMetadata and the raw body while replacing typed request/response with Gateway-hop information
- **AND** typed Response id/modelId/timestamp SHALL remain unset rather than falsely exposing native typed identity

#### Scenario: Unary response exceeds its bound
- **WHEN** the response is one byte larger than the fixed buffered-read backstop or has trailing data
- **THEN** the call SHALL fail without returning a partial or truncated result

#### Scenario: Unary result is outside supported families
- **WHEN** an output discriminator has no supported client mapper
- **THEN** the client SHALL fail explicitly rather than decoding through provider-domain JSON

#### Scenario: Unary warning is malformed
- **WHEN** a warning has an unknown discriminator, malformed field type or missing required feature/setting/message
- **THEN** the client SHALL fail through its existing bounded protocol-error path

#### Scenario: Required warning strings are empty
- **WHEN** known variants supply present empty required strings and absent or explicitly empty details
- **THEN** decoding SHALL preserve required values/order and normalize details to empty Go strings

#### Scenario: Provider-owned unary result
- **WHEN** ordered content contains a hosted call and its result
- **THEN** the client SHALL preserve call ownership, IDs, names, input, result and supported markers/metadata without requiring or emitting a result-level providerExecuted wire member

#### Scenario: Unrelated variant fields do not reject valid payloads
- **WHEN** a bounded text, call or result contains unrelated fields, including values not representable by those other variants
- **THEN** the client SHALL decode its relevant payload and ignore the unrelated fields, matching the pinned client at the supported typed projection

#### Scenario: Optional flag normalization
- **WHEN** call/result flags are omitted, null, false or true
- **THEN** null SHALL normalize to absence/zero, false SHALL retain Dynamic/Preliminary pointer presence and true SHALL remain enabled

### Requirement: Bounded unary document and required payload consumption

Required payload checks and opaque metadata structure SHALL remain unchanged. For successful unary responses, the client SHALL require JSON media type, read within the fixed buffered-read backstop, accept one complete valid document and map its supported registered content, including provider calls and results, finishReason, usage, warnings and ordinary result/content providerMetadata into provider.GenerateResult.

#### Scenario: Bounded unary document and required payload consumption
- **WHEN** a successful response has valid ordered content and ordinary metadata larger than the former 16 MiB client default
- **THEN** the client SHALL consume one complete JSON document into GenerateResult with required payload and metadata intact

### Requirement: Successful unary transport read failures retain retryability

When a successful HTTP 200 `DoGenerate` response body fails to read because of transport I/O after headers, the Grafana client SHALL return a retryable `*provider.APICallError` with HTTP status 200, a bounded locally worded primary message, and a discoverable underlying read cause, without returning a partial result or making another model request. Wrong media type, malformed JSON, malformed consumed fields, and buffered-read backstop violations SHALL remain non-retryable protocol failures.

#### Scenario: HTTP 200 response body is interrupted after headers
- **WHEN** the real HTTP handler sends JSON headers and a partial body with a declared longer `Content-Length`, then closes the connection before the body completes
- **THEN** `DoGenerate` SHALL return no result and one retryable `*provider.APICallError` with status 200, a locally bounded primary message that does not disclose the response body, and the underlying body-read failure discoverable through its cause chain, after exactly one model request

#### Scenario: Successful unary body cannot be accepted for protocol reasons
- **WHEN** a successful unary response has a wrong media type, malformed JSON, invalid consumed result fields, or exceeds the buffered-read backstop, including when an over-backstop partial body and a transport read error occur together
- **THEN** `DoGenerate` SHALL return no partial result and a non-retryable bounded protocol error after exactly one model request; the backstop SHALL take precedence over the read error, except that context cancellation or deadline expiry SHALL retain highest precedence

#### Scenario: Unary read is canceled
- **WHEN** the call context is canceled or reaches its deadline while a successful unary response body is being read
- **THEN** `DoGenerate` SHALL preserve context error identity through `errors.Is`, SHALL NOT return a retryable transport error, and SHALL NOT replay the request

#### Scenario: Stream parts precede a transport failure
- **WHEN** an established `DoStream` response delivers one or more parts and then its transport fails
- **THEN** the client SHALL NOT issue another HTTP request or replay any previously delivered part

### Requirement: SSE protocol failure without server lifecycle dialect

Every unsupported or malformed event SHALL emit at most one terminal non-retryable protocol PartError and close. Server lifecycle validation SHALL NOT be imported into the client as a new independent protocol dialect.

#### Scenario: SSE protocol failure without server lifecycle dialect
- **WHEN** an event has an unknown discriminator or is not valid UTF-8 JSON
- **THEN** the client SHALL emit at most one terminal non-retryable protocol PartError and close without importing server lifecycle validation

### Requirement: Optional bounded provider raw usage consumption

On a successful unary result and a streaming finish, the Grafana client SHALL preserve a supplied `usage.raw` as a `provider.Usage.Raw` JSON object, including nested provider-native fields and `{}`; absent raw SHALL remain absent. The client SHALL reject present null, scalar, array, malformed or incomplete JSON. It SHALL NOT cap the size of a retained raw object.

#### Scenario: Unary and finish have present or absent raw
- **WHEN** a bounded valid unary result or finish contains nested raw usage, `{}`, or no raw member
- **THEN** the resulting Go usage SHALL preserve the supplied JSON object semantics, retain an empty object when supplied, or leave `Raw` absent while preserving validated normalized counts

#### Scenario: Large raw usage is accepted
- **WHEN** a unary result or streaming finish contains a valid `usage.raw` object larger than 1 MiB
- **THEN** both paths SHALL retain the complete object without a size cap

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

### Requirement: Closed Gateway error classification

Every non-2xx model or discovery response SHALL be read within the fixed buffered-read backstop and mapped only from the registered public error envelope into the closed categories authentication, forbidden, invalid request, model not found, rate limit, failed dependency, and internal server. The resulting `GatewayError` SHALL expose category, public code, public message, HTTP status, and status-derived retryability and SHALL unwrap to a bounded `*provider.APICallError`.

#### Scenario: Registered error is returned
- **WHEN** the server returns one of the registered error type/status/code documents
- **THEN** `errors.As` SHALL find the matching `GatewayError` category and underlying `*provider.APICallError`, and retryability SHALL match the Gateway version registered in test/conformance/upstream.yaml, independently of nested native retry facts

#### Scenario: Error members use noncanonical property casing
- **WHEN** an otherwise valid HTTP error envelope or committed error payload uses case-insensitive Go matches for its member names
- **THEN** typed error decoding SHALL accept those members using standard encoding/json behavior
- **AND** retained HTTP documents and selected SSE data SHALL preserve their original JSON bytes without changing classification or retryability rules

#### Scenario: Error body is malformed or oversized
- **WHEN** a non-2xx body is malformed, has a wrong media type, or exceeds the buffered-read backstop
- **THEN** the client SHALL return a bounded local `*provider.APICallError` without accepting a partial category or exposing the full body in its message

#### Scenario: Transport fails
- **WHEN** HTTP transport fails without context cancellation
- **THEN** the client SHALL return a retryable `*provider.APICallError` that retains the transport cause

#### Scenario: Stream error event is received
- **WHEN** a valid registered error event appears in SSE
- **THEN** it SHALL become an ordered `PartError` with the equivalent bounded `APICallError`, preserving the Go provider contract without ending later valid parts by itself
- **AND** supplied bounded valid error.data SHALL be retained exactly in APICallError.Data, independently of mapped status/retryability, without manufacturing data when absent
- **AND** APICallError.ResponseBody SHALL remain empty rather than manufacturing an HTTP response document from the SSE event

#### Scenario: All-failed data survives classification
- **WHEN** a valid registered non-2xx envelope contains bounded attributed attempts and heterogeneous native errors
- **THEN** errors.As SHALL expose the existing GatewayError and API-call cause with complete envelope Data, while outer status/code/category and hop retryability remain unchanged

#### Scenario: Consumer middleware inspects stream data
- **WHEN** a consumer wrapper reads an attributed PartError followed by valid text and finish
- **THEN** it SHALL observe the retained data and original event order without enabling operator capture or changing core result/error policy

### Requirement: Authenticated public discovery
ListModels SHALL read authenticated bounded /config into public typed rows and optional configured-route facts using ordinary Go decoding. It SHALL preserve order, ignore additive fields and avoid server-policy revalidation or inferred configuration. Discovery by a BYOK caller SHALL report its fixed unsupported-operation error.

#### Scenario: Authenticated public discovery policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** `Provider.ListModels(ctx)` SHALL issue authenticated `GET /config`, read within the fixed buffered-read backstop, and return public ID, name, optional description and the specification version/provider/model-ID triple, plus optional typed `ModelInfo.Gateway *ConfiguredRoute`. `ConfiguredRoute` SHALL expose `Aliases`, `Primary` and ordered `Fallbacks`; each `ConfiguredCandidate` SHALL expose `ProviderInstance`, `Provider` and `ProviderModelID`. This SHALL remain the existing discovery method, without a second client or AGPL module dependency.
- **AND** The client SHALL use ordinary Go JSON decoding into typed ModelInfo values after enforcing the fixed buffered-read backstop and raw UTF-8 JSON validity. It SHALL NOT revalidate server-owned public-ID grammar, nonblank strings, specification/model-ID agreement, route cardinality, duplicate IDs/aliases/candidate tuples or route-group consistency. Standard Go JSON behavior SHALL apply, including case-insensitive field matching, zero/nil values for missing/null fields and U+FFFD normalization of escaped lone UTF-16 surrogates. A missing/null models collection, malformed JSON, byte overflow or type-decoding error SHALL invalidate the complete result. It SHALL preserve response order and configured alias/fallback order exactly as served, without expanding aliases into extra rows. Missing/null gateway SHALL remain nil. Unknown additive members SHALL remain ignored. Configured mappings SHALL be retained only when supplied by the server, never inferred from responses, models or inventories; credentials and arbitrary configuration SHALL NOT be exposed.

#### Scenario: Configured models and aliases are discovered
- **WHEN** the authenticated service returns canonical model rows with configured route facts
- **THEN** each row SHALL be returned in server order with its public specification fields, typed aliases, primary and ordered fallbacks
- **AND** aliases SHALL remain available as selection IDs without duplicating rows

#### Scenario: Discovery contains additive metadata
- **WHEN** otherwise valid discovery rows or the root document contain unrelated unknown members
- **THEN** the client SHALL ignore those members without exposing them through ModelInfo
- **AND** a recognized gateway extension SHALL be decoded into typed fields and retained rather than discarded

#### Scenario: Discovery is structurally unsafe
- **WHEN** the document exceeds the buffered-read backstop, is malformed JSON, has no models collection or contains a field that cannot decode into its declared Go type
- **THEN** discovery SHALL fail atomically with no partial catalog result

#### Scenario: Server has no configured extension
- **WHEN** a public discovery response omits gateway or supplies null
- **THEN** existing public rows SHALL remain consumable and each corresponding Gateway field SHALL be nil, with no inferred topology

#### Scenario: Caller inspects candidates without generation
- **WHEN** an authorized Go caller reads ListModels rows and inspects Gateway.Primary and Gateway.Fallbacks
- **THEN** configured order and provider-instance/provider/model facts SHALL be available without any model or inventory request

#### Scenario: BYOK endpoint does not provide discovery
- **WHEN** ListModels authenticates as a caller with BYOK access
- **THEN** it SHALL surface the bounded invalid-request error stating discovery is unsupported for BYOK
- **AND** it SHALL NOT return an empty catalog, try another credential or use discovery as a prerequisite for inference

### Requirement: Independent ordinary typed discovery decoding

ListModels SHALL remain the existing discovery method, without a second client or AGPL module dependency. The client SHALL use ordinary Go JSON decoding into typed ModelInfo values after enforcing the fixed buffered-read backstop and raw UTF-8 JSON validity. It SHALL NOT revalidate server-owned public-ID grammar, nonblank strings, specification/model-ID agreement, route cardinality, duplicate IDs/aliases/candidate tuples or route-group consistency.

#### Scenario: Independent ordinary typed discovery decoding
- **WHEN** discovery rows contain server-owned IDs outside the public grammar
- **THEN** bounded valid JSON SHALL decode into ModelInfo without a second client, AGPL dependency or client-side server-policy revalidation

### Requirement: Exact-pinned differential and black-box evidence

Tests SHALL compare Go and the exact Gateway version registered in test/conformance/upstream.yaml for semantic method/path/headers/body, supported result normalization, errors/retryability, cancellation, discovery, DONE, raw filtering, timestamp conversion and EOF. Native-value cases SHALL cover all warning variants/order/required empties, URL/document IDs/display/order and optional native stream identity.

#### Scenario: Equivalent text calls are compared
- **WHEN** the differential suite issues representable unary and streaming text/scalar calls through both clients
- **THEN** their semantic requests and normalized observable results SHALL agree except for documented parity-preserving Go adaptations

#### Scenario: Registered baseline changes
- **WHEN** the Gateway or provider package pin changes
- **THEN** baseline validation SHALL fail until differential evidence and every observed divergence are reviewed and updated together

#### Scenario: Client bounds are tested
- **WHEN** tests exercise the buffered-read backstop at its exact size and one byte over through an unexported seam, large SSE inputs above the former client defaults, and cancellation under blocked delivery
- **THEN** tests SHALL prove non-truncating backstop failure, body closure, channel closure, and absence of retained client goroutines without allocating a backstop-sized fixture

#### Scenario: Authenticated command is exercised
- **WHEN** the repository integration suite runs the protocol test host with test-signed platform tokens and provider fakes serving supported native values
- **THEN** both clients SHALL preserve contracted values while configured credentials and cross-tenant state remain absent
- **AND** the Go client SHALL complete discovery, unary text, streaming text, cancellation and registered errors, and pinned TS configured discovery SHALL run as a self-hosted service account
- **AND** canonical operator identity and metadata-only capture SHALL remain independent of returned native values and authorized configured discovery

#### Scenario: Configured discovery consumers are compared
- **WHEN** Go ListModels, pinned TS getAvailableModels and the shipped TS helper inspect canonical model rows with configured alias metadata
- **THEN** normalized public fields SHALL remain compatible, Go and the helper SHALL retain matching configured facts, stock TS SHALL still strip the extension and provider inference counts SHALL remain zero

### Requirement: Independent opaque stream metadata consumption

The Grafana client SHALL independently decode providerMetadata on supported text/reasoning start/delta/end, reasoning-file, source, tool-input start/delta/end, function call, basic result, custom and finish parts. It SHALL preserve namespace objects, nested values, original event order and absent versus empty presence without IncludeRawChunks.

#### Scenario: Unknown namespace survives all supported placements
- **WHEN** valid events carry future object-valued namespaces with nested null/false/zero/empty values and explicit empty metadata at supported positions
- **THEN** the independent Go client SHALL preserve those values and positions without importing server code

#### Scenario: Malformed metadata on finish
- **WHEN** a bounded finish contains a null namespace or malformed metadata shape
- **THEN** the client SHALL emit at most one bounded protocol PartError and close without delivering the invalid finish or metadata-free success

### Requirement: Independent generated-file response decoding
The Apache Grafana client SHALL decode registered generated files in unary and streaming responses without importing Gateway code. It SHALL preserve data/URL values, optional originalUrl, media type, metadata presence and ordering; unary responses SHALL remain within the fixed buffered-read backstop. Provider-local filenames SHALL not become normalized generated-file fields.

#### Scenario: Generated file response is decoded
- **WHEN** the server returns a valid generated file using data or URL in unary or SSE output
- **THEN** the client SHALL expose the corresponding provider file value with metadata intact and selected empty data preserved

#### Scenario: Malformed or oversized response
- **WHEN** a generated file has malformed consumed fields or its unary response exceeds the buffered-read backstop
- **THEN** existing bounded protocol-error, cancellation and body-ownership behavior SHALL terminate consumption

### Requirement: Platform route base URL and credential validation
Each constructor SHALL require a valid HTTP(S) ProviderWire API-prefix base URL and reject user info, query and fragment components. It SHALL accept optional configured outer headers and HTTP client, and SHALL NOT accept client-processing limits. Credentials SHALL be nonempty valid UTF-8 without whitespace or control characters and SHALL NOT be parsed or prefix-checked.

#### Scenario: Unsafe constructor input
- **WHEN** construction receives user info in its base URL or whitespace in a token
- **THEN** construction SHALL fail before authentication or Gateway I/O without parsing the token or reading defaults from the environment

## REMOVED Requirements

### Requirement: Incremental bounded SSE consumption
**Reason**: The client no longer caps cumulative stream bytes, complete event bytes or event count; the Gateway owns stream bounds.
**Migration**: Replaced by "Incremental SSE consumption". Bound streams with the call context and the caller-supplied `http.Client`.

### Requirement: Retained raw-object and original-document limits
**Reason**: The 1,048,576-byte retained raw-object cap and the configurable `UnaryBytes`/`StreamEventBytes` document bounds are removed.
**Migration**: Replaced by "Pre-decode raw usage document validation", which keeps compaction and pre-decode UTF-8/JSON validation.

### Requirement: Raw usage escape preservation and bounded intermediates
**Reason**: The cumulative-stream, event-count and per-document limits it referenced are removed.
**Migration**: Replaced by "Raw usage escape preservation", which keeps escape acceptance and preservation.
