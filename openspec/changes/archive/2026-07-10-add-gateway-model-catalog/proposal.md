## Why

Hosted gateways need a finite public model namespace that can expose one canonical model-family/version ID across provider-specific invocation IDs and composed fallbacks, while supporting discovery and request-scoped policy. `registry.ProviderRegistry` instead routes composite `provider:model` construction IDs, and `CustomProvider` supplies flat lookup without canonical alias identity, listing metadata, or request-aware contracts. Extending either registry abstraction with the full catalog responsibility would weaken its upstream-aligned contract.

## What Changes

- Add a focused `gateway/catalog` package with request-aware model resolution and listing contracts, leaving `gateway` as the namespace for independent gateway surfaces.
- Add an immutable static catalog for flat public model IDs, canonical resolution, explicit aliases, stable listing metadata, and structured unknown-model errors.
- Add an adapter that maps public catalog IDs to opaque IDs resolved through any `registry.Provider`, without changing registry behavior.
- Define minimal model metadata for future gateway discovery APIs, including canonical IDs, optional presentation fields, aliases, and typed capabilities.
- Keep public names and provider-specific ID translation as explicit host configuration; ship no built-in model names or mappings.
- Keep Assistant-specific model families, `chat-large`/`chat-small` profiles, Claude alias policy, entitlements, credentials, provider construction, and fallback policy outside ai-sdk.
- Document the catalog as a public gateway namespace distinct from provider construction and transport framing.
- Document landing-order-independent composition with the separately proposed `gateway/providerwire` HTTP server through a host-owned adapter, including non-retryable 404 mapping for unknown public models.

## Capabilities

### New Capabilities

- `gateway-model-catalog`: Request-aware resolution and listing of a finite public model namespace, including static entries, aliases, metadata, unknown-model errors, and registry-provider adaptation.

### Modified Capabilities

None.

## Impact

- Adds a public `github.com/grafana/ai-sdk/gateway/catalog` package and exported API.
- Adds focused unit tests and godoc for the new package.
- Adds a gateway catalog guide and links it from the documentation index and registry guide.
- Imports existing `provider` and `registry` packages; adds no external dependency.
- Coexists with the separately proposed `gateway/providerwire` package without either package importing the other; hosts adapt the transport-neutral catalog resolver to the HTTP resolver boundary.
- Does not change provider-wire bytes, provider requests, streaming behavior, UI chunks, `registry.Provider`, or `registry.ProviderRegistry` semantics.
