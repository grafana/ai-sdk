## Why

The Anthropic provider fully supports provider-defined tools (`web_search`, `tool_search`, etc.) at the `provider.Tool` level -- conversion, name mapping, and stream processing all work. However, the user-facing `aisdk.Tool` struct has no `Type`, `ID`, or `Args` fields, and `toolSetToProviderTools()` in `convert.go:261` hardcodes `Type: "function"` for every tool.

This means users cannot declare provider-defined tools through `StreamText` / `ToolSet`. The only way to use them today is by constructing `provider.CallOptions` directly and calling `DoStream`, bypassing the entire orchestration layer (multi-step tool loops, SSE serialization, callbacks).

This is a gap vs the upstream Vercel AI SDK, where users can include provider-defined tools alongside function tools in the same `tools` parameter.

Ref: https://github.com/grafana/ai-sdk/issues/90 (additional finding: aisdk.Tool / ToolSet doesn't support provider-defined tools)

## What Changes

- Add `Type`, `ID`, and `Args` fields to `aisdk.Tool`
- Update `toolSetToProviderTools()` to pass through these fields when `Type == "provider"`, instead of always emitting `Type: "function"`
- Warn and skip tools with unrecognized `Type` values
- When `Type` is empty or `"function"`, preserve existing behavior (backward compatible)

## Capabilities

### New Capabilities

- `provider-defined-tools`: Users can declare provider-defined tools (web_search, tool_search, code_execution, etc.) through the `ToolSet` API on `StreamText` / `GenerateText`, using `Type: "provider"` with `ID` and `Args` fields, aligned with upstream naming

### Modified Capabilities

_(none -- existing function tool behavior is unchanged)_

## Impact

- **tool.go**: Add `Type`, `ID`, `Args` fields to the `Tool` struct
- **convert.go**: Update `toolSetToProviderTools()` to branch on `Type`, pass through `ID` / `Args` for provider tools, and warn on unknown types
- **streamtext.go**: Wire tool warnings into `allWarnings`; `handleToolCall` and `executeTools` already handle `ProviderExecuted` correctly
- **provider/language_model.go**: Align `Tool.Type` comment to `"provider"` (from `"provider-defined"`)
- **anthropic/**: Align all `"provider-defined"` references to `"provider"` across convert_request.go, tool_name_mapping.go, and tests
- **No new dependencies**
