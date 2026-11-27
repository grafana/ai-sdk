# Proposal

## Why

A stack catalog can connect a provider before adding models, but writers must still list each model and keep the list current. Let a writer opt an account into serving unlisted model IDs without giving up the ability to block specific IDs.

## What Changes

- Add optional `serveAllModels` and `modelBlocklist` fields to each `ModelCatalog` provider. Omitting them still means listed-only serving. The app enables unlisted serving for newly connected providers only, including the first provider on catalog creation.
- Allow versioned and qualified public IDs up to 320 ASCII bytes, including `@` and `+` after the first alphanumeric character.
- Resolve listed IDs and aliases first. Otherwise, accept `<provider-instance>/<model-id>` only for an enabled instance and only if its exact backend ID is not blocked. Blocked selectors return the existing unknown-model error.
- Refuse malformed blocklists and entries that match a configured primary or fallback target of that provider instance. Keep configured discovery rows and add eligible qualified IDs from opted-in providers using bounded, best-effort inventory. Give discovery separate admission and a 30-second revision-scoped cache; writer listing actions stay live.
- Put a **Serve every model** switch and a **Blocked model IDs** editor on each saved provider card. Save toggles and blocklist additions and removals immediately. In listed-only mode retain saved blocked IDs and offer removal only.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-model-catalog-kind`: Add provider serving settings and validate them on catalog writes.
- `gateway-stack-catalog-runtime`: Resolve qualified unlisted IDs with a single candidate, retaining existing routing controls and errors.
- `gateway-configured-discovery`: Add eligible qualified selectors without hiding configured rows when provider inventory is unavailable.
- `gateway-catalog-management`: Display and edit the settings on the provider card through the existing versioned update.
- `gateway-ordered-text-fallback`, `gateway-provider-configuration`, and `gateway-resource-policy`: Use the widened public-ID contract.

## Impact

Only stored Grafana stack catalogs change: their additive `v1alpha1` CUE schema and generated artifacts, shared catalog preflight, catalog route selection, and the catalog editor. Operator model IDs and aliases accept the widened grammar; operator serving policy and BYOK are unchanged. The discovery schema widens accepted IDs without changing its shape or permissions. Document the setting and selector under **Store model catalogs in Grafana** in the Gateway README.
