## 1. AnthropicToolOptions type

- [x] 1.1 Add `AnthropicToolOptions` struct to `anthropic/options.go` with `DeferLoading *bool`, `AllowedCallers []string`, and `EagerInputStreaming *bool` fields (camelCase JSON tags)

## 2. convertTools changes

- [x] 2.1 Change `convertTools` signature to return `([]anthropic.BetaToolUnionParam, []provider.Warning, []string)` where the third return is auto-detected beta strings
- [x] 2.2 In the function tool branch, unmarshal `tool.ProviderOptions["anthropic"]` into `AnthropicToolOptions` and apply `DeferLoading`, `AllowedCallers`, `EagerInputStreaming` to `BetaToolParam`
- [x] 2.3 In the function tool branch, convert `tool.InputExamples` (`[]json.RawMessage`) to `[]map[string]any` and set on `BetaToolParam.InputExamples`
- [x] 2.4 Add beta auto-detection: collect `"advanced-tool-use-2025-11-20"` when `InputExamples` or `AllowedCallers` are present on any function tool
- [x] 2.5 Update all callers of `convertTools` to handle the new betas return value

## 3. Beta header application

- [x] 3.1 In `buildParams` (or the DoStream/DoGenerate call sites), merge auto-detected betas from `convertTools` with explicit betas from `AnthropicOptions.Betas`, deduplicate, and apply via `option.WithHeader("anthropic-beta", ...)`

## 4. Tests

- [x] 4.1 Add test: function tool with `deferLoading: true` in provider options produces `BetaToolParam` with `DeferLoading` set
- [x] 4.2 Add test: function tool with `allowedCallers` in provider options produces `BetaToolParam` with `AllowedCallers` set
- [x] 4.3 Add test: function tool with `eagerInputStreaming: true` in provider options produces `BetaToolParam` with `EagerInputStreaming` set
- [x] 4.4 Add test: function tool with all three options set simultaneously
- [x] 4.5 Add test: function tool with no `"anthropic"` key in provider options leaves fields unset
- [x] 4.6 Add test: function tool with malformed `"anthropic"` JSON treats options as empty
- [x] 4.7 Add test: function tool with `InputExamples` produces `BetaToolParam.InputExamples`
- [x] 4.8 Add test: beta auto-detection returns `"advanced-tool-use-2025-11-20"` for `InputExamples`
- [x] 4.9 Add test: beta auto-detection returns `"advanced-tool-use-2025-11-20"` for `AllowedCallers`
- [x] 4.10 Add test: beta deduplication when both `InputExamples` and `AllowedCallers` present
- [x] 4.11 Add test: provider-defined tool ignores `ProviderOptions["anthropic"]`
