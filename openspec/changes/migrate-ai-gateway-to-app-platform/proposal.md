# Proposal

## Why

The Gateway currently ships as a standalone service. It owns three listeners, JWT/JWKS verification, a trusted-proxy Cloud identity contract, static-key self-hosting, process lifecycle and a container image. Grafana App Platform can host it as an app plugin backend: the platform authenticates every caller, binds requests to a tenant namespace, delegates a verified identity to the plugin and provides deployment, routing and telemetry plumbing. The Gateway has no external clients yet, so it can move fully to that host now. There is no compatibility path to preserve, and every later feature (tenant catalogs, stored BYOK secrets, RBAC) builds on platform primitives instead of bespoke infrastructure.

## What Changes

- **BREAKING** Package the Gateway as a Grafana app plugin built from `ai-gateway/`. It has a routes-only app manifest that serves the existing ProviderWire discovery and language-model operations as namespaced routes under the plugin's API group.
- **BREAKING** Remove the standalone server:
  - the private JWT, Cloud and operational listeners;
  - JWT/JWKS verification and the `X-Scope-OrgID` trusted-proxy contract;
  - self-hosted static-key authentication, including the YAML `auth` and `server` blocks;
  - unsafe development verification;
  - signal-driven process lifecycle, readiness and liveness routes;
  - the container image and its packaging evidence.
- **BREAKING** Derive account access from the platform-verified caller under a required access mode.
  - `cloud`: a direct caller whose signed service identity is on a configured allowlist gets configured accounts and the catalog, including when it calls on behalf of a user. Other users and service accounts get request-scoped BYOK only.
  - `self-hosted`: users and service accounts get the catalog, and BYOK is refused.
  - Every other identity is refused. The verified user or service account is kept on each request for future usage attribution, without being emitted.

  Listener placement no longer decides policy.
- Route Gateway logs, fallback-attempt records, metrics and traces through the plugin host's log protocol, metrics collection and tracing configuration. This replaces stderr JSON, socket/pipe deadline output and the `/metrics` listener.
- Adapt shutdown to the host: the host drains in-flight streams, and the plugin performs a short, bounded final telemetry flush after the host stops it.
- **BREAKING** Update the Go Grafana Gateway client:
  - remove the Cloud stack/CAP constructor and acting-user header propagation;
  - require an explicit token-exchange audience;
  - add on-behalf-of user calls through token exchange;
  - add a service-account bearer flow for Cloud BYOK and the self-hosted catalog.
- Replace spawned-binary command tests with a protocol test host. It runs the built plugin executable through the real plugin protocol with test-signed tokens. It keeps exact-pinned Go and Vercel evidence for service accounts in both modes and Go evidence for internal callers.
- Replace image validation, publication and deployment with plugin build, licensing and validation checks that keep the validated package as a CI artifact; plugin publication is a follow-up. Rewrite the Gateway and client documentation for platform hosting.
- Track deployment and App Platform follow-ups outside this repository, such as route timeout configuration, per-request load balancing, plugin crash recovery, metrics export and deployment wiring. This change records them as dependencies rather than implementing them.

## Capabilities

### New Capabilities
- `gateway-plugin-host`: Gateway runtime as an App Platform plugin backend. Covers the manifest routes, request dispatch to ProviderWire, configuration, health, host-owned lifecycle with bounded final flush, and the protocol test host.
- `gateway-caller-access`: How the access mode, the platform-verified actor chain and the route namespace map to configured-account or request-BYOK access, and which identities are refused.
- `gateway-plugin-telemetry`: Gateway logs, fallback-attempt records, metrics and traces emitted through the plugin host's logging, metrics and tracing contracts.
- `gateway-plugin-packaging`: Building, licensing and validating the Gateway plugin package from `ai-gateway/` with same-revision workspace source.

### Modified Capabilities
- `ai-gateway-cloud-authentication`: All requirements removed. The listener, trusted-proxy and JWT contracts are superseded by `gateway-caller-access` and `gateway-plugin-host`.
- `gateway-self-hosted-auth`: All requirements removed. Self-hosting becomes running the plugin in a self-hosted Grafana in `self-hosted` access mode.
- `gateway-workspace-container`: All requirements removed. Superseded by `gateway-plugin-packaging`.
- `gateway-request-byok`: BYOK destination policy applies to platform-verified users and service accounts in `cloud` mode rather than Cloud-listener callers.
- `gateway-configured-discovery`: Discovery authorization and the BYOK unsupported response use caller access, the access mode and the plugin route instead of the Cloud listener and `/api/v1/aisdk/config`.
- `gateway-text-observability`: Model metrics are exported through the plugin host instead of `/metrics`. Exporter shutdown follows the host stop sequence with a short final-flush budget. Agent Observability export becomes optional in every deployment mode.
- `gateway-anthropic-mcp`, `gateway-ordered-text-fallback`, `gateway-provider-configuration`: Evidence, startup, readiness and discovery-route wording follows the protocol test host, plugin startup, plugin health and plugin `config` route.
- `grafana-gateway-client`: Constructor, authentication-flow, reserved-header, discovery and evidence requirements change for platform-hosted routes and identities.
- `providerwire-v4-http-contract`: The Gateway release artifact becomes the plugin package. The integration workspace serves plugin backend builds.
- `module-validation-modes`: The explicit Gateway workspace selects plugin backend build dependencies instead of container dependencies.
- `candidate-source-ci`: Gateway publication is gated on plugin build and validation instead of image build and smoke.
- `multi-module-release-management`: Gateway application releases validate the plugin package instead of images.
- `upstream-parity-governance`: The Gateway becomes a plugin-only release component with plugin validation evidence.
- `gateway-sources`, `sdk-provider-tools`: Image evidence and publication wording becomes plugin package wording.

## Impact

- **Code:**
  - `ai-gateway/cmd/grafana-ai-gateway`: removes the listener and signal code in `process`, the JWT/JWKS, Cloud and static-key code in `auth`, the JWKS client in `outbound`, and the operational router. Startup composition moves into the plugin app. Settings change, telemetry is reworked and the attempt-sink output is replaced.
  - New plugin scaffolding in `ai-gateway/`: `plugin.json`, a minimal frontend, the CUE manifest, generated manifest data and the build configuration.
  - `providers/grafana`: constructors and documentation change.
- **Unchanged:** `ai-gateway/providerwire/v4`, `ai-gateway/catalog`, BYOK, native model and option code, and ProviderWire wire compatibility with `@ai-sdk/gateway`.
- **Dependencies:** the Gateway module adds `grafana-app-sdk` (plugin, app and simple packages) and `grafana-plugin-sdk-go`, plus their transitive Kubernetes API machinery. The Gateway gains a Node toolchain for plugin packaging. The SDK modules stay independent of the Gateway.
- **Tests and CI:** spawned-command TypeScript suites move to the protocol test host; the image tasks, validation, publication and deployment are replaced by plugin build and validation, with the validated package kept as a CI artifact. The Gateway workspace moves to Go 1.26.5.
- **Docs:**
  - `ai-gateway/README.md`, `docs/guides/gateway-authentication.md` and `docs/providers/grafana-gateway.md` are rewritten.
  - `ai-gateway/docs/text-observability.md` is updated.
  - `docs/guides/ai-gateway-container.md` and `ai-gateway/docs/cloud-authentication.md` are removed.
- **External dependencies (tracked outside this repository):**
  - configurable route timeouts beyond 60s;
  - per-request gRPC load balancing;
  - plugin client recovery after crashes;
  - plugin metrics export;
  - verified caller identity on v3 in self-hosted Grafana without a token-exchange service;
  - tracing and JWKS environment variables for the plugin;
  - a termination grace period;
  - autoscaling;
  - manifest version pinning;
  - provider egress and secret injection.
