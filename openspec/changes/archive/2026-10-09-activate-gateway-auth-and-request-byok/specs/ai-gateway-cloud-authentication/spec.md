## REMOVED Requirements

### Requirement: Server-owned Cloud provider credentials
**Reason**: Cloud inference now requires request-only BYOK; server-owned accounts are restricted to authorized private access.
**Migration**: Supply native request accounts for Cloud inference and use private configured access for catalogs. Native OpenAI/Anthropic HTTP endpoints remain unsupported.

### Requirement: Independent bounded provider dependency construction
**Reason**: The unified construction contract replaces the former Cloud configured-client authority.
**Migration**: Keep configured construction private and request-only accounts isolated over credential-independent bounded transports.

### Requirement: Independent dependency construction
**Reason**: The mode-specific construction contract is replaced by simultaneous private JWT and Cloud construction; Cloud no longer removes the process's JWT dependency.
**Migration**: Use the unified dependency construction requirement, configure JWT trust and isolate BYOK factories from configured accounts.

### Requirement: Startup-selected authentication
**Reason**: Authentication is selected by isolated entry point, not a mutually exclusive process-wide mode.
**Migration**: Replace auth.mode configuration and mode-specific tests directly. The Gateway is WIP; no compatibility aliases or migration period are required.

### Requirement: Listener compatibility and coordinated lifecycle
**Reason**: The new deployment requires two isolated API listeners and a separate operational listener, without a legacy combined-listener configuration.
**Migration**: Replace listener settings and deployment manifests with the three-listener contract below.

## ADDED Requirements

### Requirement: Unified dependency construction
The command SHALL construct JWT/configured-account and request-only dependencies together. Configured secrets/catalogs SHALL remain inaccessible to BYOK, transports SHALL remain credential-independent and timeout validation SHALL cover each path safely.

#### Scenario: Unified construction policy
- **WHEN** the Gateway constructs both authenticated account populations
- **THEN** The unified command SHALL construct explicit regional JWT verification and configured-account dependencies alongside credential-independent BYOK adapter factories.
- **AND** JWT trust SHALL be configured even when Cloud requests are being served.
- **AND** Client construction SHALL preserve bounded JWKS retrieval, endpoint validation, redirect refusal, model response bounds and timeouts.
- **AND** Configured secret resolution and catalog construction SHALL have no dependency path into the BYOK selector.
- **AND** Shared transports SHALL carry no mutable account headers or SDK-environment credential defaults.
- **AND** Listener timeout validation SHALL use overflow-safe accounting for the work each path performs, including JWT verification on the private path.

#### Scenario: Concurrent authentication dependencies
- **WHEN** the unified command starts
- **THEN** the private listener SHALL have JWT verification while Cloud requests SHALL not perform a JWT lookup

#### Scenario: BYOK isolation from configured state
- **WHEN** configured providers contain distinctive keys and endpoint overrides
- **THEN** BYOK selection SHALL have no access to them and SHALL use only explicit request credentials and supported native destinations

#### Scenario: Invalid trust or timeout configuration
- **WHEN** configured JWT trust, transport bounds or listener timeout arithmetic is invalid
- **THEN** startup SHALL fail before readiness


### Requirement: Simultaneous isolated authentication entry points
Private JWT, trusted Cloud and operational listeners SHALL coexist with isolated routes and no authentication fallback. Defaults SHALL keep Cloud on 8080, operations on 8081 and private JWT on 8082; private admission SHALL verify credentials before body reads.

#### Scenario: Listener and private admission policy
- **WHEN** the process binds listeners and admits private API requests
- **THEN** One Gateway process SHALL expose a private JWT API listener, a trusted-Cloud API listener and an operational listener.
- **AND** Defaults SHALL retain the Cloud API on port 8080 and operations on port 8081, with private JWT access on port 8082.
- **AND** Operators MAY configure distinct alternative addresses.
- **AND** API listeners SHALL serve the existing exact ProviderWire paths with method and encoded-path checks.
- **AND** Operational routes SHALL exist only on the operational listener.
- **AND** The command SHALL remove exclusive startup authentication modes and SHALL NOT fall back between authenticators.
- **AND** The private listener SHALL accept exactly one access-token credential, through X-Access-Token or Authorization Bearer, and SHALL reject both together, duplicate/case-colliding/coalesced credentials and supplied Cloud identity assertions.
- **AND** It SHALL verify access-token type, signature, expiry, audience ai-sdk and supported namespace before protected body reads.
- **AND** No service-identity allowlist or mandatory serviceIdentity claim SHALL apply; verified service/subject attributes SHALL be retained when available without inventing an identity.
- **AND** Optional X-Grafana-Id SHALL verify and bind to the access-token namespace; an ID token alone SHALL NOT authenticate.

#### Scenario: Both client populations use one process
- **WHEN** valid private JWT and trusted Cloud requests arrive concurrently
- **THEN** both SHALL authenticate on their respective listeners without changing process configuration or each other's principal

#### Scenario: Existing Cloud and operational ports remain stable
- **WHEN** the command uses default listener settings
- **THEN** the Cloud and operational endpoints SHALL remain on ports 8080 and 8081, while private JWT requests use the separate port 8082

#### Scenario: Internal callers use different JWT header forms
- **WHEN** Go sends X-Access-Token or pinned Vercel sends the same valid access token as an Authorization bearer credential to the private listener
- **THEN** each SHALL receive configured-account access without a Cloud token or stack assertion

#### Scenario: Wrong or ambiguous credentials
- **WHEN** a private request supplies both access-token headers, duplicated credentials, an ID token alone, an invalid access JWT or Cloud identity assertions
- **THEN** it SHALL fail before body reads, discovery, selection or provider work
- **AND** it SHALL NOT try Cloud authentication

### Requirement: Request-level account access and namespace context
Verified authentication SHALL determine immutable configured/BYOK access. Request keys/parameters SHALL NOT change policy or tenant identity; concrete/wildcard namespaces and verified acting users SHALL follow the account-bound namespace contract.

#### Scenario: Authenticated account and namespace policy
- **WHEN** the host derives request policy and tenant context
- **THEN** The host SHALL derive an immutable account-access policy from verified authentication provenance.
- **AND** Private JWTs SHALL grant configured-account access; trusted Cloud requests SHALL grant request-BYOK access.
- **AND** Neither request parameters, the namespace's customer identity nor supplied provider keys SHALL change this policy.
- **AND** Catalog code SHALL depend on configured-access authorization rather than a hard-coded JWT mechanism.
- **AND** JWT namespaces SHALL accept concrete stacks-<positive-int64> values and wildcard *.
- **AND** A wildcard SHALL remain service-level context unless a verified acting-user token supplies a concrete namespace within its authority.
- **AND** Untrusted headers SHALL NOT select a wildcard caller's stack.
- **AND** Unsupported or malformed namespaces SHALL fail before protected work.

#### Scenario: Internal service acts for a customer
- **WHEN** a valid ai-sdk access JWT names stacks-123
- **THEN** the caller SHALL retain configured-account access and stack-123 tenant attribution

#### Scenario: Wildcard service request
- **WHEN** a valid access JWT names * without an acting-user token
- **THEN** the request SHALL have configured-account access and service-level context without an invented stack

#### Scenario: Acting user narrows wildcard context
- **WHEN** a wildcard access JWT accompanies a valid concrete-stack ID token
- **THEN** authlib namespace binding SHALL establish that acting-user context without granting a different account-access policy

#### Scenario: Request attempts account-mode selection
- **WHEN** any request parameter claims configured access for a Cloud caller
- **THEN** the claim SHALL NOT influence authorization or make configured accounts available

### Requirement: Coordinated multi-listener lifecycle
All dependencies SHALL be constructed and all three listeners bound before readiness. Partial bind or unexpected serving failure SHALL close all listeners and withdraw readiness. Shutdown SHALL withdraw readiness before canceling active calls and SHALL use one shared deadline for all listeners and bounded cleanup. No legacy combined listener or unsafe production verifier SHALL be supported; any explicit development-only unsafe verification SHALL require loopback listeners.

#### Scenario: Third listener fails to bind
- **WHEN** two listeners bind and the third fails
- **THEN** both earlier listeners SHALL close and readiness SHALL never become true

#### Scenario: Shutdown with both populations active
- **WHEN** shutdown occurs with private and Cloud streams active
- **THEN** readiness SHALL be withdrawn, both requests canceled and all serving loops reaped under the shared deadline

#### Scenario: Cross-listener routes
- **WHEN** an API request reaches the operational listener or a health/metrics request reaches either API listener
- **THEN** it SHALL NOT reach the other listener's handler

## MODIFIED Requirements

### Requirement: Trusted proxy contract
Cloud ingress SHALL require a trusted authenticating edge that replaces stack assertions, removes caller credentials and enforces route scopes. The Cloud listener SHALL grant only request-BYOK access; private authorized access SHALL own configured accounts. Native OpenAI/Anthropic HTTP endpoints SHALL remain unsupported.

#### Scenario: Trusted proxy contract policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** The operator guide SHALL document the required X-Scope-OrgID assertion and proxy-only access to the Cloud application listener. The proxy MUST authenticate and authorize Cloud requests, enforce existing route scopes, replace client-supplied stack assertions and remove client authentication credentials. The application SHALL NOT verify CAP tokens or repeat CAP access-policy checks.
- **AND** The Cloud listener MUST grant only request-BYOK access. Configured provider accounts SHALL be available only through configured-access authorization on the private path. Native OpenAI/Anthropic HTTP endpoints SHALL remain unsupported; both paths use ProviderWire.

#### Scenario: Public Cloud inference
- **WHEN** the authenticating proxy authorizes a model request
- **THEN** the application SHALL receive only trusted identity and request content, and SHALL require request-scoped provider credentials

#### Scenario: Documentation scope
- **WHEN** operator guidance describes the deployment
- **THEN** it SHALL distinguish proxy-only Cloud ingress, private authenticated ingress and operational ingress
- **AND** it SHALL NOT claim header validation alone proves network isolation

### Requirement: Distinct trusted Cloud identity
Cloud activation SHALL require externally verified proxy-only ingress. The listener SHALL validate one positive-int64 stack assertion, reject surviving authentication credentials before body reads and derive typed BYOK provenance without trusting policy headers or manufacturing service/acting-user identity.

#### Scenario: Distinct trusted Cloud identity policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** Cloud activation MUST require deployment-verified proxy-only API ingress. The application SHALL validate exactly one X-Scope-OrgID value containing positive decimal digits fitting int64, derive its namespace through the pinned CloudNamespaceFormatter and retain typed authentication provenance. It SHALL reject missing, empty, duplicate, case-colliding, comma-coalesced, control-character and invalid-whitespace assertions.
- **AND** The Cloud listener SHALL reject surviving Authorization, X-Access-Token and X-Grafana-Id before body reads. It SHALL ignore X-Cloud-Org-ID and X-Access-Policy-ID rather than retaining or trusting them, and SHALL NOT manufacture a service identity or acting user. It SHALL NOT inspect cluster policies or issue authentication requests.

#### Scenario: Stack-only identity
- **WHEN** a proxy-only request supplies a valid stack assertion without surviving credentials
- **THEN** it SHALL establish BYOK access and the corresponding stack namespace without a manufactured service identity

#### Scenario: Invalid post-edge stack assertion
- **WHEN** a Cloud request has a missing, empty, duplicated, coalesced or malformed stack assertion
- **THEN** it SHALL fail before protected body reads or provider work without trying JWT authentication

#### Scenario: Invalid numeric identity
- **WHEN** a Cloud stack assertion is zero, negative, nondecimal or outside positive int64
- **THEN** it SHALL fail before protected work without coercing or truncating the identity

#### Scenario: Incorrect edge credential handoff
- **WHEN** a Cloud request retains Authorization, X-Access-Token or X-Grafana-Id
- **THEN** it SHALL fail before protected body reads or provider work without trying JWT authentication

#### Scenario: Ignored policy headers
- **WHEN** valid trusted stack identity accompanies arbitrary policy organization or policy identifier headers
- **THEN** the application SHALL ignore those headers without using them for account selection

#### Scenario: Deployment prerequisite remains external
- **WHEN** deterministic tests accept a syntactically valid assertion
- **THEN** the result SHALL NOT be described as deployed sender authentication
- **AND** deployment evidence SHALL separately prove that internal client workloads cannot reach the Cloud application port

### Requirement: Client composition and credential privacy
Real-command tests SHALL exercise both authenticated populations with independent Go and registered Vercel clients. Dummy-edge evidence SHALL remain separate from production authorization/isolation proof. Selected provider authentication, default token limits, unmodified SDK headers/choice, metadata-only capture and streaming cancellation SHALL remain independently tested.

#### Scenario: Client composition and credential privacy policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** Tests SHALL run the real unified command with both listeners, independent Go clients and the exact registered Vercel client. A test-only Cloud edge SHALL use dummy credentials and predetermined scope/stack outcomes, replace assertions and strip credentials. It SHALL NOT claim production CAP verification. Private tests SHALL exercise both access-token header forms and concrete/wildcard namespaces.
- **AND** Configured discovery SHALL succeed only on configured access; authenticated Cloud discovery SHALL return the explicit BYOK unsupported-operation response. Both populations SHALL support represented unary and streaming calls with explicit or omitted output-token limits; omitted limits SHALL use native defaults. High-level Go StreamText and registered TypeScript generateText/streamText SHALL preserve SDK-generated automatic choice and ordinary body headers, including User-Agent, without rewriting SDK bodies. Tests SHALL separate Gateway-authentication failures from provider-credential failures.
- **AND** Automatic server diagnostics SHALL exclude credentials and customer-ID metric labels. Authentication observations SHALL use bounded entry-point/source/outcome values per request, never a process-wide mode override. Streaming middleware SHALL preserve Unwrap and cancellation. Consumer request metadata exposure and structural capture protection SHALL follow grafana-gateway-client and structured-logging-middleware.

#### Scenario: Client-to-edge outcomes
- **WHEN** Go and pinned Vercel issue supported private/configured and Cloud/BYOK unary and streaming calls
- **THEN** fake native providers SHALL observe the appropriate account source, matching content/options and no Gateway authentication credentials

#### Scenario: High-level text-only streaming through the edge
- **WHEN** Go StreamText or registered TypeScript generateText/streamText sends a supported text call with an explicit or omitted token limit
- **THEN** both entry points SHALL preserve SDK-prepared headers and automatic choice while native defaults apply only to omitted limits

#### Scenario: High-level text-only unary generation through the edge
- **WHEN** registered TypeScript generateText sends an explicit or omitted output-token limit
- **THEN** both entry points SHALL preserve its automatic choice and SDK-prepared headers, applying native defaults only to omitted limits

#### Scenario: Outbound authentication
- **WHEN** either population reaches a native provider
- **THEN** only the selected configured or BYOK account SHALL authenticate that request, without Gateway credentials or trusted Cloud assertions

#### Scenario: Application authentication diagnostics
- **WHEN** the application rejects JWT credentials or a Cloud assertion
- **THEN** it SHALL emit fixed credential-free diagnostics and bounded authentication metrics without customer-ID labels

#### Scenario: Edge denials are separate evidence
- **WHEN** the test edge rejects a credential or inference scope
- **THEN** no application handler or native provider work SHALL occur

#### Scenario: Discovery populations differ
- **WHEN** authenticated clients request discovery on each entry point
- **THEN** private clients SHALL receive configured discovery and Cloud clients SHALL receive the fixed unsupported-operation response without catalog access

#### Scenario: Streaming through authentication and telemetry
- **WHEN** either population streams through real authentication and telemetry
- **THEN** events SHALL flush incrementally, cancellation SHALL reach the provider and dummy credential markers SHALL be absent from captured server sinks

#### Scenario: Compatibility claims match test evidence
- **WHEN** support is documented
- **THEN** exact-client, fake-provider and test-edge evidence SHALL be distinguished from live provider acceptance, CAP enforcement and deployed NetworkPolicy proof

### Requirement: Authentication-mode write timeout accounting
The minimum API write timeout SHALL use overflow-checked duration addition. Cloud listener accounting SHALL exclude JWKS latency; private JWT listener accounting SHALL include it.

#### Scenario: Authentication-mode write timeout accounting
- **WHEN** the configured write timeout sum overflows or excludes the private-listener JWKS term
- **THEN** startup SHALL reject invalid accounting; valid Cloud-listener accounting SHALL exclude JWKS latency

### Requirement: Shared listener readiness and shutdown
The process SHALL construct dependencies and bind every configured listener before readiness. Shutdown MUST withdraw readiness before canceling requests. All three servers SHALL share one shutdown deadline, with forced closure on expiry and all serving goroutines reaped. Cancel-first shutdown SHALL remain unchanged.

#### Scenario: Shared listener readiness and shutdown
- **WHEN** shutdown begins with active requests on all three listeners
- **THEN** readiness SHALL be withdrawn before cancellation and all three servers SHALL close and reap serving goroutines under one deadline

