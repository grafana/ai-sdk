## Why

The runtime-based Chat codec needs a namespaced plugin route and exact-client
proof through the plugin protocol. The #581 host contract now provides generic
route registration, access, telemetry, native translation and selection, so
Chat can integrate without recreating the old standalone router/catalog hooks.

## What Changes

- Register POST `namespaces/{namespace}/v1/chat/completions` as one route-table
  entry with the Chat handler, label and host-error writer, and declare it in
  the CUE/generated manifests.
- Construct Chat with the same execution.Runtime as ProviderWire; keep the
  shared access-aware selector and per-attempt translation policy.
- Prove Go and TypeScript OpenAI client behavior against the built plugin via
  platformhost and synthetic native Responses, Anthropic and compatible APIs.
- Document access/error boundaries, supported subset and remaining #483 work.

## Non-goals

No Chat BYOK controls, inbound credential translator, new standalone server,
shared lifecycle-validator delivery, live-provider acceptance or deployed
Grafana proof. The adapter is not ready before #483 integration is verified.
#583's ProviderWire sequential-response work does not authorize silent Chat
truncation or a new multi-response protocol mapping.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-openai-chat-completions-adapter`: platform registration, access/error
  ownership and official-client evidence.
- `gateway-plugin-host`: add the Chat operation to the routes-only manifest.

## Impact

AGPL Gateway composition, manifest, route tests, native Chat client harness,
test tasks/workspace and documentation. Existing generic catalog wrapping,
selection, dispatch, access middleware and telemetry machinery stay owned by
the host. The adapter's new route does not add configured access for BYOK-only
callers or change ProviderWire behavior.
