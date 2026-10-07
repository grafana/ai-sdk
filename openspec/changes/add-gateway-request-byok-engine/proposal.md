## Why

Request-only credentials cannot use a startup catalog or its configured accounts.
A separately testable execution engine and catalog-independent ProviderWire
selection boundary are needed before the service exposes a BYOK endpoint.

## What Changes

- **BREAKING:** replace ProviderWire's catalog resolver dependency with a host selector.
- Start one execution deadline before selection and retain it through invocation and streaming.
- Add bounded request-only native Anthropic/OpenAI construction and ordered default fallback.
- Consume host controls before native middleware, retaining credential/destination protections.
- Keep the command on catalog selection; no new auth policy or listener is activated.

## Capabilities

### New Capabilities

- `gateway-request-byok`: internal request-only selector/credential engine and attempt semantics.

### Modified Capabilities

- `providerwire-v4-unary-runtime`: host selection, protected controls, opaque native options and shared deadline.
- `providerwire-v4-streaming-runtime`: shared selection and execution budget.

## Impact

Stack 2/3 for #317, depending on `protect-gateway-client-byok-capture`.
Changes the wire handler API, adds an internal native engine and updates existing
call sites to the catalog adapter. `activate-gateway-auth-and-request-byok`
owns service authorization, discovery, observability and three-listener activation.
