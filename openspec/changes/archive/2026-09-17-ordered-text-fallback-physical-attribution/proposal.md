## Why

The current fallback stream peek treats a leading provider `PartError` as a reason to change candidates and treats premature EOF as a successful empty stream. That conflicts with the strict Gateway lifecycle: every accepted provider part is protocol-visible commitment, while only retryable setup failures and EOF before any part may select the next configured backend.

WP9 must establish this boundary before internal activation so operators can use ordered text fallback without mixing providers in one response, leaking private topology to clients, or leaving Gateway-owned bridge goroutines behind.

## What Changes

- **BREAKING**: correct `fallback.Model.DoStream` so a leading provider `PartError` commits and is replayed like every other part; it no longer triggers candidate selection.
- Treat a candidate stream that closes before its first part as a retry-eligible pre-commit failure, subject to the same decider and request context as setup errors.
- Make the successful stream bridge and abandoned-candidate cleanup context-aware and bounded from the Gateway's perspective, preserving order without allowing an uncooperative producer to retain Gateway-owned goroutines indefinitely.
- Extend reusable fallback attempt hooks with closed decision outcomes and retry/selection facts needed for private physical attribution, without importing Gateway configuration or observability packages.
- Extend strict Gateway YAML with multiple named Anthropic instances and ordered direct candidate references, rejecting missing references, duplicate candidates, cycles/recursive routes, and empty routes at startup.
- Compose one fallback model beneath the WP8 logical middleware chain, consume the service-local correlation accessor over WP8's immutable observation snapshot, and project attempt facts into a WP9-owned bounded private physical sink. Public discovery, responses, errors, logs, metrics, and logical Agent Observability continue to use only canonical public identity.
- Keep effectful tool fallback, request-selected topology, provider expansion, image capacity, production rollout, and client protocol changes out of this work package.

## Capabilities

### New Capabilities

- `gateway-ordered-text-fallback`: Host-owned ordered route construction, strict startup configuration, immutable Anthropic candidate construction, authenticated text execution, fallback composition, private physical attribution, and public-topology privacy. These requirements are ADDED here because the integrated baseline has no canonical authenticated-Anthropic-service capability to modify.

### Modified Capabilities

- `fallback-stream-error`: Replace leading-error fallback with a protocol commitment rule, make premature EOF a pre-commit failure, and bound bridge/cleanup ownership.

## Impact

- Root Apache module: `fallback/` public observer data and stream behavior, focused unit/race tests, and fallback documentation.
- AGPL Gateway module: strict configuration validation, catalog construction, middleware composition, private attempt telemetry, privacy tests, and deterministic service integration tests.
- Existing users that relied on a leading `PartError` switching candidates receive the provider error stream instead; this is an intentional safety correction needed to prevent mixed protocol output and future duplicated effects.
- The registered Vercel baseline remains `@ai-sdk/gateway` 4.0.52 / `ai` 7.0.65 at commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`. WP9 changes no client request, response, SSE, or discovery schema; provider-independent tests prove the fallback boundary, while existing strict V4 fixtures prove the unchanged public wire.
- Depends on the completed WP5 service interfaces and on WP8's logical telemetry plus narrow correlation-accessor seam. WP9 owns the physical record, projection, queue/sink, and drop accounting. WP6 retains image/capacity ownership, WP7 requires no fallback-specific client API, and WP10 owns production configuration and rollout evidence.
