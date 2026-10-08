## Why

The registered `@ai-sdk/gateway` client sends `includeRawChunks` and filters `{type:"raw",rawValue}` stream parts unless the caller asked for them. The Gateway rejected the flag as `unsupported capability: raw-output`, so callers who want native provider events could not get them, and a caller sending `true` could not use the Gateway at all (#116).

## What Changes

- Accept `includeRawChunks` and pass it to the selected model in unary and streaming calls.
- Write requested `PartRaw` values as `raw` stream events; drop unrequested and value-less raw parts.
- Remove credential echoes from OpenAI Responses MCP tool definitions before writing.
- Remove `unsupported capability: raw-output` from the error vocabulary and add the `raw` event to the stream-event schema.

## Capabilities

### New Capabilities

- `gateway-raw-output`: caller-requested raw stream events with bounded framing, credential projection and telemetry exclusion.

### Modified Capabilities

- `providerwire-v4-streaming-runtime`: raw parts are no longer an unsupported stream family.
- `providerwire-v4-unary-runtime`: raw output is no longer an unsupported request family.
- `providerwire-v4-http-contract`: raw output leaves the deferred stream families.
- `sdk-provider-transport-context`: the Gateway passes the caller's flag instead of rejecting it.

## Impact

Changes AGPL `ai-gateway/providerwire/v4`; Apache client and adapter modules are unchanged. Registered baseline: `5d12eaa6caa193d3901cbab98a734403eb6bf622` (Gateway 4.0.96, provider 4.0.18). No upstream pins or provider recordings change.
