## MODIFIED Requirements

### Requirement: Context-aware Grafana authentication
Every Gateway request SHALL authenticate using only the flow selected at construction against the corresponding public Cloud or private JWT endpoint; the client SHALL NOT detect token types, fall back between flows, or retry with another credential after rejection.

The Cloud-credential flow SHALL send exactly one outer `Authorization: Bearer <stack-id>:<CAP-token>` header with the stack ID formatted as decimal digits. It SHALL NOT perform token exchange, construct an authlib exchanger, send `X-Access-Token` or `X-Grafana-Id`, or emit trusted stack/policy assertions. Constructor-supplied credentials SHALL NOT be inserted into ProviderWire request bodies or client-generated diagnostics. A nonempty context-carried acting-user token SHALL cause an error before Gateway network I/O for this flow.

The token-exchange and pre-minted access-token flows SHALL obtain one access token using the constructor's context-aware token source and place it in `X-Access-Token`. For these JWT flows, `WithUserIDToken` SHALL attach an optional acting-user token to a context and the client SHALL place a non-empty attached token in `X-Grafana-Id`. The Go token source SHALL remain authoritative; unrelated caller-supplied Authorization SHALL NOT replace it. The client MUST NOT mint tokens locally or implement an additional CAP-token cache. Authentication failures and cancellation SHALL occur before Gateway network I/O when detected during request preparation.

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

### Requirement: Cloud credential client evidence
Automated tests SHALL compare the Go Cloud flow with the exact registered Vercel client configured with apiKey "<stack-id>:<CAP-token>". Both SHALL issue catalog-independent BYOK unary/streaming requests through a deterministic dummy authenticating edge and one unified command also serving private JWT requests. Successful configured discovery SHALL use private JWT access; Cloud discovery SHALL prove authenticated unsupported-operation handling. Dummy edges SHALL NOT be described as production CAP verification.

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
- **AND** local BYOK request metadata SHALL retain its submitted subtree while credential-aware consumer capture excludes it
- **AND** synthetic edge/provider evidence SHALL not establish live policy or network isolation

#### Scenario: Evidence is described accurately
- **WHEN** client compatibility is documented
- **THEN** exact-client captures and dummy edge/provider tests SHALL be distinguished from deployed CAP enforcement, network isolation and live provider acceptance

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
- **AND** it SHALL explain that caller-owned request metadata contains BYOK and demonstrate structural capture redaction

#### Scenario: Guide remains external-facing
- **WHEN** the shared guide describes client setup
- **THEN** it SHALL use public client APIs and documentation links, leaving internal deployment/JWKS/listener configuration in operator guidance

#### Scenario: Documentation examples are verified
- **WHEN** documentation examples are accepted
- **THEN** Go examples SHALL build, TypeScript examples SHALL typecheck against the registered baseline, demonstrated behavior SHALL have deterministic tests and documentation navigation SHALL remain valid

### Requirement: Authenticated public discovery
`Provider.ListModels(ctx)` SHALL issue authenticated `GET /config`, read within the configured discovery limit, and return public ID, name, optional description and the specification version/provider/model-ID triple, plus optional typed `ModelInfo.Gateway *ConfiguredRoute`. `ConfiguredRoute` SHALL expose `Aliases`, `Primary` and ordered `Fallbacks`; each `ConfiguredCandidate` SHALL expose `ProviderInstance`, `Provider` and `ProviderModelID`. This SHALL remain the existing discovery method, without a second client or AGPL module dependency.

The client SHALL use ordinary Go JSON decoding into typed ModelInfo values after enforcing the existing configurable document-byte limit and raw UTF-8 JSON validity. It SHALL NOT revalidate server-owned public-ID grammar, nonblank strings, specification/model-ID agreement, route cardinality, duplicate IDs/aliases/candidate tuples or route-group consistency. Standard Go JSON behavior SHALL apply, including case-insensitive field matching, zero/nil values for missing/null fields and U+FFFD normalization of escaped lone UTF-16 surrogates. A missing/null models collection, malformed JSON, byte overflow or type-decoding error SHALL invalidate the complete result. It SHALL preserve response order and configured alias/fallback order exactly as served, without expanding aliases into extra rows. Missing/null gateway SHALL remain nil. Unknown additive members SHALL remain ignored. Configured mappings SHALL be retained only when supplied by the server, never inferred from responses, models or inventories; credentials and arbitrary configuration SHALL NOT be exposed.

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
