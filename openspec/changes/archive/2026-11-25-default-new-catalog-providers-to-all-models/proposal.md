## Why

A catalog created in the app can already connect a provider, but it cannot serve a model until the writer adds a model or enables unlisted serving. A newly connected provider should be usable immediately.

## What Changes

- In both catalog creation and **Connect a provider**, tell writers that the new provider will serve every model and save it with `serveAllModels: true`.
- Leave existing providers, API defaults, discovery, and routing rules unchanged.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-catalog-management`: Default newly connected providers to **Serve every model** in the app.

## Impact

The shared provider form, Gateway README, and catalog management tests change. The kind and wire protocols do not change.
