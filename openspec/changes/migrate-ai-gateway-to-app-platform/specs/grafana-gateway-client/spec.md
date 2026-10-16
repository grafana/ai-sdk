# Spec Delta

## MODIFIED Requirements

### Requirement: Exchange and token constructor defaults and immutability

Token exchange SHALL require CAP token, token-exchange URL, namespace and audience, with no default audience; direct access-token authentication SHALL require a non-empty access token, and service-account authentication SHALL require a non-empty service-account token. Construction SHALL not discover credentials or endpoints from ambient environment variables. Constructors SHALL preserve immutable client/header configuration and refusal to follow redirects.

#### Scenario: Exchange and token constructor defaults and immutability
- **WHEN** token-exchange construction omits audience, or valid construction is followed by caller code mutating its header map
- **THEN** construction without an audience SHALL fail before I/O, and valid client/header configuration SHALL remain immutable with redirects refused

### Requirement: Exact ProviderWire routes and protected headers

The configured base URL SHALL be treated as the ProviderWire API prefix, which for a platform-hosted Gateway is `/apis/<group>/<version>/namespaces/<namespace>` on the platform origin, and the client SHALL append exactly `/config` for discovery and `/language-model` for calls without discarding an existing path prefix. Every model call SHALL use `POST`, JSON content type, the requested model ID, specification version `4`, exact streaming value `true` or `false`, and the matching JSON or SSE accept type.

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

### Requirement: Authenticated public discovery
ListModels SHALL read authenticated bounded /config into public typed rows and optional configured-route facts using ordinary Go decoding. It SHALL preserve order, ignore additive fields and avoid server-policy revalidation or inferred configuration. Discovery by a BYOK caller SHALL report its fixed unsupported-operation error.

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
- **WHEN** ListModels authenticates as a caller with BYOK access
- **THEN** it SHALL surface the bounded invalid-request error stating discovery is unsupported for BYOK
- **AND** it SHALL NOT return an empty catalog, try another credential or use discovery as a prerequisite for inference

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
- **WHEN** the repository integration suite runs the protocol test host with test-signed platform tokens and provider fakes serving supported native values
- **THEN** both clients SHALL preserve contracted values while configured credentials and cross-tenant state remain absent
- **AND** the Go client SHALL complete discovery, unary text, streaming text, cancellation and registered errors, and pinned TS configured discovery SHALL run as a self-hosted service account
- **AND** canonical operator identity and metadata-only capture SHALL remain independent of returned native values and authorized configured discovery

#### Scenario: Configured discovery consumers are compared
- **WHEN** Go ListModels, pinned TS getAvailableModels and the shipped TS helper inspect canonical model rows with configured alias metadata
- **THEN** normalized public fields SHALL remain compatible, Go and the helper SHALL retain matching configured facts, stock TS SHALL still strip the extension and provider inference counts SHALL remain zero

### Requirement: Authenticated client evidence provenance and discovery comparison

Authenticated black-box protocol-test-host tests SHALL run over HTTP without Apache production imports of Gateway code. Synthetic responses SHALL NOT establish live provider or private Vercel-service parity, and authentic provider fixture inputs SHALL NOT be rewritten. Configured-discovery tests SHALL additionally prove Go typed retention and the approved TS helper against the same protocol test host, while explicitly preserving stock normalized TS extension loss.

#### Scenario: Authenticated client evidence provenance and discovery comparison
- **WHEN** Go and the approved TS helper inspect configured discovery through the protocol test host
- **THEN** both SHALL retain matching configured facts while stock normalized TS loses the extension, with no Apache production Gateway import or synthetic live-parity claim

### Requirement: Public authentication guide security and reference boundaries

The shared guide SHALL cover URL and credential selection, least-privilege service account roles, access policy scopes and namespace access, HTTPS, secure storage and rotation without exposing internal deployment names, host configuration or test-harness details. Exhaustive Go API reference SHALL remain in godoc.

#### Scenario: Public authentication guide security and reference boundaries
- **WHEN** a user reads credential provisioning instructions
- **THEN** the guide SHALL cover least privilege, HTTPS, storage and rotation without internal deployment/host/harness details, leaving exhaustive API reference in godoc

## ADDED Requirements

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
Each constructor SHALL require a valid HTTP(S) ProviderWire API-prefix base URL and reject user info, query and fragment components. It SHALL accept optional configured outer headers, HTTP client and positive client-processing limits. Credentials SHALL be nonempty valid UTF-8 without whitespace or control characters and SHALL NOT be parsed or prefix-checked.

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

## REMOVED Requirements

### Requirement: Validated provider construction
**Reason**: The Cloud stack/CAP constructor and the `ai-sdk` audience default are removed.
**Migration**: Use "Validated platform credential constructors".

### Requirement: Gateway constructor URL and Cloud credential validation
**Reason**: Cloud stack/CAP credentials are removed.
**Migration**: Use "Platform route base URL and credential validation".

### Requirement: Context-aware Grafana authentication
**Reason**: The Cloud flow and acting-user propagation are removed.
**Migration**: Use "Selected platform credential flows".

### Requirement: Cloud authentication isolation and acting-user refusal
**Reason**: The Cloud flow and acting-user propagation are removed.
**Migration**: Use "Selected platform credential flows".

### Requirement: Context-aware JWT headers and token ownership
**Reason**: Acting-user headers are removed; credential ownership is per flow.
**Migration**: Use "Selected platform credential flows".

### Requirement: Client header precedence and Cloud reserved names
**Reason**: Cloud assertion headers are no longer reserved.
**Migration**: Use "Exact ProviderWire routes and protected headers".

### Requirement: Cloud reserved-header refusal timing
**Reason**: Cloud assertion headers are no longer reserved.
**Migration**: Use "Exact ProviderWire routes and protected headers".

### Requirement: Cloud credential client evidence
**Reason**: The dummy edge and unified command are removed.
**Migration**: Use "Platform credential client evidence".

### Requirement: Renamed JWT and exchange regression evidence
**Reason**: The rename transition is complete and superseded by the new constructors.
**Migration**: Use "Validated platform credential constructors" and "Platform credential client evidence".

### Requirement: Shared Go and Vercel authentication guidance
**Reason**: Cloud and private-endpoint guidance is replaced.
**Migration**: Use "Platform-hosted authentication guidance".
