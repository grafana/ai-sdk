# gateway-plugin-host Specification

## Purpose
Run the Gateway as a Grafana App Platform app plugin backend that serves the ProviderWire operations as namespaced platform routes, with configuration, health and lifecycle owned by the plugin host.

## Requirements

### Requirement: Routes-only app plugin
The Gateway SHALL be distributed as one Grafana app plugin whose app manifest declares no kinds and serves one API version with exactly two namespaced routes: `GET namespaces/{namespace}/config` and `POST namespaces/{namespace}/language-model`. The manifest SHALL declare no other routes, kinds or admission and conversion hooks.

#### Scenario: Committed manifest declares only the two routes
- **WHEN** the committed generated manifest is inspected
- **THEN** it SHALL declare one version with no kinds and only the two routes

#### Scenario: Plugin identity is fixed
- **WHEN** the backend is built by any supported method, including plain `go build` in tests
- **THEN** it SHALL identify as the plugin ID declared in the plugin metadata

#### Scenario: Undeclared path or method
- **WHEN** a request targets another path under the plugin's group version, or a declared route with another method
- **THEN** the Gateway SHALL perform no discovery, selection or provider work for it

### Requirement: ProviderWire semantics on platform routes
The `config` and `language-model` routes SHALL accept and return the registered ProviderWire V4 discovery and language-model protocol: request headers, JSON bodies, SSE framing, terminal frames and Gateway error documents. Clients SHALL address them by using `/apis/<group>/<version>/namespaces/<namespace>` as the ProviderWire API prefix.

#### Scenario: Registered Vercel client uses the route prefix
- **WHEN** the registered `createGateway` client is configured with the route prefix as `baseURL` and performs unary and streaming calls
- **THEN** it SHALL reach the `language-model` route with unchanged method, protocol headers and semantic body, and SHALL consume the responses as it consumes ProviderWire responses

#### Scenario: Gateway-produced errors keep their document
- **WHEN** the Gateway rejects an accepted route request for validation, policy or provider reasons
- **THEN** the response SHALL use the existing fixed ProviderWire error document and status for that reason

#### Scenario: Credential headers reach the plugin
- **WHEN** a route request reaches the Gateway with `Authorization`, `X-Access-Token`, `X-Grafana-Id` or `Cookie` headers that the host did not strip
- **THEN** those values SHALL NOT enter logs, metrics, exports, ProviderWire bodies or native provider requests

#### Scenario: Platform-produced errors are outside the Gateway document
- **WHEN** the platform rejects a request before the plugin handles it, such as failed authentication or an exceeded platform deadline
- **THEN** the Gateway SHALL NOT be required to produce that response, and documentation SHALL state that such responses are not ProviderWire error documents

### Requirement: Incremental streaming through the host
The `language-model` route SHALL deliver every complete SSE frame to the plugin host as soon as it is written, without buffering the remaining stream. Caller cancellation propagated by the host SHALL cancel the model call. Dispatch SHALL take the namespace from the route information the host verified, not from a separately parsed path segment.

#### Scenario: Stream frames arrive incrementally
- **WHEN** a provider emits stream parts with delays between them
- **THEN** the client SHALL receive each corresponding frame before the provider emits the next part

#### Scenario: Caller abandons a stream
- **WHEN** the caller disconnects or cancels during streaming
- **THEN** the model context SHALL be canceled and the stream SHALL be drained within the existing drain bounds

### Requirement: No standalone server surface
The Gateway SHALL NOT create network listeners or HTTP servers of its own beyond the plugin protocol transport, install signal-driven cancellation, serve liveness, readiness or metrics HTTP routes, or be distributed as a standalone command or container image. Request admission, transport security and network exposure SHALL belong to the plugin host.

#### Scenario: Plugin starts
- **WHEN** the plugin process starts under its host
- **THEN** it SHALL expose its functionality only through the plugin host protocol, with no Gateway-owned listener or signal handler

#### Scenario: Legacy paths are requested
- **WHEN** a caller requests `/api/v1/aisdk/config`, `/api/v1/aisdk/language-model`, `/live`, `/ready` or `/metrics`
- **THEN** no Gateway handler SHALL serve the request

### Requirement: Host-compatible configuration
The Gateway SHALL read settings as `GF_PLUGIN_<KEY>` variables, which Grafana derives from its plugin configuration section, and the bounded strict model file, without command-line arguments. Operator documentation SHALL state how each host supplies them. Configuration SHALL NOT accept authentication, listener or Cloud-ingress settings, and invalid configuration SHALL fail plugin startup before any request is served, without exposing secret values.

#### Scenario: Legacy authentication blocks are present
- **WHEN** the model configuration file contains an `auth` or `server` block
- **THEN** startup SHALL fail with a secret-safe unknown-field error

#### Scenario: Settings from the Grafana configuration section
- **WHEN** Grafana's configuration has a `[plugin.grafana-aigateway-app]` section with `access_mode` and `config_file`
- **THEN** the Gateway SHALL start with those settings, read from `GF_PLUGIN_ACCESS_MODE` and `GF_PLUGIN_CONFIG_FILE`

#### Scenario: Startup failure is logged with its reason
- **WHEN** configuration or construction fails at startup
- **THEN** the Gateway SHALL log the failure with its reason before exiting
- **AND** the reason SHALL name the failing setting, file or provider without its value, the configuration file path or credentials

#### Scenario: Environment-only startup
- **WHEN** the host starts the plugin with no arguments and valid environment and configuration file
- **THEN** the Gateway SHALL construct its catalog, BYOK factories and telemetry, and serve requests

### Requirement: Plugin health reporting
The plugin health check SHALL be served only after configuration, the catalog, telemetry and request handlers are constructed; construction failure SHALL end startup instead. Health results SHALL use fixed messages without configuration values, endpoints or credentials.

#### Scenario: Health after successful startup
- **WHEN** the host checks plugin health after startup completes
- **THEN** the result SHALL be healthy with a fixed message

### Requirement: Host-owned lifecycle and bounded final flush
Draining in-flight requests SHALL belong to the host; the Gateway SHALL NOT depend on receiving a termination signal and SHALL NOT cancel requests on its own when shutdown begins. After the plugin protocol server stops, the Gateway SHALL flush Agent Observability and attempt records concurrently under one shared deadline of 1 second, then exit. Records emitted after the flush starts SHALL be dropped and counted, not block or fail.

#### Scenario: Host stops the plugin
- **WHEN** the host stops the plugin process through the plugin protocol, with host tracing unconfigured or its shutdown completing promptly
- **THEN** the Gateway SHALL run its final flush without a termination signal and exit before the host's forced-kill delay

#### Scenario: Final flush exceeds budget
- **WHEN** queued telemetry cannot be delivered within the final-flush budget
- **THEN** the Gateway SHALL abandon it, account for the loss with bounded counters or fixed diagnostics, and exit before the budget ends

#### Scenario: Deployment guidance matches request bounds
- **WHEN** operator documentation describes deployment
- **THEN** it SHALL state that the host's termination grace period must exceed the configured maximum model duration plus drain time, and that host tracer shutdown shares the final-flush window

### Requirement: Protocol test host
Deterministic tests SHALL serve the Gateway through the real plugin route protocol, from HTTP through the plugin route RPC and route adapter to the Gateway handlers, with test-signed delegated tokens and fake native providers. Cross-language suites SHALL launch the built plugin executable as a plugin host does. The test host SHALL be test-only and SHALL NOT be built, packaged or documented as a supported server, or as evidence of the platform's route deadline or host draining.

#### Scenario: Test host exercises the protocol hop
- **WHEN** a client test sends a request to the test host
- **THEN** the request SHALL reach the Gateway through the same route protocol and identity delegation the plugin host uses, rather than calling Gateway handlers directly

#### Scenario: Real plugin process under test
- **WHEN** a cross-language suite starts the test host
- **THEN** the Gateway SHALL run as the built plugin executable, and health, metrics collection, relayed logs and stop SHALL be observed through the plugin protocol

#### Scenario: Legacy command evidence
- **WHEN** an existing requirement cites authenticated command evidence
- **THEN** that evidence SHALL be produced through this test host

#### Scenario: Test host is not shipped
- **WHEN** the plugin package is built
- **THEN** it SHALL contain no test host executable or configuration

### Requirement: Test host credential shapes
The test host SHALL accept service account tokens only as an `Authorization` bearer and access tokens only as `X-Access-Token`. It SHALL delegate them with actor chains shaped like the platform's, SHALL verify delegated token audience and namespace through the same plugin authenticator and plugin ID as production, and SHALL run in either access mode.

#### Scenario: Platform actor chains
- **WHEN** the test host delegates a direct service call, an on-behalf-of user call, a relayed call or a front-door user or service-account call
- **THEN** the delegated token SHALL carry the same actor order and service identities the platform produces for that flow

#### Scenario: Access token in the wrong header
- **WHEN** a client sends an access token as an `Authorization` bearer
- **THEN** the test host SHALL NOT map it to an internal service identity

### Requirement: Exact-client evidence through the test host
Go and the exact registered Vercel clients SHALL prove service-account behavior through the protocol test host in both access modes. Go SHALL prove configured discovery, unary and streaming behavior for internal callers, including on-behalf-of user calls. Vercel internal-caller access SHALL NOT be claimed until verified against the platform. Synthetic provider responses SHALL NOT be described as live-provider or deployed-platform evidence.

#### Scenario: Internal caller uses configured models
- **WHEN** the Go client authenticates as an allowlisted internal service in `cloud` mode, directly or on behalf of a user
- **THEN** configured discovery, unary and streaming calls SHALL succeed using configured accounts and canonical identity

#### Scenario: User or service account uses BYOK
- **WHEN** both clients authenticate as a service account in `cloud` mode and send request-scoped provider credentials
- **THEN** unary and streaming BYOK calls SHALL succeed, and discovery SHALL return the fixed BYOK unsupported-operation error

#### Scenario: Service account uses the catalog in self-hosted mode
- **WHEN** both clients authenticate as a service account in `self-hosted` mode
- **THEN** configured discovery, unary and streaming calls SHALL succeed using configured accounts

#### Scenario: High-level calls keep SDK-prepared requests
- **WHEN** Go `StreamText` or registered TypeScript `generateText` and `streamText` send supported text-only calls with explicit or omitted output-token limits
- **THEN** the tests SHALL preserve SDK-prepared automatic choice and body headers, including User-Agent, and native defaults SHALL apply only to omitted limits

#### Scenario: Credentials stay out of payloads and sinks
- **WHEN** requests carry sentinel platform credentials and BYOK keys
- **THEN** platform credentials SHALL be absent from ProviderWire bodies, native provider requests and automatic server logs, metrics and exports, while BYOK keys reach only the selected native provider
