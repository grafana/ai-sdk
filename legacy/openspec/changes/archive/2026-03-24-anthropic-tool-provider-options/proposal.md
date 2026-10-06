## Why

The Grafana assistant team uses `defer_loading: true` on tool definitions combined with `tool_search` server tools to let Claude dynamically discover tools instead of receiving all 100+ definitions in every request. The upstream TypeScript SDK supports `deferLoading`, `allowedCallers`, and `eagerInputStreaming` as Anthropic-specific provider options on function tools, but the Go SDK's `convertTools()` ignores `tool.ProviderOptions["anthropic"]` entirely.

## What Changes

- Extract and apply `deferLoading` (bool), `allowedCallers` (string array), and `eagerInputStreaming` (bool) from `tool.ProviderOptions["anthropic"]` during function tool conversion in `anthropic/convert_request.go`
- Define an `AnthropicToolOptions` struct for deserializing these tool-level provider options (distinct from the existing `AnthropicOptions` which is for call-level options)
- Pass `inputExamples` through to the Anthropic API (currently supported on the `provider.Tool` struct but not forwarded in `convertTools()`)
- Auto-detect when `advanced-tool-use-2025-11-20` beta header is needed (triggered by `inputExamples` or `allowedCallers`) and include it in requests

## Capabilities

### New Capabilities
- `anthropic-tool-options`: Anthropic-specific provider options on function tool definitions (`deferLoading`, `allowedCallers`, `eagerInputStreaming`) and `inputExamples` passthrough

### Modified Capabilities
(none)

## Impact

- **Code**: `anthropic/convert_request.go` (tool conversion), `anthropic/convert_request_test.go` (new tests)
- **APIs**: No public API changes -- uses existing `ProviderOptions` map on `provider.Tool`
- **Dependencies**: No new dependencies -- the Anthropic Go SDK v1.27.1 (already pinned) has `DeferLoading`, `AllowedCallers`, `EagerInputStreaming`, and `InputExamples` fields on `BetaToolParam`
- **Wire format**: Adds optional fields to Anthropic API tool definitions; no change to SSE output format
