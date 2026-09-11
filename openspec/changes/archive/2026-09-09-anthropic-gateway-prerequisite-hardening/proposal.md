## Why

Later authenticated Gateway service composition needs two independently useful security prerequisites: host-level failures must reuse fixed ProviderWire documents without accepting private error data, and direct Anthropic construction must not allow ambient SDK environment configuration to override explicit provider configuration.

## What Changes

- Add a narrow host-safe ProviderWire error writer for the package-owned authentication, permission, and internal documents, with invalid categories falling back to the fixed internal document.
- **BREAKING** Correct direct `providers/anthropic.New` construction to disable all Anthropic SDK environment defaults before applying the explicit API key, so ambient base URL, auth token, profile/federation, and custom-header variables cannot override or augment provider configuration.
- Add focused API and poisoned-environment tests for both behaviors.

## Capabilities

### New Capabilities

- `anthropic-provider-construction`: Explicit direct-Anthropic client construction that disables ambient Anthropic SDK environment defaults before applying provider-owned credentials and request options.

### Modified Capabilities

- `providerwire-v4-unary-runtime`: Add a narrow host-safe error writer that reuses package-owned fixed authentication, permission, and internal documents so host authentication and discovery failures do not duplicate protocol bytes or expose caller-controlled error data.

## Impact

- `providers/anthropic.New` keeps its API but no longer consumes ambient Anthropic SDK environment defaults on the direct path; Vertex construction remains separate.
- `ai-gateway/providerwire/v4` gains a closed, non-fallible host-composition writer without introducing service dependencies.
- Runnable service configuration, authentication, routing, discovery, telemetry, lifecycle, and process integration remain deferred to the later authenticated-service change.
