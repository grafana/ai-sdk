## Why

The authenticated Gateway created by work package 5 is usable from the registered TypeScript client but Go applications have no supported provider-shaped client after the legacy Go-to-Go ProviderWire module was retired. Reintroducing `providers/grafana` now gives Go callers the same strict ProviderWire V4 text path without reviving the legacy codec or coupling the Apache SDK to the AGPL Gateway product.

## What Changes

- Add `providers/grafana` as a separately versioned Apache-2.0 Go module and the only public Go client for Grafana AI Gateway.
- Implement `registry.Provider` and `provider.LanguageModel` over the strict `/api/v1/aisdk/config` and `/api/v1/aisdk/language-model` endpoints, explicitly projecting the currently executable text/scalar subset of `provider.CallOptions` to ProviderWire V4.
- Match the observable request and response behavior of registered `@ai-sdk/gateway@4.0.52`: semantic body presence, fixed protocol headers, client-owned unary metadata, stream filtering and timestamp conversion, optional `[DONE]`, finish followed by clean EOF, cancellation, Gateway error categories, and retryability.
- Restore the still-required Grafana adaptations from the retired client: Cloud Access Policy token exchange, direct short-lived access tokens, optional acting-user identity, configured base URL, and injectable HTTP transport.
- Bound discovery, unary, error, and incremental SSE processing, and reject unsupported or malformed input/output without exposing credentials, server internals, or partially decoded results.
- Add focused differential tests against the exact registered TypeScript client plus authenticated black-box tests against the work-package-5 command.
- Do not add a public low-level ProviderWire package, legacy codec/mode, server-side fallback, observability controls, image/deployment behavior, or an SDK dependency on `ai-gateway`.

## Capabilities

### New Capabilities

- `grafana-gateway-client`: Provider construction, authentication, discovery, strict ProviderWire V4 text calls, bounded result consumption, error behavior, and exact-pinned differential evidence for the single public Go Gateway client.

### Modified Capabilities

- `provider-wire`: preserve retirement of the tolerant unversioned server and client behavior while allowing the independently implemented strict ProviderWire V4 client at `providers/grafana`.

## Impact

- **Public API:** restores `github.com/grafana/ai-sdk/providers/grafana` with cloud-token and access-token constructors, model discovery, registry integration, and an acting-user context helper.
- **Dependencies:** the separate client module depends on the root Apache SDK and Grafana authlib, but must have no source or module dependency on `github.com/grafana/ai-sdk/ai-gateway`; the root SDK remains independently buildable when `ai-gateway/` is absent.
- **Protocol evidence:** extends the exact-pinned ProviderWire contract workspace and parity map for Go-versus-Vercel semantic request/result comparisons without making the Go client or server validator a second protocol authority.
- **Testing:** adds client-local fake-server tests and repository black-box tests against the authenticated Gateway command. Work package 10 remains responsible for deployed-image smoke and rollout/rollback evidence.
