## 1. Core Interface and Helpers (provider package)

- [x] 1.1 Define `ProviderOption` interface with `ProviderKey() string` method in `provider/`
- [x] 1.2 Define `RawProviderOption` struct (Key string, Raw json.RawMessage) implementing `ProviderOption` in `provider/`
- [x] 1.3 Implement `BuildProviderOptions(opts ...ProviderOption) map[string]ProviderOption` in `provider/`
- [x] 1.4 Implement generic `ResolveOption[T any](opts map[string]ProviderOption, key string) (T, bool, error)` in `provider/`
- [x] 1.5 Add unit tests for `BuildProviderOptions` (single, multiple, duplicate keys, empty)
- [x] 1.6 Add unit tests for `ResolveOption` (typed, raw, missing, malformed JSON, unexpected type)

## 2. Provider Package Field Type Changes

- [x] 2.1 Change `CallOptions.ProviderOptions` from `map[string]json.RawMessage` to `map[string]ProviderOption` in `provider/language_model.go`
- [x] 2.2 Change `FunctionTool.ProviderOptions` from `map[string]json.RawMessage` to `map[string]ProviderOption` in `provider/language_model.go`
- [x] 2.3 Change `ProviderOptions` on all message types (`SystemMessage`, `UserMessage`, `AssistantMessage`, `ToolMessage`) in `provider/message.go`
- [x] 2.4 Change `ProviderOptions` on all content part types (`TextContentPart`, `FileContentPart`, `ReasoningContentPart`, `ToolCallContentPart`, `ToolResultContentPart`, `CustomContentPart`, `ReasoningFileContentPart`, `ToolApprovalResponseContentPart`) in `provider/content.go`
- [x] 2.5 Change `ToolResultOutput.ProviderOptions` and `ToolResultContentValue.ProviderOptions` in `provider/types.go`
- [x] 2.6 Fix any compilation errors in provider package tests from field type changes

## 3. Root Package (aisdk) Field Type Changes

- [x] 3.1 Change `StreamTextParams.ProviderOptions` from `map[string]json.RawMessage` to `map[string]provider.ProviderOption` in `text.go`
- [x] 3.2 Change `PrepareStepResult.ProviderOptions` in `text.go`
- [x] 3.3 Change `SystemModelMessage.ProviderOptions` in `text.go`
- [x] 3.4 Change `Tool.ProviderOptions` in `tool.go`
- [x] 3.5 Update `providerMetadataToOptions` in `convert.go` to wrap `ProviderMetadata` entries as `RawProviderOption` and return `map[string]provider.ProviderOption`
- [x] 3.6 Fix any compilation errors in root package tests from field type changes

## 4. Anthropic Module Updates

- [x] 4.1 Add `ProviderKey() string` method to `AnthropicOptions` returning `"anthropic"` in `anthropic/options.go`
- [x] 4.2 Add `ProviderKey() string` method to `AnthropicToolOptions` returning `"anthropic"` in `anthropic/options.go`
- [x] 4.3 Implement `CacheControl(cacheType string) provider.ProviderOption` convenience helper in `anthropic/`
- [x] 4.4 Update `applyProviderOptions` in `anthropic/convert_request.go` to use `provider.ResolveOption[AnthropicOptions]` instead of JSON unmarshal
- [x] 4.5 Update cache control extraction in `anthropic/cache_control.go` to use `provider.ResolveOption` for both typed and raw options
- [x] 4.6 Update tool options extraction in `anthropic/convert_request.go` to use `provider.ResolveOption[AnthropicToolOptions]`
- [x] 4.7 Update `hasProviderThinkingOrEffort` in `anthropic/reasoning.go` to use `provider.ResolveOption`
- [x] 4.8 Update signature extraction and MCP tool use detection in `anthropic/convert_request.go` to use typed option resolution
- [x] 4.9 Add compile-time interface checks: `var _ provider.ProviderOption = AnthropicOptions{}` and `var _ provider.ProviderOption = AnthropicToolOptions{}`
- [x] 4.10 Fix any compilation errors in anthropic module tests from field type changes

## 5. Conformance Test Adaptation

- [x] 5.1 Update `Config.ProviderOptions` handling in `test/conformance/runner.go` to marshal YAML values to JSON and wrap as `RawProviderOption`
- [x] 5.2 Verify conformance tests compile and pass with updated provider options construction

## 6. Verification

- [x] 6.1 Run `make build` -- both modules compile cleanly
- [x] 6.2 Run `make test` -- all tests pass
- [x] 6.3 Run `make lint` -- no lint issues
