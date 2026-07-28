## 1. Tool Name Mapping

- [x] 1.1 Add 4 new entries to `providerToolNames` map in `anthropic/tool_name_mapping.go`: `memory_20250818` -> `"memory"`, `web_search_20260209` -> `"web_search"`, `web_fetch_20250910` -> `"web_fetch"`, `web_fetch_20260209` -> `"web_fetch"`
- [x] 1.2 Add unit tests for the new mapping entries (verify both directions)

## 2. Request-Side Tool Definitions

- [x] 2.1 Add `anthropic.memory_20250818` case to `convertProviderTool` in `anthropic/convert_request.go`: type `memory_20250818`, name `memory`, beta `context-management-2025-06-27`, no args
- [x] 2.2 Add `anthropic.web_fetch_20250910` case: type `web_fetch_20250910`, name `web_fetch`, args `maxUses`/`allowedDomains`/`blockedDomains`/`citations`/`maxContentTokens`, beta `web-fetch-2025-09-10`
- [x] 2.3 Add `anthropic.web_fetch_20260209` case: type `web_fetch_20260209`, same args as v1, beta `code-execution-web-tools-2026-02-09`
- [x] 2.4 Add `anthropic.web_search_20260209` case: type `web_search_20260209`, name `web_search`, args `maxUses`/`allowedDomains`/`blockedDomains`/`userLocation`, beta `code-execution-web-tools-2026-02-09`
- [x] 2.5 Add unit tests for all 4 new tool definitions (verify SDK types, beta headers, arg mapping)

## 3. web_fetch_tool_result Response Handling

- [x] 3.1 Add `web_fetch_tool_result` case to streaming path in `anthropic/convert_stream.go`: handle `web_fetch_result` success (emit `PartToolResult` with structured JSON, push to `citationDocuments`) and `web_fetch_tool_result_error` (emit `PartToolResult` with `IsError: true`)
- [x] 3.2 Add `web_fetch_tool_result` case to non-streaming path in `anthropic/convert_response.go`: same semantics as streaming (produce `GenerateContentPart` with `Type: "tool-result"`, push to `citationDocuments` on success)
- [x] 3.3 Add unit tests for streaming web_fetch_tool_result: success with text source, success with PDF source, success with nil title (fallback to URL), error case
- [x] 3.4 Add unit tests for non-streaming web_fetch_tool_result: success and error cases

## 4. Verification

- [x] 4.1 Run existing conformance tests (`test/conformance/anthropic/upstream/web-fetch-tool/` and `web-fetch-tool-20260209/`) and verify they pass
- [x] 4.2 Run full test suite (`make test`) and verify no regressions
