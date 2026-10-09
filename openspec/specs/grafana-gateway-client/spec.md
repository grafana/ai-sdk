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

### Requirement: Validated provider construction

The client SHALL support direct Cloud Access Policy authentication, CAP-to-access-token exchange, and caller-managed short-lived access tokens through distinct typed constructors. `NewWithCloudCredentials` SHALL accept `CloudCredentialsConfig` containing `StackID int64` and `CAPToken`; `NewWithTokenExchange` SHALL accept `TokenExchangeConfig`; `NewWithAccessToken` SHALL retain `AccessTokenConfig`. The public API SHALL remove `NewWithCloudAuth` and `CloudAuthConfig` without compatibility aliases.

#### Scenario: Cloud credential provider is constructed
- **WHEN** valid stack ID, CAP token and base URL are supplied to `NewWithCloudCredentials`
- **THEN** the client SHALL be ready for direct Cloud authentication without an audience, namespace, exchange URL or token exchanger

#### Scenario: Token-exchange provider is constructed
- **WHEN** valid token-exchange configuration omits the audience
- **THEN** the provider SHALL construct an authlib token exchanger that requests audience `ai-sdk` for the configured namespace

#### Scenario: Pre-minted token provider is constructed
- **WHEN** valid direct access-token configuration is supplied
- **THEN** model and discovery calls SHALL use that token without making a token-exchange request

#### Scenario: Configuration is invalid
- **WHEN** a required credential, namespace, endpoint, or model ID is empty or invalid, the Cloud stack ID is non-positive, a URL is unsafe, or a configured limit is non-positive or cannot be represented safely
- **THEN** construction or model resolution SHALL fail before authentication or Gateway network I/O

#### Scenario: Exchange API migration
- **WHEN** the public API and repository-owned token-exchange call sites are inspected
- **THEN** they SHALL use `NewWithTokenExchange` and `TokenExchangeConfig`, with no old-name aliases
- **AND** exchange caching, audience defaults, cancellation and sanitized failures SHALL retain their existing behavior

### Requirement: Gateway constructor URL and Cloud credential validation

Each constructor SHALL require a valid HTTP(S) ProviderWire API-prefix base URL, SHALL reject user info, query, and fragment components, and SHALL accept optional configured outer headers, HTTP client, and positive client-processing limits. Direct Cloud credentials SHALL require a positive stack ID and a nonempty, valid-UTF-8 CAP token without whitespace or control characters, without parsing the opaque token or requiring a token prefix.

#### Scenario: Gateway constructor URL and Cloud credential validation
- **WHEN** Cloud construction receives user info in its base URL or whitespace in CAPToken
- **THEN** construction SHALL fail before authentication or Gateway I/O without parsing the opaque token or discovering defaults from environment

### Requirement: Exchange and token constructor defaults and immutability

Token exchange SHALL require CAP token, token-exchange URL, and namespace and SHALL default an omitted audience to `ai-sdk`; direct access-token authentication SHALL require a non-empty access token. Construction SHALL not discover credentials or endpoints from ambient environment variables. Constructors SHALL preserve immutable client/header configuration and refusal to follow redirects.

#### Scenario: Exchange and token constructor defaults and immutability
- **WHEN** token-exchange construction omits audience and later caller code mutates its header map
- **THEN** audience SHALL default to ai-sdk and client/header configuration SHALL remain immutable with redirects refused

### Requirement: Context-aware Grafana authentication
Each request SHALL use only its constructor-selected flow, without credential fallback. Cloud credentials SHALL use one outer stack/CAP bearer header; JWT flows SHALL use the context-aware token source and optional bound acting-user header. Preparation failure/cancellation SHALL occur before Gateway I/O.

#### Scenario: Context-aware Grafana authentication policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** Every Gateway request SHALL authenticate using only the flow selected at construction against the corresponding public Cloud or private JWT endpoint; the client SHALL NOT detect token types, fall back between flows, or retry with another credential after rejection.
- **AND** The Cloud-credential flow SHALL send exactly one outer `Authorization: Bearer <stack-id>:<CAP-token>` header with the stack ID formatted as decimal digits. It SHALL NOT perform token exchange, construct an authlib exchanger, send `X-Access-Token` or `X-Grafana-Id`, or emit trusted stack/policy assertions. Constructor-supplied credentials SHALL NOT be inserted into ProviderWire request bodies or client-generated diagnostics. A nonempty context-carried acting-user token SHALL cause an error before Gateway network I/O for this flow.
- **AND** The token-exchange and pre-minted access-token flows SHALL obtain one access token using the constructor's context-aware token source and place it in `X-Access-Token`. For these JWT flows, `WithUserIDToken` SHALL attach an optional acting-user token to a context and the client SHALL place a non-empty attached token in `X-Grafana-Id`. The Go token source SHALL remain authoritative; unrelated caller-supplied Authorization SHALL NOT replace it. The client MUST NOT mint tokens locally or implement an additional CAP-token cache. Authentication failures and cancellation SHALL occur before Gateway network I/O when detected during request preparation.

#### Scenario: CAP authenticates every supported operation
- **WHEN** `ListModels`, `DoGenerate` or `DoStream` is invoked with Cloud credentials
- **THEN** the request SHALL carry the same configured stack/CAP bearer credential only in the outer Authorization header; inference SHALL require BYOK and authenticated discovery SHALL return the unsupported-operation error
- **AND** no token-exchange request SHALL occur

#### Scenario: Cloud token is exchanged
- **WHEN** a model or discovery call uses the token-exchange constructor
- **THEN** the configured CAP token SHALL be exchanged through authlib using the call context and the returned access token SHALL be sent as `X-Access-Token`

#### Scenario: Acting user is present
- **WHEN** a JWT-flow call context contains a non-empty token from `WithUserIDToken`
- **THEN** the Gateway request SHALL contain that token once as `X-Grafana-Id`

#### Scenario: Acting user is absent
- **WHEN** the call context has no acting-user token or an empty token
- **THEN** the request SHALL omit `X-Grafana-Id`

#### Scenario: Acting user is incompatible with Cloud credentials
- **WHEN** a Cloud-credential call context contains a nonempty token from `WithUserIDToken`
- **THEN** the call SHALL fail before network I/O without silently discarding the acting-user intent

#### Scenario: Exchange is canceled or fails
- **WHEN** token exchange returns an error or its context is canceled
- **THEN** the client SHALL retain existing sanitized exchange errors and recognizable context cancellation and SHALL not issue the Gateway request

#### Scenario: Cloud call is already canceled
- **WHEN** a Cloud discovery or model call begins with a canceled context
- **THEN** the client SHALL return a recognizable context error without network I/O

#### Scenario: Cloud credential is rejected
- **WHEN** the edge rejects the CAP credential or its authorization
- **THEN** the client SHALL use existing bounded error handling without trying exchange, JWT headers or another endpoint

#### Scenario: Redirect attempts to forward CAP
- **WHEN** the endpoint responds with a redirect
- **THEN** the client SHALL not follow it or send credentials to the redirect target

### Requirement: Cloud authentication isolation and acting-user refusal

The Cloud-credential flow SHALL NOT perform token exchange, construct an authlib exchanger, send `X-Access-Token` or `X-Grafana-Id`, or emit trusted stack/policy assertions. Constructor-supplied credentials SHALL NOT be inserted into ProviderWire request bodies or client-generated diagnostics. A nonempty context-carried acting-user token SHALL cause an error before Gateway network I/O for the Cloud-credential flow.

#### Scenario: Cloud authentication isolation and acting-user refusal
- **WHEN** a Cloud call carries a nonempty context acting-user token
- **THEN** it SHALL fail before Gateway I/O rather than exchanging tokens, emitting JWT/assertion headers or leaking constructor credentials into body/diagnostics

### Requirement: Context-aware JWT headers and token ownership

The token-exchange and pre-minted access-token flows SHALL obtain one access token using the constructor's context-aware token source and place it in `X-Access-Token`. For these JWT flows, `WithUserIDToken` SHALL attach an optional acting-user token to a context and the client SHALL place a non-empty attached token in `X-Grafana-Id`. Authorization alone SHALL NOT replace JWT authentication for those flows. The client MUST NOT mint tokens locally or implement an additional CAP-token cache.

#### Scenario: Context-aware JWT headers and token ownership
- **WHEN** a token-exchange call supplies a nonempty WithUserIDToken token
- **THEN** the request SHALL use X-Access-Token and X-Grafana-Id without substituting Authorization, local minting or a second CAP cache

### Requirement: Authentication preparation failure boundary

Authentication failures and cancellation SHALL occur before Gateway network I/O when detected during request preparation.

#### Scenario: Authentication preparation failure boundary
- **WHEN** authentication preparation observes cancellation or a token-source failure
- **THEN** the Gateway request SHALL NOT be issued

### Requirement: Exact ProviderWire routes and protected headers

The configured base URL SHALL be treated as the ProviderWire API prefix and the client SHALL append exactly `/config` for discovery and `/language-model` for calls without discarding an existing path prefix. Every model call SHALL use `POST`, JSON content type, the requested model ID, specification version `4`, exact streaming value `true` or `false`, and the matching JSON or SSE accept type.

#### Scenario: Header ownership applies to every authentication flow
- **WHEN** constructor or call-level authentication headers are supplied
- **THEN** For every authentication flow, `Authorization`, `X-Access-Token`, `X-Grafana-Id`, `X-Scope-OrgID`, `X-Cloud-Org-ID`, and `X-Access-Policy-ID` SHALL be reserved case-insensitively. Supplying any reserved name in configured headers SHALL fail construction. Supplying any reserved name in call-level headers SHALL fail before request serialization and network I/O, including when its value is empty. JWT constructors SHALL own X-Access-Token and optional acting-user header composition; unrelated configured/call headers SHALL NOT add a competing Authorization credential or trusted Cloud assertion.

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

### Requirement: Client header precedence and Cloud reserved names

Configured headers SHALL be applied before call-level headers, followed by client-owned content negotiation, selected authentication, acting-user, and protocol headers so client-owned values each have one effective value and cannot be overridden. Accepted call-level headers SHALL also remain in the JSON request body. For the Cloud-credential flow, `Authorization`, `X-Access-Token`, `X-Grafana-Id`, `X-Scope-OrgID`, `X-Cloud-Org-ID`, and `X-Access-Policy-ID` SHALL be reserved case-insensitively.

#### Scenario: Client header precedence and Cloud reserved names
- **WHEN** configured and call headers collide with client-owned protocol values
- **THEN** client-owned values SHALL have one effective value, accepted call headers SHALL remain in the body and Cloud credential/identity names SHALL remain reserved case-insensitively

### Requirement: Cloud reserved-header refusal timing

Supplying any Cloud reserved header name in configured headers SHALL fail construction. Supplying any Cloud reserved header name in call-level headers SHALL fail before request serialization and network I/O, including when its value is empty. JWT flows SHALL retain existing header ownership behavior.

#### Scenario: Cloud reserved-header refusal timing
- **WHEN** a call-level Authorization header has an empty value
- **THEN** the Cloud flow SHALL fail before serialization/network I/O, while configured reserved names SHALL fail construction and JWT ownership SHALL remain unchanged

### Requirement: Cloud credential client evidence
Go and registered Vercel clients SHALL prove equivalent Cloud/BYOK requests through a dummy authenticating edge and a unified command concurrently serving private JWT requests. Configured discovery SHALL be private; Cloud discovery SHALL prove authenticated unsupported handling, without claiming real CAP authorization.

#### Scenario: Cloud credential client evidence policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** Automated tests SHALL compare the Go Cloud flow with the exact registered Vercel client configured with apiKey "<stack-id>:<CAP-token>". Both SHALL issue catalog-independent BYOK unary/streaming requests through a deterministic dummy authenticating edge and one unified command also serving private JWT requests. Successful configured discovery SHALL use private JWT access; Cloud discovery SHALL prove authenticated unsupported-operation handling. Dummy edges SHALL NOT be described as production CAP verification.

#### Scenario: Both clients use the Cloud edge contract
- **WHEN** Go and pinned Vercel send equivalent supported BYOK calls
- **THEN** the edge SHALL observe equivalent Cloud bearer credentials and standard providerOptions.gateway.byok bodies
- **AND** the application SHALL receive the edge's stack assertion without Gateway auth credentials
- **AND** native providers SHALL receive only selected BYOK authentication and matching content/options

#### Scenario: Edge rejects credentials or scope
- **WHEN** the test edge rejects Cloud credentials or inference scope
- **THEN** no application/provider work SHALL occur and neither client SHALL switch authentication methods

#### Scenario: Two populations share one command
- **WHEN** private JWT and Cloud clients concurrently call the same process
- **THEN** their configured and BYOK accounts SHALL remain isolated and only the private clients SHALL receive configured discovery

#### Scenario: Privacy and isolation remain intact
- **WHEN** request emission and capture are inspected
- **THEN** Gateway authentication credentials SHALL be absent from request bodies, provider requests and automatic server capture
- **AND** local BYOK request metadata SHALL retain its submitted subtree while consumer logger capture applies configured sensitive-field redaction without censoring ordinary fields or returned provider data
- **AND** synthetic edge/provider evidence SHALL not establish live policy or network isolation

#### Scenario: Evidence is described accurately
- **WHEN** client compatibility is documented
- **THEN** exact-client captures and dummy edge/provider tests SHALL be distinguished from deployed CAP enforcement, network isolation and live provider acceptance

### Requirement: Renamed JWT and exchange regression evidence

Existing JWT and exchange evidence SHALL remain passing under the renamed API.

#### Scenario: Renamed JWT and exchange regression evidence
- **WHEN** JWT and token-exchange tests run using the renamed API
- **THEN** their existing evidence SHALL remain passing

### Requirement: Shared Go and Vercel authentication guidance
The shared user-facing guide under docs SHALL explain the public Cloud/BYOK endpoint and private JWT/configured endpoint of one deployment. It SHALL distinguish application-user identity, Gateway authentication and provider credentials, with tested Go and exact-pinned Vercel examples and no legacy-mode or migration guidance. Exhaustive API reference SHALL remain in godoc; network/JWKS/listener configuration SHALL remain in operator guidance.

#### Scenario: User chooses a Cloud client
- **WHEN** a Go or server-side Vercel caller follows the public setup
- **THEN** the example SHALL use stack/CAP Gateway credentials, inference scope, provider/model selection and request-scoped API keys
- **AND** it SHALL explicitly state that configured discovery and configured-account fallback are unavailable

#### Scenario: User chooses a separately provided JWT-enabled URL
- **WHEN** an internal caller follows the private setup
- **THEN** Go SHALL use its access-token or token-exchange constructor and Vercel SHALL use apiKey with an explicit short-lived JWT
- **AND** the example SHALL use configured discovery/model names without BYOK
- **AND** audience ai-sdk, concrete/wildcard namespace context and caller-owned token refresh SHALL be explained

#### Scenario: User provisions least privilege
- **WHEN** guidance describes token/key handling and telemetry
- **THEN** it SHALL require appropriate trusted execution, HTTPS or the documented protected internal transport, least privilege and rotation
- **AND** it SHALL explain that caller-owned request metadata contains BYOK and demonstrate configured field-aware capture redaction

#### Scenario: Guide remains external-facing
- **WHEN** the shared guide describes client setup
- **THEN** it SHALL use public client APIs and documentation links, leaving internal deployment/JWKS/listener configuration in operator guidance

#### Scenario: Documentation examples are verified
- **WHEN** documentation examples are accepted
- **THEN** Go examples SHALL build, TypeScript examples SHALL typecheck against the registered baseline, demonstrated behavior SHALL have deterministic tests and documentation navigation SHALL remain valid

### Requirement: Public authentication guide security and reference boundaries

The shared guide SHALL cover URL and credential selection, least-privilege CAP scopes and stack access, HTTPS, secure storage and rotation without exposing internal proxy names, backend listener configuration or test-harness details. Exhaustive Go API reference SHALL remain in godoc.

#### Scenario: Public authentication guide security and reference boundaries
- **WHEN** a user reads Cloud credential provisioning instructions
- **THEN** the guide SHALL cover least privilege, HTTPS, storage and rotation without internal proxy/listener/harness details, leaving exhaustive API reference in godoc

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
- **WHEN** the response is one byte larger than the configured unary limit or has trailing data
- **THEN** the call SHALL fail without returning a partial result or retaining an unbounded body

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

Required payload checks, opaque metadata structure and transport resource bounds SHALL remain unchanged. For successful unary responses, the client SHALL require JSON media type, read within its unary limit, accept one complete valid document and map its supported registered content, including provider calls and results, finishReason, usage, warnings and ordinary result/content providerMetadata into provider.GenerateResult.

#### Scenario: Bounded unary document and required payload consumption
- **WHEN** a successful response has valid ordered content and ordinary metadata at the configured byte limit
- **THEN** the client SHALL consume one complete JSON document into GenerateResult with required payload, metadata and transport bounds intact

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

When a successful HTTP 200 `DoGenerate` response body fails to read because of transport I/O after headers, the Grafana client SHALL return a retryable `*provider.APICallError` with HTTP status 200, a bounded locally worded primary message, and a discoverable underlying read cause, without returning a partial result or making another model request. Wrong media type, malformed JSON, malformed consumed fields, and unary byte-limit violations SHALL remain non-retryable protocol failures.

#### Scenario: HTTP 200 response body is interrupted after headers
- **WHEN** the real HTTP handler sends JSON headers and a partial, below-limit body with a declared longer `Content-Length`, then closes the connection before the body completes
- **THEN** `DoGenerate` SHALL return no result and one retryable `*provider.APICallError` with status 200, a locally bounded primary message that does not disclose the response body, and the underlying body-read failure discoverable through its cause chain, after exactly one model request

#### Scenario: Successful unary body cannot be accepted for protocol reasons
- **WHEN** a successful unary response has a wrong media type, malformed JSON, invalid consumed result fields, or exceeds the configured unary byte limit, including when an over-limit partial body and a transport read error occur together
- **THEN** `DoGenerate` SHALL return no partial result and a non-retryable bounded protocol error after exactly one model request; the unary byte limit SHALL take precedence over the read error, except that context cancellation or deadline expiry SHALL retain highest precedence

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
- **WHEN** the client receives file, custom, approval or another unsupported later-package stream part
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

### Requirement: Supported SSE tool payload and marker preservation

The mapper SHALL accept supported text, function-tool and provider-tool calls/results, safe error parts and bounded raw parts needed for registered filtering behavior. Supported execution/dynamic/preliminary markers and opaque tool metadata SHALL be preserved under gateway-provider-metadata. Input-start dynamic SHALL retain absent, explicit false and true independently through decoding; it SHALL NOT be eagerly defaulted. A deferred result SHALL NOT require a repeated call in the same response.

#### Scenario: Supported SSE tool payload and marker preservation
- **WHEN** input-start contains dynamic false and a deferred provider result arrives without a repeated call
- **THEN** the client SHALL preserve explicit false presence, supported ownership/preview markers and opaque metadata without requiring repeated calls

### Requirement: SSE protocol failure without server lifecycle dialect

Every unsupported, malformed or oversized event SHALL emit at most one terminal non-retryable protocol PartError and close. Server lifecycle validation SHALL NOT be imported into the client as a new independent protocol dialect.

#### Scenario: SSE protocol failure without server lifecycle dialect
- **WHEN** an event has an unknown discriminator or exceeds its configured event limit
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

On a successful unary result and a streaming finish, the Grafana client SHALL preserve a supplied `usage.raw` as a `provider.Usage.Raw` JSON object, including nested provider-native fields and `{}`; absent raw SHALL remain absent. The client SHALL reject present null, scalar, array, malformed or incomplete JSON, and a retained raw object larger than 1,048,576 bytes.

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

### Requirement: Retained raw-object and original-document limits

The 1,048,576-byte raw-object limit applies to the retained representation after JSON decoding/compaction removes insignificant whitespace around and inside the value; the original complete unary response or event SHALL remain bounded by its configured `UnaryBytes` or `StreamEventBytes`. The client SHALL validate those original bounded documents for UTF-8 and JSON syntax before Go decoding can normalize invalid bytes.

#### Scenario: Retained raw-object and original-document limits
- **WHEN** usage.raw contains substantial JSON whitespace in an otherwise bounded document
- **THEN** the 1,048,576-byte raw cap SHALL apply after compaction, while original response/event bounds and pre-decode UTF-8/syntax validation SHALL still apply

### Requirement: Raw usage escape preservation and bounded intermediates

Valid JSON with lone or paired escaped surrogates SHALL be accepted, with raw-object escapes preserved in `provider.Usage.Raw`. The existing cumulative-stream and event-count limits SHALL still apply, and intermediate `decodeFields`/usage-map copies SHALL remain bounded by the full-response or event limits.

#### Scenario: Raw usage escape preservation and bounded intermediates
- **WHEN** usage.raw contains a lone escaped UTF-16 surrogate in a valid object
- **THEN** the escape SHALL be accepted and retained with cumulative/event-count bounds and bounded intermediate usage copies

### Requirement: Raw usage independence from raw stream filtering

The client SHALL continue to validate known normalized token counts and filter unrelated unknown usage and unrepresented transport fields without filtering ordinary registered providerMetadata; distinct `type: "raw"` stream-part filtering SHALL remain governed by `IncludeRawChunks` and SHALL NOT filter `usage.raw`.

#### Scenario: Raw usage independence from raw stream filtering
- **WHEN** IncludeRawChunks is false and finish carries normalized counts plus usage.raw
- **THEN** normalized counts SHALL still be validated and usage.raw and registered metadata SHALL survive independently of raw-part filtering

### Requirement: Closed Gateway error classification

Every non-2xx model or discovery response SHALL be read within the configured error-body limit and mapped only from the registered public error envelope into the closed categories authentication, forbidden, invalid request, model not found, rate limit, failed dependency, and internal server. The resulting `GatewayError` SHALL expose category, public code, public message, HTTP status, and status-derived retryability and SHALL unwrap to a bounded `*provider.APICallError`.

#### Scenario: Registered error is returned
- **WHEN** the server returns one of the registered error type/status/code documents
- **THEN** `errors.As` SHALL find the matching `GatewayError` category and underlying `*provider.APICallError`, and retryability SHALL match the Gateway version registered in test/conformance/upstream.yaml, independently of nested native retry facts

#### Scenario: Error members use noncanonical property casing
- **WHEN** an otherwise valid HTTP error envelope or committed error payload uses case-insensitive Go matches for its member names
- **THEN** typed error decoding SHALL accept those members using standard encoding/json behavior
- **AND** retained HTTP documents and selected SSE data SHALL preserve their original JSON bytes without changing classification or retryability rules

#### Scenario: Error body is malformed or oversized
- **WHEN** a non-2xx body is malformed, has a wrong media type, or exceeds the error limit
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
ListModels SHALL read authenticated bounded /config into public typed rows and optional configured-route facts using ordinary Go decoding. It SHALL preserve order, ignore additive fields and avoid server-policy revalidation or inferred configuration. Cloud/BYOK discovery SHALL report its fixed unsupported-operation error.

#### Scenario: Authenticated public discovery policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** `Provider.ListModels(ctx)` SHALL issue authenticated `GET /config`, read within the configured discovery limit, and return public ID, name, optional description and the specification version/provider/model-ID triple, plus optional typed `ModelInfo.Gateway *ConfiguredRoute`. `ConfiguredRoute` SHALL expose `Aliases`, `Primary` and ordered `Fallbacks`; each `ConfiguredCandidate` SHALL expose `ProviderInstance`, `Provider` and `ProviderModelID`. This SHALL remain the existing discovery method, without a second client or AGPL module dependency.
- **AND** The client SHALL use ordinary Go JSON decoding into typed ModelInfo values after enforcing the existing configurable document-byte limit and raw UTF-8 JSON validity. It SHALL NOT revalidate server-owned public-ID grammar, nonblank strings, specification/model-ID agreement, route cardinality, duplicate IDs/aliases/candidate tuples or route-group consistency. Standard Go JSON behavior SHALL apply, including case-insensitive field matching, zero/nil values for missing/null fields and U+FFFD normalization of escaped lone UTF-16 surrogates. A missing/null models collection, malformed JSON, byte overflow or type-decoding error SHALL invalidate the complete result. It SHALL preserve response order and configured alias/fallback order exactly as served, without expanding aliases into extra rows. Missing/null gateway SHALL remain nil. Unknown additive members SHALL remain ignored. Configured mappings SHALL be retained only when supplied by the server, never inferred from responses, models or inventories; credentials and arbitrary configuration SHALL NOT be exposed.

#### Scenario: Configured models and aliases are discovered
- **WHEN** the authenticated service returns canonical model rows with configured route facts
- **THEN** each row SHALL be returned in server order with its public specification fields, typed aliases, primary and ordered fallbacks
- **AND** aliases SHALL remain available as selection IDs without duplicating rows

#### Scenario: Discovery contains additive metadata
- **WHEN** otherwise valid discovery rows or the root document contain unrelated unknown members
- **THEN** the client SHALL ignore those members without exposing them through ModelInfo
- **AND** a recognized gateway extension SHALL be decoded into typed fields and retained rather than discarded

#### Scenario: Discovery is structurally unsafe
- **WHEN** the document exceeds the read budget, is malformed JSON, has no models collection or contains a field that cannot decode into its declared Go type
- **THEN** discovery SHALL fail atomically with no partial catalog result

#### Scenario: Server has no configured extension
- **WHEN** a public discovery response omits gateway or supplies null
- **THEN** existing public rows SHALL remain consumable and each corresponding Gateway field SHALL be nil, with no inferred topology

#### Scenario: Caller inspects candidates without generation
- **WHEN** an authorized Go caller reads ListModels rows and inspects Gateway.Primary and Gateway.Fallbacks
- **THEN** configured order and provider-instance/provider/model facts SHALL be available without any model or inventory request

#### Scenario: BYOK endpoint does not provide discovery
- **WHEN** ListModels authenticates through the public Cloud/BYOK endpoint
- **THEN** it SHALL surface the bounded invalid-request error stating discovery is unsupported for BYOK
- **AND** it SHALL NOT return an empty catalog, try the private endpoint or use discovery as a prerequisite for inference

### Requirement: Independent ordinary typed discovery decoding

ListModels SHALL remain the existing discovery method, without a second client or AGPL module dependency. The client SHALL use ordinary Go JSON decoding into typed ModelInfo values after enforcing the existing configurable document-byte limit and raw UTF-8 JSON validity. It SHALL NOT revalidate server-owned public-ID grammar, nonblank strings, specification/model-ID agreement, route cardinality, duplicate IDs/aliases/candidate tuples or route-group consistency.

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
- **WHEN** fake endpoints exercise exact-limit and one-byte/one-event-over-limit discovery, unary, error, and SSE inputs plus cancellation under blocked delivery
- **THEN** tests SHALL prove bounded allocation/read behavior, body closure, channel closure, and absence of retained client goroutines

#### Scenario: Authenticated command is exercised
- **WHEN** the repository integration suite starts the command with deterministic auth and provider fakes serving supported native values
- **THEN** both clients SHALL preserve contracted values while configured credentials and cross-tenant state remain absent
- **AND** the Go client SHALL complete discovery, unary text, streaming text, acting-user propagation, cancellation and registered errors
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

Authenticated black-box command tests SHALL run over HTTP without Apache production imports of Gateway code. Synthetic responses SHALL NOT establish live provider or private Vercel-service parity, and authentic provider fixture inputs SHALL NOT be rewritten. Configured-discovery tests SHALL additionally prove Go typed retention and the approved TS helper against the same command, while explicitly preserving stock normalized TS extension loss.

#### Scenario: Authenticated client evidence provenance and discovery comparison
- **WHEN** Go and the approved TS helper inspect configured discovery through the command
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

The Grafana client SHALL independently decode providerMetadata on supported text/reasoning start/delta/end, reasoning-file, source, tool-input start/delta/end, function call, basic result and finish parts. It SHALL preserve namespace objects, nested values, original event order and absent versus empty presence without IncludeRawChunks. Original accepted events, retained copies and metadata cardinality SHALL remain bounded by the existing event, cumulative byte and count limits.

#### Scenario: Unknown namespace survives all supported placements
- **WHEN** valid events carry future object-valued namespaces with nested null/false/zero/empty values and explicit empty metadata at supported positions
- **THEN** the independent Go client SHALL preserve those values and positions under existing limits without importing server code

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
