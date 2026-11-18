# Proposal

## Why

Catalog writers type provider model IDs by hand. A typo or retired ID can be saved and fail only at call time. The catalog health check already has a bounded way to ask the provider what the saved account can list.

## What Changes

- Add a searchable list to **Add model** on each saved provider card. Selecting a listed ID fills the field; writers can always enter an ID that the provider does not list.
- Recommend **(a), saved providers only**. The create flow still uses manual IDs for its initial models. Once saved, writers can browse for additional models. No new request accepts draft credentials, and neither creating nor saving a catalog triggers a listing.
- Add one writer-gated `available-models` action on the parent catalog. It uses only the selected named provider's saved key, reuses the health check's listing implementation and bounds, and briefly caches successful results per catalog revision and provider.
- Support Anthropic and OpenAI on this base. Custom endpoints and `openai-compatible` remain refused by the shared preflight until the operator allowlist is available.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-catalog-management`: searchable suggestions and free-form IDs for adding models, with manual-only creation.
- `gateway-stack-catalog-runtime`: bounded, secret-safe inventory for one saved provider, with revision-scoped caching.
- `gateway-model-catalog-kind`: grant the new subresource permission to catalog writers only by default.
- `gateway-plugin-host`: declare and serve one more catalog-scoped action.

## Impact

Stacks on #854, which is stacked on #779. Changes the catalog UI, Gateway route and manifests, and shared health listing internals. Does not change the stored spec, admission, health response, discovery or inference APIs. No new provider client or dependency.

**Not parity-sensitive.** The registered Vercel `@ai-sdk/gateway` 4.0.146 implementation at `0e8e0543` reads Gateway `/config` in `getAvailableModels()`. This Grafana catalog administration action lists an upstream account's models using its saved key, not SDK discovery metadata. No change to ProviderWire, provider adapters, SSE, React hooks or parity evidence.
