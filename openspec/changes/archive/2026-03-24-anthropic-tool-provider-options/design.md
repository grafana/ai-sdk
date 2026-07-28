## Context

The Anthropic provider's `convertTools()` function in `anthropic/convert_request.go` converts `provider.Tool` entries to Anthropic SDK params. For function tools (the `default` branch), it builds `BetaToolParam` with `Name`, `Description`, `InputSchema`, and `CacheControl` -- but ignores `tool.ProviderOptions["anthropic"]` and `tool.InputExamples`.

The Anthropic Go SDK v1.27.1 (already pinned) has `DeferLoading`, `AllowedCallers`, `EagerInputStreaming`, and `InputExamples` fields on `BetaToolParam`. The upstream TypeScript SDK reads these from `tool.providerOptions.anthropic` in `prepareTools()` and auto-adds the `advanced-tool-use-2025-11-20` beta when `inputExamples` or `allowedCallers` are present.

Currently, the Go SDK has no beta auto-detection mechanism -- betas are only passed explicitly via `AnthropicOptions.Betas` in call-level provider options.

## Goals / Non-Goals

**Goals:**
- Pass `deferLoading`, `allowedCallers`, and `eagerInputStreaming` from `tool.ProviderOptions["anthropic"]` to Anthropic API tool definitions
- Pass `tool.InputExamples` through to `BetaToolParam.InputExamples`
- Auto-add `advanced-tool-use-2025-11-20` beta header when `inputExamples` or `allowedCallers` are present
- Maintain upstream parity with the TypeScript SDK's `prepareTools()` behavior

**Non-Goals:**
- Adding these options to provider-defined tools (server tools) -- upstream doesn't support this either
- Migrating from Beta to non-Beta API types (separate effort)
- Supporting `strict` tool option (separate feature, not requested)

## Decisions

### 1. Define `AnthropicToolOptions` struct in `anthropic/options.go`

Create a new struct for tool-level Anthropic options, separate from the existing `AnthropicOptions` (which is for call-level options like thinking/effort):

```go
type AnthropicToolOptions struct {
    DeferLoading        *bool    `json:"deferLoading,omitempty"`
    AllowedCallers      []string `json:"allowedCallers,omitempty"`
    EagerInputStreaming *bool    `json:"eagerInputStreaming,omitempty"`
}
```

**Rationale**: Keeps the JSON keys camelCase to match the upstream TypeScript interface. Using `*bool` for optional booleans follows Go conventions and the project's style.

### 2. Extract and apply options in `convertTools()`

In the `default` (function tool) branch of `convertTools()`, unmarshal `tool.ProviderOptions["anthropic"]` into `AnthropicToolOptions` and set the corresponding fields on `BetaToolParam`. Also convert `tool.InputExamples` (which are `[]json.RawMessage`) to `[]map[string]any` for the SDK.

**Rationale**: Keeps the change contained to the existing function. No need to restructure `convertTools()` -- the options extraction fits naturally in the existing code path.

### 3. Thread beta detection through `convertTools()` return value

Change `convertTools()` to return a `[]string` of required beta headers alongside the tools and warnings. The caller (`buildParams`) will merge these with any explicit betas from `AnthropicOptions.Betas`.

**Rationale**: `convertTools()` already knows when beta-triggering options are present. Returning betas from the function keeps the detection logic co-located with the conversion logic, matching the upstream pattern where `prepareTools()` returns a `betas: Set<string>`.

### 4. Apply betas as request options

Use `option.WithHeader("anthropic-beta", ...)` to add the beta header when auto-detected betas are present. This merges with any explicit betas from `AnthropicOptions.Betas`.

**Rationale**: The Anthropic Go SDK accepts beta headers via request options. This is the standard mechanism and avoids having to modify the request struct directly.

## Risks / Trade-offs

- **[Risk] Beta header format** -- Anthropic beta headers are comma-separated strings. Multiple auto-detected betas plus user-explicit betas need deduplication. → Mitigation: deduplicate betas before joining. Keep a `map[string]struct{}` to track unique betas.

- **[Risk] SDK field type mismatch** -- `BetaToolParam.InputExamples` is `[]map[string]any` while `provider.Tool.InputExamples` is `[]json.RawMessage`. → Mitigation: Unmarshal each `json.RawMessage` into `map[string]any` during conversion. Skip malformed entries silently (matching the project's style of non-fatal errors during conversion).

- **[Trade-off] No validation of `allowedCallers` values** -- The upstream TypeScript SDK uses a union type to constrain values to `"direct" | "code_execution_20250825" | "code_execution_20260120"`. We pass strings through without validation. → Acceptable because the Anthropic API will reject invalid values with a clear error, and new allowed values won't require a Go SDK update.
