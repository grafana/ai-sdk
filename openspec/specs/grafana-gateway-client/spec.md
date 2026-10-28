# grafana-gateway-client Specification

## Purpose

Define the single Apache-licensed Go client for authenticated, bounded, strict ProviderWire V4 discovery and text calls through Grafana AI Gateway.

## Requirements

### Requirement: Single Apache-licensed public Gateway client
The repository SHALL expose `github.com/grafana/ai-sdk/providers/grafana` as a separate Apache-2.0 Go module. It SHALL be the only public Go ProviderWire client, SHALL implement `registry.Provider` and return `provider.LanguageModel` values, and MUST NOT import or require `github.com/grafana/ai-sdk/ai-gateway`. Request codecs, response codecs, error decoders, and SSE readers SHALL remain private implementation details; no legacy wire codec or compatibility mode SHALL be restored.

#### Scenario: Provider composes with the registry
- **WHEN** a caller registers a Grafana provider and resolves a requested public model ID
- **THEN** the model SHALL implement `provider.LanguageModel`, report specification version `v4`, provider `grafana`, and the requested model ID

#### Scenario: Gateway source is unavailable
- **WHEN** the Grafana client and root SDK are built with `GOWORK=off` and the `ai-gateway/` directory is unavailable
- **THEN** both Apache modules SHALL build without an AGPL module dependency

#### Scenario: Public API is inspected
- **WHEN** exported declarations in the module are enumerated
- **THEN** no low-level ProviderWire transport, legacy mode, server DTO, or server validator SHALL be public

### Requirement: Exchange and token constructor defaults and immutability

Token exchange SHALL require CAP token, token-exchange URL, namespace and audience, with no default audience; direct access-token authentication SHALL require a non-empty access token, and service-account authentication SHALL require a non-empty service-account token. Construction SHALL not discover credentials or endpoints from ambient environment variables. Constructors SHALL preserve immutable client/header configuration and refusal to follow redirects.

#### Scenario: Exchange and token constructor defaults and immutability
- **WHEN** token-exchange construction omits audience, or valid construction is followed by caller code mutating its header map
- **THEN** construction without an audience SHALL fail before I/O, and valid client/header configuration SHALL remain immutable with redirects refused

### Requirement: Authentication preparation failure boundary

Authentication failures and cancellation SHALL occur before Gateway network I/O when detected during request preparation.

#### Scenario: Authentication preparation failure boundary
- **WHEN** authentication preparation observes cancellation or a token-source failure
- **THEN** the Gateway request SHALL NOT be issued

### Requirement: Exact ProviderWire routes and protected headers

The configured base URL SHALL be the ProviderWire API prefix, `/apis/<group>/<version>/namespaces/<namespace>` on the platform origin for a platform-hosted Gateway, and the client SHALL append exactly `/config` for discovery and `/language-model` for calls without discarding an existing path prefix. Every model call SHALL use `POST`, JSON content type, the requested model ID, specification version `4`, exact streaming value `true` or `false`, and the matching JSON or SSE accept type.

#### Scenario: Header ownership applies to every authentication flow
- **WHEN** constructor or call-level authentication headers are supplied
- **THEN** For every authentication flow, `Authorization`, `X-Access-Token` and `X-Grafana-Id` SHALL be reserved case-insensitively. Supplying any reserved name in configured headers SHALL fail construction. Supplying any reserved name in call-level headers SHALL fail before request serialization and network I/O, including when its value is empty. Each constructor SHALL own its single credential header; configured and call headers SHALL NOT add a competing credential.

#### Scenario: Unary request is emitted
- **WHEN** `DoGenerate` is invoked for model `assistant`
- **THEN** the request SHALL be `POST <base-prefix>/language-model` with streaming `false`, model ID `assistant`, specification `4`, JSON content and accept types, and exactly one value for each client-owned header

#### Scenario: Streaming request is emitted
- **WHEN** `DoStream` is invoked
- **THEN** the request SHALL use the same method, route, model, and specification headers with streaming `true` and SSE accept type

#### Scenario: Header names collide
- **WHEN** configured or call-level headers attempt to set client-owned headers and are not rejected by the authentication reserved-header rule
- **THEN** the client-owned effective values SHALL win and no duplicate client-owned protocol value SHALL be emitted

#### Scenario: Cloud reserved headers are configured
- **WHEN** configured headers contain a reserved credential or identity header under any casing
- **THEN** construction SHALL fail without network I/O or echoing its value in the error

#### Scenario: Cloud reserved headers are supplied per call
- **WHEN** call-level headers contain a reserved credential or identity header under any casing
- **THEN** the call SHALL fail before serializing that value into request metadata or issuing an HTTP request

#### Scenario: Body-carried headers are supplied
- **WHEN** `CallOptions.Headers` contains a representable header entry accepted by the selected authentication flow
- **THEN** it SHALL participate in outer-header composition and remain present in the serialized `headers` body member for the server to accept or reject

### Requirement: Public authentication guide security and reference boundaries

The shared guide SHALL cover URL and credential selection, least-privilege service account roles, access policy scopes and namespace access, HTTPS, secure storage and rotation without exposing internal deployment names, host configuration or test-harness details. Exhaustive Go API reference SHALL remain in godoc.

#### Scenario: Public authentication guide security and reference boundaries
- **WHEN** a user reads credential provisioning instructions
- **THEN** the guide SHALL cover least privilege, HTTPS, storage and rotation without internal deployment/host/harness details, leaving exhaustive API reference in godoc

### Requirement: Explicit request projection and presence

The client SHALL explicitly map the complete current `provider.CallOptions` shape into the registered LanguageModelV4 Gateway request projection without importing server DTOs or calling server validators. It SHALL preserve every representable absent, explicit zero, explicit false, empty string, empty array, empty object, nested null, selected empty union arm, URL string, and supported binary-to-base64 distinction.

#### Scenario: Presence-sensitive text request is encoded
- **WHEN** a text/scalar call contains explicit zero, false, empty collections, empty strings, and opaque nested JSON
- **THEN** the semantic body SHALL match the equivalent request emitted by the exact `@ai-sdk/gateway` version registered in `test/conformance/upstream.yaml` for every Go-representable distinction

#### Scenario: File data is representable
- **WHEN** a selected binary, URL, reference, or text arm is supplied in an ordinary prompt or tool-result file
- **THEN** bytes SHALL become standard base64, URLs SHALL remain strings, selected empty payloads SHALL remain selected, and filename absence SHALL remain distinct from explicit empty
- **AND** the strict server SHALL own current runtime capability rejection

#### Scenario: Reasoning uses the Go default
- **WHEN** `Reasoning` is `ReasoningProviderDefault`
- **THEN** the body SHALL omit `reasoning` and parity evidence SHALL classify the unrepresentable explicit JavaScript `provider-default` presence as a parity-preserving Go adaptation

#### Scenario: Provider-domain input is invalid
- **WHEN** any selected value cannot be mapped unambiguously to the registered projection
- **THEN** both unary and streaming setup SHALL fail before token acquisition or HTTP and SHALL not silently omit or reinterpret the value

#### Scenario: Reasoning file uses a forbidden arm
- **WHEN** a reasoning file selects reference or text, or supplies an inactive filename
- **THEN** client encoding SHALL fail before authentication or network I/O

### Requirement: Client reasoning and invalid input projection

The client SHALL omit the Go zero value `ReasoningProviderDefault` and encode every non-zero registered reasoning value. The client SHALL reject invalid UTF-8, non-finite numeric values, invalid raw JSON, unknown discriminators, conflicting selected arms, and any other value without an unambiguous registered representation before authentication or network I/O.

#### Scenario: Client reasoning and invalid input projection
- **WHEN** a call supplies ReasoningProviderDefault or non-finite scalar input
- **THEN** the default SHALL be omitted and invalid input SHALL fail before authentication/network I/O without silent reinterpretation

### Requirement: Ordinary and reasoning file projection scopes

Ordinary prompt and tool-result file projection SHALL support data, URL, reference, and text arms, preserving selected empties and absent/empty/non-empty filenames. Message and file-part options SHALL retain their registered scopes. Reasoning files SHALL retain their narrower data/URL-only projection without implying server runtime support.

#### Scenario: Ordinary and reasoning file projection scopes
- **WHEN** a prompt file selects empty reference data and a reasoning file selects URL data
- **THEN** the ordinary selected empty and filename presence SHALL survive, while reasoning files SHALL remain data/URL-only without claiming server execution support

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

### Requirement: Unary malformed input and Gateway-hop transport replacement

The client SHALL reject malformed required fields, unknown content/finish/warning discriminators, invalid usage, trailing JSON and oversized input. Supported source and reasoning families SHALL remain governed by their capabilities. The client SHALL replace server request/response: Request.Body SHALL be the locally encoded request; Response.Headers/Body SHALL be the bounded Gateway HTTP response.

#### Scenario: Unary malformed input and Gateway-hop transport replacement
- **WHEN** a valid response has server transport fields and an invalid response has trailing JSON
- **THEN** the valid result SHALL replace request/response with local request and bounded Gateway response, and invalid input SHALL fail atomically

### Requirement: Unary native identity and shared warning consumption

Native response id/modelId/timestamp SHALL remain available inside that raw body but SHALL NOT populate typed Response identity, matching the registered TS client's replacement. No native diagnostic access carrier SHALL be introduced by this behavior. Warnings SHALL preserve active registered fields, required empty strings, order and multiplicity, defaulting to a non-nil empty slice when absent/null. Unary/stream decoding SHALL share warning validation.

#### Scenario: Unary native identity and shared warning consumption
- **WHEN** a response contains native identity and ordered warnings with required empty fields
- **THEN** raw body SHALL retain identity without typed identity or a diagnostic carrier, and shared warning validation SHALL retain active fields and default absent/null warnings to an empty slice

### Requirement: Unary warning details and opaque metadata validation

Optional absent/empty details SHALL decode to the same empty Go string as a documented representation adaptation. Ordinary providerMetadata SHALL be independently decoded as opaque object-valued namespaces under gateway-provider-metadata, without Gateway imports, inventories or IncludeRawChunks gating. Present null, scalar, array, malformed or invalid-UTF-8 metadata SHALL fail explicitly under the bounded protocol-error path.

#### Scenario: Unary warning details and opaque metadata validation
- **WHEN** result metadata contains an unknown namespace object and empty warning details
- **THEN** the object SHALL survive independently of raw filtering and details SHALL normalize to empty Go string; null/scalar/array/malformed metadata SHALL fail through bounded protocol errors

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

### Requirement: Unary read cancellation and existing classification boundaries

Context cancellation or deadline expiry SHALL remain discoverable with `errors.Is` and SHALL NOT be classified as a retryable transport failure. Non-2xx Gateway error responses and discovery retain their existing classification; this requirement does not add any new public service-error category.

#### Scenario: Unary read cancellation and existing classification boundaries
- **WHEN** context cancellation interrupts a successful response read
- **THEN** errors.Is SHALL retain context identity without retryable transport classification or a new public service-error category

### Requirement: Supported SSE tool payload and marker preservation

The mapper SHALL accept supported text, function-tool and provider-tool calls/results, safe error parts and bounded raw parts needed for registered filtering behavior. Supported execution/dynamic/preliminary markers and opaque tool metadata SHALL be preserved under gateway-provider-metadata. Input-start dynamic SHALL retain absent, explicit false and true independently through decoding; it SHALL NOT be eagerly defaulted. A deferred result SHALL NOT require a repeated call in the same response.

#### Scenario: Supported SSE tool payload and marker preservation
- **WHEN** input-start contains dynamic false and a deferred provider result arrives without a repeated call
- **THEN** the client SHALL preserve explicit false presence, supported ownership/preview markers and opaque metadata without requiring repeated calls

### Requirement: SSE protocol failure without server lifecycle dialect

Every unsupported or malformed event SHALL emit at most one terminal non-retryable protocol PartError and close. Server lifecycle validation SHALL NOT be imported into the client as a new independent protocol dialect.

#### Scenario: SSE protocol failure without server lifecycle dialect
- **WHEN** an event has an unknown discriminator or is not valid UTF-8 JSON
- **THEN** the client SHALL emit at most one terminal non-retryable protocol PartError and close without importing server lifecycle validation

### Requirement: Registered stream normalization

The client SHALL ignore an SSE data payload exactly equal to `[DONE]`, SHALL treat transport clean EOF as clean completion with or without a preceding finish, SHALL filter `raw` parts unless `IncludeRawChunks` is true, SHALL convert valid response-metadata timestamps into `time.Time`, and SHALL preserve response order. Context cancellation SHALL end the stream without manufacturing a provider error.

#### Scenario: Native stream identity is not a public route
- **WHEN** native id/modelId contain valid Unicode or punctuation outside route-ID syntax
- **THEN** those values SHALL be retained without route validation or normalization

#### Scenario: Response metadata is partial or empty
- **WHEN** metadata supplies no identity, only one field or explicit empty optional strings
- **THEN** the client SHALL decode only the supplied values with Go zero-value normalization and no canonical fallback

#### Scenario: Response metadata is malformed
- **WHEN** an optional identity field is null/wrong-type, timestamp is invalid or the bounded original document has malformed UTF-8
- **THEN** the client SHALL emit at most one bounded protocol PartError and close without returning substituted identity

#### Scenario: DONE terminates no value
- **WHEN** the stream contains exact `[DONE]`
- **THEN** the sentinel SHALL not appear as a `provider.StreamPart` and subsequent clean EOF SHALL be accepted

#### Scenario: Response timestamp is present
- **WHEN** response metadata contains a valid registered timestamp string
- **THEN** the corresponding `provider.StreamPart.Timestamp` SHALL represent the same instant

#### Scenario: Raw output is not requested
- **WHEN** a valid raw part arrives and `IncludeRawChunks` is false or omitted
- **THEN** the raw part SHALL be consumed but not delivered

#### Scenario: Raw output is requested
- **WHEN** a valid raw part arrives and `IncludeRawChunks` is true
- **THEN** the bounded raw value SHALL be delivered unchanged

#### Scenario: Clean EOF occurs without finish
- **WHEN** a valid stream or `[DONE]` is followed by transport EOF before any finish
- **THEN** the client SHALL close cleanly to match the registered client's observable EOF behavior

### Requirement: Client stream finish and optional native identity normalization

A finish received before EOF SHALL be delivered before channel closure; the client SHALL not create a synthetic finish or require the server to emit `[DONE]`. Native response-metadata id/modelId/timestamp SHALL be optional. Supplied identity strings SHALL be preserved without public route syntax checks, trimming, canonical replacement or fabricated defaults. Absent and explicitly empty optional wire strings SHALL decode to empty Go strings; absent timestamp SHALL decode to zero time.

#### Scenario: Client stream finish and optional native identity normalization
- **WHEN** finish follows partial native metadata with an explicit empty modelId
- **THEN** finish SHALL be delivered before closure and optional identity SHALL normalize to Go zero values without fabricated defaults or route checks

### Requirement: Stream identity document validation boundary

Bounded original-document UTF-8/type validation and valid RFC3339Nano timestamp parsing SHALL remain effective. Present null/wrong-type identity fields or invalid timestamps SHALL fail through the bounded protocol-error path. Native provider identity, native diagnostic bodies/headers and opaque metadata are not added by this identity mapper.

#### Scenario: Stream identity document validation boundary
- **WHEN** response-metadata supplies null modelId or an invalid timestamp
- **THEN** the bounded protocol-error path SHALL reject it without adding provider identity, native diagnostic transport or opaque metadata through the identity mapper

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

### Requirement: Raw usage independence from raw stream filtering

The client SHALL continue to validate known normalized token counts and filter unrelated unknown usage and unrepresented transport fields without filtering ordinary registered providerMetadata; distinct `type: "raw"` stream-part filtering SHALL remain governed by `IncludeRawChunks` and SHALL NOT filter `usage.raw`.

#### Scenario: Raw usage independence from raw stream filtering
- **WHEN** IncludeRawChunks is false and finish carries normalized counts plus usage.raw
- **THEN** normalized counts SHALL still be validated and usage.raw and registered metadata SHALL survive independently of raw-part filtering

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

### Requirement: Complete bounded error envelope retention

For a valid registered envelope the client SHALL retain the complete bounded JSON document in the APICallError cause's Data and ResponseBody, including additive providerMetadata.gateway.evidence, rather than reconstructing only the registered error fields. Decoding SHALL remain independent of Gateway imports and SHALL NOT introduce a new public error API or interpret every opaque evidence member.

#### Scenario: Complete bounded error envelope retention
- **WHEN** a valid error envelope contains additive providerMetadata.gateway.evidence
- **THEN** the API-call cause SHALL retain the entire bounded document as Data and ResponseBody without importing Gateway or adding an evidence interpretation API

### Requirement: Error fallback messages and standard Go member matching

Unknown or malformed error bodies, wrong media types, and transport failures SHALL use local bounded error text rather than copying arbitrary response bytes into the primary message. Context cancellation and deadlines SHALL remain discoverable with `errors.Is`. HTTP error envelopes and committed error payloads SHALL use standard Go JSON struct-member matching, including case-insensitive matches and standard duplicate-member processing, rather than a custom exact-name filter.

#### Scenario: Error fallback messages and standard Go member matching
- **WHEN** a valid envelope uses case-variant members and a malformed envelope contains arbitrary response bytes
- **THEN** valid typed decoding SHALL follow standard Go matching/duplicate processing, while malformed input SHALL use bounded local primary text and preserve context error identity

### Requirement: Opaque error bytes remain unmodified

Opaque raw data SHALL remain unmodified by that typed decoding.

#### Scenario: Opaque error bytes remain unmodified
- **WHEN** typed error decoding accepts case-variant JSON member names
- **THEN** retained opaque data SHALL preserve the original bytes

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

### Requirement: Discovery normalization atomicity and served order

Standard Go JSON behavior SHALL apply, including case-insensitive field matching, zero/nil values for missing/null fields and U+FFFD normalization of escaped lone UTF-16 surrogates. A missing/null models collection, malformed JSON, byte overflow or type-decoding error SHALL invalidate the complete result. It SHALL preserve response order and configured alias/fallback order exactly as served, without expanding aliases into extra rows. Missing/null gateway SHALL remain nil.

#### Scenario: Discovery normalization atomicity and served order
- **WHEN** a valid discovery document uses case-variant fields, null optional gateway and ordered fallbacks
- **THEN** standard Go normalization SHALL apply and served order SHALL survive without alias expansion; missing/null models or a decoding error SHALL invalidate the whole result

### Requirement: Discovery additive data and configured-fact provenance

Unknown additive members SHALL remain ignored. Configured mappings SHALL be retained only when supplied by the server, never inferred from responses, models or inventories; credentials and arbitrary configuration SHALL NOT be exposed.

#### Scenario: Discovery additive data and configured-fact provenance
- **WHEN** a row has unknown members and no gateway extension
- **THEN** unknown members SHALL be ignored and topology SHALL NOT be inferred or expose credentials/arbitrary configuration

### Requirement: No implicit client retry or backend selection

The Grafana client SHALL issue at most one Gateway model request per `DoGenerate` or `DoStream` invocation after token acquisition. It SHALL preserve retryability for existing SDK retry/fallback orchestration but MUST NOT select physical providers, traverse Gateway candidates, retry through the retired endpoint, or retry after any response or stream event.

#### Scenario: Retryable setup error occurs
- **WHEN** the Gateway returns a retryable non-2xx response
- **THEN** the invocation SHALL return that retryable error after one Gateway request and leave any retry decision to its caller

#### Scenario: Stream error occurs after output
- **WHEN** an error event or transport failure occurs after a stream part has been delivered
- **THEN** the client SHALL not issue another HTTP request or change model identity

#### Scenario: Configured candidates are inspected
- **WHEN** ListModels exposes more than one configured candidate
- **THEN** the client SHALL not contact, select or probe any of them

### Requirement: Configured discovery is not runtime backend selection

Discovery SHALL retain authorized configured facts only through the approved optional gateway field; inspecting them SHALL NOT implement backend selection. This discovery exception SHALL NOT change runtime public result/error projection, which remains governed by its separate contracts. Native source and streaming response identity SHALL be retained under their runtime contracts without enabling client backend selection.

#### Scenario: Configured discovery is not runtime backend selection
- **WHEN** a caller inspects authorized primary and fallback facts
- **THEN** inspection SHALL NOT select candidates, alter runtime projections or infer backend selection from retained source/stream identity

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

### Requirement: Unary raw authority baseline coherence and hostile client bounds

Unary tests SHALL prove warning preservation, raw native response identity and typed transport replacement rather than comparing only permissively parsed success. Raw HTTP/schema assertions SHALL independently establish strict server correctness. A baseline change SHALL update pins/lockfiles/captures/classification/client behavior coherently. Hostile fake-server tests SHALL prove bounded reads and cleanup independently of Gateway implementation.

#### Scenario: Unary raw authority baseline coherence and hostile client bounds
- **WHEN** the pinned client permissively accepts output and a fake endpoint exceeds limits
- **THEN** raw/schema checks SHALL remain independent authority, hostile tests SHALL prove bounded cleanup and baseline changes SHALL update all associated evidence coherently

### Requirement: Authenticated client evidence provenance and discovery comparison

Authenticated black-box protocol-test-host tests SHALL run over HTTP without Apache production imports of Gateway code. Synthetic responses SHALL NOT establish live provider or private Vercel-service parity, and authentic provider fixture inputs SHALL NOT be rewritten. Configured-discovery tests SHALL additionally prove Go typed retention and the approved TS helper against the same protocol test host, while explicitly preserving stock normalized TS extension loss.

#### Scenario: Authenticated client evidence provenance and discovery comparison
- **WHEN** Go and the approved TS helper inspect configured discovery through the protocol test host
- **THEN** both SHALL retain matching configured facts while stock normalized TS loses the extension, with no Apache production Gateway import or synthetic live-parity claim

### Requirement: Configured discovery privacy and inference boundary

Configured facts SHALL be available without inference and without credentials or unrelated account state; this SHALL NOT imply new runtime response/error identity retention.

#### Scenario: Configured discovery privacy and inference boundary
- **WHEN** an authorized discovery test returns configured candidates without model calls
- **THEN** facts SHALL be available without inference, credentials or unrelated account state and SHALL NOT imply runtime response/error identity retention

### Requirement: Source response consumption

The independent Go client SHALL decode URL/document sources in unary/stream responses without Gateway imports. Required document title and source ID SHALL accept empty strings but reject missing/wrong-type required fields. Optional URL title/document filename absence and empty string SHALL normalize to empty Go strings. Native IDs, display, order and currently supplied object-valued metadata SHALL survive without rewriting, deduplication or variant-specific collision repair.

#### Scenario: URL and document consumption
- **WHEN** bounded readers receive both registered variants
- **THEN** native variant fields, identity and display SHALL survive, including empty required document title and source ID
- **AND** missing required title/ID SHALL fail

#### Scenario: Equal and repeated source IDs
- **WHEN** repeated URL sources and a document share a native ID
- **THEN** all SHALL retain the supplied ID and relative order without deduplication

### Requirement: Source decoder errors legacy fields and evidence limits

Unknown source discriminators and malformed metadata SHALL use the existing bounded protocol-error path. Unary Title SHALL be populated and Text retained for legacy consumers. Metadata preservation in this decoder SHALL NOT imply that the server's outstanding metadata projection gap is solved.

#### Scenario: Source decoder errors legacy fields and evidence limits
- **WHEN** a unary source has valid display fields and a later source has an unknown discriminator
- **THEN** the valid source SHALL populate Title and legacy Text, and the invalid source SHALL use bounded protocol failure without claiming the server metadata gap is solved

### Requirement: Independent opaque stream metadata consumption

The Grafana client SHALL independently decode providerMetadata on supported text/reasoning start/delta/end, reasoning-file, source, tool-input start/delta/end, function call, basic result, custom and finish parts. It SHALL preserve namespace objects, nested values, original event order and absent versus empty presence without IncludeRawChunks.

#### Scenario: Unknown namespace survives all supported placements
- **WHEN** valid events carry future object-valued namespaces with nested null/false/zero/empty values and explicit empty metadata at supported positions
- **THEN** the independent Go client SHALL preserve those values and positions without importing server code

#### Scenario: Malformed metadata on finish
- **WHEN** a bounded finish contains a null namespace or malformed metadata shape
- **THEN** the client SHALL emit at most one bounded protocol PartError and close without delivering the invalid finish or metadata-free success

### Requirement: Malformed stream metadata terminal behavior

Invalid metadata SHALL use the bounded terminal non-retryable protocol-error path without reflecting the value or delivering the invalid part. Gateway lifecycle and raw filtering SHALL remain unchanged.

#### Scenario: Malformed stream metadata terminal behavior
- **WHEN** finish contains a null metadata namespace
- **THEN** the client SHALL emit a bounded non-retryable terminal error without reflecting the value or delivering invalid finish, leaving lifecycle and raw filtering unchanged

### Requirement: BYOK request projection without catalog coupling
The Go client SHALL emit representable provider/model IDs and providerOptions.gateway.byok using the same outer model header and JSON provider-options namespace as the registered Vercel client. It SHALL preserve multiple-provider maps and ordered credential arrays for unary and streaming calls without client-side catalog lookup, automatic discovery, endpoint changes or native account selection.

#### Scenario: Ordered credentials are serialized
- **WHEN** CallOptions contains multiple provider entries and ordered credentials
- **THEN** unary and streaming request captures SHALL match equivalent pinned-client semantic bodies and preserve credential order
- **AND** Gateway host controls SHALL remain ordinary representable client options, with service-owned capability validation

#### Scenario: BYOK model is not configured
- **WHEN** a caller constructs a model for a valid provider/model string absent from configured discovery
- **THEN** the client SHALL issue inference directly without requiring a catalog entry

#### Scenario: Request metadata is caller-owned
- **WHEN** either client returns metadata for a BYOK request
- **THEN** its submitted credential subtree SHALL remain part of the local request representation
- **AND** documentation and tests SHALL distinguish that representation from server reflection and redacted consumer capture

### Requirement: Caller-owned request metadata and capture guidance
Both Go and exact-pinned Vercel requests SHALL emit the standard BYOK subtree in unary and streaming bodies. Their returned caller-owned request metadata SHALL retain the submitted arguments under the existing client contract; the server SHALL NOT claim it can sanitize those local objects. Guidance SHALL separate credential-aware automatic capture from direct application inspection.

#### Scenario: Actual consumer capture
- **WHEN** Go logging enables provider-options and request-body capture for both unary and streaming BYOK calls
- **THEN** its configured redactor SHALL protect matching credential fields while retaining noncredential request content and leaving caller/provider data unchanged

#### Scenario: Stock Vercel request metadata
- **WHEN** the registered Vercel client returns request.body after BYOK submission
- **THEN** evidence SHALL show the local BYOK subtree is present
- **AND** guidance SHALL identify direct logging of that metadata as caller-owned credential exposure, without changing the HTTP request or caller result

### Requirement: Credential-aware client capture boundaries
Go logger capture SHALL apply its existing Redactor policy to matching sensitive fields in supported structured SDK captures. Direct application logging, including TypeScript logging, SHALL remain caller-owned; guidance SHALL warn against logging credential-bearing metadata and recommend omitting credentials from a copy. This change SHALL NOT provide a TypeScript redaction helper.

#### Scenario: Capture evidence respects supported APIs
- **WHEN** enrichment and Agent Observability capture paths are inspected and tested with dummy markers
- **THEN** evidence SHALL NOT enable unsupported capture APIs or remove ordinary application content

### Requirement: Transport-safe model selectors
Model construction SHALL accept nonempty valid UTF-8 selectors up to 2,048 bytes
without whitespace or control characters, independently of configured-catalog
grammar. The server SHALL remain responsible for selector capability validation.

#### Scenario: Native suffix exceeds catalog grammar
- **WHEN** a valid selector contains native punctuation or exceeds the former 128-byte catalog limit
- **THEN** the client SHALL preserve it without discovery or rewriting

#### Scenario: Invalid transport selector
- **WHEN** a selector is empty, invalid UTF-8, over 2,048 bytes or contains whitespace/control characters
- **THEN** model construction SHALL fail before I/O

### Requirement: Provider approval consumption and continuation
The independent Go client SHALL decode unary and streamed provider approval requests, retain opaque metadata and tool/approval IDs, and encode required false approval decisions and presence-aware execution-denied outputs compatibly with the registered Vercel client. Client encoding SHALL preserve provider prompt role boundaries.

#### Scenario: Approval output round trip
- **WHEN** a provider approval request is received and a valid V4 decision continuation is submitted
- **THEN** the independent client SHALL preserve the IDs, decision and registered metadata/options without adding application-only approval requests to provider prompts

#### Scenario: High-level Go continuation
- **WHEN** Go GenerateText or StreamText resumes actual response history containing a provider-owned approval and decision
- **THEN** core prompt conversion SHALL retain only registered approval-response fields at the provider boundary after handling ownership, without mutating the caller's history, and the next Gateway/native request SHALL succeed
- **AND** GenerateText response messages SHALL contain a correlated execution-denied result for each denied provider-owned approval while StreamText response messages SHALL NOT, matching the registered Vercel client

### Requirement: Independent generated-file response decoding
The Apache Grafana client SHALL decode registered generated files in unary and streaming responses without importing Gateway code. It SHALL preserve data/URL values, optional originalUrl, media type, metadata presence and ordering; unary responses SHALL remain within the fixed buffered-read backstop. Provider-local filenames SHALL not become normalized generated-file fields.

#### Scenario: Generated file response is decoded
- **WHEN** the server returns a valid generated file using data or URL in unary or SSE output
- **THEN** the client SHALL expose the corresponding provider file value with metadata intact and selected empty data preserved

#### Scenario: Malformed or oversized response
- **WHEN** a generated file has malformed consumed fields or its unary response exceeds the buffered-read backstop
- **THEN** existing bounded protocol-error, cancellation and body-ownership behavior SHALL terminate consumption

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

### Requirement: Validated platform credential constructors
The client SHALL provide distinct typed constructors for CAP-to-access-token exchange (`NewWithTokenExchange`), caller-managed access tokens (`NewWithAccessToken`) and Grafana service account tokens (`NewWithServiceAccountToken`). The token-exchange flow SHALL support a per-call on-behalf-of user. The public API SHALL remove `NewWithCloudCredentials`, `CloudCredentialsConfig` and `WithUserIDToken` without compatibility aliases.

#### Scenario: Token-exchange provider is constructed
- **WHEN** valid token-exchange configuration with an explicit audience is supplied
- **THEN** the provider SHALL construct an authlib token exchanger for that audience and namespace

#### Scenario: Pre-minted token provider is constructed
- **WHEN** valid direct access-token configuration is supplied
- **THEN** model and discovery calls SHALL use that token without making a token-exchange request

#### Scenario: Service account provider is constructed
- **WHEN** a valid service account token and base URL are supplied to `NewWithServiceAccountToken`
- **THEN** the client SHALL be ready without an audience, namespace, exchange URL or token exchanger

#### Scenario: Removed API is absent
- **WHEN** exported declarations are enumerated
- **THEN** no Cloud stack/CAP constructor, Cloud credential configuration type or acting-user context helper SHALL exist

### Requirement: Platform route base URL and credential validation
Each constructor SHALL require a valid HTTP(S) ProviderWire API-prefix base URL and reject user info, query and fragment components. It SHALL accept optional configured outer headers and HTTP client, and SHALL NOT accept client-processing limits. Credentials SHALL be nonempty valid UTF-8 without whitespace or control characters and SHALL NOT be parsed or prefix-checked.

#### Scenario: Unsafe constructor input
- **WHEN** construction receives user info in its base URL or whitespace in a token
- **THEN** construction SHALL fail before authentication or Gateway I/O without parsing the token or reading defaults from the environment

### Requirement: Selected platform credential flows
Each request SHALL use only its constructor-selected credential flow, without detection, fallback or retry with another credential. Exchange and access-token flows SHALL send exactly one `X-Access-Token`. A call context MAY carry an on-behalf-of subject token for the exchange flow. The service account flow SHALL send exactly one `Authorization: Bearer <token>`. No flow SHALL send `X-Grafana-Id`, and preparation failures or cancellation SHALL occur before Gateway I/O.

#### Scenario: Token is exchanged
- **WHEN** a model or discovery call uses the token-exchange constructor
- **THEN** the configured CAP token SHALL be exchanged through authlib using the call context and the returned token SHALL be sent once as `X-Access-Token`

#### Scenario: Call on behalf of a user
- **WHEN** a token-exchange call context carries a user ID token or a delegated access token received from the platform
- **THEN** the exchange SHALL use that token as its subject for the configured namespace, and the request SHALL send only the returned on-behalf-of token as `X-Access-Token`

#### Scenario: On-behalf-of user with another flow
- **WHEN** a service-account or access-token call context carries an on-behalf-of subject token, or the exchange namespace is a wildcard
- **THEN** the call SHALL fail before token exchange or Gateway I/O

#### Scenario: Service account authenticates every operation
- **WHEN** `ListModels`, `DoGenerate` or `DoStream` is invoked with a service account token
- **THEN** the request SHALL carry that token only in one outer `Authorization` bearer header and no token exchange SHALL occur

#### Scenario: Exchange is canceled or fails
- **WHEN** token exchange returns an error or its context is canceled
- **THEN** the client SHALL return a sanitized exchange error or recognizable context error and SHALL NOT issue the Gateway request

#### Scenario: Credential is rejected
- **WHEN** the platform rejects the credential
- **THEN** the client SHALL use existing bounded error handling without trying another flow

#### Scenario: Redirect attempts to forward credentials
- **WHEN** the endpoint responds with a redirect
- **THEN** the client SHALL NOT follow it or send credentials to the redirect target

### Requirement: Platform credential client evidence
Client credential evidence SHALL use the protocol test host's platform header shapes and actor chains. The pinned Vercel client SHALL use a service account token as `apiKey`, and its internal-caller access SHALL be described as unverified until checked against the platform. Test-signed tokens SHALL NOT be described as production platform authorization.

#### Scenario: Both clients as a service account
- **WHEN** Go `NewWithServiceAccountToken` and pinned Vercel `createGateway` send equivalent BYOK unary and streaming calls in `cloud` mode
- **THEN** the test host SHALL observe equivalent bearer credentials and standard `providerOptions.gateway.byok` bodies, and native providers SHALL receive only the selected BYOK credentials

#### Scenario: Go as an internal service
- **WHEN** Go exchange or access-token clients call configured models in `cloud` mode, directly or on behalf of a user
- **THEN** the test host SHALL observe the access token only in `X-Access-Token`, and configured discovery, unary and streaming calls SHALL succeed without BYOK

#### Scenario: Both clients as a self-hosted service account
- **WHEN** Go `NewWithServiceAccountToken` and pinned Vercel `createGateway` call configured models in `self-hosted` mode
- **THEN** configured discovery, unary and streaming calls SHALL succeed without BYOK

#### Scenario: Evidence is described accurately
- **WHEN** client compatibility is documented
- **THEN** exact-client captures and test-host results SHALL be distinguished from deployed platform authentication, network isolation and live provider acceptance

### Requirement: Platform-hosted authentication guidance
The shared guide under docs SHALL explain the plugin route prefix URL and both access modes. It SHALL include tested Go examples and an exact-pinned Vercel service-account example, with no legacy-mode or migration guidance. Deployment configuration SHALL remain in operator guidance.

#### Scenario: External caller follows the guide
- **WHEN** a Go or server-side Vercel caller follows the service account setup
- **THEN** the example SHALL use the route prefix, a service account token, provider/model selection and request-scoped API keys, and SHALL state that configured discovery is unavailable in `cloud` mode

#### Scenario: Internal caller follows the guide
- **WHEN** an internal service follows the token-exchange setup
- **THEN** the example SHALL use configured model names without BYOK and explain the audience `grafana-aigateway-app`, namespace, caller-owned token refresh and on-behalf-of calls with a user ID token or delegated access token

#### Scenario: Self-hosted caller follows the guide
- **WHEN** an application follows the self-hosted setup
- **THEN** the example SHALL use a Grafana service account token per application or person to attribute, and configured model names without BYOK, and SHALL state that catalog access follows Grafana's plugin app access

#### Scenario: Platform error responses are explained
- **WHEN** the guide describes failures
- **THEN** it SHALL state that platform-produced errors are not ProviderWire error documents and are classified as protocol errors by the clients

#### Scenario: Documentation examples are verified
- **WHEN** documentation examples are accepted
- **THEN** Go examples SHALL build, TypeScript examples SHALL typecheck against the registered baseline and documentation navigation SHALL remain valid

### Requirement: Server-owned resource bounds and buffered-read backstop

The client SHALL expose no configurable response-processing limits. The Gateway SHALL own resource bounds, and callers SHALL bound work through their `http.Client` and call context. Every buffered read SHALL share one fixed 2 GiB backstop. This covers unary success, non-2xx error bodies including stream setup failures, and discovery. Exceeding the backstop SHALL fail without truncation. This is a parity-preserving match of the registered upstream download limit.

#### Scenario: Limit configuration is absent
- **WHEN** exported declarations and constructor configuration fields are enumerated
- **THEN** no limit type, default-limit function or per-config limit field SHALL exist

#### Scenario: Buffered body crosses the backstop
- **WHEN** a unary, error or discovery body is exactly at the backstop or one byte above it
- **THEN** the body at the backstop SHALL be accepted, and the body above it SHALL fail with a non-retryable bounded protocol error that returns no partial or truncated result

#### Scenario: Declared length crosses the backstop
- **WHEN** a response to a unary, error or discovery read declares a `Content-Length` above the backstop
- **THEN** the client SHALL fail with the same non-retryable bounded protocol error before reading any of the body, and a declared length at or below the backstop, or an unknown length, SHALL be bounded while reading
- **AND** context cancellation or deadline expiry SHALL take precedence over the declared-length failure and remain discoverable with `errors.Is`, with no body read

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
