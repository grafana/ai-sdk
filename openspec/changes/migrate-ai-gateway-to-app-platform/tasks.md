# Tasks

## 1. Plugin scaffolding and dependencies

- [x] 1.1 Add the plugin root files in `ai-gateway/`:
  - `package.json` with `build` and `generate:kinds` scripts;
  - `src/plugin.json` (`grafana-aigateway-app`, backend `gpx_aigateway`) and a `src/module.tsx` registering an empty `AppPlugin`;
  - the create-plugin `.config/` scaffold with its webpack build and local Grafana compose environment.

  Register `ai-gateway` in `pnpm-workspace.yaml` and the `mise.toml` deps sources, extending `allowBuilds` only as the toolchain requires. Gitignore `ai-gateway/dist/` and exclude `node_modules` from the Go module with an `ignore` directive. Verify that:
  - `mise deps` installs with the frozen lockfile;
  - `pnpm --filter ./ai-gateway build` produces `dist/plugin.json` and `dist/module.js`;
  - `go list ./...` in `ai-gateway/` includes no `node_modules` package.
- [x] 1.2 Add the app-sdk generator config, with a pinned generator version, and a routes-only CUE manifest under `ai-gateway/kinds/`:
  - app name `aigateway`, version `v1alpha1`, no kinds;
  - OpenAPI paths `namespaces/{namespace}/config` (GET) and `namespaces/{namespace}/language-model` (POST).

  Commit the generated manifest data. Verify that `pnpm --filter ./ai-gateway generate:kinds` leaves no diff, and that a Go test asserts the generated manifest declares only those two routes and no kinds.
- [x] 1.3 Add `grafana-app-sdk` `v0.60.13` (app, simple) and `plugin/v0.60.13`, plus `grafana-plugin-sdk-go`, to `ai-gateway/go.mod`. Move `ai-gateway/go.mod` and `go.gateway.work` to Go 1.26.5 with `go work use`. Add a `Magefile.go` with `RootPackagePath: ./cmd/grafana-ai-gateway`, pin mage in `mise.toml` `[tools]`, and add a `build-ai-gateway-plugin` mise task. The task runs mage with `GOWORK=$PWD/../go.gateway.work` and `GOFLAGS=-mod=readonly -trimpath`. Verify:
  - the task builds Linux amd64/arm64 backends into `ai-gateway/dist/`;
  - `mise run verify-gateway-workspace`, `mise run test-ai-gateway`, `mise run verify-ai-gateway-boundary` and `mise run verify-sdk-gateway-isolation` pass.

## 2. Protocol test host package

- [x] 2.1 Add a test-only Go package under `ai-gateway/test/` that:
  - accepts service account tokens only as an `Authorization` bearer and access tokens only as `X-Access-Token`;
  - signs delegated tokens with an in-memory JWKS and builds platform-shaped actor chains:
    - direct service call, with the service identity on the first actor;
    - on-behalf-of user call;
    - relayed call;
    - front-door user and service-account calls;
    - anonymous;
  - forwards HTTP through app-sdk `httpadapter` → v3 route client → the app route adapter on an in-memory gRPC listener, setting the route info `httpadapter` needs for the namespace;
  - exposes the bare v3 route client for requests carrying credential headers the host would strip;
  - verifies tokens with an authlib access-token authenticator built from that JWKS, with an explicit plugin ID.

  Verify Go tests against a stub `http.Handler` that:
  - streams frames incrementally;
  - observes the delegated `AuthInfo`, actor chain and route namespace;
  - accepts both plugin ID and API group audiences;
  - refuses a foreign audience, an uncovered namespace and an access token sent as a bearer before the handler runs.

## 3. Go client platform flows

- [x] 3.1 In `providers/grafana`:
  - add `NewWithServiceAccountToken`, which sends one `Authorization: Bearer`;
  - make `TokenExchangeConfig.Audience` required;
  - add the call-context helper for on-behalf-of subject tokens, either a user ID token or a delegated access token. The exchange flow uses it as `SubjectToken`; other flows and wildcard namespaces refuse the call before I/O.

  Add an `Audience` to the `gateway-command.test.ts` token-exchange fixture, and add the service-account and on-behalf-of flows to `providers/grafana/internal/capture/main.go`. Verify:
  - the constructor, flow, on-behalf-of, header-precedence, refusal-timing and redirect tests, and the public-surface test;
  - `mise run test-grafana` and `mise run test-ai-gateway-command` pass.

## 4. Plugin host runtime and caller access

- [x] 4.1 Implement the Gateway authenticator, passed with `plugin.WithAuthenticator`. It wraps authlib's access-token verifier with app-sdk's audiences, reimplements app-sdk's key-source rules for `GRAFANA_JWKS_URL` and `GRAFANA_JWKS`, and adds the first actor's service identity and the identity actor's typed UID to `GetExtra()`. Switch the in-process test host to serve with this authenticator built from the test JWKS. Verify table tests over the test host's actor chains, plus key-source config tests for both variables set, a non-http(s) URL, and a private key or missing key ID:
  - direct service, on-behalf-of user, relayed, front-door user and service account, and no actor;
  - unsigned request data never contributes.
- [x] 4.2 Implement the required `access.mode` / `GRAFANA_AI_GATEWAY_ACCESS_MODE` setting (`cloud` or `self-hosted`, no default), and the `cloud`-only `access.internal-services` / `GRAFANA_AI_GATEWAY_ACCESS_INTERNAL_SERVICES` allowlist with bounded, unique, exact-match validation. Implement the access middleware producing the private caller value:
  - `cloud`: an allowlisted direct caller gets configured access; otherwise a user or service-account identity actor gets BYOK;
  - the emitted `caller_service` is set only for configured `cloud` access;
  - `self-hosted`: a user or service-account identity actor gets configured access, with BYOK controls refused;
  - everything else is refused with `HostErrorPermission` before selection.

  Verify:
  - table tests across modes, actor chains, wildcard tokens and route namespaces, with zero catalog and provider calls on refusal;
  - config tests for a missing or unknown mode, an allowlist in `self-hosted` mode, an empty allowlist, case and whitespace near-matches, and invalid entries.
- [x] 4.3 Move startup composition from `process.Run` into the app constructor and replace `main` with `plugin.Run`, passing `plugin.WithPluginID` with a constant. The app's `RouteHandlerProvider` is a `ServeMux` dispatching the two route patterns through the access middleware, with the namespace from `RouteRequestInfo`. Also:
  - make `Telemetry` take its registerer and gatherer as options, with `main` passing the default registry and tests private ones;
  - normalize route metric labels to `config` and `language_model`;
  - port the `process` startup tests to the constructor;
  - delete the listener, serve and signal code and the operational router.

  Verify with in-process test host tests for:
  - unary and streaming dispatch;
  - undeclared paths and methods doing no work;
  - legacy `/api/v1/aisdk/*`, `/live`, `/ready` and `/metrics` not being served;
  - route labels without namespace values;
  - the plugin ID constant matching `src/plugin.json`;
  - the ported startup tests passing.
- [x] 4.4 Prove incremental streaming and cancellation through the in-process test host. Verify a test that delays provider parts and asserts each frame arrives before the next part is emitted, and a test where caller cancellation cancels the model context and drains within existing bounds.
- [x] 4.5 Remove the listener, HTTP-server timeout, JWT/JWKS, audience, unsafe and Cloud settings, and make the strict config file reject `auth` and `server` blocks. Keep `deployment.mode`, and fail production startup unless `GRAFANA_JWKS_URL` or `GRAFANA_JWKS` is set. In development mode with `GF_PLUGIN_INSECURE_SKIP_AUTHENTICATION=true`, build the Gateway authenticator on authlib's unsafe verifier; production refuses it. Delete JWT/JWKS, header normalization, Cloud assertion and static-key code, and the `outbound` JWKS client. Verify config tests for:
  - secret-safe unknown-field failures;
  - environment-only startup with no arguments;
  - the production signing-key guard and refusal of insecure skip, with development permitting the test host and the unsafe verifier;
  - unchanged ProviderWire, provider and Agent Observability setting validation.
- [x] 4.6 Add the plugin-process test binary. It builds a host-platform plugin executable with `go build`, launches it through a go-plugin client and `grpcplugin.NewClientV3`, and serves HTTP through the test host package. It exposes `CollectMetrics` and `CheckHealth` results, sending a `PluginContext` with the plugin ID and empty app instance settings, relays plugin logs and stops the plugin through Shutdown. It reads the access mode, fake providers and catalog from the environment like the current command fixtures. Verify:
  - a Go smoke test gets a healthy check, a fixed response and host-collected Gateway metrics from the built plugin;
  - plugin build output contains no test binary.
- [x] 4.7 Move the TypeScript ProviderWire suites that spawn `grafana-ai-gateway` to the test binary:
  - private-JWT cases become internal-service cases. Vercel private-JWT cases become raw HTTP or TS-helper cases with `X-Access-Token`, or self-hosted service-account cases;
  - Cloud cases become service-account BYOK in `cloud` mode;
  - keep the pinned `getAvailableModels` extension-stripping check on its fake-server test;
  - replace `/metrics` and `/ready` scraping with the test binary's metrics and health;
  - remove the dummy edge and static-key fixtures;
  - update the `test:command` script, the `test-ai-gateway-command` mise task and `test/conformance/UPGRADING.md`.

  Verify that `mise run test-ai-gateway-command` and `mise run test-ai-gateway-source-integration` pass.
- [x] 4.8 Prove that request content cannot change access, and attribution privacy. Verify tests that:
  - tenant headers are ignored;
  - configured attribution in `cloud` mode equals the direct caller's service identity;
  - the private caller value holds the identity actor's UID;
  - BYOK, relayed and `self-hosted` observations carry no caller service;
  - user and service-account UIDs, subjects, logins and emails never reach logs, metrics, exports or provider requests;
  - credential headers sent through the bare v3 route client reach no logs, metrics, exports, bodies or provider requests.
- [x] 4.9 Remove `NewWithCloudCredentials`, `CloudCredentialsConfig` and `WithUserIDToken` from `providers/grafana`, and shrink the reserved headers to `Authorization`, `X-Access-Token` and `X-Grafana-Id`. Move their remaining users to the platform flows: `providers/grafana/internal/capture/main.go`, `test/integration/testserver/grafana_logger_capture_test.go` and `ai-gateway/test/providerwire-v4/go-client-differential.test.ts`. Verify that `mise run test-grafana`, `mise run test-integration`, `mise run test-providerwire-v4` and `mise run test-ai-gateway-command` pass.
- [x] 4.10 Remove `ai-gateway/docs/cloud-authentication.md` and its links from `docs/guides/gateway-authentication.md` and `docs/guides/ai-gateway-container.md`. Rewrite `ai-gateway/README.md` as the operator overview, covering:
  - plugin hosting and the route prefix;
  - access modes: the `cloud` allowlist with service identities, on-behalf-of users, BYOK open to every stack user, and `self-hosted` catalog access for users and service accounts;
  - the self-hosted requirements today: the router toggle, plus a token-exchange service and signing keys until vanilla Grafana passes v3 identity; and one service account per application or person to attribute;
  - that self-hosted catalog access follows Grafana's plugin app access, which basic roles usually include, and how operators restrict it;
  - settings, and how hosts supply environment: `forward_host_env_vars` for Grafana, the router host environment;
  - the grace period above model duration plus drain;
  - current platform limitations: route deadline, platform errors outside the ProviderWire document.

  Verify that `mise run lint-docs` passes and the README's relative links resolve.

## 5. Host telemetry

- [x] 5.1 Add the `slog.Handler` bridge to the plugin SDK logger, with level mapping, dotted group flattening, `LogValuer` resolution and trace/span IDs from context, and use it for every Gateway logger. Verify:
  - unit tests on emitted hclog JSON for levels, flattened keys, trace IDs and records bounded below 64 KiB;
  - a plugin-process test that the plugin writes nothing outside the log protocol;
  - a plugin-process test that a request with a `traceparent` and host tracing configured produces a relayed request log carrying that trace ID, and no exporter without tracing configuration;
  - `mise run test-ai-gateway-command` passes.
- [x] 5.2 Route attempt records through the logger from the existing bounded worker, give `Close` a context deadline that abandons a stalled worker, and delete `physical_output.go`. Verify:
  - with the real hclog writer over a blocked pipe, enqueueing does not block, saturation drops are counted and `Close` returns by its deadline;
  - `mise run test-ai-gateway-command` passes.
- [x] 5.3 Remove the `/metrics` handler, `authentication_total`, the `ready` gauge and `SetReady`, and add the bounded access-decision counter. Verify:
  - tests gathering after unary and streaming calls;
  - access-decision labels limited to `configured`, `byok` and `refused`;
  - the existing duplicate-registration refusal still fails startup;
  - `mise run test-ai-gateway-command` passes.
- [x] 5.4 Give the Agent Observability and attempt-sink `Close` methods a context, and run them in parallel from `main` after `plugin.Run` returns under one 1 s context. Each step's deadline derives from that context, capped by its configured duration, and records emitted after close are dropped with counters. Verify:
  - a unit test that undeliverable telemetry is abandoned before the budget ends;
  - a config test for production startup with export enabled and default timeouts;
  - a plugin-process test where Shutdown leads to exit before the host's kill delay.
- [x] 5.5 Make Agent Observability export optional in every deployment mode, keeping production TLS and credential validation when it is enabled. Verify config tests for production without export and for production cleartext export failing.
- [x] 5.6 Update `ai-gateway/docs/text-observability.md` and the README telemetry section. Cover host log relay, host-collected metrics, tracing from the host, and the final-flush budget shared with tracer shutdown. Verify that `mise run lint-docs` passes and relative links resolve.

## 6. Client documentation and cross-language evidence

- [x] 6.1 Update the godoc and `docs/providers/grafana-gateway.md`, and rewrite `docs/guides/gateway-authentication.md`. Cover:
  - the route prefix and the two access modes;
  - internal token exchange with configured models, the `grafana-aigateway-app` audience, namespace, caller-owned token refresh and on-behalf-of calls, in Go;
  - service-account BYOK in `cloud` mode, stating that configured discovery is unavailable there, and the service-account catalog in `self-hosted` mode with one service account per application or person, in Go and pinned Vercel;
  - the platform-error caveat.

  Put the Go examples in `providers/grafana` example tests and the TypeScript example in the ProviderWire workspace. Verify that `mise run test-grafana`, `mise run test-providerwire-v4` and `mise run lint-docs` pass.
- [x] 6.2 Cover exact-client evidence through the test binary:
  - Go configured discovery, unary and streaming for internal callers in `cloud` mode, directly and on behalf of a user with both an ID-token and a delegated-access-token subject, with the token only in `X-Access-Token`;
  - Go and pinned Vercel service-account BYOK unary and streaming in `cloud` mode, plus unsupported discovery;
  - Go and pinned Vercel service-account configured discovery, unary and streaming in `self-hosted` mode;
  - refusal for relayed and anonymous callers;
  - high-level calls preserving SDK headers and output-token defaults;
  - sentinel credentials absent from bodies, provider requests and server sinks.

  Update the configured-discovery fixtures' header values. Verify that `mise run test-ai-gateway-command` and `mise run test-providerwire-v4` pass.
- [x] 6.3 Update the Gateway rows in `test/conformance/PARITY.md`: host composition, request-only accounts, execution overviews, runtime and Go client, and provider metadata. Replace command, dummy-edge, static-key and image evidence with test host evidence, the Vercel internal-caller gap and the documented platform boundaries. Verify that `mise run validate-parity-baseline` passes.

## 7. Packaging, CI and release

- [x] 7.1 Make `build-ai-gateway-plugin` package:
  - the AGPL `LICENSE` and `NOTICE` and the Apache SDK license;
  - a per-platform Go module inventory with each module's license and notice files, ported from the Dockerfile;
  - frontend dependency licenses;
  - source-revision attribution, marking local builds as unverified development builds.

  Verify that a local build contains them and that a CI-style build records the commit SHA.
- [x] 7.2 Extend `test/module-policy.sh`'s candidate-source overlay check to the mage-built plugin backend, and add a `validate-ai-gateway-plugin` mise task running the pinned plugin validator on the package. Verify that `mise run test-module-policy` and `mise run validate-ai-gateway-plugin` pass.
- [x] 7.3 Replace the `ci.yml` image validation, publish and deploy jobs with plugin validation jobs on pull requests and eligible pushes:
  - plugin build;
  - manifest regeneration diff;
  - plugin validator;
  - Gateway tests and test host suites.

  On eligible pushes, use a clean push-SHA checkout and keep the validated package as a workflow artifact. Rewrite `test/conformance/tools/ci-workflow.test.mts` for those jobs and their `needs`. Verify that `mise run test-ci-workflow` passes and that the PR run executes all plugin jobs.
- [x] 7.4 Delete:
  - `ai-gateway/Dockerfile`, `ai-gateway/Dockerfile.dockerignore` and `ai-gateway/.dockerignore`;
  - the `build-ai-gateway-image` and `test-ai-gateway-image-source` tasks and `gateway-image-source.test.mts`;
  - `docs/guides/ai-gateway-container.md`.

  Update image references in `CONTRIBUTING.md`, `release/README.md`, `AGENTS.md` and the docs navigation. Verify that:
  - `mise tasks` lists no image tasks;
  - `mise run lint-docs` passes;
  - `rg -i 'dockerfile|container image' CONTRIBUTING.md release AGENTS.md docs` finds no Gateway image references.
- [x] 7.5 Align the release-please `ai-gateway` component with the plugin package version source without enabling release automation. Verify that `mise run release-check` passes.

## 8. Integration verification

- [x] 8.1 Run the full checks and verify that all pass:
  - `mise run check`, `mise run build`, `mise run lint`;
  - `mise run parity-check`, `mise run test-ai-gateway`;
  - `mise run test-integration`, `mise run test-ci-workflow`, `mise run test-module-policy`;
  - `mise run verify-ai-gateway-boundary`, `mise run verify-sdk-gateway-isolation`;
  - `mise run validate-ai-gateway-plugin`;
  - `openspec validate migrate-ai-gateway-to-app-platform --strict`.
- [x] 8.2 Run the built plugin in a local Grafana nightly (create-plugin docker compose, app-sdk feature toggles including the router toggle, `forward_host_env_vars`, development mode with `insecure_skip_authentication`, so Grafana mints local tokens) in `self-hosted` mode. Exercise discovery and unary and streaming calls as a service account. Record the outcome in the PR description: either incremental responses with Gateway log records relayed at info level, or the exact blocker.

## Workflow follow-up

- Open the draft PR with breaking release intent for `providers/grafana` and the list of external follow-ups from design.md.
- Track the external App Platform and deployment follow-ups with their owners outside this repository.
- After dev deployment, validate the delegated identity shape (first-actor service identity, on-behalf-of chains, front-door user traffic), router credential-header stripping and the Vercel internal-caller headers before relying on them.
- Archive the change after review and verify the archived specs.
