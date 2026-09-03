## Context

Phases 2-4 established the strict ProviderWire V4 request contract plus bounded unary and streaming text handlers in `ai-gateway/providerwire/v4`. The handler remains host-neutral: it owns relative `/language-model` protocol processing but not authentication, `/config`, route prefixes, provider construction, process lifecycle, or deployment. `ai-gateway/catalog` resolves aliases to canonical public IDs and lists canonical metadata, while `providers/anthropic` implements the direct Anthropic V4 model.

The registered baseline is upstream commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`, including `@ai-sdk/gateway@4.0.52`, `@ai-sdk/provider@4.0.7`, and `@ai-sdk/anthropic@4.0.38`. Matching Gateway source establishes that the client reads `{baseURL}/config`, requires a name and V4 specification row, and posts model calls to `{baseURL}/language-model`. The service base URL is `/api/v1/aisdk`; Grafana authentication headers are a host extension.

Phase 5 crosses security, process, protocol, catalog, provider, metrics, and cross-language boundaries. In particular, authlib's default key retriever uses `http.DefaultClient`, has no response-byte/key-count limit, and can refetch for attacker-selected key IDs. The Anthropic SDK follows ordinary `http.Client` redirect behavior and reads unary/error bodies eagerly; its SSE parser has a large line limit and can accumulate multiline events. Neither dependency's default HTTP behavior is acceptable at an authenticated production boundary, so the service must inject bounded clients rather than relying on defaults.

## Goals / Non-Goals

**Goals:**

- Provide one runnable internal service that authenticates, discovers, and invokes direct Anthropic through the completed strict text runtime.
- Validate all process, auth, provider, model, alias, secret-reference, and resource-limit configuration before serving.
- Bound outbound JWKS and Anthropic network work, response allocation, redirect behavior, key retention, and attacker-driven refreshes.
- Keep verified caller service and optional acting-user identity separate without retaining authlib token-bearing state.
- Give `/config` independent closed DTO/schema, header-safe IDs, bounded encoding, privacy, and raw response authority.
- Preserve incremental SSE through telemetry and make shutdown cancellation and graceful timeout ownership deterministic.
- Build and spawn the real command for registered-client evidence rather than duplicating service composition in tests.

**Non-Goals:**

- Dockerfiles, deployment manifests, production rollout, or migration from an existing deployment.
- A reusable Go V4 client or restoration of `providers/grafana`.
- Provider options, body-header forwarding, raw chunks, tools, files, reasoning output, or later runtime capabilities.
- Vertex, Bedrock, OpenAI, OpenAI-compatible, fallback, quotas, billing, IAM, or dynamic provider selection.
- Model-generation logger, Prometheus, enrichment, or Agent Observability middleware; phase 5 records process and HTTP lifecycle only.
- Eager JWKS or Anthropic network probes for readiness.

## Decisions

### Place the binary inside the isolated AGPL module

Add `ai-gateway/cmd/grafana-ai-gateway` with a small `main` and internal packages for configuration, bounded outbound HTTP, auth, discovery, service construction, and process lifecycle. The existing `ai-gateway` module depends on explicitly pinned root SDK and `providers/anthropic` revisions plus authlib, Kingpin, Prometheus, and YAML. It remains excluded from `go.work` and is built, tested, vetted, linted, and resolved independently with `GOWORK=off`.

This preserves the one-way AGPL Gateway-to-Apache SDK dependency and prevents service-only dependencies from entering root consumers. A command in the root module or a nested command module was rejected because either would weaken the established product boundary or its standalone validation.

### Make every process setting explicit

Kingpin uses an injected application rather than package-global state. Every setting has one exact flag and explicit service-prefixed environment variable:

| Setting | Flag | Environment | Default | Validation |
| --- | --- | --- | --- | --- |
| Config file | `--config.file` | `GRAFANA_AI_GATEWAY_CONFIG_FILE` | none | required readable regular file |
| Config bytes | `--config.max-bytes` | `GRAFANA_AI_GATEWAY_CONFIG_MAX_BYTES` | `1048576` | positive, safe `limit+1` |
| Deployment mode | `--deployment.mode` | `GRAFANA_AI_GATEWAY_DEPLOYMENT_MODE` | `production` | `production` or `development` |
| Listen address | `--server.listen-address` | `GRAFANA_AI_GATEWAY_SERVER_LISTEN_ADDRESS` | `:8080` | numeric TCP host/port syntax; unsafe auth additionally requires loopback host |
| Read-header timeout | `--server.read-header-timeout` | `GRAFANA_AI_GATEWAY_SERVER_READ_HEADER_TIMEOUT` | `5s` | positive |
| Request-read timeout | `--server.read-timeout` | `GRAFANA_AI_GATEWAY_SERVER_READ_TIMEOUT` | `30s` | positive |
| Response-write timeout | `--server.write-timeout` | `GRAFANA_AI_GATEWAY_SERVER_WRITE_TIMEOUT` | `165s` | positive and at least checked minimum below |
| Idle timeout | `--server.idle-timeout` | `GRAFANA_AI_GATEWAY_SERVER_IDLE_TIMEOUT` | `120s` | positive |
| Go `MaxHeaderBytes` | `--server.max-header-bytes` | `GRAFANA_AI_GATEWAY_SERVER_MAX_HEADER_BYTES` | `65536` | positive integer safe for value + 4096; effective parser read bound includes that slop |
| Response grace | `--server.response-grace` | `GRAFANA_AI_GATEWAY_SERVER_RESPONSE_GRACE` | `5s` | positive |
| Shutdown timeout | `--server.shutdown-timeout` | `GRAFANA_AI_GATEWAY_SERVER_SHUTDOWN_TIMEOUT` | `15s` | positive |
| Discovery response bytes | `--discovery.response-bytes` | `GRAFANA_AI_GATEWAY_DISCOVERY_RESPONSE_BYTES` | `1048576` | positive, safe `limit+1` |
| Unsafe auth | `--auth.unsafe` | `GRAFANA_AI_GATEWAY_AUTH_UNSAFE` | `false` | true only in development |
| JWKS URL | `--auth.jwks-url` | `GRAFANA_AI_GATEWAY_AUTH_JWKS_URL` | empty | required HTTPS in production; HTTP(S) in development; empty with unsafe auth |
| Allowed audiences | `--auth.audiences` | `GRAFANA_AI_GATEWAY_AUTH_AUDIENCES` | `ai-sdk` | comma-separated non-empty unique values |
| JWKS request timeout | `--auth.jwks-timeout` | `GRAFANA_AI_GATEWAY_AUTH_JWKS_TIMEOUT` | `5s` | positive |
| JWKS response bytes | `--auth.jwks-response-bytes` | `GRAFANA_AI_GATEWAY_AUTH_JWKS_RESPONSE_BYTES` | `1048576` | positive, safe `limit+1` |
| JWKS maximum keys | `--auth.jwks-max-keys` | `GRAFANA_AI_GATEWAY_AUTH_JWKS_MAX_KEYS` | `128` | positive integer |
| JWKS refresh interval | `--auth.jwks-refresh-interval` | `GRAFANA_AI_GATEWAY_AUTH_JWKS_REFRESH_INTERVAL` | `5m` | positive |
| JWKS maximum age | `--auth.jwks-max-age` | `GRAFANA_AI_GATEWAY_AUTH_JWKS_MAX_AGE` | `15m` | at least refresh interval |
| Anthropic header timeout | `--anthropic.response-header-timeout` | `GRAFANA_AI_GATEWAY_ANTHROPIC_RESPONSE_HEADER_TIMEOUT` | `10s` | positive and no greater than model duration |
| Anthropic response bytes | `--anthropic.response-bytes` | `GRAFANA_AI_GATEWAY_ANTHROPIC_RESPONSE_BYTES` | `16777216` | positive, safe `limit+1`, below SDK 32 MiB line limit |
| ProviderWire request bytes | `--providerwire.request-bytes` | `GRAFANA_AI_GATEWAY_PROVIDERWIRE_REQUEST_BYTES` | `1048576` | existing `v4.Limits` validation |
| ProviderWire unary bytes | `--providerwire.unary-response-bytes` | `GRAFANA_AI_GATEWAY_PROVIDERWIRE_UNARY_RESPONSE_BYTES` | `8388608` | existing validation |
| ProviderWire stream parts | `--providerwire.stream-parts` | `GRAFANA_AI_GATEWAY_PROVIDERWIRE_STREAM_PARTS` | `100000` | existing validation |
| ProviderWire frame bytes | `--providerwire.stream-frame-bytes` | `GRAFANA_AI_GATEWAY_PROVIDERWIRE_STREAM_FRAME_BYTES` | `1048576` | existing validation and fallback fit |
| ProviderWire model duration | `--providerwire.model-duration` | `GRAFANA_AI_GATEWAY_PROVIDERWIRE_MODEL_DURATION` | `120s` | existing validation |
| ProviderWire stream idle | `--providerwire.stream-idle-duration` | `GRAFANA_AI_GATEWAY_PROVIDERWIRE_STREAM_IDLE_DURATION` | `30s` | positive and no greater than model duration |
| ProviderWire drain duration | `--providerwire.stream-drain-duration` | `GRAFANA_AI_GATEWAY_PROVIDERWIRE_STREAM_DRAIN_DURATION` | `1s` | existing validation |

The minimum write timeout is calculated with overflow-checked duration addition:

```text
minimumWriteTimeout = requestReadTimeout
                    + jwksRequestTimeout
                    + modelDuration
                    + responseGrace
writeTimeout >= minimumWriteTimeout
```

This accounts for body availability, one bounded authentication refresh, total model work, and bounded final encoding/writes. Anthropic response-header time is inside model duration, not additive. Any overflow or violated inequality fails before secret resolution. TCP listen syntax is validated with every other scalar, and the scalar JWKS endpoint is validated immediately afterward; both therefore fail before YAML loading, secret resolution, client/component construction, or listener binding.

Both cloned outbound transports use named constants: `outboundDialTimeout = 5s`, `outboundTLSHandshakeTimeout = 5s`, `outboundExpectContinueTimeout = 1s`, `outboundIdleConnTimeout = 90s`, `outboundMaxIdleConns = 32`, `outboundMaxIdleConnsPerHost = 8`, and `outboundMaxConnsPerHost = 32`. Tests inspect both transport instances so a later SDK/default change cannot silently remove connection bounds. `server.max-header-bytes` maps directly to `http.Server.MaxHeaderBytes`; it is not advertised as an exact wire ceiling because Go reads `MaxHeaderBytes + 4096` bytes for parser buffering. Boundary tests document that effective slop.

Phase 5 bounds each accepted inbound connection and request through header, read, write, idle, body, protocol, and shutdown limits. It intentionally does not add a process-wide concurrent connection or request budget. Phase 6 owns those aggregate budgets and an explicit health-route capacity strategy so model-route saturation cannot starve `/live`, `/ready`, or `/metrics`.

All settings in YAML was rejected because process/container settings need flags and environment variables. Inferred Kingpin environment names were rejected because security settings need a stable explicit contract.

### Strictly decode route configuration and constrain public IDs

Read the YAML through `limit+1`, reject overflow, enable known-field and duplicate-key rejection, and require EOF after one document. The schema remains:

```yaml
providers:
  anthropic-primary:
    type: anthropic
    apiKeyEnv: ANTHROPIC_API_KEY
    baseURL: https://api.anthropic.com
models:
  grafana/assistant:
    name: Grafana Assistant
    description: General-purpose assistant
    primary:
      provider: anthropic-primary
      model: claude-sonnet-...
    aliases:
      - assistant
```

Map keys are provider instance and canonical public IDs. Literal API-key fields do not exist, so strict decoding rejects them. An injected `LookupEnv` resolves each referenced secret once and errors mention only field paths and variable names.

The shared endpoint validator requires an absolute hierarchical URL with scheme, non-empty host, `Opaque == ""`, `User == nil`, `RawQuery == ""`, `ForceQuery == false`, and `Fragment == ""`. Production permits only `https`; development permits `http` or `https`. Paths are allowed because JWKS is a concrete path and an Anthropic-compatible endpoint may include a path prefix. Rejecting userinfo prevents YAML such as `https://user:password@host` from becoming a second literal credential channel; rejecting query/fragment/opaque forms keeps routing and secret-like values out of configuration.

Canonical IDs and aliases must be 1-128 bytes of ASCII matching `^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`. This forbids whitespace, controls, commas, Unicode ambiguity, and header delimiters outside the deliberately small set while preserving planned IDs such as `grafana/assistant`. Validation happens before catalog construction.

### Inject hardened outbound transports

Build independent cloned `http.Transport` instances for JWKS and Anthropic with the exact named dial/TLS/expect-continue/idle/connection constants above and configured response-header behavior. Both `http.Client` values reject every redirect through `CheckRedirect`; redirects are not followed even when same-origin because retaining credentials across redirect semantics is unnecessary. Endpoints must pass the credential-free hierarchical URL validator before client construction.

Wrap each transport after Go's automatic decompression with a response-body limiter. The wrapper permits exactly the configured bytes and returns a typed over-limit error when the first extra decompressed byte is read, then closes the original body. This bounds Anthropic unary success, error `io.ReadAll`, SSE lines, aggregate multiline events, and total stream bytes. The Anthropic limit must remain below its SDK's 32 MiB per-line allowance, so the transport bound wins first. ProviderWire's model context remains the total request deadline; the configured response-header timeout gives a tighter pre-body bound.

Redirect, gzip expansion, exact-limit, over-limit, no-header, stalled-body, and cancellation tests use hostile local servers. Using `http.DefaultClient`, SDK defaults, or an unbounded `io.Reader` wrapper was rejected.

### Replace default JWKS caching with one bounded joinable snapshot

Implement the `authn.KeyRetriever` interface in the service. Under one mutex it stores one immutable `map[kid]jose.JSONWebKey` with at most `maxKeys`, fetch time, last refresh-attempt time, and an optional `refreshFlight`. The fixed-size flight contains `done chan struct{}` plus its final error/result publication state. Unknown key IDs are never retained.

`Get(ctx, kid)` follows this order under the mutex where state is inspected or changed:

1. reject empty `kid` locally;
2. return a matching key from a snapshot no older than `maxAge`;
3. if `flight != nil`, copy its completion channel, unlock, wait for either that channel or `ctx.Done()`, then retry lookup against the published snapshot without authorizing another fetch in the same reserved interval;
4. if no flight exists, apply cadence to no-snapshot, expired-snapshot, or unknown-key demand;
5. if cadence forbids refresh, return `authn.ErrInvalidSigningKey` locally;
6. otherwise reserve `lastAttempt = now`, install a new flight, unlock, launch one service-owned bounded refresh, and wait on the same flight like every other caller;
7. on completion, the refresh locks, atomically publishes a fully validated snapshot or flight error, clears the flight, closes `done`, and unlocks; all uncanceled waiters then retry lookup.

The shared refresh context derives from the service-lifetime context plus `jwksRequestTimeout`, not from one request. The starter and every joiner wait for either `flight.done` or their own `ctx.Done()`; canceling any request returns that caller promptly without canceling refresh work needed by another caller. Process shutdown cancels the service context.

Refresh requires HTTP 200 JSON, bounds decompressed bytes, parses no more than `maxKeys`, rejects empty/duplicate key IDs and invalid verification keys, and replaces the snapshot only after complete success. Failed refresh preserves a prior snapshot only while within `maxAge`. Barrier-based tests hold the network response open, release concurrent rotated-key callers, and schedule one caller across completion; all must join or observe the published snapshot, exactly one fetch must occur, and no second approved caller may race into a new fetch.

Authlib's `DefaultKeyRetriever` was rejected because its miss behavior and key retention do not satisfy these bounds. Eager startup fetch was rejected because readiness must remain local.

### Build immutable Anthropic models with the hardened client

Each named provider entry is resolved once into private configuration containing the API key and validated base URL. Before service wiring, correct `providers/anthropic.New` itself to construct the SDK client as:

```go
anthropic.NewClient(
    option.WithoutEnvironmentDefaults(),
    option.WithAPIKey(apiKey),
)
```

These options must be passed directly to `NewClient`; supplying `WithoutEnvironmentDefaults` later through provider `WithRequestOptions` cannot stop the SDK from reading environment defaults during client construction. Poisoned-environment provider tests set conflicting `ANTHROPIC_BASE_URL`, `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_PROFILE`, fallback profile/federation and identity-token variables, organization ID, and `ANTHROPIC_CUSTOM_HEADERS`. Unary and streaming calls must use only the explicit API key/base URL/client/options, must not contact a poison endpoint or load profiles/federation, and must not send poisoned headers. Vertex retains its explicit Google-auth construction.

For every model route, catalog construction looks up the resolved named provider and calls the corrected `anthropic.New` exactly once with the backend model ID and explicit reviewed request options for the hardened HTTP client and validated base URL. Every model is retained by one `catalog.StaticEntry`; no provider/model is rebuilt during resolution. This service-boundary hardening changes ambient-environment behavior without changing the Go API. Suppressing `ANTHROPIC_BASE_URL` is an intentional Go deviation from the registered TypeScript provider, which reads it when no explicit base URL is supplied; the Go SDK's profile, federation, identity-file, organization, and custom-header environment defaults have no registered TypeScript equivalent. Existing provider request fixtures do not exercise process environment discovery, so focused poisoned-environment tests are the regression authority; recorded provider inputs remain unchanged.

Adding a new public Anthropic provider-factory API solely for service wiring was rejected. Passing API keys through request context or YAML was rejected because credentials are process state.

### Validate headers before authlib and separate identities

A pre-auth boundary iterates header keys case-insensitively and accepts exactly one effective `X-Access-Token` value and zero or one `X-Grafana-Id` value. It rejects case variants, repeated field lines, comma-coalesced values, and empty values. Only after this check may one exact `Bearer ` prefix be removed. The normalized values are supplied through a private `authn.TokenProvider`; `Authorization` is ignored.

Production builds one access and one ID verifier over the bounded shared key retriever. Access verification uses configured audiences. Unsafe development mode uses authlib's unsafe verifiers, retains type/expiry/audience/namespace checks, and logs one fixed warning. Because signatures are deliberately not verified, unsafe mode also requires a loopback TCP listen host (`127.0.0.0/8`, `::1`, or `localhost`). Wildcard, empty-host, and non-loopback listeners are invalid. `production + unsafe`, `safe + empty JWKS URL`, and `unsafe + JWKS URL` are invalid.

After `DefaultAuthenticator.Authenticate`, service identity is accepted only when `AuthInfo.GetExtra()[authn.ServiceIdentityKey]` contains exactly one non-empty value. There is no subject/identifier fallback: those methods can return an acting user when an ID token or embedded actor exists. Namespace must be non-empty. Acting user is extracted separately only when identity type is not access policy.

The request context receives a private value with service identity, namespace, and optional acting-user subject/type. It never receives `types.AuthInfo`, the extras map, raw/parsed tokens, email, groups, or permissions; authlib extras may contain the raw ID token. Authentication is outside both protected handlers, so failure precedes deep request decoding, listing, resolution, and provider calls. No protocol Policy abstraction exists before a concrete host-policy capability lands.

### Add a host-safe V4 error writer

Authentication and discovery failures need the same closed V4 byte authority as model-handler failures. Add a narrow exported, non-fallible writer to `ai-gateway/providerwire/v4`. Callers choose only authentication, permission, or internal categories; they cannot provide messages, causes, status, type, code, or retryability. The writer reuses package-owned fixed documents directly and maps an invalid category to the fixed internal document.

This behavior is normative under the modified `providerwire-v4-unary-runtime` capability. A configurable error budget, production schema compilation, dynamic error encoding, and copying fixed JSON into the service were rejected because the documents are already small fixed package-owned bytes.

### Give discovery independent strict response authority

The service owns private DTOs for root, model row, and specification. A test-only draft 2020-12 schema closes every object, requires `models`, `id`, `name`, and specification fields, fixes `specificationVersion` to `v4` and provider to `grafana`, and permits only an optional string description.

After authentication, the handler calls `ListModels`, expands each canonical row and alias into independent complete public rows, sorts by row ID, and incrementally JSON-encodes root, rows, fields, and strings through a `limit+1` buffer. Encoding stops as soon as the first byte beyond the limit would be retained, so one description repeated across many aliases cannot create a whole-response temporary allocation. Only the complete bounded document is committed as HTTP 200. Invalid UTF-8, overflow, mapping, or listing failure uses the host-safe V4 writer; partial discovery never escapes.

Each alias row uses the alias as both `id` and `specification.modelId`, because that is what the client sends. Provider instance name/type, backend ID, API-key environment name, base URL, and alias relationship are absent. Raw closure/privacy/boundary tests remain authoritative because the pinned client parser is permissive. A registered-client test invokes every returned model ID, proving it survives header construction and resolution.

### Use exact routing and explicitly add `/metrics`

Use one standard-library dispatcher with explicit method checks before handler selection. Do not rely solely on Go's `GET` ServeMux pattern because it also accepts HEAD. The five routes are:

- unauthenticated `GET /live`;
- unauthenticated `GET /ready`;
- unauthenticated `GET /metrics`;
- authenticated `GET /api/v1/aisdk/config`;
- authenticated `POST /api/v1/aisdk/language-model`.

The authoritative delivery plan is updated with `/metrics` as the fifth operational route, on the same listener, so phase 5 health collectors are scrapeable. It is not a ProviderWire route and requires no model API credential.

The language-model route strips `/api/v1/aisdk` so the V4 handler still receives authoritative relative `/language-model`. Unsupported methods on recognized paths return 405 with `Allow: GET` for live, ready, metrics, and config or `Allow: POST` for language-model. Unmatched paths return 404. Neither path performs auth or protected work.

### Preserve flushing and normalize telemetry cardinality

One outer middleware allocates a request-scoped telemetry state pointer before routing. Auth may populate normalized service/namespace fields in that state. The outer layer emits exactly one log/metric terminal observation even for auth failure, 404, or 405.

Its response writer tracks status and implements `Unwrap() http.ResponseWriter`. `http.NewResponseController` can therefore traverse to the real writer and flush every SSE frame. Tests keep a stream open after the first initial frame and assert the client receives that frame immediately; clean EOF alone is insufficient evidence.

Route labels are exactly `live`, `ready`, `metrics`, `config`, `language_model`, or `unmatched`. Method labels are `GET`, `POST`, or `other`; status labels are `1xx` through `5xx`. Logs use the same normalized vocabulary. Arbitrary paths, methods, model IDs, callers, provider identity, URLs, and error text never become metric labels. Authentication is recorded only as the closed completion class `not_attempted`, `authenticated`, or `authentication_failed`; verifier text and caller values are never logged.

A service-owned Prometheus registry registers Go/process, readiness, in-flight, request-count, and duration collectors once and is exposed through `/metrics`. Existing model-generation middleware is deferred to phase 9.

Process lifecycle logging uses one fixed message and the closed events `process_starting`, `process_ready`, `process_shutdown_started`, and `process_shutdown_completed`. These records contain no settings, URLs, provider/model identity, credentials, or error text. Startup is recorded before scalar parsing; ready is recorded only after listener binding; shutdown-start follows readiness becoming false; shutdown-complete is emitted after graceful or forced server termination.

### Make graceful shutdown use an independent context

`http.Server` uses the configured read/write/idle limits, sets `MaxHeaderBytes` to the configured Go parser value, and uses a cancelable process `BaseContext`. Tests cover requests below the configured value, inside the documented 4096-byte parser slop where syntactically possible, and above the effective read bound; the service makes no exact-wire-ceiling claim. Shutdown proceeds:

1. set readiness false;
2. cancel the process context so established SSE and Anthropic requests exit;
3. create the graceful timeout from `context.Background()` (or `context.WithoutCancel`), never the canceled process context;
4. call `Server.Shutdown` with that independent deadline;
5. call `Server.Close` only if graceful shutdown fails or the deadline expires.

Tests separately prove graceful completion when handlers honor cancellation and forced close when one deliberately ignores it. Depending on `Shutdown` to cancel requests was rejected because Go does not do so; deriving its deadline from the process context was rejected because it would already be canceled.

### Build and spawn the real command for integration

Focused Go tests cover units and real-listener internals. Cross-language integration is different: it runs `go build` for `ai-gateway/cmd/grafana-ai-gateway`, starts a separate fake-Anthropic process/server, writes temporary public YAML, supplies only documented flags/environment, spawns the built executable, and waits for `/ready`. It never imports or reconstructs the service router.

The exact registered Gateway client supplies `X-Access-Token` and optional `X-Grafana-Id` through configured headers and covers discovery plus unary/streaming calls. Streaming has three independent scenarios:

1. fake Anthropic emits valid initial, text, and finish events; the client observes finish and clean EOF while the command stays alive;
2. fake Anthropic emits one valid initial event and stays silent; after the initial ProviderWire frame arrives while open, the client aborts and the fake observes request cancellation;
3. a fresh silent stream reaches the same established point; the process receives SIGTERM, becomes unready, cancels Anthropic, and exits within the independent shutdown bound.

The initial event is mandatory because Anthropic `DoStream` pre-reads before returning. Fake payloads remain focused integration data, never provider conformance evidence.

## Risks / Trade-offs

- [Unsafe auth or HTTP is enabled in production] → Typed deployment validation rejects unsafe auth and non-HTTPS outbound URLs before listener binding.
- [Unique attacker `kid` values amplify JWKS traffic or memory] → One bounded snapshot, no negative-key retention, and one mutex-owned joinable flight reserve cadence before network work while allowing concurrent waiters to share the result.
- [Compressed or multiline provider output bypasses bounds] → Limit the post-decompression response body cumulatively below the SDK scanner line limit and test gzip expansion plus multiline SSE.
- [Acting user becomes caller service] → Require exactly one `ServiceIdentityKey`; never fall back to subject/identifier and never retain `AuthInfo`.
- [The pinned client accepts malformed discovery] → Closed private DTO/schema, header-safe ID grammar, bounded precommit encoding, and raw tests remain authority.
- [Telemetry buffers SSE] → Require `Unwrap`, test first-frame delivery while open, and keep one outer telemetry owner.
- [Canceled process context cancels graceful shutdown immediately] → Derive the shutdown deadline independently and test graceful and forced paths.
- [Strict no-redirect behavior rejects a legitimate endpoint migration] → Require operators to configure the final HTTPS endpoint explicitly; avoiding credential forwarding is more important than redirect convenience.
- [Cumulative Anthropic response bounds reject unusually large valid streams] → Make the positive limit operator-configurable but capped below the SDK scanner allocation; later streaming families can revisit the cap with evidence.
- [Aggregate inbound concurrency exhausts process capacity] → Phase 5 bounds each connection/request but accepts this deployment-stage gap; phase 6 owns global connection/request budgets and reserved health-route capacity.
- [A fake response is mistaken for provider parity] → Keep it outside recorded/upstream fixture directories and classify it only as process/transport evidence.

## Migration Plan

1. Correct direct Anthropic provider construction to disable SDK environment defaults and add poisoned-environment/parity evidence.
2. Add the modified ProviderWire host-safe error writer and its schema/bound/fallback tests.
3. Add the command inside the isolated `ai-gateway` module, exact process-setting table, checked write-budget validation, and strict header-safe/credential-free-URL YAML loading.
4. Add hardened outbound transports and hostile JWKS/Anthropic tests, then the bounded snapshot retriever with an explicit joinable flight.
5. Add strict header normalization, authlib verifiers, service-identity separation, and normalized caller/telemetry state.
6. Add resolved named Anthropic configuration, immutable catalog, closed bounded discovery, and pinned-client invocation of every row.
7. Add exact routing/Allow headers, `/metrics`, flush-preserving telemetry, documented Go header slop, local readiness, and independent-deadline shutdown.
8. Build/spawn the real command for separate finish, abort, and process-shutdown integration scenarios.
9. Run ProviderWire, integration, provider, parity, module, and full repository validation.

Rollback removes the service command and composition from the `ai-gateway` module plus the additive host-safe error-writer API. The strict ProviderWire runtimes, catalog, Anthropic provider, and registered-client contract remain independently usable. No image or production deployment depends on this phase yet.

## Open Questions

None. The authoritative delivery plan now includes `/metrics` as the fifth unauthenticated operational route so phase 5 Prometheus health metrics have an actual scrape surface.
