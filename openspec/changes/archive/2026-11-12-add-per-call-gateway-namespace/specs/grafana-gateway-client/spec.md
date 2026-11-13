# Spec Delta

## ADDED Requirements

### Requirement: Per-call namespace selection
A token-exchange call context SHALL be able to carry a namespace, set with `WithNamespace`, that selects the stack for `ListModels`, `DoGenerate` and `DoStream`. The client SHALL resolve one namespace per call and SHALL use it both as the route namespace and, unless the configured namespace is the wildcard `*`, as the exchange namespace.

#### Scenario: One provider serves two stacks
- **WHEN** one token-exchange provider without a configured namespace serves a call with namespace `stacks-1` and a call with namespace `stacks-2`
- **THEN** each exchange SHALL request its call's namespace, and each Gateway request SHALL target `/namespaces/<namespace>` under the configured root and carry only the token exchanged for that namespace

#### Scenario: Every operation uses the call namespace
- **WHEN** `ListModels`, `DoGenerate` and `DoStream` run with the same call namespace
- **THEN** discovery, unary and streaming requests SHALL all use that namespace for route and exchange

### Requirement: Call namespace resolution
With a concrete configured namespace, a call without a namespace or with the same namespace SHALL use the configured one, and any other call namespace SHALL be a conflict. With no configured namespace, the call namespace SHALL be required. With the wildcard, the call namespace SHALL be required and SHALL select only the route, while the exchange SHALL remain for every namespace.

#### Scenario: Configured namespace is the default
- **WHEN** a provider configured with namespace `stacks-1` serves a call without a namespace or with namespace `stacks-1`
- **THEN** the call SHALL use `stacks-1` for route and exchange

#### Scenario: Call namespace conflicts with the configured namespace
- **WHEN** a provider configured with namespace `stacks-1` serves a call with namespace `stacks-2`
- **THEN** the call SHALL fail before token exchange or Gateway I/O

#### Scenario: Call namespace is missing
- **WHEN** a provider with no configured namespace or the wildcard serves a call without a namespace
- **THEN** the call SHALL fail before token exchange or Gateway I/O

#### Scenario: Wildcard exchange with per-call routes
- **WHEN** a provider configured with namespace `*` serves calls with namespaces `stacks-1` and `stacks-2` as itself
- **THEN** both exchanges SHALL request namespace `*`, and each Gateway request SHALL target its call's namespace

### Requirement: Call namespace validation and conflicts
A call namespace SHALL be a DNS label: 1 to 63 lowercase alphanumerics or `-`, starting and ending with an alphanumeric. A missing, invalid or conflicting call namespace, and any call namespace on a service-account or access-token provider, SHALL fail before token exchange or Gateway I/O without echoing the value in the error.

#### Scenario: Call namespace is invalid
- **WHEN** a call namespace is empty, `*`, uppercase, longer than 63 bytes or contains `/`, `.` or whitespace
- **THEN** the call SHALL fail before token exchange or Gateway I/O

#### Scenario: Call namespace on a fixed-stack credential
- **WHEN** a service-account or access-token provider serves a call with a namespace
- **THEN** the call SHALL fail before Gateway I/O rather than ignore the namespace

### Requirement: Namespace-scoped exchange sharing
Concurrent token exchanges SHALL be shared only between calls with the same exchange namespace and subject. An access token exchanged for one namespace SHALL NOT be sent on a call for another namespace, except that a token exchanged for the wildcard SHALL serve every route namespace of its provider. Each caller SHALL still return when its own context ends.

#### Scenario: Concurrent calls for different namespaces
- **WHEN** calls with the same subject and namespaces `stacks-1` and `stacks-2` exchange concurrently
- **THEN** the client SHALL perform one exchange per namespace, and each call SHALL send the token for its own namespace

#### Scenario: Concurrent calls for the same namespace and subject
- **WHEN** several calls with the same namespace and subject exchange concurrently
- **THEN** they SHALL share one exchange

### Requirement: Token-exchange configured namespace
The token-exchange namespace SHALL be optional and, when set, SHALL be a DNS label or the wildcard `*`.

#### Scenario: Configured namespace is invalid
- **WHEN** token-exchange construction receives a namespace that is neither a DNS label nor `*`
- **THEN** construction SHALL fail before I/O

### Requirement: Base URL shape per credential flow
For a platform-hosted Gateway, the service-account and access-token base URL SHALL be the namespaced ProviderWire API prefix, `/apis/<group>/<version>/namespaces/<namespace>` on the platform origin. The token-exchange base URL SHALL be the Gateway API root, `/apis/<group>/<version>`, and the client SHALL append `/namespaces/<namespace>` for each call's resolved namespace.

#### Scenario: Token-exchange request is emitted
- **WHEN** a token-exchange provider with base URL `https://grafana.example/apis/aigateway.ext.grafana.app/v1alpha1` invokes `DoGenerate` for resolved namespace `stacks-1`
- **THEN** the request SHALL be `POST https://grafana.example/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-1/language-model`, and discovery SHALL use `.../namespaces/stacks-1/config`

## MODIFIED Requirements

### Requirement: Exchange and token constructor defaults and immutability

Token exchange SHALL require CAP token, token-exchange URL and audience, with no default audience. Direct access-token authentication SHALL require a non-empty access token, and service-account authentication SHALL require a non-empty service-account token. Construction SHALL not discover credentials, endpoints or namespaces from ambient environment variables. Constructors SHALL preserve immutable client/header configuration and refuse redirects, including on token exchanges.

#### Scenario: Exchange and token constructor defaults and immutability
- **WHEN** token-exchange construction omits audience, or valid construction is followed by caller code mutating its header map
- **THEN** construction without an audience SHALL fail before I/O, and valid client/header configuration SHALL remain immutable with redirects refused

#### Scenario: Caller trusts a private certificate authority
- **WHEN** token-exchange construction receives an HTTP client whose transport trusts a private certificate authority
- **THEN** the token exchange and every Gateway request SHALL use that client

### Requirement: Exact ProviderWire routes and protected headers

The client SHALL append exactly `/config` for discovery and `/language-model` for calls to the namespaced ProviderWire API prefix, without discarding an existing path prefix. Every model call SHALL use `POST`, JSON content type, the requested model ID, specification version `4`, exact streaming value `true` or `false`, and the matching JSON or SSE accept type.

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

### Requirement: Validated platform credential constructors
The client SHALL provide distinct typed constructors for CAP-to-access-token exchange (`NewWithTokenExchange`), caller-managed access tokens (`NewWithAccessToken`) and Grafana service account tokens (`NewWithServiceAccountToken`). The token-exchange flow SHALL support a per-call on-behalf-of user and a per-call namespace. The public API SHALL remove `NewWithCloudCredentials`, `CloudCredentialsConfig` and `WithUserIDToken` without compatibility aliases.

#### Scenario: Token-exchange provider is constructed
- **WHEN** valid token-exchange configuration with an explicit audience is supplied
- **THEN** the provider SHALL construct an authlib token exchanger for that audience, with the configured namespace, when set, as the default for calls

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
Each constructor SHALL require a valid HTTP(S) base URL of the shape its flow defines and reject user info, query and fragment components. The token-exchange base URL SHALL NOT contain a `namespaces` path segment. Each constructor SHALL accept optional configured outer headers and HTTP client, and SHALL NOT accept client-processing limits. Credentials SHALL be nonempty valid UTF-8 without whitespace or control characters and SHALL NOT be parsed or prefix-checked.

#### Scenario: Unsafe constructor input
- **WHEN** construction receives user info in its base URL or whitespace in a token
- **THEN** construction SHALL fail before authentication or Gateway I/O without parsing the token or reading defaults from the environment

#### Scenario: Namespaced token-exchange base URL
- **WHEN** token-exchange construction receives a base URL ending in `/namespaces/stacks-1`
- **THEN** construction SHALL fail before I/O

### Requirement: Selected platform credential flows
Each request SHALL use only its constructor-selected credential flow, without detection, fallback or retry with another credential. Exchange and access-token flows SHALL send exactly one `X-Access-Token`. A call context MAY carry an on-behalf-of subject token and a namespace for the exchange flow. The service account flow SHALL send exactly one `Authorization: Bearer <token>`. No flow SHALL send `X-Grafana-Id`, and preparation failures or cancellation SHALL occur before Gateway I/O.

#### Scenario: Token is exchanged
- **WHEN** a model or discovery call uses the token-exchange constructor
- **THEN** the configured CAP token SHALL be exchanged through authlib for the call's resolved exchange namespace using the call context, and the returned token SHALL be sent once as `X-Access-Token`

#### Scenario: Call on behalf of a user
- **WHEN** a token-exchange call context carries a user ID token or a delegated access token received from the platform
- **THEN** the exchange SHALL use that token as its subject for the call's resolved namespace, and the request SHALL send only the returned on-behalf-of token as `X-Access-Token`

#### Scenario: On-behalf-of user with another flow
- **WHEN** a service-account or access-token call context carries an on-behalf-of subject token, or the resolved exchange namespace is the wildcard
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

#### Scenario: Go as an internal service for two stacks
- **WHEN** one Go exchange client without a configured namespace calls configured models in `cloud` mode for two namespaces, directly and on behalf of a user
- **THEN** the test host SHALL observe each request on its call's namespace route with a token whose namespace covers that route, and configured discovery, unary and streaming calls SHALL succeed for both namespaces

#### Scenario: Both clients as a self-hosted service account
- **WHEN** Go `NewWithServiceAccountToken` and pinned Vercel `createGateway` call configured models in `self-hosted` mode
- **THEN** configured discovery, unary and streaming calls SHALL succeed without BYOK

#### Scenario: Evidence is described accurately
- **WHEN** client compatibility is documented
- **THEN** exact-client captures and test-host results SHALL be distinguished from deployed platform authentication, network isolation and live provider acceptance

### Requirement: Platform-hosted authentication guidance
The shared guide under docs SHALL explain the plugin route prefix URL, the token-exchange API root URL and both access modes. It SHALL include tested Go examples and an exact-pinned Vercel service-account example, with no legacy-mode or migration guidance. Deployment configuration SHALL remain in operator guidance.

#### Scenario: External caller follows the guide
- **WHEN** a Go or server-side Vercel caller follows the service account setup
- **THEN** the example SHALL use the route prefix, a service account token, provider/model selection and request-scoped API keys, and SHALL state that configured discovery is unavailable in `cloud` mode

#### Scenario: Internal caller follows the guide
- **WHEN** an internal service follows the token-exchange setup
- **THEN** the guide SHALL list its inputs: the access policy token, the token-exchange URL, the Gateway API root URL, the stack namespace and, when calling the platform inside its network, an HTTP client that trusts the certificate authority signing the platform's certificate
- **AND** the example SHALL use configured model names without BYOK and explain the Grafana Cloud audience `aigateway.ext.grafana.app`, which the caller's access policy must allow, namespace selection, caller-owned token refresh and on-behalf-of calls with a user ID token or delegated access token

#### Scenario: Multi-stack service follows the guide
- **WHEN** a service that serves several stacks follows the token-exchange setup
- **THEN** the guide SHALL show one provider without a configured namespace whose calls each select their stack namespace, and SHALL state that tokens are exchanged and cached per namespace

#### Scenario: Self-hosted caller follows the guide
- **WHEN** an application follows the self-hosted setup
- **THEN** the example SHALL use a Grafana service account token per application or person to attribute, and configured model names without BYOK, and SHALL state that catalog access follows Grafana's plugin app access

#### Scenario: Platform error responses are explained
- **WHEN** the guide describes failures
- **THEN** it SHALL state that platform-produced errors are not ProviderWire error documents and are classified as protocol errors by the clients

#### Scenario: Documentation examples are verified
- **WHEN** documentation examples are accepted
- **THEN** Go examples SHALL build, TypeScript examples SHALL typecheck against the registered baseline and documentation navigation SHALL remain valid
