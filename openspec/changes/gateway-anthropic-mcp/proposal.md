## Why

Provider-tool transport alone cannot execute Anthropic-hosted MCP: native requests need server configuration and continuation needs validated server-name metadata. This final WP13 slice isolates remote MCP acceptance and its security policy from the ordinary tool runtime in #239.

## What Changes

- Enable only registered request-level `providerOptions.anthropic.mcpServers`, with bounded server definitions and HTTPS destination restrictions.
- Permit forwarding only on configured direct Anthropic routes; reject fallback and other backends before physical invocation, including requests without tool definitions.
- Validate MCP continuation and output metadata against caller-configured server names; expose only type/serverName, never endpoints or tokens.
- Add separate MCP captures, deferred-result cases, authenticated native transport and privacy tests while retaining provider-only coverage.

## Capabilities

### New Capabilities

- `gateway-anthropic-mcp`: narrow remote-server options, configured-route gating, validated MCP metadata and both-client acceptance.

### Modified Capabilities

None. This change enables the separately gated MCP extension anticipated by `gateway-provider-tools`; ordinary provider-tool behavior remains unchanged.

## Impact

Depends on `sdk-provider-tool-contract` (#238) and `gateway-provider-tool-runtime` (#239). Changes AGPL option mapping, service composition, continuation metadata and tests/docs; consumes the already-published Apache prerequisites. The reference remains Anthropic 4.0.38, Gateway 4.0.52 and provider 4.0.7 at registered commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`. No provider recordings or upstream pins change. Live remote-egress/deployment approval is outside deterministic validation. All three OpenSpec changes remain unarchived for review.
