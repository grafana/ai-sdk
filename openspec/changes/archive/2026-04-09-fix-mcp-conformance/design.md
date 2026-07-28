## Context

The Anthropic Go SDK represents the `mcp_tool_result` content field as a `BetaMCPToolResultBlockContentUnion` struct -- a Go union with `OfString` and `OfBetaMCPToolResultBlockContent` fields. When we `json.Marshal` this struct, Go produces `{"OfBetaMCPToolResultBlockContent":[...],"OfString":""}` instead of the original wire JSON `[{"type":"text","text":"..."}]`.

The upstream TypeScript implementation simply passes `part.content` through as-is after Zod parsing, preserving the original shape. Our Go code needs to match this passthrough behavior.

The SDK provides a `RawJSON()` method on the union type that returns the unmodified JSON string from the API response.

## Goals / Non-Goals

**Goals:**
- Fix MCP tool result content serialization to preserve the original wire JSON
- Pass the MCP conformance test (`upstream/mcp`)
- Maintain round-trip correctness for MCP tool results in multi-step prompts

**Non-Goals:**
- Changes to MCP tool use (input) handling -- that path already works correctly
- Changes to the conformance test infrastructure itself
- Changes to the expected.jsonl fixtures

## Decisions

### Use `RawJSON()` for MCP tool result content serialization

In both the streaming adapter (`convert_stream.go`) and non-streaming response converter (`convert_response.go`), replace `json.Marshal(mtr.Content)` with `json.RawMessage(mtr.Content.RawJSON())`.

This directly mirrors the upstream TypeScript behavior of passing `part.content` through without transformation. The `RawJSON()` method returns the exact JSON bytes received from the API, which is exactly what we want in the `ToolResultOutput.JSON` field.

**Alternative considered**: Extracting the inner content via `AsBetaMCPToolResultBlockContent()` and re-marshaling. Rejected because it adds unnecessary transformation and could lose fidelity with edge cases (e.g., string content vs array content).

## Risks / Trade-offs

- [Risk] `RawJSON()` could return empty string if the SDK didn't parse correctly -> Mitigation: The SDK always populates `JSON.raw` during deserialization; this is the standard pattern across all SDK types.
- [Risk] Round-trip `serializeMCPToolResultContent` already handles the `ToolResultOutput.JSON` field correctly by attempting to unmarshal back into structured types -> No change needed there; it already works with valid raw JSON.
