# Proposal

## Why

Catalog admission can validate the shape of a `ModelCatalog`, but it cannot tell an administrator that a saved provider rejected a key or that a configured model ID was not found. The [catalog page review](https://github.com/grafana/ai-sdk/pull/779#issuecomment-6981760226) exposed this gap: a catalog can save successfully and still fail on its first application call. This proposal addresses [#836](https://github.com/grafana/ai-sdk/issues/836) with an explicit check after save, not by making save depend on a provider.

## What Changes

- Add a bodyless `POST …/modelcatalogs/{name}/health` action authorized as `get` on `modelcatalogs/health`, granted to `aigateway:catalogwriter` only among the default catalog roles.
- Use the stored catalog and the same key, shape and endpoint policy as selection. Probe model-listing APIs, not inference. Return fixed outcomes for each provider and each model's primary and fallback targets; do not return upstream data or errors.
- On the saved catalog page, add a permission-gated **Check health** button and flag only outcomes that need attention. Beside **Providers**, show the problem count or a neutral confirmation instead of per-row success badges.
- Keep draft checks, save-time probes, background polling, model pickers, operator-catalog health and end-to-end inference checks out of scope.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-stack-catalog-runtime`: define stored-catalog checks, outcomes and egress bounds.
- `gateway-plugin-host`: declare and register the additional catalog subresource route.
- `gateway-model-catalog-kind`: grant the subresource action to catalog writers, independently of invocation and ordinary reads.
- `gateway-catalog-management`: display on-demand health results for the saved catalog.

## Impact

- Gateway route table, generated manifests, stack-catalog service, and the stored catalog page and its tests. No change to the kind schema, credential storage, discovery or model invocation.
- The [catalog page PR](https://github.com/grafana/ai-sdk/pull/779) supplies the UI. The [custom endpoints PR](https://github.com/grafana/ai-sdk/pull/778) is not on this base: that base's shared check refuses overrides and `openai-compatible`. Once the allowlist lands, use the same selection inputs and normalized endpoints; do not add a separate health allowlist.
- This is a Gateway administration action, not a Vercel SDK or ProviderWire surface. No change to conformance fixtures or the parity coverage map.
