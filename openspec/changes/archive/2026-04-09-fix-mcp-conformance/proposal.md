## Why

The MCP conformance test is failing because `mcp_tool_result` content is serialized by marshaling the Anthropic Go SDK's union struct (`BetaMCPToolResultBlockContentUnion`) directly, which produces `{"OfBetaMCPToolResultBlockContent":[...],"OfString":""}` instead of the raw content array `[{"type":"text","text":"..."}]`. The upstream TypeScript implementation passes the parsed content through directly without transformation. Our Go implementation must use the SDK's `RawJSON()` method to preserve the original wire format.

## What Changes

- Fix `mcp_tool_result` content serialization in the streaming adapter (`convert_stream.go`) to use `RawJSON()` instead of `json.Marshal` on the union struct
- Fix `mcp_tool_result` content serialization in the non-streaming response converter (`convert_response.go`) to use `RawJSON()` instead of `json.Marshal` on the union struct
- Update the MCP spec to explicitly require raw JSON passthrough for MCP tool result content, matching upstream behavior

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `mcp-server-tools`: The MCP tool result streaming and non-streaming requirements need to specify that content must be serialized as raw JSON passthrough (not re-marshaled through the SDK's union type), matching the upstream TypeScript behavior where `part.content` is passed through directly.

## Impact

- `anthropic/convert_stream.go` -- MCP tool result handling in streaming path
- `anthropic/convert_response.go` -- MCP tool result handling in non-streaming path
- Conformance test `upstream/mcp` will pass after this fix
- Unit tests for MCP tool result serialization may need updating to verify raw JSON output
