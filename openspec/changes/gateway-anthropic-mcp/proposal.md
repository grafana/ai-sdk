## Why

Provider-tool transport alone cannot execute Anthropic-hosted MCP: native requests need server configuration and continuation needs validated server-name metadata. This final WP13 slice isolates remote MCP acceptance and its security policy from the ordinary tool runtime in #239.

## What Changes

- Enable bounded request-level `providerOptions.anthropic.mcpServers` on direct Anthropic routes without disabling the Gateway's existing safe provider options and call headers.
- Permit forwarding only on configured direct Anthropic routes; reject fallback and other backends before physical invocation, including requests without tool definitions.
- Validate MCP continuation and output metadata against caller-configured server names; expose only type/serverName, never endpoints or tokens.
- Add separate MCP captures, deferred-result cases, authenticated native transport and privacy tests while retaining provider-only coverage.

## Capabilities

### New Capabilities

- `gateway-anthropic-mcp`: narrow remote-server options, configured-route gating, validated MCP metadata and both-client acceptance.

### Modified Capabilities

None. This change enables the separately gated MCP extension anticipated by `gateway-provider-tools`; ordinary provider-tool behavior remains unchanged.

## Impact

Depends on `sdk-provider-tool-contract` (#238) and `gateway-provider-tool-runtime` (#239). Changes AGPL option mapping, service composition, continuation metadata and tests/docs; candidate-source checks exercise the Apache prerequisites with merged-only internal pins. The reference follows merged main: Anthropic 4.0.58, Gateway 4.0.87 and provider 4.0.17 at registered commit `08ae5ad05bc12496dd1ffcf64e34419e0831300d`. No provider recordings or upstream pins change. Live remote-egress/deployment approval is outside deterministic validation. All three OpenSpec changes remain unarchived for review.
