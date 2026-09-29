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

Each constructor SHALL require a valid HTTP(S) ProviderWire API-prefix base URL, SHALL reject user info, query, and fragment components, and SHALL accept optional configured outer headers, HTTP client, and positive client-processing limits. Direct Cloud credentials SHALL require a positive stack ID and a nonempty, valid-UTF-8 CAP token without whitespace or control characters, without parsing the opaque token or requiring a token prefix. Token exchange SHALL require CAP token, token-exchange URL, and namespace and SHALL default an omitted audience to `ai-sdk`; direct access-token authentication SHALL require a non-empty access token. Construction SHALL not discover credentials or endpoints from ambient environment variables. Constructors SHALL preserve immutable client/header configuration and refusal to follow redirects.

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

### Requirement: Context-aware Grafana authentication
Every Gateway request SHALL authenticate using only the flow selected at construction; the client SHALL NOT detect token types, fall back between flows, or retry with another credential after rejection.

The Cloud-credential flow SHALL send exactly one outer `Authorization: Bearer <stack-id>:<CAP-token>` header with the stack ID formatted as decimal digits. It SHALL NOT perform token exchange, construct an authlib exchanger, send `X-Access-Token` or `X-Grafana-Id`, or emit trusted stack/policy assertions. Constructor-supplied credentials SHALL NOT be inserted into ProviderWire request bodies or client-generated diagnostics. A nonempty context-carried acting-user token SHALL cause an error before Gateway network I/O for this flow.

The token-exchange and pre-minted access-token flows SHALL obtain one access token using the constructor's context-aware token source and place it in `X-Access-Token`. For these JWT flows, `WithUserIDToken` SHALL attach an optional acting-user token to a context and the client SHALL place a non-empty attached token in `X-Grafana-Id`. Authorization alone SHALL NOT replace JWT authentication for those flows. The client MUST NOT mint tokens locally or implement an additional CAP-token cache. Authentication failures and cancellation SHALL occur before Gateway network I/O when detected during request preparation.

#### Scenario: CAP authenticates every supported operation
- **WHEN** `ListModels`, `DoGenerate` or `DoStream` is invoked with Cloud credentials
- **THEN** the request SHALL carry the same configured stack/CAP bearer credential only in the outer Authorization header
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

### Requirement: Exact ProviderWire routes and protected headers
The configured base URL SHALL be treated as the ProviderWire API prefix and the client SHALL append exactly `/config` for discovery and `/language-model` for calls without discarding an existing path prefix. Every model call SHALL use `POST`, JSON content type, the requested model ID, specification version `4`, exact streaming value `true` or `false`, and the matching JSON or SSE accept type. Configured headers SHALL be applied before call-level headers, followed by client-owned content negotiation, selected authentication, acting-user, and protocol headers so client-owned values each have one effective value and cannot be overridden. Accepted call-level headers SHALL also remain in the JSON request body.

For the Cloud-credential flow, `Authorization`, `X-Access-Token`, `X-Grafana-Id`, `X-Scope-OrgID`, `X-Cloud-Org-ID`, and `X-Access-Policy-ID` SHALL be reserved case-insensitively. Supplying any reserved name in configured headers SHALL fail construction. Supplying any reserved name in call-level headers SHALL fail before request serialization and network I/O, including when its value is empty. JWT flows SHALL retain existing header ownership behavior.

#### Scenario: Unary request is emitted
- **WHEN** `DoGenerate` is invoked for model `assistant`
- **THEN** the request SHALL be `POST <base-prefix>/language-model` with streaming `false`, model ID `assistant`, specification `4`, JSON content and accept types, and exactly one value for each client-owned header

#### Scenario: Streaming request is emitted
- **WHEN** `DoStream` is invoked
- **THEN** the request SHALL use the same method, route, model, and specification headers with streaming `true` and SSE accept type

#### Scenario: Header names collide
- **WHEN** configured or call-level headers attempt to set client-owned headers and are not rejected by the Cloud reserved-header rule
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

### Requirement: Cloud credential client evidence
Automated tests SHALL compare the Go Cloud-credential flow with the exact registered Vercel Gateway client configured with `apiKey: "<stack-id>:<CAP-token>"`. They SHALL exercise discovery, unary generation and streaming through a deterministic test-only authenticating edge and the existing Cloud-mode Gateway command. The edge SHALL use dummy credentials and predetermined scope/stack outcomes, not claim to implement production CAP validation. Existing JWT and exchange evidence SHALL remain passing under the renamed API.

#### Scenario: Both clients use the Cloud edge contract
- **WHEN** Go and pinned Vercel clients perform equivalent supported calls with the same dummy stack/CAP credential
- **THEN** the edge SHALL observe equivalent bearer authentication with no token exchange
- **AND** the Gateway SHALL receive only the edge's stack assertion, without Authorization, access-token or acting-user headers
- **AND** both clients SHALL consume the expected discovery, unary and streaming results without changing the existing ProviderWire body projection

#### Scenario: Edge rejects credentials or scope
- **WHEN** either client supplies an invalid dummy credential or a credential without the fixture's required read/write authorization
- **THEN** the edge SHALL reject it without reaching Gateway handlers or model providers

#### Scenario: Privacy and isolation remain intact
- **WHEN** Cloud credential calls traverse the test edge and command
- **THEN** configured model-provider credentials SHALL remain server-owned
- **AND** outbound provider requests, application logs, metrics and client-generated request metadata SHALL not contain the Cloud credential

#### Scenario: Evidence is described accurately
- **WHEN** test results or support status are documented
- **THEN** local edge-shim evidence SHALL be distinguished from live CAP validation, policy revocation/expiry and deployment network-isolation evidence

### Requirement: Shared Go and Vercel authentication guidance
User-facing guidance SHALL explain how Go and server-side Vercel clients authenticate to Grafana AI Gateway, using one shared guide under `docs/` linked from the documentation index and existing client/server entry points. The guide SHALL distinguish application-user login, Gateway credentials and server-owned model-provider credentials. It SHALL cover URL and credential selection, least-privilege CAP scopes and stack access, HTTPS, secure storage and rotation without exposing internal proxy names, backend listener configuration or test-harness details. Exhaustive Go API reference SHALL remain in godoc.

#### Scenario: User chooses a Cloud client
- **WHEN** a Go or Vercel user follows the public Cloud setup
- **THEN** the guide SHALL show the public Gateway URL and direct stack/CAP configuration for that client, with no token-exchange prerequisite
- **AND** examples SHALL stay server-side and use placeholder credentials and tested request options

#### Scenario: User chooses a separately provided JWT-enabled URL
- **WHEN** a Go user has a JWT-enabled Gateway URL and either token-exchange credentials or a short-lived access token
- **THEN** the guide SHALL identify the separate URL, appropriate Go constructor and caller-owned refresh when supplying an access token
- **AND** it SHALL not suggest using a JWT for the public Cloud URL or a built-in Grafana JWT configuration for the Vercel client

#### Scenario: User provisions least privilege
- **WHEN** a user follows credential provisioning guidance
- **THEN** it SHALL explain `ai-gateway:read` for discovery, `ai-gateway:write` for inference, and limiting policy access to the needed stacks
- **AND** it SHALL keep CAP credentials on trusted servers rather than in browsers or untrusted workers

#### Scenario: Guide remains external-facing
- **WHEN** authentication guidance is updated
- **THEN** the shared guide SHALL focus on URL choice and client configuration, without private proxy names, internal credential-forwarding headers, migration notes, troubleshooting steps or local test-evidence caveats
- **AND** server trust-boundary and conformance evidence SHALL remain in their separate operator and parity documents

#### Scenario: Documentation examples are verified
- **WHEN** the guide's Go and Vercel examples are accepted
- **THEN** their configurations and demonstrated operations SHALL be compiled or typechecked and exercised by deterministic tests using the registered package versions
- **AND** documentation links/navigation SHALL pass the repository docs checks

### Requirement: Explicit request projection and presence
The client SHALL explicitly map the complete current `provider.CallOptions` shape into the registered LanguageModelV4 Gateway request projection without importing server DTOs or calling server validators. It SHALL preserve every representable absent, explicit zero, explicit false, empty string, empty array, empty object, nested null, selected empty union arm, URL string, and supported binary-to-base64 distinction. It SHALL omit the Go zero value `ReasoningProviderDefault` and encode every non-zero registered reasoning value. It SHALL reject invalid UTF-8, non-finite numeric values, invalid raw JSON, unknown discriminators, conflicting selected arms, and any other value without an unambiguous registered representation before authentication or network I/O.

#### Scenario: Presence-sensitive text request is encoded
- **WHEN** a text/scalar call contains explicit zero, false, empty collections, empty strings, and opaque nested JSON
- **THEN** the semantic body SHALL match the equivalent request emitted by registered `@ai-sdk/gateway@4.0.87` for every Go-representable distinction

#### Scenario: File data is representable
- **WHEN** a currently representable selected binary or URL data arm is supplied
- **THEN** bytes SHALL become standard base64, URLs SHALL become strings, the selected arm SHALL remain selected, and the strict server SHALL own current capability rejection

#### Scenario: Reasoning uses the Go default
- **WHEN** `Reasoning` is `ReasoningProviderDefault`
- **THEN** the body SHALL omit `reasoning` and parity evidence SHALL classify the unrepresentable explicit JavaScript `provider-default` presence as a parity-preserving Go adaptation

#### Scenario: Provider-domain input is invalid
- **WHEN** any selected value cannot be mapped unambiguously to the registered projection
- **THEN** both unary and streaming setup SHALL fail before token acquisition or HTTP and SHALL not silently omit or reinterpret the value

### Requirement: Bounded normalized unary consumption
For a successful unary response, the client SHALL require a JSON media type, read no more than the configured unary-response byte limit, accept one complete JSON document, and map only registered text content, finish reason, and usage into provider.GenerateResult. It SHALL reject malformed required fields, unknown content or finish discriminators, negative or non-JavaScript-safe known usage, trailing JSON, and oversized input. The client SHALL replace server-supplied request and response: Request.Body SHALL be the locally encoded request, and Response.Headers and Response.Body SHALL come from the bounded HTTP response. Warnings SHALL preserve valid server warning fields in order, defaulting to a non-nil empty slice when absent or null. Warning decoding SHALL use the same registered types and validation as streaming. Server response identity and provider-private metadata SHALL not be adopted.

#### Scenario: Minimal unary success is consumed
- **WHEN** the server returns valid ordered text content, registered finish reason, and valid usage
- **THEN** the client SHALL return those fields plus local request metadata, HTTP response headers/body, and empty warnings when none were supplied

#### Scenario: Server supplies client-owned fields
- **WHEN** a successful body includes warnings, request metadata, response ID, model ID, timestamp, headers, body, or additional provider metadata
- **THEN** the result SHALL preserve valid warnings but ignore the other server-owned values in favor of the registered client-owned replacements

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
A successful streaming setup SHALL require SSE media type and return a `StreamResult` whose request body and response headers are client-owned. One goroutine SHALL own the body, parse incrementally under configured cumulative-byte, complete-event-byte, and event-count limits, send mapped parts with context-aware backpressure, close the body, and close the output channel exactly once. It SHALL not buffer the full response or an unbounded line/event. The initial WP7 mapper SHALL accept the WP5 text stream family, safe error parts, and bounded raw parts needed for pinned filtering parity; every unsupported, malformed, or oversized event SHALL emit at most one terminal non-retryable protocol `PartError` and close.

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
- **WHEN** the WP5 text client receives a reasoning, tool, file, source, custom, approval, or other later-package stream part
- **THEN** it SHALL produce an explicit protocol error rather than decoding through provider-domain JSON accidentally

### Requirement: Registered stream normalization
The client SHALL ignore an SSE data payload exactly equal to `[DONE]`, SHALL treat transport clean EOF as clean completion with or without a preceding finish, SHALL filter `raw` parts unless `IncludeRawChunks` is true, SHALL convert valid response-metadata timestamps into `time.Time`, and SHALL preserve response order. Context cancellation SHALL end the stream without manufacturing a provider error. A finish received before EOF SHALL be delivered before channel closure; the client SHALL not create a synthetic finish or require the server to emit `[DONE]`.

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

### Requirement: Closed Gateway error classification
Every non-2xx model or discovery response SHALL be read within the configured error-body limit and mapped only from the registered public error envelope into the closed categories authentication, forbidden, invalid request, model not found, rate limit, failed dependency, and internal server. The resulting `GatewayError` SHALL expose category, public code, public message, HTTP status, and status-derived retryability and SHALL unwrap to a bounded `*provider.APICallError`. Unknown or malformed error bodies, wrong media types, and transport failures SHALL use local bounded error text rather than copying arbitrary response bytes into the primary message. Context cancellation and deadlines SHALL remain discoverable with `errors.Is`.

#### Scenario: Registered error is returned
- **WHEN** the server returns one of the registered error type/status/code documents
- **THEN** `errors.As` SHALL find the matching `GatewayError` category and underlying `*provider.APICallError`, and retryability SHALL match `@ai-sdk/gateway@4.0.87`

#### Scenario: Error body is malformed or oversized
- **WHEN** a non-2xx body is malformed, has a wrong media type, or exceeds the error limit
- **THEN** the client SHALL return a bounded local `*provider.APICallError` without accepting a partial category or exposing the full body in its message

#### Scenario: Transport fails
- **WHEN** HTTP transport fails without context cancellation
- **THEN** the client SHALL return a retryable `*provider.APICallError` that retains the transport cause

#### Scenario: Stream error event is received
- **WHEN** a valid registered error event appears in SSE
- **THEN** it SHALL become an ordered `PartError` with the equivalent bounded `APICallError`, preserving the Go provider contract without ending later valid parts by itself

### Requirement: Authenticated public discovery
`Provider.ListModels(ctx)` SHALL issue authenticated `GET /config`, read within the configured discovery limit, and return only public ID, name, optional description, and the specification version/provider/model-ID triple. It SHALL validate required values, registered `v4` and `grafana` identity, model-ID consistency, valid public IDs, and duplicate IDs; it SHALL ignore unknown additive members accepted by the registered client. It SHALL preserve response order, represent aliases as independent rows exactly as served, and MUST NOT infer or expose canonical-to-backend mappings, credentials, provider instances, or fallback topology.

#### Scenario: Canonical and alias rows are discovered
- **WHEN** the authenticated service returns a canonical model and alias row
- **THEN** both rows SHALL be returned in server order with only their public discovery fields

#### Scenario: Discovery contains additive metadata
- **WHEN** otherwise valid discovery rows or the root document contain unknown members
- **THEN** the client SHALL ignore those members without exposing them through `ModelInfo`

#### Scenario: Discovery is structurally unsafe
- **WHEN** the document is oversized, malformed, duplicated, contains an invalid ID, mismatched specification model ID, non-`v4` specification, or non-`grafana` provider
- **THEN** discovery SHALL fail atomically with no partial catalog result

### Requirement: No implicit client retry or backend selection
The Grafana client SHALL issue at most one Gateway model request per `DoGenerate` or `DoStream` invocation after token acquisition. It SHALL preserve retryability for existing SDK retry/fallback orchestration but MUST NOT select physical providers, traverse Gateway candidates, retry through the retired endpoint, or retry after any response or stream event. Public results and errors MUST NOT expose backend identity or fallback topology.

#### Scenario: Retryable setup error occurs
- **WHEN** the Gateway returns a retryable non-2xx response
- **THEN** the invocation SHALL return that retryable error after one Gateway request and leave any retry decision to its caller

#### Scenario: Stream error occurs after output
- **WHEN** an error event or transport failure occurs after a stream part has been delivered
- **THEN** the client SHALL not issue another HTTP request or change model identity

### Requirement: Exact-pinned differential and black-box evidence
Automated tests SHALL compare equivalent Go and registered `@ai-sdk/gateway@4.0.87` scenarios for semantic method, path, effective protocol and call headers, body presence, normalized unary and stream results, error category, retryability, cancellation, discovery, `[DONE]`, raw filtering, timestamp conversion, and EOF. Tests SHALL use the versions registered in `test/conformance/upstream.yaml`; a baseline change SHALL update pins, lockfiles, captures, classification, and client behavior together. Separate hostile fake-server tests SHALL prove all client bounds and resource cleanup. Authenticated black-box tests SHALL exercise the work-package-5 command over HTTP without importing Gateway implementation packages into Apache production code.

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
- **THEN** the Go client SHALL complete discovery, unary text, streaming text, acting-user propagation, cancellation, and registered errors without exposing configured credentials or private backend identity
