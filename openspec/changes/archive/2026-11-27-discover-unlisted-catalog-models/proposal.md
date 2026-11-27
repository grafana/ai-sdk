## Why

A caller can invoke an unlisted model but cannot discover its selector. The catalog editor also offers blocklist editing when the blocklist has no effect.

## What Changes

- Add bounded provider inventory to stack catalog discovery for providers that serve unlisted models.
- Hide the blocklist controls in listed-only mode, without clearing saved IDs.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-configured-discovery`: Discover eligible qualified selectors.
- `gateway-catalog-management`: Edit blocked IDs only in serve-every mode.

## Impact

Catalog discovery can make read-only provider requests. No kind, wire shape, or permission changes.
