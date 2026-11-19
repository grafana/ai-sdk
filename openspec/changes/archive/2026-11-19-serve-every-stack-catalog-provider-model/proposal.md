# Proposal

## Why

A stack catalog can connect a provider before adding models, but writers must still list each model and keep the list current. Let a writer opt an account into serving unlisted model IDs without giving up the ability to block specific IDs.

## What Changes

- Add optional `serveAllModels` and `modelBlocklist` fields to each `ModelCatalog` provider. The default is still listed models only.
- Resolve listed IDs and aliases first. Otherwise, accept `<provider-instance>/<model-id>` only for an enabled instance and only if its exact backend ID is not blocked. Blocked selectors return the existing unknown-model error.
- Refuse malformed blocklists and entries that match a configured primary or fallback target of that provider instance. Keep discovery limited to listed models; provider inventory remains an explicit admin action.
- Put a **Serve every model** switch and a **Blocked model IDs** field together on each saved provider card, with an explicit save for both settings.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-model-catalog-kind`: Add provider serving settings and validate them on catalog writes.
- `gateway-stack-catalog-runtime`: Resolve qualified unlisted IDs with a single candidate, retaining existing routing controls and errors.
- `gateway-configured-discovery`: Keep stack catalog discovery explicitly limited to configured entries, even when unlisted IDs are callable.
- `gateway-catalog-management`: Display and edit the settings on the provider card through the existing versioned update.

## Impact

Only stored Grafana stack catalogs change: their additive `v1alpha1` CUE schema and generated artifacts, shared catalog preflight, catalog route selection, and the catalog editor. The operator catalog, namespace routes, BYOK, provider adapters, and existing wire formats and permissions are unchanged. Document the setting and selector under **Store model catalogs in Grafana** in the Gateway README.
