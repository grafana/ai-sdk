## MODIFIED Requirements

### Requirement: Bounded normalized unary consumption
Successful unary responses SHALL require JSON media type, read within the configured unary byte limit and contain one complete UTF-8/JSON document. The client SHALL map supported registered text/function-call/source/reasoning/reasoning-file content, finish reason and usage into GenerateResult with strict discriminator/required-field/known-usage validation. #280 SHALL own ordinary supported providerMetadata preservation at result/content placements, including unknown object-valued namespaces, nested values and absent/empty distinctions under reviewed bounds, not raw filtering. Unsupported execution/content behavior SHALL remain an explicit protocol failure.

Request.Body SHALL be locally encoded Gateway request data; Response.Headers/Body SHALL come from bounded Gateway HTTP transport, replacing server-native request/response. Supported unary server response id/modelId/timestamp SHALL remain inspectable in bounded Response.Body, not typed ResponseInfo, to match the pinned TS overwrite boundary. Warnings SHALL preserve valid registered server fields in order and default to a non-nil empty slice when absent/null, with documented optional-empty Go normalization and strict warning validation. Supported metadata SHALL NOT be discarded as provider-private; native transport, invented topology and unsupported fields SHALL remain unadopted.

#### Scenario: Supported unary success is consumed
- **WHEN** the server returns valid supported content/metadata, registered finish reason and usage
- **THEN** the client SHALL preserve those fields with local request, bounded Gateway response and preserved or empty warnings

#### Scenario: Server supplies transport and identity
- **WHEN** the body includes request/response metadata, actual response identity, warnings and supported providerMetadata
- **THEN** warnings/ordinary metadata SHALL survive while local transport replaces typed request/response
- **AND** actual unary identity SHALL remain only in bounded raw response body, not be substituted with the requested route or invented typed fields

#### Scenario: Unary response exceeds its bound
- **WHEN** the response exceeds the configured limit or has trailing JSON
- **THEN** the call SHALL fail without partial result or unbounded retained body

#### Scenario: Unsupported unary output
- **WHEN** the body uses an unsupported content discriminator or execution behavior
- **THEN** the closed mapper SHALL fail explicitly without accepting it via metadata or generic provider-domain decoding

#### Scenario: Unary warning is malformed
- **WHEN** a warning has an unknown type, missing required field or invalid represented value
- **THEN** the call SHALL fail with a bounded protocol error rather than expose unvalidated warning data

### Requirement: Closed Gateway error classification
Non-2xx model/discovery responses SHALL be read within configured error-body limits and mapped from registered message/type/code/param envelopes using the closed authentication, forbidden, invalid-request, model-not-found, rate-limit, failed-dependency and internal-server categories. Strict fixed Gateway errors and gateway-caller-response-policy's bounded dynamic provider projections SHALL be accepted independently of a fixed code/status pair. The client SHALL retain registered JSON string/number/null codes without stringifying numeric values; GatewayError.Code SHALL be json.RawMessage and Param SHALL be bounded json.RawMessage. It SHALL expose category, public message, HTTP status and pinned status-derived retryability and unwrap to a bounded APICallError containing only the reviewed public envelope, not provider-native transport/cause data.

Unknown/malformed/oversized envelope fields, wrong media types and unsafe categories/status combinations SHALL use bounded local protocol text without copying raw bytes into primary messages. Context cancellation/deadline identity SHALL remain discoverable with errors.Is. Host authentication/permission errors SHALL retain their fixed safe contract; provider authorization errors SHALL remain failed-dependency rather than Gateway auth guidance.

#### Scenario: Registered error is returned
- **WHEN** the server returns a fixed Gateway error or reviewed provider envelope with numeric code and projected param
- **THEN** errors.As SHALL find GatewayError and its APICallError with preserved public diagnostics
- **AND** category/status/retry behavior SHALL match the exact registered public client, with TS code/param inspected through cause/body rather than typed fields

#### Scenario: Error body is malformed or oversized
- **WHEN** the response violates media type, bounds or registered projection
- **THEN** a bounded local APICallError SHALL result without partial category/data or the complete native body in its primary message

#### Scenario: Transport fails
- **WHEN** transport fails without context cancellation
- **THEN** a retryable bounded APICallError SHALL retain the local transport cause without exposing it through server diagnostics

#### Scenario: Stream error event is received
- **WHEN** a valid reviewed or fixed error event appears in SSE
- **THEN** it SHALL become an ordered PartError with bounded public APICallError, preserving the existing status/retryable member and non-terminal provider semantics

#### Scenario: Old fixed envelope remains readable
- **WHEN** an upgraded client talks to an old server with fixed string codes and param null
- **THEN** it SHALL decode the same message/category/status/retry behavior through its JSON-valued public fields

### Requirement: No implicit client retry or backend selection
The Grafana client SHALL issue at most one Gateway model request per `DoGenerate` or `DoStream` invocation after token acquisition. It SHALL preserve retryability for existing SDK retry/fallback orchestration but MUST NOT select physical providers, traverse Gateway candidates, retry through the retired endpoint, or retry after any response or stream event. Supported actual response identity, ordinary metadata and reviewed provider diagnostics SHALL remain caller-visible at registered fields; configured credential material, another tenant's state and fallback/operator topology SHALL NOT be exposed. Logical model identity SHALL remain the requested public route.

#### Scenario: Retryable setup error occurs
- **WHEN** the Gateway returns a retryable non-2xx response
- **THEN** the invocation SHALL return that retryable error after one Gateway request and leave any retry decision to its caller

#### Scenario: Stream error occurs after output
- **WHEN** an error event or transport failure occurs after a stream part has been delivered
- **THEN** the client SHALL not issue another HTTP request or change model identity

### Requirement: Exact-pinned differential and black-box evidence
Automated tests SHALL compare equivalent Go and the exact registered `@ai-sdk/gateway` scenarios for semantic method, path, effective protocol and call headers, body presence, supported unary and stream metadata/warnings/sources/actual identity with the typed unary boundary, error diagnostics/category, retryability, cancellation, discovery, `[DONE]`, raw filtering, timestamp conversion, and EOF. Tests SHALL use the versions registered in `test/conformance/upstream.yaml`; a baseline change SHALL update pins, lockfiles, captures, classification, and client behavior together. Separate hostile fake-server tests SHALL prove all client bounds and resource cleanup. Authenticated black-box tests SHALL exercise the work-package-5 command over HTTP without importing Gateway implementation packages into Apache production code.

#### Scenario: Equivalent text calls are compared
- **WHEN** the differential suite issues representable unary and streaming text/scalar calls through both clients
- **THEN** their semantic requests and normalized observable results SHALL agree except for documented parity-preserving Go adaptations

#### Scenario: Registered baseline changes
- **WHEN** the Gateway or provider package pin changes
- **THEN** baseline validation SHALL fail until differential evidence and every observed divergence are reviewed and updated together

#### Scenario: Client bounds are tested
- **WHEN** fake endpoints exercise exact-limit and one-byte/one-event-over-limit discovery, unary, error, and SSE inputs plus cancellation under blocked delivery
- **THEN** tests SHALL prove bounded allocation/read behavior, body closure, channel closure, and absence of retained client goroutines

#### Scenario: Authenticated command is exercised
- **WHEN** the repository integration suite starts the WP5 command with deterministic auth and provider fakes
- **THEN** the Go client SHALL complete discovery, unary text, streaming text, acting-user propagation, cancellation, and registered errors without exposing configured credentials or another tenant's state or private operator topology

### Requirement: Optional bounded provider raw usage consumption
On a successful unary result and a streaming finish, the Grafana client SHALL preserve a supplied `usage.raw` as a `provider.Usage.Raw` JSON object, including nested provider-native fields and `{}`; absent raw SHALL remain absent. The client SHALL reject present null, scalar, array, malformed or incomplete JSON, and a retained raw object larger than 1,048,576 bytes. This raw-object limit applies to the retained representation after JSON decoding/compaction removes insignificant whitespace around and inside the value; the original complete unary response or event SHALL remain bounded by its configured `UnaryBytes` or `StreamEventBytes`. The client SHALL validate those original bounded documents for UTF-8 and JSON syntax before Go decoding can normalize invalid bytes. Valid JSON with lone or paired escaped surrogates SHALL be accepted, with raw-object escapes preserved in `provider.Usage.Raw`. The existing cumulative-stream and event-count limits SHALL still apply, and intermediate `decodeFields`/usage-map copies SHALL remain bounded by the full-response or event limits. The client SHALL continue to validate known normalized token counts and filter unrelated unknown usage and native transport while preserving supported providerMetadata through #280; distinct `type: "raw"` stream-part filtering SHALL remain governed by `IncludeRawChunks` and SHALL NOT filter `usage.raw`.

#### Scenario: Unary and finish have present or absent raw
- **WHEN** a bounded valid unary result or finish contains nested raw usage, `{}`, or no raw member
- **THEN** the resulting Go usage SHALL preserve the supplied JSON object semantics, retain an empty object when supplied, or leave `Raw` absent while preserving validated normalized counts

#### Scenario: Hostile unary response contains invalid raw
- **WHEN** a successful HTTP 200 unary response contains malformed, null, non-object, or over-limit retained `usage.raw`
- **THEN** the client SHALL return a bounded non-retryable protocol error without any partial result or raw data in its error text

#### Scenario: Hostile streaming finish contains invalid raw
- **WHEN** a streaming finish contains malformed, null, non-object, or over-limit retained `usage.raw`
- **THEN** the client SHALL emit at most one bounded terminal non-retryable protocol `PartError` and close, without delivering the invalid finish

#### Scenario: UTF-8 and JSON escapes in both paths
- **WHEN** bounded unary or streaming finish JSON contains invalid UTF-8 in `usage.raw`, including a nested key or value
- **THEN** the client SHALL reject the response with the path's bounded protocol error, without retaining normalized replacement characters in raw usage
- **WHEN** `usage.raw` contains valid JSON with lone or paired escaped surrogates
- **THEN** both paths SHALL accept the object under the same size and shape limits and preserve its raw JSON escapes

#### Scenario: Raw part filtering does not erase usage
- **WHEN** a finish includes `usage.raw` and `IncludeRawChunks` is false
- **THEN** the finish SHALL retain its raw usage even when independent `type: "raw"` stream parts are filtered

### Requirement: Source response consumption

The independent Go client SHALL decode registered URL and document sources in unary and streaming responses without importing Gateway code. Required document title SHALL accept an empty string. Optional URL title and document filename absence and empty string SHALL normalize to empty Go strings. Native source identity/display and order SHALL survive; #280 SHALL own ordinary bounded object-valued providerMetadata including unknown namespaces/nested values, independent of raw filtering. Missing required fields, malformed types and unknown source discriminators SHALL use the existing bounded protocol-error path. Unary source Title SHALL be populated, with Text retained for compatibility with older consumers.

#### Scenario: URL and document consumption
- **WHEN** both registered variants arrive through bounded unary or SSE readers
- **THEN** the source content SHALL retain the appropriate variant fields, identity and metadata
- **AND** a missing document title SHALL fail while an explicitly empty title SHALL succeed
