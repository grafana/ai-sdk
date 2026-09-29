## MODIFIED Requirements

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

## ADDED Requirements

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
User-facing authentication guidance SHALL explain how Go and server-side Vercel clients authenticate to Grafana AI Gateway, using one shared guide under `docs/` linked from the documentation index and existing client/server entry points. The guide SHALL distinguish application-user login, gateway credentials and server-owned model-provider credentials. It SHALL describe credential/endpoint compatibility, CAP scopes and realms, stack identity, secure storage and transport, rotation, troubleshooting, and migration from the old exchange API. Exhaustive Go API reference SHALL remain in godoc.

#### Scenario: User chooses a Cloud client
- **WHEN** a Go or Vercel user follows the public Cloud setup
- **THEN** the guide SHALL show the public API-prefix URL and direct stack/CAP configuration for that client, with no token-exchange prerequisite
- **AND** examples SHALL stay server-side and use placeholder credentials and tested request options

#### Scenario: User chooses an internal JWT client
- **WHEN** a user reads about pre-minted JWTs or token exchange
- **THEN** the guide SHALL identify the required access-token-enabled endpoint and distinguish `X-Access-Token` from Cloud Authorization
- **AND** it SHALL explain that Vercel `apiKey` does not exchange CAP tokens or automatically send Grafana access-token headers
- **AND** it SHALL show the Go constructor/type rename and token refresh ownership

#### Scenario: User provisions least privilege
- **WHEN** a user follows credential provisioning guidance
- **THEN** it SHALL explain `ai-gateway:read` for discovery, `ai-gateway:write` for inference, and a realm authorizing the target stack
- **AND** it SHALL distinguish stack IDs from organization IDs and prohibit examples that distribute system CAP credentials to browsers or customer-controlled workers

#### Scenario: User encounters authentication failure
- **WHEN** a request fails
- **THEN** the guide SHALL help distinguish endpoint/header mismatch, local configuration errors, invalid credentials, insufficient scope/realm access and protocol capability rejection without suggesting auth bypass or fallback

#### Scenario: Documentation examples are verified
- **WHEN** the guide's Go and Vercel examples are accepted
- **THEN** their configurations and demonstrated operations SHALL be compiled or typechecked and exercised by deterministic tests using the registered package versions
- **AND** documentation links/navigation SHALL pass the repository docs checks
- **AND** the guide SHALL explicitly leave acting-user Cloud delegation, k6 session migration and live deployment verification outside the delivered feature
