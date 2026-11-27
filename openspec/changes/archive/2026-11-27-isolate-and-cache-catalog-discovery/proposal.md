## Why

Catalog discovery should not contend with a writer's health check or model picker. Repeated discovery can reuse a recent provider listing.

## What Changes

- Give discovery independent listing admission.
- Cache successful or partial discovery inventory for 30 seconds per catalog revision and provider. Writer actions remain live.

## Capabilities

### Modified Capabilities

- `gateway-configured-discovery`: Separate admission and bounded, revision-scoped inventory caching.

## Impact

Catalog discovery can show up to 30 seconds of provider-side inventory staleness. The wire format and invocation policy are unchanged.
