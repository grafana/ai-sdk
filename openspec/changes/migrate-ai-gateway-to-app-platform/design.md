# Design

## Context

See proposal.md for motivation. The current Gateway command in `ai-gateway/cmd/grafana-ai-gateway` composes four layers:
- **Process and listeners:** `process.Run`, which builds the catalog, BYOK factories and telemetry and serves the private JWT, Cloud and operational listeners.
- **Authentication:** JWT/JWKS verification, Cloud assertions and static keys in `auth` and `config`.
- **Request routing:** a small router in front of the ProviderWire V4 handlers.
- **Service code:** catalog, selection, BYOK, telemetry and the attempt sink.

Everything below the router is host-independent and stays. That includes `providerwire/v4`, `catalog`, `byok`, `nativemodel`, `nativeoptions` and the provider client and endpoint validation in `outbound`.

Relevant host facts, checked against `grafana-app-sdk` `v0.60.13` / `plugin/v0.60.13`, `grafana-plugin-sdk-go` v0.296.5 and `go-plugin` v1.8.0, and rechecked against `v0.60.16` and v0.297.0 (the create-plugin 7.14.0 pins):
- **Cloud routing and auth:** user traffic reaches the Grafana router through the stack front door, which authenticates sessions and service account tokens and forwards an access token carrying the caller. Internal services call the router directly with their own access tokens. The router authenticates the caller's `X-Access-Token`. It exchanges the caller's access token for one addressed to the plugin and calls the plugin's v3 `RouteService.CallRoute` over gRPC. App-sdk's server verifies that token's signature, audience (plugin ID or API group) and namespace before the handler runs.
- **Delegated token shape:** each exchange wraps the subject token as the next actor and keeps its signed `serviceIdentity`, which only internal access policies can carry. The first actor is therefore the router's direct caller, and the innermost actor is the user or service account, if any. authlib's `AuthInfo` exposes neither the first actor's service identity nor, for on-behalf-of calls, the right one.
- **Self-hosted Grafana:** v3 routes need the experimental `grafana.useRouterMiddleware` toggle. A caller identity reaches the plugin only when `[plugins] v3_token_exchange_url` and `v3_cap_token` point at a token-exchange service. Otherwise requests carry none, except in development mode with `insecure_skip_authentication`, where Grafana mints locally signed tokens with the caller's identity. The App Platform team plans v3 to work unmodified, with identity out of the box, on self-hosted Grafana.
- **Request and response shape:** `CallRoute` carries the full request body in one message and streams responses back, one message per `Flush`. With `RouteHandlerProvider`, the original `/apis/<group>/<version>/...` URL reaches a plain `http.Handler`, with the verified group, version and namespace in `RouteRequestInfo`. The host-side `httpadapter` strips `Authorization`, `X-Access-Token`, `X-Grafana-Id` and `Cookie`; the plugin-side route adapter copies headers unchanged.
- **Process lifecycle:** the plugin runs as a child process and receives no termination signal. The host gracefully stops its own gRPC server, draining in-flight calls, then sends the go-plugin Shutdown RPC, which hard-stops the plugin's gRPC server. `backend.Manage` then shuts down the tracer provider with a 5 s timeout before `plugin.Run` returns. The host kills the process about 2 s after Shutdown.
- **Environment:** the plugin sees the environment its host passes. Grafana forwards host variables only for plugins listed in `forward_host_env_vars`, and passes plugin settings as `GF_PLUGIN_*`. The router host passes its own environment.
- **Logs:** the host parses plugin stderr as hclog JSON and relays it at the encoded level. Anything else is relayed at debug, and lines over 64 KiB are split.
- **Metrics:** the host gathers plugin metrics through `CollectMetrics`, served from `prometheus.DefaultGatherer`.
- **Tracing:** configured by `GF_INSTANCE_OTLP_*` inside `Manage`. The plugin gRPC server, including v3 routes, is instrumented with `otelgrpc`.

## Goals / Non-Goals

**Goals:**
- Keep the ProviderWire handlers and their tests unchanged. The plugin host is a thin adapter around them.
- Replace every standalone host concern with a host-provided equivalent, and delete the old code instead of keeping a second host.
- Keep exact-pinned client evidence by exercising the real plugin process and route protocol in tests.

**Non-Goals:**
- App Platform kinds: tenant catalogs, stored BYOK secrets as secure values, or model routes as resources.
- Manifest roles or RBAC route checks, which Grafana does not apply to custom routes yet (decision 2). BYOK needs no Gateway role, as `gateway-caller-access` specifies.
- Deployment wiring, catalog publishing and a plugin publish job. These are tracked as follow-ups.
- A plugin memory-limit setting. It is a deployment concern, tracked as a follow-up.
- Changing ProviderWire wire format or error documents.

## Decisions

### 1. Host through app-sdk `plugin.Run` with one `http.Handler`
`main` calls `plugin.Run` with an app-sdk app built from the generated manifest.
- **Composition:** the catalog, BYOK, endpoint validation and telemetry wiring move from `process.Run` into the app constructor. Its startup tests are ported; only listener, serve and signal code is deleted.
- **Dispatch:** the app implements `RouteHandlerProvider` and returns one `http.ServeMux` with method and path patterns for the two routes. The namespace comes from `RouteRequestInfo`. The language-model handler sees `providerv4.LanguageModelPath`, as the current router does.
- **Route labels:** HTTP request metrics label routes as `config` and `language_model` instead of matching legacy paths.
- **Alternative:** per-route `CallCustomRoute` handlers on `simple.App`. Rejected because that path is deprecated in the adapter and would need a second handler signature around code that is already an `http.Handler`.

### 2. Routes-only manifest, plugin ID `grafana-aigateway-app`
- **Manifest:** a CUE manifest under `ai-gateway/kinds/` declares app name `aigateway`, giving the group `aigateway.ext.grafana.app`. It declares version `v1alpha1` with no kinds and two namespaced OpenAPI paths.
- **Why routes-only:** a routes-only manifest gets the platform's lighter routes-only handler, with no storage.
- **Generated outputs:** committed, as `gateway-plugin-packaging` specifies.
- **Alternative:** a kind with subresource routes. Rejected because there is nothing to store yet, and it would force folder semantics on callers.
- **Route authorization:** Grafana authorizes custom routes with plugin app access and optional per-operation checks declared with the `x-grafana-declared-authz-*` extensions. Manifest roles only grant access to kinds, and Grafana does not convert their route lists, so a routes-only manifest registers no roles; the default roles app-sdk generates have no effect. A declared check would refuse every user and service account, because only kind roles grant the permission it names. The Gateway therefore relies on app access and its own caller-access decision, and declares no route checks. This deviates from the scaffold guidance to keep authorization out of app code: account selection by caller chain and mode is not expressible in the manifest. App access is granted to every authenticated caller in Cloud router deployments, and is a basic-role permission an administrator can restrict in self-hosted Grafana.
- **Alternative:** a placeholder kind to obtain a bindable role. Rejected as a resource kept only for permissions.

### 3. Caller access from the verified actor chain
The Gateway supplies its own authenticator through `plugin.WithAuthenticator`. It wraps authlib's access-token verifier with the same audiences (plugin ID and API group) as app-sdk's default, and reimplements app-sdk's unexported key-source rules: exactly one of `GRAFANA_JWKS_URL` (absolute http(s), read with `authn.NewKeyRetriever`) or `GRAFANA_JWKS` (public signing keys, each with a key ID). From the verified claims it adds two values to `GetExtra()`, which app-sdk passes through:
- the direct caller's service identity: the first actor's `serviceIdentity`, ignoring the top-level signer, which is the plugin host;
- the identity actor: the innermost actor's typed UID when it is a user or service account.

A middleware turns those values, the route namespace and the access mode into the private caller value (access class, service, identity actor, namespace) consumed by selection and observation:

| Mode | Caller | Access |
|---|---|---|
| `cloud` | Direct caller on the allowlist | configured |
| `cloud` | Otherwise, user or service-account identity actor | BYOK |
| `self-hosted` | User or service-account identity actor | configured; BYOK controls refused |
| either | Anything else | refused with `HostErrorPermission` |

- **Why the first actor:** a plugin calling for a user produces `plugin → router → front door → user`. The innermost service is the router and authlib's `InnermostServiceIdentityKey` returns the user's empty value. Only the first actor names the service that called the Gateway. Any-hop matching was rejected because a non-allowlisted service relaying an allowlisted service's token would gain the catalog.
- **Access mode:** `access.mode` / `GRAFANA_AI_GATEWAY_ACCESS_MODE`, required, `cloud` or `self-hosted`. Running Cloud as `self-hosted` would give every stack user the catalog, so there is no default.
- **Allowlist setting:** comma-separated `access.internal-services` / `GRAFANA_AI_GATEWAY_ACCESS_INTERNAL_SERVICES`, valid only in `cloud` mode. Entries are the calling services' signed service identities.
- **Emitted attribution:** the existing `caller_service` log and export field carries the direct caller's service identity only for configured `cloud` access, and is omitted otherwise.
- **Attribution:** the identity actor's UID stays in the private caller value for future usage attribution and is not emitted.
- **Production guard:** `deployment.mode` stays, defaulting to production. Production startup fails unless `GRAFANA_JWKS_URL` or `GRAFANA_JWKS` is set.
- **Local development:** Grafana's development-mode tokens are signed with an unpublished key. In development mode with `GF_PLUGIN_INSECURE_SKIP_AUTHENTICATION=true`, the Gateway authenticator uses authlib's unsafe verifier, which skips signatures but still checks audience and expiry; app-sdk still requires exactly one token and checks the namespace. Production refuses that combination.

The `auth` package shrinks to this authenticator and mapping. JWKS fetching, header normalization, Cloud assertions, static keys, unsafe verifiers and the `outbound` JWKS client are deleted.
- **Alternative:** a route split, with separate `byok/` routes. Rejected because identity already determines the population, and one route set keeps one client base URL per stack.
- **Alternative:** classic plugin resources for self-hosted Grafana, which forward a Grafana-signed ID token today. Rejected because App Platform plans identity on v3 for self-hosted Grafana, and a second entry point would outlive its need.

### 4. Logging through a `slog` → plugin SDK bridge
- **Bridge:** one `slog.Handler` adapts records to `backend/log.Logger`, mapping levels, flattening groups into dotted keys, resolving `LogValuer`s and adding trace and span IDs from the context.
- **Unchanged:** all existing loggers keep their `*slog.Logger` API, so `middleware/logger`, the Gateway allowlist and the redactor are untouched.
- **Attempt sink:** it keeps its bounded queue and worker, but writes records through the logger. `physical_output.go`, the socket/pipe deadline code, is deleted. hclog serializes writes under one lock, so `Close` takes a deadline and abandons a stalled worker.
- **Alternative:** slog's JSON handler with `ReplaceAttr` emitting `@level`/`@message`/`@timestamp`. Rejected because it couples the Gateway to go-plugin's private parsing format, including an exact timestamp layout.

### 5. Metrics on an injected registry
`Telemetry` takes its registerer and gatherer as construction options. `main` passes `prometheus.DefaultRegisterer` and `DefaultGatherer`; tests keep private registries, so existing service tests do not collide.
- **Removed:** the `/metrics` handler, `authentication_total` and the `ready` gauge with `SetReady`. Go and process collectors are not re-registered.
- **Added:** an access-decision counter.
- **Kept:** HTTP request metrics, because route requests still pass through the Gateway handler.

### 6. Tracing from the plugin SDK only
The plugin SDK configures OpenTelemetry inside `Manage`, and `otelgrpc` already creates the server span for each route call. The Gateway adds no span middleware and no exporter; the log bridge reads trace and span IDs from the request context.
- **Alternative:** a Gateway span middleware. Rejected because it would duplicate the existing server span.

### 7. Lifecycle and health
- **Shutdown:** `plugin.Run` returns after Shutdown and the SDK's tracer shutdown. `main` then calls `Close(ctx)` on Agent Observability and the attempt sink in parallel under one 1 s context. Each internal step (waiting for recorders, flush, client shutdown) uses a deadline derived from that context, capped by its configured duration, so default settings stay valid and never exceed the budget. Records emitted after close are dropped and counted.
- **Budget:** a slow tracer exporter can use the SDK's 5 s timeout and leave the Gateway's flush killed. This is accepted and documented, with a configurable tracer shutdown timeout as an upstream follow-up.
- **Health:** the app-sdk default instance answers `CheckHealth` with a fixed healthy result. `plugin.Run` constructs the app before serving, so construction failure ends startup and no readiness flag is needed.
- **Request bounds:** `providerwire.model-duration` remains the request bound. Operator documentation states the grace-period requirement.

### 8. Settings
Removed settings:
- listener addresses and HTTP server timeouts;
- JWT/JWKS, audiences and unsafe auth;
- the YAML `auth` and `server` blocks;
- the Agent Observability "required in production" rule.

Kept: `deployment.mode`, and kingpin and environment parsing for the ProviderWire limits, provider transport, observation, Agent Observability and config file settings. The plugin starts with no arguments, so values come from the host environment. Operator documentation covers `forward_host_env_vars` for Grafana and the router host environment.

### 9. Protocol test host
A test-only Go package under `ai-gateway/test/` acts as the platform:
- it maps test credentials to identities, accepting service account tokens only as an `Authorization` bearer and access tokens only as `X-Access-Token`;
- it signs delegated tokens with an in-memory JWKS, with the audience set to the plugin ID or API group, and builds platform-shaped actor chains: direct service, on-behalf-of user, relayed, and front-door user or service account;
- it forwards through app-sdk `httpadapter` to a v3 route client, and also offers the bare v3 route client so tests can send credential headers the host would strip;
- it runs the Gateway in either access mode.

It has two modes:
- **In-process**, for Go unit tests: the Gateway app is served on an in-memory gRPC listener with an explicit plugin ID.
- **Plugin process**, for cross-language suites: a small test binary builds a host-platform plugin executable with plain `go build` and launches it through a go-plugin client and `grpcplugin.NewClientV3`, as a host does. It exposes metrics and health from the plugin's `CollectMetrics` and `CheckHealth`, sending a `PluginContext` with the plugin ID and empty app instance settings as Grafana does, relays the plugin's logs and stops the plugin through Shutdown.

`main` passes `plugin.WithPluginID` with a constant checked against `src/plugin.json`, so a plain `go build` gets the same plugin ID and audience as a mage build.

The TypeScript suites that spawn `grafana-ai-gateway` spawn the test binary instead.
- **Alternative:** testing handlers directly. Rejected because it would skip the protocol hop the specs require.
- **Real Grafana in CI** complements the test host for the frontend: plugin validation runs the Playwright tests against the packaged plugin in the create-plugin environment on a digest-pinned Grafana nightly, the only line with the router middleware. The pin keeps unrelated Grafana changes from failing PRs and is bumped deliberately. Protocol and access behavior stay on the test host, since the environment authenticates callers anonymously.

### 10. Packaging in `ai-gateway/`
`ai-gateway/` becomes the plugin root and a pnpm workspace package, so it inherits the workspace supply-chain policy:
- the create-plugin 7.14.0 app scaffold with the `experimental-app-sdk` addition, kept close to its output so `create-plugin update` codemods apply cleanly. `.config/` is verbatim, and the Gateway module uses the app-sdk and plugin SDK versions its generator pins. The scaffold's scripts, frontend tooling, `kinds/` layout and root configuration files stay. `src/plugin.json` (`grafana-aigateway-app`, backend `gpx_aigateway`) declares one overview page that shows the stack's Gateway endpoint and a quick start;
- deviations only where the repository requires them: the existing `cmd/grafana-ai-gateway` entry point, pnpm workspace membership, repository CI instead of the scaffold workflows, the AGPL license and release version, local Gateway settings in `docker-compose.yaml`, and an ESLint and Prettier scope limited to the frontend. Install scripts of `@swc/core`, `@parcel/watcher` and `unrs-resolver` are denied because they only build or download fallbacks for the locked native binaries;
- the scaffold `Magefile.go`, with a build callback setting `RootPackagePath: ./cmd/grafana-ai-gateway`, and mage pinned in `mise.toml`;
- `dist/` gitignored and `node_modules` excluded from the Go module.

The plugin SDK and app-sdk `plugin` module declare `go 1.26.5`, so `ai-gateway/go.mod` and `go.gateway.work` move to 1.26.5 with the new dependencies.

The backend builds through `mise run build-ai-gateway-plugin` with `GOWORK=$PWD/../go.gateway.work` and `GOFLAGS=-mod=readonly -trimpath`. The build writes a per-platform Go module inventory with license and notice files, ported from the Dockerfile, plus frontend dependency licenses. The module-policy overlay check extends to the mage-built backend to prove same-revision source.

`ci.yml` replaces the image jobs with validation jobs that run on pull requests and eligible pushes:
- frontend typecheck, lint and unit tests, plugin build and frontend build;
- Playwright tests against the packaged plugin in Grafana;
- manifest regeneration diff;
- plugin validator;
- Gateway tests and test host suites.

Eligible pushes keep the validated package as a workflow artifact. Publish and deploy jobs are removed; a plugin publish workflow is a follow-up. The image `mise` tasks, `Dockerfile`s and image-source tests are deleted.

### 11. Go client
In `providers/grafana`:
- remove `NewWithCloudCredentials`, `CloudCredentialsConfig` and `WithUserIDToken`;
- add a call-context helper carrying an on-behalf-of subject token, either a user ID token or the delegated access token a v3 plugin receives: the token-exchange flow uses it as the exchange `SubjectToken`, so the returned token keeps the user and chain, and other flows or a wildcard namespace refuse the call. authlib caches exchanges per subject;
- document `grafana-aigateway-app` as the token-exchange audience for internal callers;
- require `TokenExchangeConfig.Audience`;
- add `NewWithServiceAccountToken`, which sends `Authorization: Bearer`;
- shrink the reserved headers to `Authorization`, `X-Access-Token` and `X-Grafana-Id`.

The base URL stays the full route prefix, so no namespace parameter is added. In self-hosted Grafana the same service-account constructor reaches the catalog. `providers/grafana` is a released module, so the PR title carries breaking release intent.

The pinned Vercel client always sends its `apiKey` as an `Authorization` bearer. That fits service accounts. Internal callers would need an access token in `X-Access-Token` next to that bearer, and the platform's handling of both headers together is unverified, so Vercel internal access is not claimed.

## Risks / Trade-offs

- **Hard 60s platform deadline on routes.** Until the platform's configurable timeout lands, longer streams are aborted mid-stream. → Tracked as an external dependency and documented as a current limitation. The test host does not model it.
- **Platform errors are not ProviderWire documents.** Platform-generated 401, 403, 404, 503 and 504 responses use Kubernetes `Status` JSON, which clients classify as protocol errors. → Accepted limitation, documented in the client and Gateway guides.
- **Uneven load across replicas.** The router uses one gRPC connection per pod. → External follow-up for per-request balancing and connection max age.
- **A crashed plugin can leave a pod failing.** It can leave the host with a stale client. → #419 contains panics. Host-side recovery is an external follow-up.
- **Final telemetry can be lost on shutdown.** The final flush shares the 2 s kill window with the SDK tracer shutdown. → Bounded loss, documented; upstream follow-up for the tracer timeout.
- **Experimental plugin protocol.** v3 can change. → Pin tagged app-sdk and plugin SDK versions; the protocol test host catches drift on upgrade.
- **Delegated identity shape is verified by reading code, not on a deployed platform.** This covers the first actor's service identity, on-behalf-of chains and the shape of front-door user traffic. → Prove it in the dev deployment before the allowlist is relied on.
- **Self-hosted identity depends on the platform.** Until vanilla Grafana passes v3 identity, self-hosted mode works only with a configured token-exchange service and signing keys, and v3 routes need the experimental router toggle. → Documented as the current self-hosted requirement; the App Platform team plans out-of-the-box support.
- **Service accounts are the self-hosted attribution unit.** Grafana tokens always belong to service accounts, which have no owner. → The self-hosted guide recommends one service account per application or person to attribute.
- **Heavier dependencies and local development.** Kubernetes API machinery enters the Gateway module, and there is no standalone run. → The test host gives a fast local loop; the create-plugin docker compose with nightly Grafana covers full manual runs.
- **Self-hosted Grafana support for routes-only plugin manifests is unverified at runtime.** → The local Grafana check records the result; documentation states the support status.

## Migration Plan

1. Land the whole migration in one PR on this branch. The Gateway has no clients, so there is no dual running.
2. After merge, in dev:
   - create the catalog stub, add a publish workflow and publish the plugin, and add the router plugin deployment entry;
   - configure the environment, secrets and egress;
   - enable routing for test stacks;
   - validate the delegated identity shape, the Vercel internal-caller headers, router credential-header stripping, streaming and telemetry.
3. Remove the old standalone dev deployment once the plugin serves dev traffic.
4. **Rollback:** revert the PR. Until step 3, the old dev deployment keeps running its last image.

## External Follow-ups

Tracked with the App Platform and deployment owners, outside this repository:
- **App Platform:**
  - verified caller identity on v3 in self-hosted Grafana without a token-exchange service;
  - configurable route timeout beyond 60s;
  - per-request gRPC load balancing to plugin backends;
  - re-resolving the plugin client after a plugin restart;
  - exporting plugin metrics from the plugin host;
  - a configurable tracer shutdown timeout in the plugin SDK;
  - authorizing custom routes from manifest roles, after which the Gateway can bind a route role to the basic roles.
- **Router plugin deployment:**
  - plugin tracing environment;
  - `GRAFANA_JWKS_URL`;
  - termination grace period above the model duration;
  - memory limit for the plugin process;
  - autoscaler object;
  - manifest version pinning;
  - IAM URL for future roles;
  - secret environment injection;
  - provider egress.
- **Publishing:** catalog stub, signing and a publish workflow that builds from the same-revision workspace and depends on the plugin validation jobs.
- **Front-door limits:** stack front-door timeouts and body limits for long streams.

