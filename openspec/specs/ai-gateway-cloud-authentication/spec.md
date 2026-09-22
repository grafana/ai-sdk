# AI Gateway Cloud authentication

## Purpose

Accept trusted reverse-proxy identity in the Gateway application without changing internal JWT authentication or server-owned provider credentials.

## Requirements

### Requirement: Trusted proxy contract

The Gateway-owned guide SHALL document the required `X-Scope-OrgID` header and proxy-only API access prerequisite. The proxy MUST authenticate and authorize requests and remove client credentials. It MUST replace client-supplied `X-Scope-OrgID` with the authenticated stack ID before forwarding. The application SHALL NOT verify proxy credentials or repeat access-policy checks.

Provider credentials SHALL remain server-owned application configuration. Request-scoped BYOK and native OpenAI/Anthropic endpoints remain unsupported.

#### Scenario: Documentation scope

- **WHEN** the guide describes Cloud authentication
- **THEN** it documents application settings, the required stack header, isolation, and client limitations
- **AND** it does not describe private proxy configuration, deployment endpoints, rollout procedures, or future credential designs

### Requirement: Startup-selected authentication

The application MUST select one Gateway-owned `RequestAuthenticator` at startup through `--auth.mode` or `GRAFANA_AI_GATEWAY_AUTH_MODE`. Supported values SHALL be `access-token` and `cloud-gateway`, with `access-token` as the default. Unknown modes SHALL fail startup. The application MUST preserve internal authentication and never fall back between trust models.

The authentication boundary SHALL accept context and headers independently of body decoding. Authentication failure formatting SHALL be injected at the middleware boundary. Current ProviderWire routes SHALL retain the fixed `HostErrorWriter` authentication document.

#### Scenario: Existing internal caller

- **WHEN** an internal caller supplies a valid `X-Access-Token` with accompanying `Authorization`
- **THEN** the protected handler runs with the existing JWT caller identity
- **AND** the accompanying header does not change authentication
- **AND** audience, namespace, signature, service identity, and optional `X-Grafana-Id` acting-user verification retain their existing behavior

#### Scenario: Invalid internal token with Cloud assertions

- **WHEN** a caller submits an invalid access token and valid-looking Cloud assertions to internal mode
- **THEN** the application returns the fixed ProviderWire authentication error
- **AND** it does not call the protected handler or try Cloud authentication

#### Scenario: Authorization alone in internal mode

- **WHEN** an internal-mode request supplies `Authorization` without `X-Access-Token`
- **THEN** application authentication fails without reading the protected body or invoking discovery, model resolution, or provider work

#### Scenario: Unknown mode

- **WHEN** startup receives an authentication mode other than `access-token` or `cloud-gateway`
- **THEN** startup fails before readiness

### Requirement: Distinct trusted Cloud identity

Cloud-mode activation MUST wait for deployment-verified proxy-only API ingress. Header syntax alone does not establish the sender's identity. The application MUST NOT inspect cluster policies to establish this deployment prerequisite.

Cloud mode MUST validate `X-Scope-OrgID` without verifying CAP tokens or checking CAP scopes. It MUST require exactly one value containing positive decimal digits fitting `int64`. The application MUST ignore `X-Cloud-Org-ID` and `X-Access-Policy-ID`; neither header is required.

Validation SHALL reuse `exactlyOneHeader` and reject missing, empty, duplicated, case-colliding, comma-coalesced, control-character, and invalid-whitespace `X-Scope-OrgID` assertions. The application SHALL NOT normalize malformed assertions into valid credentials.

`Caller` MUST retain a typed `Source` and private `stackID`. It MUST derive `Namespace` from the stack through the pinned `types.CloudNamespaceFormatter`. It MUST NOT retain policy organization or policy identifier fields or manufacture a service identity or acting user.

#### Scenario: Deployment prerequisite remains external

- **WHEN** local application tests accept syntactically valid trusted assertions
- **THEN** those results do not authorize deployment activation
- **AND** deployment owners must separately verify proxy-only API ingress and approve its trust model
- **AND** the application does not query cluster policies

#### Scenario: Stack-only identity

- **WHEN** the application receives a valid `X-Scope-OrgID` with no policy headers or client credential
- **THEN** it derives `Namespace` through `types.CloudNamespaceFormatter`
- **AND** it retains the typed `Source` and private `stackID`
- **AND** it does not manufacture a service identity or acting user
- **AND** it makes no outgoing authentication request

#### Scenario: Ignored policy headers

- **WHEN** a request has a valid stack assertion, no client credential, and any values for `X-Cloud-Org-ID` or `X-Access-Policy-ID`
- **THEN** the application ignores those headers, including empty, duplicate, or malformed values
- **AND** it accepts the request without retaining either header in `Caller`

#### Scenario: Invalid post-edge stack assertion

- **WHEN** a controlled fixture injects a missing, empty, duplicated, case-colliding, comma-coalesced, control-character, or invalid-whitespace `X-Scope-OrgID` assertion after shim header replacement
- **THEN** application authentication fails before protected body reads, discovery, model resolution, or provider work

#### Scenario: Invalid numeric identity

- **WHEN** a stack assertion contains zero, a sign, nondigits, or a value exceeding `int64`
- **THEN** application authentication fails with the fixed ProviderWire authentication error

#### Scenario: Incorrect edge credential handoff

- **WHEN** the Cloud ProviderWire entry point receives surviving `Authorization`, `X-Access-Token`, or `X-Grafana-Id`
- **THEN** it returns the fixed authentication error before reading the body or calling a protected handler
- **AND** that restriction remains specific to Cloud mode and the ProviderWire adapter

### Requirement: Independent dependency construction

Cloud mode MUST operate without JWKS endpoint validation, client construction, verifier construction, or retrieval. It SHALL reject unsafe JWT verification and nonempty JWKS URLs. JWKS-specific limit validation SHALL apply only to the JWT path.

JWKS and Anthropic client construction SHALL be independent. Constructed clients MUST retain existing transport bounds, redirect rejection, endpoint validation, timeouts, and response-size protections. `ResolveProviderSecrets`, `BuildCatalog`, and provider configuration SHALL remain server-owned. Requests SHALL NOT control provider keys, URLs, backend model configuration, or shared-client mutation.

The minimum API write timeout SHALL use overflow-checked duration addition. Cloud mode SHALL exclude JWKS latency; internal mode SHALL preserve existing timeout accounting.

#### Scenario: Cloud startup without JWKS

- **WHEN** the command starts with valid Cloud-mode configuration and no JWKS URL
- **THEN** it constructs no JWKS client or verifier and performs no JWKS request
- **AND** JWKS latency does not contribute to the API timeout requirement
- **AND** unused JWKS-specific limits do not prevent Cloud construction

#### Scenario: Contradictory authentication configuration

- **WHEN** Cloud mode is configured with unsafe JWT verification or a nonempty JWKS URL
- **THEN** startup fails before readiness

#### Scenario: Mode-specific timeout minimum

- **WHEN** the configured write timeout is validated
- **THEN** Cloud mode requires at least read timeout plus model duration plus response grace
- **AND** internal mode retains its additional JWKS timeout term
- **AND** insufficient or overflowing duration sums fail startup

#### Scenario: Provider configuration remains authoritative

- **WHEN** an authenticated request supplies incoming provider credentials or request-controlled routing options
- **THEN** the application does not use them as provider configuration
- **AND** unsupported ProviderWire options retain their existing rejection behavior
- **AND** any provider request uses only the configured provider credential and reviewed endpoint

### Requirement: Listener compatibility and coordinated lifecycle

The application MUST support optional `--server.operational-listen-address` and `GRAFANA_AI_GATEWAY_SERVER_OPERATIONAL_LISTEN_ADDRESS`. Cloud mode MUST require separate API and operational listeners. Default internal-mode listener behavior MUST remain unchanged. Unsafe development authentication MUST require development mode and loopback addresses for every configured listener.

The API dispatcher SHALL serve only `GET /api/v1/aisdk/config` and `POST /api/v1/aisdk/language-model`. The operational dispatcher SHALL serve only `GET /live`, `GET /ready`, and `GET /metrics`. Dispatch SHALL preserve method checks and encoded-path rejection.

The process SHALL construct dependencies and bind every configured listener before readiness. Shutdown MUST withdraw readiness before canceling requests. Both servers SHALL share one shutdown deadline, with forced closure on expiry and both serving goroutines reaped. Cancel-first shutdown SHALL remain unchanged.

#### Scenario: Separate dispatch

- **WHEN** a client calls operational routes on the API listener or API routes on the operational listener
- **THEN** neither request reaches the handler for the other listener

#### Scenario: Exact routing remains enforced

- **WHEN** a request uses the wrong method or an encoded variant of a configured route
- **THEN** the selected dispatcher retains existing method checks and encoded-path rejection

#### Scenario: Legacy internal listener

- **WHEN** the command starts in internal mode without an operational address
- **THEN** the existing combined listener serves both route groups

#### Scenario: Explicit internal split

- **WHEN** internal mode configures an operational address
- **THEN** API and operational routes use separate listeners

#### Scenario: Cloud requires an operational listener

- **WHEN** Cloud mode omits an operational address
- **THEN** startup fails before readiness

#### Scenario: Unsafe authentication on a non-loopback listener

- **WHEN** unsafe development authentication configures any listener on a non-loopback address
- **THEN** startup fails before readiness

#### Scenario: Partial bind failure

- **WHEN** the first listener binds and the second listener fails to bind
- **THEN** the process closes the first listener and returns the bind failure
- **AND** readiness never becomes true

#### Scenario: Listener failure

- **WHEN** either configured listener fails to bind or its serving loop fails unexpectedly
- **THEN** the process withdraws readiness, closes both listeners, and returns the failure
- **AND** no serving goroutine survives the shared shutdown deadline

#### Scenario: Shutdown cancels active streams

- **WHEN** the process shuts down with an active model stream
- **THEN** it withdraws readiness before canceling requests and provider work
- **AND** it shuts down both servers under one deadline rather than granting separate full timeouts
- **AND** longer deployment termination grace does not imply drain-before-cancel behavior

### Requirement: Client composition and credential privacy

The registered low-level Gateway client MUST work through a test-only edge shim with the real command. The shim MUST use fixed dummy credentials and scope outcomes, not production CAP verification or policy evaluation. It SHALL strip Cloud and internal credentials, replace `X-Scope-OrgID` with its stack assertion, and strip other client identity headers. It SHALL forward the unchanged path.

Model-call evidence SHALL use `doGenerate` and `doStream` with explicit `maxOutputTokens`. High-level `generateText` and `streamText` SHALL remain documented as unsupported. Tests SHALL NOT rewrite client requests to conceal rejected body headers or tool-choice defaults.

Inbound credentials and assertions MUST NOT become provider credentials or appear in public application diagnostics. Application authentication failures SHALL use the fixed ProviderWire authentication document. Telemetry SHALL retain one registry, existing HTTP metrics, and fixed authentication source/outcome values without credential or customer-ID labels. Middleware SHALL preserve `responseWriter.Unwrap` for streaming flush support.

#### Scenario: Client-to-edge outcomes

- **WHEN** the registered low-level client sends discovery or model requests through the shim using the cases below
- **THEN** the fixture observes the corresponding result

| Case | Expected result |
| --- | --- |
| Valid dummy credentials and read scope | Discovery returns the configured public catalog without a token exchange. |
| Valid dummy credentials and write scope | Unary generation and streaming return the fake provider's expected results with explicit `maxOutputTokens`. |
| Invalid dummy credentials | The shim rejects the request; application and provider call counts remain zero. |
| Read-only inference or write-only discovery | The shim rejects the configured scope denial; application and provider call counts remain zero. |
| Spoofed client assertions with valid dummy credentials | The application receives the shim's stack assertion; the shim strips other client identity headers. |

#### Scenario: Outbound authentication

- **WHEN** an authorized client sends dummy credentials, incoming provider keys, and spoofed identity assertions through the local edge shim
- **THEN** fake Anthropic receives only the configured provider credential
- **AND** it receives no CAP credential, internal JWT, incoming provider key, or forwarded identity assertion

#### Scenario: Application authentication diagnostics

- **WHEN** application middleware rejects an assertion or internal token and the fixture captures the response, logs, and metrics
- **THEN** the response uses the fixed ProviderWire authentication document
- **AND** those outputs contain no test credential or customer identity value
- **AND** authentication observations use only fixed source and outcome values

#### Scenario: Edge denials are separate evidence

- **WHEN** the shim denies dummy credentials or a configured scope outcome
- **THEN** the fixture does not treat that response as an application authentication error
- **AND** the result does not establish a deployed proxy's credential verification or access-policy enforcement

#### Scenario: Streaming through authentication and telemetry

- **WHEN** the registered client streams through the shim and the real command
- **THEN** incremental server-sent events flush through the authentication and telemetry composition
- **AND** client cancellation cancels provider work
- **AND** the response wrapper preserves `Unwrap`

#### Scenario: Compatibility claims match test evidence

- **WHEN** command coverage is recorded after implementation
- **THEN** the parity map identifies the registered baseline and low-level calls with explicit output-token limits
- **AND** it records high-level text calls and default unary token limits as remaining compatibility work
- **AND** fake-provider fixtures are not presented as recorded provider conformance evidence
