# Proposal

## Why

A Grafana Cloud backend service that serves many stacks cannot call the Gateway for all of them through one token-exchange provider: the provider fixes the stack at construction in both its base URL and its exchange namespace. Such a service must build and cache one provider per stack, which duplicates token caches and HTTP setup in every consumer. The documented exchange audience, the Gateway's plugin ID, is also not one the platform accepts from an internal service.

## What Changes

- A call context can select the stack namespace for a token-exchange provider. One namespace selects both the route and the exchange namespace for `ListModels`, `DoGenerate` and `DoStream`.
- **BREAKING**: `TokenExchangeConfig.BaseURL` becomes the Gateway API root (`/apis/<group>/<version>`), and the client appends `/namespaces/<namespace>`. The namespace is no longer duplicated in the URL.
- **BREAKING**: `TokenExchangeConfig.Namespace` becomes optional. A concrete namespace is the provider's default, empty means every call names its namespace, and `*` keeps exchanging for every namespace while each call names the route namespace. Namespaces must be DNS labels.
- A call namespace that conflicts with the provider's configuration fails before any network request. That covers a different namespace on a provider with a concrete one, and any call namespace on a service-account or access-token provider.
- Calls on behalf of a user still need a concrete exchange namespace; a wildcard exchange is still refused.
- Concurrent exchanges are shared per exchange namespace and subject, so tokens are never reused across stacks.
- The `TokenExchangeConfig` godoc and the authentication guide document the Gateway's API group, `aigateway.ext.grafana.app`, as the Grafana Cloud audience, and require the caller's access policy to allow it. The guide lists the inputs an internal service needs and shows the multi-stack pattern.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `grafana-gateway-client`: per-call namespace selection for the token-exchange flow, exchange base URL as the Gateway API root, namespace-scoped exchange sharing, and the documented audience and internal-service guidance.

`gateway-caller-access` is unchanged. Its audience rule concerns the delegated token that the plugin host verifies, not the caller's exchange audience. Its rule that a token covering every namespace is attributed to the route namespace already covers wildcard exchanges with per-call routes.

## Impact

- **Code:** `providers/grafana` (configuration, request preparation, exchange sharing, godoc, examples), and the Go client capture program under `providers/grafana/internal/capture`.
- **APIs:** a new context option in `providers/grafana`; breaking changes to the meaning of `TokenExchangeConfig.BaseURL` and `Namespace`. Other constructors are unchanged.
- **Tests:** provider unit tests; the black-box command tests that drive the Go client through the protocol test host, including one provider serving two namespaces.
- **Docs:** `docs/guides/gateway-authentication.md`, `docs/providers/grafana-gateway.md`.
- **Parity:** Grafana client extension with no upstream counterpart. The ProviderWire request body, headers, SSE and error envelopes are unchanged; no fixture or baseline change.
