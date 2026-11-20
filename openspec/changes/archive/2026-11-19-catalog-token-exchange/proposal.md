## Why

A Grafana app backend can exchange a credential on behalf of a signed-in user, but the Go client can only route that token to the namespace-level Gateway endpoints. A stored stack catalog has its own URL under the namespace, and the token-exchange constructor intentionally rejects namespaced base URLs. Today, callers have to use a service account or manage access tokens themselves to use a stack catalog.

## What Changes

- Add `grafana.WithCatalog(ctx, name)` to select a stored `ModelCatalog` on a token-exchange call. It composes with `WithNamespace` and `WithOnBehalfOf` and applies to discovery, unary generation, and streaming.
- Build catalog URLs under the same resolved stack namespace used for a concrete token exchange. Validate path-segment safety before exchange or Gateway I/O; Grafana validates resource names. Keep the API-root requirement, namespace rules, and credential handling.
- Document catalog selection alongside the existing service-account and caller-managed access token paths.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `grafana-gateway-client`: Add per-call catalog selection to the existing client routing and authentication contract.

## Impact

This is an additive, non-breaking feature of the independently released `providers/grafana` module. The PR title is `feat(grafana): call stack-catalog routes with token exchange`, with no breaking marker or migration. Existing constructors, base URL contracts, and requests without the option behave as before. No Gateway route, access policy, ProviderWire payload, TypeScript client, or core SDK API changes.
