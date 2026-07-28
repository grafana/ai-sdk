## 1. Provider package — new content types and sealed interfaces

- [x] 1.1 Add `ToolMessageContentPart` sealed interface with `toolMessageContentPart()` marker method in `provider/content.go`
- [x] 1.2 Add `toolMessageContentPart()` method to `ToolResultContentPart` (now implements both `AssistantContentPart` and `ToolMessageContentPart`)
- [x] 1.3 Add `CustomContentPart` struct (`Kind string`, `ProviderOptions`) with `assistantContentPart()` marker in `provider/content.go`
- [x] 1.4 Add `ReasoningFileContentPart` struct (`Data DataContent`, `MediaType string`, `ProviderOptions`) with `assistantContentPart()` marker in `provider/content.go`
- [x] 1.5 Add `ToolApprovalResponseContentPart` struct (`ApprovalID string`, `Approved bool`, `Reason string`, `ProviderOptions`) with `toolMessageContentPart()` marker in `provider/content.go`
- [x] 1.6 Change `ToolMessage.Content` from `[]ToolResultContentPart` to `[]ToolMessageContentPart` in `provider/message.go`
- [x] 1.7 Add compile-time interface checks in `provider/message_test.go` for new types
- [x] 1.8 Verify existing compile-time check that `ToolResultContentPart` implements `AssistantContentPart` in `provider/message_test.go`

## 2. Provider package — ImageContentPart removal

- [x] 2.1 Remove `ImageContentPart` struct and its `userContentPart()` marker from `provider/content.go`
- [x] 2.2 Update `UserContentPart` doc comment to list only `TextContentPart` and `FileContentPart`
- [x] 2.3 Remove `ImageContentPart` compile-time check from `provider/message_test.go`

## 3. Provider package — stream parts and generate content

- [x] 3.1 Add `PartCustom StreamPartType = "custom"` and `PartReasoningFile StreamPartType = "reasoning-file"` constants in `provider/stream_part.go`
- [x] 3.2 Add `Kind string` field to `StreamPart` for `PartCustom`
- [x] 3.3 Add `ApprovalID string`, `Approved *bool`, `Reason string` fields to `StreamPart` for tool approval parts
- [x] 3.4 Add `Kind string` field to `GenerateContentPart` in `provider/language_model.go`
- [x] 3.5 Update `provider/stream_part_test.go` to include `PartCustom` and `PartReasoningFile` in the stream part type completeness test

## 4. Provider package — warning type changes

- [x] 4.1 Add warning type constants: `WarnUnsupported = "unsupported"`, `WarnCompatibility = "compatibility"`, `WarnOther = "other"` in `provider/types.go`
- [x] 4.2 Update `Warning` struct doc comment from `"unsupported-setting" | "other"` to `"unsupported" | "compatibility" | "other"`

## 5. Provider package — CallOptions.Reasoning

- [x] 5.1 Add reasoning effort constants (`ReasoningProviderDefault`, `ReasoningNone`, `ReasoningMinimal`, `ReasoningLow`, `ReasoningMedium`, `ReasoningHigh`, `ReasoningXHigh`) in `provider/types.go`
- [x] 5.2 Add `Reasoning *string` field to `CallOptions` in `provider/language_model.go`

## 6. Anthropic module — ImageContentPart removal and FileContentPart image handling

- [x] 6.1 Remove `case provider.ImageContentPart` from `convertUserContent` in `anthropic/convert_request.go`
- [x] 6.2 Update `case provider.FileContentPart` in `convertUserContent` to handle image media types (produce `BetaImageBlockParam` for `image/*`, keep `BetaRequestDocumentBlockParam` for others)
- [x] 6.3 Update `anthropic/convert_request_test.go` — replace `ImageContentPart` test cases with `FileContentPart` using image media types

## 7. Anthropic module — warning type updates

- [x] 7.1 Replace all `"unsupported-setting"` string literals with `WarnUnsupported` constant in anthropic module
- [x] 7.2 Update any warning-related test assertions from `"unsupported-setting"` to `"unsupported"`

## 8. Anthropic module — unsupported content warnings

- [x] 8.1 Add `case provider.CustomContentPart` and `case provider.ReasoningFileContentPart` to `convertAssistantContent` in `anthropic/convert_request.go` — produce warning and skip
- [x] 8.2 Add `case provider.ToolApprovalResponseContentPart` to `convertToolContent` in `anthropic/convert_request.go` — produce warning and skip (or handle if Anthropic supports it)
- [x] 8.3 Add tests for unsupported content part warnings

## 9. Anthropic module — model capabilities and reasoning resolution

- [x] 9.1 Add `getModelCapabilities(modelID string)` function returning `maxOutputTokens`, `supportsAdaptiveThinking`, `isKnownModel` with model ID substring matching (specific-first order)
- [x] 9.2 Add `resolveReasoningConfig` function implementing dual-path reasoning resolution: adaptive path (thinking: adaptive + effort mapping) vs budget path (thinking: enabled + computed budgetTokens)
- [x] 9.3 Add effort mapping helper: `minimal`→`low`, `low`→`low`, `medium`→`medium`, `high`→`high`, `xhigh`→`max` with compatibility warnings when mapped value differs
- [x] 9.4 Add budget mapping helper: compute `clamp(round(maxOutputTokens * pct), 1024, maxOutputTokens)` with percentages `minimal` 2%, `low` 10%, `medium` 30%, `high` 60%, `xhigh` 90%
- [x] 9.5 Handle `"none"` → `thinking: disabled` (no effort, no budget)
- [x] 9.6 Handle `nil` / `"provider-default"` → no-op (skip reasoning mapping)
- [x] 9.7 Integrate into `buildParams`: add precedence check — skip reasoning mapping if EITHER provider option `thinking` OR `effort` is already set
- [x] 9.8 Register beta headers from reasoning mapping: `interleaved-thinking-2025-05-14` for enabled/adaptive, `effort-2025-11-24` for effort
- [x] 9.9 Add tests: model capability detection (all model families + unknown)
- [x] 9.10 Add tests: adaptive path reasoning mapping (all values, compatibility warnings)
- [x] 9.11 Add tests: budget path reasoning mapping (all values, budget clamping)
- [x] 9.12 Add tests: reasoning none disables thinking
- [x] 9.13 Add tests: precedence (provider options skip reasoning mapping)
- [x] 9.14 Add tests: nil and provider-default are no-ops
- [x] 9.15 Add tests: beta header registration from reasoning mapping

## 10. Orchestration layer — content type updates

- [x] 10.1 Update `streamtext.go` to remove any ImageContentPart → FileContentPart conversion logic (callers now use FileContentPart directly)
- [x] 10.2 Update orchestration layer types that reference `ToolResultContentPart` from tool messages to use `ToolMessageContentPart` (if any type-switches exist)
- [x] 10.3 Update `textstream.go` / `text.go` to handle new stream part types (`PartCustom`, `PartReasoningFile`) in content accumulation

## 11. Root package tests

- [x] 11.1 Update `streamtext_test.go` — replace any `ImageContentPart` usage with `FileContentPart`
- [x] 11.2 Update test mock providers to use `WarnUnsupported` constant instead of `"unsupported-setting"`
- [x] 11.3 Add tests for new stream part types flowing through orchestration (if applicable)

## 12. Verification

- [x] 12.1 Run `make build` — both modules compile
- [x] 12.2 Run `make test` — all tests pass
- [x] 12.3 Run `make lint` — no lint issues
