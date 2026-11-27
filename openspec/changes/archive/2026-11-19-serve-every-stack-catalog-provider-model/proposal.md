# Proposal

## Why

A stack catalog can connect a provider before adding models, but writers must still list each model and keep the list current. Let a writer opt an account into serving unlisted model IDs without giving up the ability to block specific IDs.

## What Changes

- Add optional `serveAllModels` and `modelBlocklist` fields to each `ModelCatalog` provider. Omitting them still means listed-only serving. The app enables unlisted serving for newly connected providers only, including the first provider on catalog creation.
- Resolve listed IDs and aliases first. Otherwise, accept `<provider-instance>/<model-id>` only for an enabled instance and only if its exact backend ID is not blocked. Blocked selectors return the existing unknown-model error.
- Refuse malformed blocklists and entries that match a configured primary or fallback target of that provider instance. Keep configured discovery rows and add eligible qualified IDs from opted-in providers using bounded, best-effort inventory. Give discovery separate admission and a 30-second revision-scoped cache; writer listing actions stay live.
- Put a **Serve every model** switch and a **Blocked model IDs** editor on each saved provider card. Save toggles and blocklist additions and removals immediately. Hide the editor in listed-only mode without clearing saved blocked IDs.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-model-catalog-kind`: Add provider serving settings and validate them on catalog writes.
- `gateway-stack-catalog-runtime`: Resolve qualified unlisted IDs with a single candidate, retaining existing routing controls and errors.
- `gateway-configured-discovery`: Add eligible qualified selectors without hiding configured rows when provider inventory is unavailable.
- `gateway-catalog-management`: Display and edit the settings on the provider card through the existing versioned update.

## Impact

Only stored Grafana stack catalogs change: their additive `v1alpha1` CUE schema and generated artifacts, shared catalog preflight, catalog route selection, and the catalog editor. The operator catalog, namespace routes, BYOK, provider adapters, and existing wire formats and permissions are unchanged. Document the setting and selector under **Store model catalogs in Grafana** in the Gateway README.
