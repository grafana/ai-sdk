## Purpose

Define the bounded authenticated direct-Anthropic Grafana AI Gateway service, including configuration, routing, authentication, discovery, telemetry, and lifecycle behavior.

## Requirements

### Requirement: Runnable isolated Gateway service
The repository SHALL provide a runnable AI Gateway command inside the existing AGPL-3.0-only `ai-gateway` Go module that composes `ai-gateway/catalog` and `ai-gateway/providerwire/v4` with the separately pinned Apache-2.0 direct Anthropic provider module. The service SHALL use one HTTP listener and SHALL mount exactly `GET /live`, `GET /ready`, `GET /metrics`, `GET /api/v1/aisdk/config`, and `POST /api/v1/aisdk/language-model`. `/metrics` SHALL be the unauthenticated fifth operational route in the authoritative delivery plan so phase 5 health metrics are scrapeable. Unsupported methods, including `HEAD /api/v1/aisdk/config`, and unmatched paths SHALL NOT authenticate, list models, resolve models, or invoke a provider.

#### Scenario: Service exposes the phase 5 routes
- **WHEN** a valid service configuration starts successfully
- **THEN** all five declared health, metrics, discovery, and language-model routes SHALL be reachable on the configured listener
- **AND** no legacy ProviderWire route SHALL be mounted

#### Scenario: HEAD discovery is rejected
- **WHEN** a caller sends `HEAD /api/v1/aisdk/config`
- **THEN** the service SHALL return method not allowed with `Allow: GET` without entering authentication or discovery

#### Scenario: Recognized path uses the wrong method
- **WHEN** a caller uses an unsupported method on any of the five recognized paths
- **THEN** the service SHALL return 405 with `Allow: GET` for operational/discovery paths or `Allow: POST` for the language-model path
- **AND** protected work SHALL not run

#### Scenario: Service dependencies remain isolated
- **WHEN** phase 5 dependencies are added to the existing `ai-gateway` module and the root module graph is compared before and after the change
- **THEN** the root module graph SHALL remain unchanged
- **AND** newly introduced authlib, Kingpin, Prometheus, and Anthropic-service dependencies SHALL remain absent from the root graph
- **AND** `ai-gateway` SHALL remain absent from the root `go.work`

### Requirement: Concrete bounded process settings
Kingpin SHALL own every scalar process setting through the exact flag, environment variable, default, enum, and validation contract recorded in the phase 5 design. Settings SHALL include the configuration-file path and byte limit; deployment mode; listener address; HTTP read-header, request-read, response-write, idle, maximum-header-size, response-grace, and shutdown limits; discovery-response bytes; authentication mode, audiences, JWKS URL, request timeout, response bytes, maximum key count, minimum refresh interval, and maximum snapshot age; Anthropic response-header timeout and decompressed response bytes; and every `providerwire/v4.Limits` field.

All byte, count, and duration settings SHALL be positive and safe for any required `limit+1` operation. The listener SHALL use valid TCP host/port syntax with a numeric port in every deployment mode. The service SHALL compute the minimum HTTP write budget using checked duration addition over request-read timeout, JWKS request timeout, ProviderWire model duration, and response grace, and SHALL reject overflow or a response-write timeout below that minimum.

Phase 5 SHALL bound each accepted inbound connection and request through header, read, write, idle, body, protocol, and shutdown limits, but SHALL NOT claim a process-wide concurrent connection or request budget. Phase 6 SHALL own those aggregate budgets and the capacity strategy that keeps `/live`, `/ready`, and `/metrics` available during model-route saturation.

#### Scenario: Defaults satisfy the write budget
- **WHEN** the command parses no optional scalar overrides
- **THEN** every documented default SHALL be selected
- **AND** the checked response-write inequality SHALL hold

#### Scenario: Write budget overflows
- **WHEN** configured durations overflow checked addition or response-write timeout is below the computed minimum
- **THEN** startup SHALL fail before reading secrets, constructing clients, or binding the listener

#### Scenario: Unknown process setting is supplied
- **WHEN** arguments contain an unknown flag or a typed setting contains an unknown enum value
- **THEN** parsing SHALL fail without falling back to an inferred value

#### Scenario: Scalar endpoint or listener is malformed
- **WHEN** the listener lacks valid numeric TCP host/port syntax or the production JWKS scalar URL violates endpoint policy
- **THEN** startup SHALL fail before YAML loading, secret resolution, component construction, or listener binding

### Requirement: Strict model configuration and public IDs
A bounded strict YAML document SHALL own named providers and public model routes. YAML decoding SHALL reject unknown fields, duplicate mapping keys, and trailing documents. Configuration SHALL reject empty names, unknown provider types, missing provider or model references, duplicate or colliding canonical IDs and aliases, missing required presentation names, and an empty model set.

Each phase 5 provider SHALL have `type: anthropic`, a non-empty `apiKeyEnv` environment-variable reference, and an optional absolute base URL. The referenced environment variable SHALL exist and contain a non-empty API key at startup. Literal provider credentials SHALL not be representable in the YAML schema.

Every canonical public ID and alias SHALL be 1-128 ASCII bytes and SHALL match `^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`. This grammar SHALL be the only accepted model-ID surface for discovery and the `ai-language-model-id` header.

#### Scenario: Minimal Anthropic configuration is valid
- **WHEN** YAML defines one named Anthropic provider by API-key environment reference and one header-safe public model route that references it and a backend model ID
- **THEN** startup SHALL construct the service without contacting Anthropic or JWKS

#### Scenario: Unsafe public ID is configured
- **WHEN** a canonical ID or alias contains whitespace, control bytes, commas, non-ASCII bytes, or exceeds 128 bytes
- **THEN** startup SHALL fail before catalog construction

#### Scenario: Referenced credential is unavailable
- **WHEN** `apiKeyEnv` names an unset or empty environment variable
- **THEN** startup SHALL fail before constructing the catalog or serving requests
- **AND** logs and errors SHALL NOT contain any environment-variable value

### Requirement: Hardened outbound HTTP
Every configured JWKS and Anthropic endpoint SHALL be an absolute hierarchical URL with a non-empty host and no userinfo, opaque component, query, forced query, or fragment. Production mode SHALL require HTTPS. Development mode and tests MAY use HTTP. Both clients SHALL use cloned, service-owned transports with fixed `MaxIdleConns: 32`, `MaxIdleConnsPerHost: 8`, and `MaxConnsPerHost: 32` plus bounded dial, TLS handshake, response-header, idle-connection, and expect-continue behavior; SHALL reject redirects before credentials can be forwarded; and SHALL bound bytes after automatic content decompression. An over-limit response SHALL stop on the first byte above the configured bound.

Anthropic requests SHALL honor their model/request context. The JWKS refresh starter and every joiner SHALL honor their own request contexts while one shared refresh uses the service-lifetime context plus the configured JWKS timeout, so canceling any request returns that caller promptly without canceling work joined by other requests. Anthropic SHALL apply the configured response-header timeout and one cumulative decompressed response-byte bound to unary success, non-success bodies, and the complete SSE response. The bound SHALL be lower than the SDK SSE scanner's maximum line allocation. JWKS SHALL apply its independent request timeout and decompressed response-byte bound.

#### Scenario: Production uses a cleartext URL
- **WHEN** production configuration supplies an HTTP JWKS or Anthropic base URL
- **THEN** startup SHALL fail before any outbound request

#### Scenario: Endpoint contains authority or routing data outside its path
- **WHEN** either deployment mode supplies a JWKS or Anthropic URL with userinfo, opaque form, query, forced query, fragment, or an empty host
- **THEN** startup SHALL fail before secret resolution, client construction, or outbound work

#### Scenario: Credential-bearing redirect is returned
- **WHEN** JWKS or Anthropic returns a redirect to the same or a different origin
- **THEN** the client SHALL not follow it
- **AND** no access token or Anthropic API key SHALL reach the redirect target

#### Scenario: Outbound response hangs or exceeds its bound
- **WHEN** an outbound server does not produce response headers, stalls after headers, or emits more decompressed bytes than configured
- **THEN** the operation SHALL terminate through the applicable timeout, request cancellation, or over-limit error
- **AND** retained response memory SHALL remain bounded

### Requirement: Amplification-resistant joinable JWKS retrieval
The production authenticator SHALL use a service-owned `authn.KeyRetriever` backed by one bounded immutable JWKS snapshot rather than authlib's default per-key cache. A successful snapshot SHALL contain at most the configured maximum key count, unique non-empty key IDs, and only valid verification keys.

Under one mutex, the retriever SHALL store an optional explicit in-flight refresh containing a completion channel and final result plus the reserved `lastAttempt` time. A caller that needs refresh SHALL first join any existing flight regardless of cadence. Only when no flight exists may cadence decide whether the caller starts a new one; the starter SHALL reserve `lastAttempt`, install the flight, launch one service-owned bounded refresh, and then wait on the same completion channel versus its own context like every joiner. Completion SHALL publish the validated snapshot/result, clear the flight, and close its completion channel under defined synchronization. This prevents approved callers from starting a second fetch after an earlier flight finishes while allowing all concurrent rotated-key waiters to share and succeed from one refresh.

A new fetch SHALL start only when no snapshot exists, the snapshot exceeds its maximum age, or an unknown key ID is requested after the minimum refresh interval. Regardless of attacker-controlled key IDs or concurrency, at most one outbound refresh SHALL begin during one minimum refresh interval. Unknown key IDs before the next permitted refresh SHALL fail locally and SHALL never be retained in a negative-key cache. A failed, malformed, oversized, over-key-count, duplicate-key, or empty refresh SHALL leave the last still-valid snapshot unchanged. The retriever SHALL retain at most one configured-size snapshot, one fixed-size flight, and fixed refresh metadata.

#### Scenario: Unique-key-ID flood occurs
- **WHEN** concurrent requests present many distinct unknown JWT key IDs within one refresh interval
- **THEN** the service SHALL issue at most one JWKS request for that interval
- **AND** memory usage SHALL not grow with the number of unknown IDs

#### Scenario: Oversized JWKS is returned
- **WHEN** the JWKS response exceeds its byte or key-count bound or contains duplicate or empty key IDs
- **THEN** the refresh SHALL fail without replacing a still-valid prior snapshot

#### Scenario: Concurrent rotated-key waiters join one active refresh
- **WHEN** a barrier releases concurrent requests for a rotated key after refresh cadence and the first request blocks inside the bounded JWKS fetch
- **THEN** every later waiter SHALL join that active flight even though `lastAttempt` is already reserved
- **AND** exactly one fetch SHALL occur
- **AND** every uncanceled waiter SHALL succeed after the returned snapshot contains the rotated key

#### Scenario: Caller arrives as a refresh completes
- **WHEN** a barrier schedules another rotated-key caller across refresh completion
- **THEN** the caller SHALL either join the active flight or observe its published snapshot
- **AND** it SHALL not start a second fetch in the reserved interval

#### Scenario: Refresh starter is canceled
- **WHEN** the request that installed a flight is canceled while another request waits for the same key
- **THEN** the starter SHALL return promptly with its request cancellation
- **AND** the service-owned refresh SHALL continue and satisfy the uncanceled waiter

### Requirement: Immutable direct-Anthropic construction
The service SHALL construct every named Anthropic provider configuration once, construct every configured backend model once with the hardened HTTP client, and build one immutable static catalog once at startup. Direct provider construction SHALL use the `anthropic-provider-construction` contract so ambient Anthropic SDK environment defaults cannot alter the service-owned endpoint, credential, auth mechanism, profile/federation behavior, or headers. A public model route SHALL reference exactly one named direct-Anthropic provider and one backend model ID. Repeated resolution of a canonical ID or alias SHALL return the same composed model instance and canonical catalog identity. ProviderWire streaming metadata SHALL use canonical public identity when that stream part is emitted; minimal unary output SHALL not add response metadata that registered clients replace.

#### Scenario: Alias and canonical ID share a model
- **WHEN** a canonical ID and one of its aliases are resolved repeatedly
- **THEN** all resolutions SHALL return the same model instance
- **AND** alias resolution SHALL report the configured canonical public ID

#### Scenario: Anthropic base URL is configured
- **WHEN** a provider instance declares an allowed base URL
- **THEN** requests from every model using that instance SHALL use that URL through the hardened transport
- **AND** discovery, logs, metrics, and public errors SHALL NOT expose it

### Requirement: Exact Grafana authentication headers
Protected requests SHALL contain exactly one case-insensitively matched `X-Access-Token` field value and at most one `X-Grafana-Id` field value. The service SHALL reject duplicate fields, multiple values, comma-coalesced values, and empty values before calling authlib. An optional exact `Bearer ` prefix MAY be removed only after single-value validation. `Authorization` SHALL not satisfy internal authentication.

The service SHALL authenticate accepted values with `github.com/grafana/authlib/authn`. Access-token audience SHALL match a non-empty configured set defaulting to `ai-sdk`, and authlib namespace consistency SHALL be enforced when an ID token exists.

#### Scenario: Duplicate access-token headers are received
- **WHEN** a request contains case-variant duplicates, multiple field values, or one comma-coalesced `X-Access-Token`
- **THEN** authentication SHALL fail before authlib verification, listing, resolution, or model invocation

#### Scenario: Optional ID-token header is duplicated
- **WHEN** a request contains more than one effective `X-Grafana-Id` value
- **THEN** the whole request SHALL fail rather than selecting one value

### Requirement: Verified service and acting-user identity separation
A successful protected request SHALL require exactly one non-empty value at `authn.ServiceIdentityKey` in the verified `AuthInfo` extras and a non-empty verified namespace. Absence, duplication, or emptiness SHALL fail authentication. The service SHALL never fall back to `GetSubject` or `GetIdentifier` for service identity because those methods can return an acting user.

Acting-user identity SHALL be extracted independently only when authlib reports a non-access-policy identity. Request context SHALL store a private normalized caller containing service identity, namespace, and optional acting-user subject/type only. It SHALL NOT store `types.AuthInfo`, raw or parsed tokens, the `id-token` extra, email, groups, permissions, or the full extras map.

Production mode SHALL share the bounded JWKS retriever between access-token and ID-token verifiers. Unsafe development mode SHALL use authlib unsafe verifiers, SHALL still validate token structure, type, expiry, audience, and namespace consistency, SHALL emit a prominent startup warning, and SHALL require a loopback TCP listener host. Production SHALL reject unsafe mode and a missing JWKS URL; unsafe mode SHALL reject wildcard, empty-host, and non-loopback listeners.

#### Scenario: Acting user accompanies a service token
- **WHEN** a valid access token with one service identity and a compatible valid acting-user token authenticates
- **THEN** caller service SHALL equal only `ServiceIdentityKey`
- **AND** acting-user subject/type SHALL appear only in the optional acting-user field

#### Scenario: Service identity is absent but acting user exists
- **WHEN** authlib verifies an acting user but `ServiceIdentityKey` is absent, empty, or multi-valued
- **THEN** authentication SHALL fail instead of promoting the acting user to caller service

#### Scenario: Normalized caller is inspected
- **WHEN** authentication succeeds with an ID token
- **THEN** request context SHALL contain none of the raw ID token, `AuthInfo`, email, permissions, groups, or extras map

### Requirement: Authentication precedes protected service work
Authentication SHALL run before deep request-body decoding, catalog listing, catalog resolution, or model invocation. Cheap route, method, and protocol-envelope selection MAY precede authentication, but no protected body read or schema/mapping work may do so. No protocol Policy abstraction SHALL be introduced before a concrete host-policy capability exists. Any authentication failure SHALL return a fixed non-retryable ProviderWire-compatible 401 authentication error through the shared host-safe V4 error writer and SHALL not expose verifier details.

#### Scenario: Authorization alone is rejected
- **WHEN** a caller sends a valid-looking `Authorization` header but omits `X-Access-Token`
- **THEN** the service SHALL return the fixed 401 authentication response
- **AND** decoding, listing, resolution, and model invocation SHALL not occur

#### Scenario: Invalid ID token accompanies a valid access token
- **WHEN** `X-Access-Token` is valid and `X-Grafana-Id` is invalid or belongs to an incompatible namespace
- **THEN** the entire request SHALL fail authentication before protected service work
- **AND** the acting-user token SHALL not be ignored or downgraded

### Requirement: Closed bounded public model discovery
`GET /api/v1/aisdk/config` SHALL authenticate before calling `catalog.ModelLister`. Discovery SHALL use private closed DTOs and a test-only draft 2020-12 schema with no additional properties. Every canonical ID and alias SHALL appear as a separate complete row sorted by row ID, with configured name and optional description plus `specificationVersion: "v4"`, `provider: "grafana"`, and `modelId` equal to that row's invocable public ID.

Root, rows, fields, and strings SHALL be incrementally JSON-encoded into a `limit+1` bounded buffer before status 200 is committed. Encoding SHALL stop immediately when the first byte beyond the limit would be retained; repeated aliases SHALL NOT cause a whole-response temporary allocation. Overflow, invalid UTF-8, listing failure, or encoding failure SHALL produce a fixed safe error without writing a partial discovery document. The test-only schema SHALL validate fixtures and raw completed responses rather than execute in the production handler. Discovery SHALL NOT expose alias topology beyond public rows, provider instance names or types, backend model IDs, credentials, environment-variable names, base URLs, or fallback topology.

#### Scenario: Canonical model and alias are discovered
- **WHEN** an authenticated caller lists a model configured with one alias
- **THEN** the raw response SHALL contain one closed complete canonical row and one closed complete alias row
- **AND** the pinned client SHALL successfully invoke both returned `modelId` values

#### Scenario: Discovery response reaches its byte boundary
- **WHEN** the complete encoded response is below or exactly at the configured limit
- **THEN** the service SHALL commit the valid document
- **AND WHEN** the first byte above the limit would be written
- **THEN** the service SHALL emit only a bounded safe error

#### Scenario: Discovery DTO is inspected for closure and privacy
- **WHEN** raw discovery bytes are schema-validated and searched using sensitive configured values
- **THEN** unknown fields SHALL be rejected
- **AND** no provider instance, backend ID, credential, environment name, or base URL SHALL be present

### Requirement: Local liveness and readiness
`GET /live` SHALL be unauthenticated and SHALL report process liveness without contacting authentication JWKS or provider APIs. `GET /ready` SHALL be unauthenticated and SHALL return success only after local flag/YAML validation, credential-reference resolution, authentication construction, provider construction, catalog construction, strict ProviderWire and discovery handler construction, metrics registration, and listener binding are complete. Readiness SHALL become false before shutdown starts and SHALL never depend on a live Anthropic or JWKS request.

#### Scenario: Locally constructed service is ready
- **WHEN** all local components are constructed and the listener is accepting requests while Anthropic and JWKS are unreachable
- **THEN** `/ready` SHALL still report ready

#### Scenario: Shutdown begins
- **WHEN** the service receives its shutdown signal
- **THEN** readiness SHALL fail before in-flight request cancellation

### Requirement: Closed privacy-safe process lifecycle logging
The service SHALL emit fixed structured process events `process_starting`, `process_ready`, `process_shutdown_started`, and `process_shutdown_completed` at their corresponding transitions. These records SHALL NOT contain configuration values, endpoints, credentials, provider/model identity, request data, or arbitrary errors. Ready SHALL be emitted only after listener binding; shutdown-start SHALL follow readiness becoming false; shutdown-complete SHALL follow graceful or forced server termination.

#### Scenario: Process starts, becomes ready, and shuts down
- **WHEN** the real command completes a normal local startup and receives a shutdown signal
- **THEN** each fixed lifecycle event SHALL be emitted exactly once in transition order
- **AND** the records SHALL contain no configured secret, provider URL, or backend model ID

### Requirement: Bounded HTTP lifecycle and independent shutdown deadline
The HTTP server SHALL configure the validated read-header, request-read, response-write, idle, `http.Server.MaxHeaderBytes`, and shutdown limits. `MaxHeaderBytes` SHALL be safe for Go's internal 4096-byte parser-buffer addition. The configured header value SHALL be documented as Go's parser setting rather than an exact wire ceiling: the effective read bound is `MaxHeaderBytes + 4096` bytes because `net/http` reserves parser-buffer slop. Request contexts SHALL derive from a process context that is canceled at shutdown so silent or active SSE calls observe cancellation.

Shutdown SHALL set readiness false, cancel request work, and then derive the graceful `Server.Shutdown` timeout from `context.Background()` or `context.WithoutCancel`, never from the canceled process context. The service SHALL force-close remaining connections only if that independent deadline expires or graceful shutdown fails. Startup and shutdown SHALL not wait for provider API availability.

#### Scenario: Active stream exits gracefully during shutdown
- **WHEN** process cancellation causes an established silent stream to close before the independent shutdown deadline
- **THEN** `Server.Shutdown` SHALL complete gracefully without force-closing connections

#### Scenario: Handler ignores cancellation
- **WHEN** a test handler remains active through the independent graceful deadline
- **THEN** the service SHALL force-close after that deadline and terminate within the configured outer bound

#### Scenario: Client sends headers too slowly or exceeds Go's effective bound
- **WHEN** a request exceeds the configured header timeout or the effective `MaxHeaderBytes + 4096` read bound
- **THEN** the HTTP server SHALL terminate it without reaching authentication, resolution, or model invocation

#### Scenario: Header request falls within parser slop
- **WHEN** a syntactically valid request is larger than `MaxHeaderBytes` but remains within Go's effective parser allowance
- **THEN** tests SHALL document the actual `net/http` result rather than claim the configured value is an exact wire ceiling

### Requirement: Flush-preserving privacy-safe HTTP telemetry
One outer telemetry middleware SHALL allocate request-scoped telemetry state, emit exactly one completion record and one metric observation for every request including authentication failures, and permit auth middleware to populate normalized caller service/namespace. Its response writer wrapper SHALL implement `Unwrap() http.ResponseWriter` so `http.NewResponseController` can reach the underlying flusher and preserve incremental SSE.

Telemetry SHALL normalize route labels to exactly `live`, `ready`, `metrics`, `config`, `language_model`, or `unmatched`; method labels to `GET`, `POST`, or `other`; and status labels to `1xx`, `2xx`, `3xx`, `4xx`, or `5xx`. Arbitrary methods and paths SHALL never become label values. Logs SHALL use the same normalized route/method/status vocabulary and the closed authentication classes `not_attempted`, `authenticated`, and `authentication_failed`.

Logs and metrics SHALL NOT include tokens, API keys, authorization or provider headers, request or response bodies, provider errors, provider base URLs or instance names, backend model IDs, arbitrary paths/methods, acting-user email, or unbounded error text.

#### Scenario: Stream remains open after its first frame
- **WHEN** an authenticated stream emits its initial ProviderWire frame and then remains silent
- **THEN** the client SHALL receive that frame before the stream closes or another frame is emitted
- **AND** telemetry SHALL still emit exactly once when the request later ends

#### Scenario: Authentication fails
- **WHEN** token verification fails
- **THEN** the outer telemetry layer SHALL emit exactly one request record with a fixed authentication class
- **AND** caller fields and raw authlib error text SHALL be absent

#### Scenario: Arbitrary route and method are requested
- **WHEN** a caller sends an unrecognized path and method
- **THEN** metrics and logs SHALL use `unmatched` and `other`
- **AND** the raw values SHALL not enter label values

### Requirement: Service health metrics
The service SHALL register Prometheus process, Go runtime, readiness, HTTP request count, in-flight request, and request-duration metrics exactly once against a service-owned registry and SHALL expose that registry on unauthenticated `GET /metrics`. Metrics SHALL use only the closed telemetry labels and SHALL not label by caller, namespace, requested model, provider instance, backend model, error message, URL, or arbitrary request data.

#### Scenario: Health metrics are scraped
- **WHEN** `/metrics` is requested after startup
- **THEN** the response SHALL contain process/runtime metrics, a ready value, and bounded-cardinality HTTP lifecycle metrics

#### Scenario: Credential and model values are used
- **WHEN** requests use arbitrary public IDs, provider configuration, or authentication credentials
- **THEN** no metric name, label name, or label value SHALL contain those values

### Requirement: Authenticated direct-Anthropic text execution
An authenticated registered Gateway client SHALL complete the strict runtime's supported unary and streaming text requests through the configured direct Anthropic provider. The service SHALL preserve the existing ProviderWire validation/mapping order after authentication, forward supported mapped scalar and text options, normalize successful output to canonical public identity, and preserve bounded safe unary and SSE behavior.

#### Scenario: Unary alias call reaches Anthropic
- **WHEN** an authenticated client invokes an alias with supported unary text and scalar options
- **THEN** the fake Anthropic transport SHALL receive the configured backend model ID and intended mapped options exactly once
- **AND** the minimal unary result SHALL expose no backend identity or redundant response metadata

#### Scenario: Normal streaming finish completes
- **WHEN** fake Anthropic emits a valid initial event, deterministic text, and finish
- **THEN** the registered client SHALL consume normalized start, text, finish, and clean EOF while the real command remains running

### Requirement: Real-command cancellation and process evidence
Cross-language integration SHALL build and spawn the real isolated gateway command as an operating-system process against a separate fake-Anthropic HTTP endpoint. It SHALL configure and authenticate the process only through its public flags, environment, YAML, and HTTP surface; it SHALL NOT duplicate service composition in a testserver.

Streaming evidence SHALL use three independent scenarios: normal finish and clean EOF; an established silent stream followed by client abort; and an established silent stream followed by process shutdown. Each silent-stream fake SHALL emit one valid initial Anthropic event before becoming silent so `DoStream` completes its pre-read and ProviderWire commits. Tests SHALL observe the first ProviderWire frame while the connection remains open before triggering abort or shutdown.

#### Scenario: Established stream is aborted by the client
- **WHEN** the real command has emitted the initial ProviderWire frame for a silent Anthropic stream and the client aborts
- **THEN** the fake endpoint SHALL observe cancellation of the Anthropic request context
- **AND** the command SHALL remain ready for another request

#### Scenario: Established stream is interrupted by process shutdown
- **WHEN** the real command has emitted the initial ProviderWire frame for a separate silent stream and receives its shutdown signal
- **THEN** the fake endpoint SHALL observe request cancellation
- **AND** the command SHALL become unready and exit within the independent shutdown bound

#### Scenario: Process integration composition is inspected
- **WHEN** the integration scenario starts the Gateway
- **THEN** it SHALL execute the built command path
- **AND** no in-process replacement router, catalog, authenticator, or ProviderWire handler SHALL be used

### Requirement: Phase 5 contract evidence
Tests SHALL include strict configuration and discovery-schema tests; authentication unit tests; hostile JWKS and Anthropic transport tests; real-listener lifecycle, flush, and telemetry tests; raw HTTP privacy/order assertions; and exact registered `@ai-sdk/gateway@4.0.52` real-command discovery and language-model calls. Provider input conformance fixtures SHALL not be synthesized or edited for service-only evidence. The registered upstream baseline SHALL remain unchanged unless this change explicitly performs a baseline upgrade.

#### Scenario: Full phase 5 verification runs
- **WHEN** repository verification executes for this change
- **THEN** service tests, ProviderWire V4 checks, real-command cross-language integration, module build/vet/lint, parity baseline validation, and existing provider tests SHALL pass
- **AND** deterministic fake transport data SHALL remain in focused service/integration tests rather than recorded or upstream provider fixture directories
