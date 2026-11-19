# Proposal

## Why

A writer should be able to reserve a catalog URL, connect a provider account, and add models as separate saves. Stack catalogs currently inherit the operator catalog's minimum provider and model counts, so the app has to collect all three before it can create a catalog. The same minimum prevents a writer from removing the last model.

## What Changes

- Allow zero providers and zero models in a stack catalog. Keep the key requirement for every declared provider and all validation for model targets that are present.
- A valid catalog with no models serves an empty discovery list and returns the ordinary unknown-model error on invocation. A malformed stored catalog still fails its configuration checks.
- Create with a name alone, then open the saved catalog's page. Allow removing the last model and removing a provider together with its primary models and key, unless a model that would remain falls back to it.
- Check a connected provider's listing on demand even before models are configured. An accepted health check of a catalog without providers returns no results; an available-models request for an absent provider remains a not-found error.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-model-catalog-kind`: stack catalogs can have empty provider and model maps.
- `gateway-plugin-host`: distinguish an invalid route parent spec from a valid empty catalog.
- `gateway-configured-discovery`: return an empty model array for a catalog that serves no models.
- `gateway-stack-catalog-runtime`: describe selection, health, and provider inventory for these states.
- `gateway-catalog-management`: name-only create, independent provider and model saves, and removal back to an empty catalog.

## Impact

- Keep the resource schema, routes, permissions, provider policy, and secure storage contract. No change to the operator catalog, namespace routes, or BYOK.
- Keep the saved provider card as the place to configure how a provider serves models; no automatic serving, blocklist, or new provider fields in this change.
- Document empty catalogs under **Store model catalogs in Grafana**. Verify storage and the app flow against Grafana in CI.
