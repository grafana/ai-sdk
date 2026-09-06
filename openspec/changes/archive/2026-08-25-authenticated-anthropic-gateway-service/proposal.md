## Why

The strict ProviderWire V4 runtimes are transport components rather than a runnable internal service: callers still cannot authenticate, discover configured public models, or invoke Anthropic through the required `/api/v1/aisdk` routes. Phase 5 adds the first bounded service composition so direct Anthropic text generation can be exercised as an authenticated process before image and production-deployment work begins.

## What Changes

- Add a runnable AI Gateway service with strict startup configuration, one bounded HTTP listener, graceful shutdown, `/live`, `/ready`, `/metrics`, authenticated `/api/v1/aisdk/config`, and authenticated `/api/v1/aisdk/language-model` routes. The authoritative delivery plan now includes `/metrics` as the fifth operational route so phase 5 health metrics are scrapeable.
- Integrate Grafana authlib verification for exactly one `X-Access-Token` with exactly one verified service identity and namespace plus an optional single `X-Grafana-Id` acting-user identity; default the accepted audience to `ai-sdk`, support bounded JWKS verification in production, and provide an explicit unsafe local-development verifier that production mode rejects.
- Construct named direct Anthropic provider instances from environment-backed API-key references and optional base URLs, then construct immutable canonical public models and aliases once at startup.
- **BREAKING** Correct direct `providers/anthropic.New` construction to disable all Anthropic SDK environment defaults before applying the explicit API key, so ambient base URL, auth token, profile/federation, and custom-header variables cannot override or augment provider configuration.
- Harden JWKS and Anthropic outbound HTTP with credential-free endpoint validation, deployment-mode URL policy, redirect rejection, exact connection bounds, timeouts, decompressed response bounds, bounded JWKS key snapshots and refresh cadence, joinable in-flight refreshes, and amplification-resistant unknown-key handling.
- Return schema-valid, byte-bounded discovery rows for header-safe canonical IDs and aliases without exposing provider instance names, backend model IDs, credentials, base URLs, or routing details.
- Add flush-preserving process and HTTP lifecycle logging plus bounded-cardinality service health metrics that exclude credentials, request bodies, provider response bodies, and private backend identity.
- Build and spawn the real gateway command in authenticated integration tests against a fake Anthropic endpoint, separately covering normal finish, client abort, process shutdown, authentication ordering, readiness, startup failure, and bounded shutdown.

## Capabilities

### New Capabilities
- `authenticated-anthropic-gateway-service`: Runnable service configuration and lifecycle, Grafana authentication, private model construction, public discovery, health/telemetry, and authenticated direct-Anthropic ProviderWire V4 execution.
- `anthropic-provider-construction`: Explicit direct-Anthropic client construction that disables ambient Anthropic SDK environment defaults before applying provider-owned credentials and request options.

### Modified Capabilities

- `providerwire-v4-unary-runtime`: Add a narrow host-safe error writer that reuses package-owned fixed authentication, permission, and internal documents so authentication and discovery failures do not duplicate protocol bytes or expose caller-controlled error data.

## Impact

- Primary code: a service command and internal packages under the existing isolated AGPL `ai-gateway` module for configuration, authentication, routing, discovery, lifecycle, and health telemetry.
- Existing components: `ai-gateway/catalog` and `ai-gateway/providerwire/v4` retain their protocol/catalog semantics; `providers/anthropic.New` keeps its API but no longer consumes ambient Anthropic SDK environment defaults.
- Dependencies: Grafana authlib, Kingpin, strict YAML decoding, Prometheus instrumentation, and the pinned Anthropic provider module are confined to `ai-gateway`; service-owned bounded HTTP transports wrap authlib and Anthropic, while the root module graph and workspace remain unchanged.
- Tests and automation: service build/test tasks, hostile outbound transport tests, real-command fake-Anthropic process tests, authenticated registered-Gateway-client integration, parity baseline validation, and ProviderWire checks.
- Deferred scope: Docker/image packaging, production deployment manifests, reusable Go V4 client, `providers/grafana`, generation observability middleware, fallback, and non-Anthropic providers.
