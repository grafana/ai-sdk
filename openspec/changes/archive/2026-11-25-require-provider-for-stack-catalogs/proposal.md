## Why

A catalog with no provider cannot be used or checked. A writer should be able to connect one account before choosing which models to expose.

## What Changes

- Require at least one keyed provider on every stack catalog write. Allow no models and keep the same discovery and model-not-found responses.
- Create with the catalog name and shared provider fields, then check the saved account once on arrival. Disable removal of the last provider, not the last model.
- Keep operator validation and endpoint policy unchanged.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-model-catalog-kind`: keep the provider minimum on stack writes.
- `gateway-catalog-management`: connect the first provider during creation and check it after save.
- `gateway-stack-catalog-runtime`: a catalog without providers is invalid.
- `gateway-configured-discovery`: list no models when none are configured.

## Impact

Grafana stack management only. The registered upstream reference is `vercel/ai@0e8e0543aeda7d4dd4ce6eade0d5b7d5a43644c1` (`@ai-sdk/gateway@4.0.146`); its `/config` client still accepts an empty model array. No SDK or wire changes.
