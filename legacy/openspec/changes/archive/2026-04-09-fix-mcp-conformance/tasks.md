## 1. Fix MCP tool result content serialization

- [x] 1.1 In `anthropic/convert_stream.go`, replace `json.Marshal(mtr.Content)` with `json.RawMessage(mtr.Content.RawJSON())` in the `mcp_tool_result` case of the streaming adapter
- [x] 1.2 In `anthropic/convert_response.go`, replace `json.Marshal(mtr.Content)` with `json.RawMessage(mtr.Content.RawJSON())` in the `mcp_tool_result` case of the non-streaming response converter

## 2. Update unit tests

- [x] 2.1 Update `TestStreamAdapter_MCPToolResult` in `convert_stream_test.go` to verify that `Output.JSON` contains the raw content JSON (not the union struct JSON)
- [x] 2.2 Update `TestStreamAdapter_MCPFullSequence` in `convert_stream_test.go` if it asserts on MCP tool result output format
- [x] 2.3 Update `TestConvertResponse_MCPToolResult` in `convert_response_test.go` to verify that `Result` contains the raw content JSON

## 3. Verification

- [x] 3.1 Run `go test ./...` in the anthropic module to confirm unit tests pass
- [x] 3.2 Run `make test-conformance` to confirm the MCP conformance test passes
- [x] 3.3 Run `make test` to confirm no regressions across the full test suite
