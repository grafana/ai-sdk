## Why

Upstream `@ai-sdk/anthropic` 4.0.0-canary.50 dropped the obsolete `fine-grained-tool-streaming-2025-05-14` beta header and replaced it with per-tool `eager_input_streaming: true` defaulted on streaming requests, controlled by a new model-level `toolStreaming` option (default `true`). The Go port never adopted the beta header but also never adopted the default per-tool behavior, so streaming requests with function tools currently do not benefit from eager tool input streaming unless the caller sets `EagerInputStreaming` on every tool. Aligning with upstream restores the expected user-observable behavior on `DoStream`.

## What Changes

- Add `ToolStreaming *bool` field to `AnthropicOptions` (model-level provider options), with `nil` treated as `true` to match upstream's `?? true` default.
- Thread a `stream bool` flag from `model.DoStream` / `model.DoGenerate` through `buildParams` and into `convertTools` so tool conversion knows whether the request is streaming. The same flag is also threaded into `applyResponseFormat` so the synthetic JSON fallback tool (used for non-native-structured-output models) participates in the default.
- In `convertTools`, when the request is streaming and `ToolStreaming` resolves to `true`, default `EagerInputStreaming` to `true` for each function tool that does not already set it explicitly via `AnthropicToolOptions.EagerInputStreaming`. The synthetic JSON response-format fallback tool receives the same default.
- Wire format matches upstream `...(eagerInputStreaming ? { eager_input_streaming: true } : {})`: only a truthy resolved value emits the field. An explicit per-tool `eagerInputStreaming: false` SHALL omit the field rather than send `eager_input_streaming: false`.
- Provider-defined tools (server tools, MCP tools, etc.) are unaffected — `eager_input_streaming` only applies to custom function tools, matching upstream.
- No beta header changes (the `fine-grained-tool-streaming-2025-05-14` beta was never emitted by the Go port).

## Capabilities

### New Capabilities

_None._

### Modified Capabilities
- `anthropic-tool-options`: adds the requirement that function tools default `eager_input_streaming` to `true` on streaming requests, gated by a new model-level `ToolStreaming` option. Existing requirement around the explicit per-tool `eagerInputStreaming` field is preserved (explicit values always win over the default).

## Impact

- `providers/anthropic/options.go`: new `ToolStreaming *bool` field on `AnthropicOptions`.
- `providers/anthropic/convert_request.go`: `buildParams`, `convertTools`, and `applyResponseFormat` gain a streaming-context parameter; `convertTools` applies the per-tool default and `applyResponseFormat` applies the same default to the JSON fallback tool.
- `providers/anthropic/model.go`: `DoStream` and `DoGenerate` pass `stream=true` / `stream=false` to `buildParams`.
- `providers/anthropic/convert_request_test.go` and related test files: new coverage for the defaulting behavior, the `ToolStreaming=false` opt-out, the non-streaming case, wire-format suppression on explicit `false`, and the JSON fallback tool defaulting.
- No wire-format changes for `@ai-sdk/react` (SSE chunks unaffected). No breaking change for callers — purely additive behavior on streaming requests, controllable via `ToolStreaming`.
