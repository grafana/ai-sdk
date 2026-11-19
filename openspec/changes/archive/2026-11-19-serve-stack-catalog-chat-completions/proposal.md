## Why

A stored stack catalog can serve ProviderWire calls but not official OpenAI
clients. A cloud-mode user or service account on the namespace Chat route
cannot supply the required BYOK controls. Catalog invocation can instead use
the named catalog's keys where the platform supplies them. Chat also lacks
coverage for tenant defaults and extension failures through the Gateway.

## What Changes

- Register `POST .../modelcatalogs/{name}/v1/chat/completions` as a
  `ModelCatalog` subresource with its own route label. Use the same
  `aigateway:modelinvoker` role, parent check and account access as catalog
  `language-model`.
- Specify caller values, then tenant defaults, then candidate-local Chat
  defaults, and prove the order at the shared runtime and native boundary.
- Prove operator- and tenant-scope extension behavior on Chat: fixed HTTP
  403 and 503 failures before stream setup, and terminal in-band errors after
  the stream starts.
- Exercise unary and streaming catalog calls with an official OpenAI SDK
  through the built plugin in both Gateway modes. Use the pinned Grafana
  Playwright suite to verify the actual mount, RBAC and decrypted keys.
- Document the catalog base URL, permissions and platform limitations in the
  Chat guide and existing stack catalog documentation.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `gateway-openai-chat-completions-adapter`: route, access, defaults,
  extensions and SDK evidence for catalog Chat.
- `gateway-tenant-call-defaults`: caller presence and ordering on Chat.
- `gateway-model-catalog-kind`: grant for the Chat subresource.
- `gateway-plugin-host`: include catalog Chat in the route-table contract.
- `gateway-stack-catalog-runtime`: distinguish the ProviderWire execution
  overview from Chat's response contract.

## Impact

One additive route and one grant on an existing role. No new Chat request
fields, success response fields, discovery route or BYOK support. No changes
to catalog provider policy, ProviderWire, or aggregated customer telemetry.
