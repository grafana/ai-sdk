## 1. Provider package type definitions

- [x] 1.1 Add `ToolChoiceType` type and constants (`ToolChoiceAuto`, `ToolChoiceNone`, `ToolChoiceRequired`, `ToolChoiceTool`) in `provider/types.go`; change `ToolChoice.Type` field to `ToolChoiceType`
- [x] 1.2 Add `WarningType` type in `provider/types.go`; change `WarnUnsupported`, `WarnCompatibility`, `WarnOther` constants to typed `WarningType`; change `Warning.Type` field to `WarningType`
- [x] 1.3 Add `ReasoningEffort` type in `provider/types.go`; change `ReasoningProviderDefault`, `ReasoningNone`, `ReasoningMinimal`, `ReasoningLow`, `ReasoningMedium`, `ReasoningHigh`, `ReasoningXHigh` constants to typed `ReasoningEffort`
- [x] 1.4 Add `ToolResultContentType` type and constants (`ToolContentText`, `ToolContentFileData`, `ToolContentFileURL`, `ToolContentFileID`, `ToolContentImageData`, `ToolContentImageURL`, `ToolContentImageFileID`, `ToolContentCustom`) in `provider/types.go`; change `ToolResultContentValue.Type` field to `ToolResultContentType`; rename `ProviderReference` field to `FileID` (json `"fileId"`) to match upstream
- [x] 1.5 Add `ResponseFormatType` type and constants (`ResponseFormatText`, `ResponseFormatJSON`) in `provider/language_model.go`; change `ResponseFormat.Type` field to `ResponseFormatType`
- [x] 1.6 Add `ToolType` type and constants (`ToolTypeFunction`, `ToolTypeProvider`) in `provider/language_model.go`; change `FunctionTool.Type` and `ProviderTool.Type` fields to `ToolType`
- [x] 1.7 Add `GenerateContentType` type and constants (`ContentText`, `ContentReasoning`, `ContentToolCall`, `ContentToolResult`, `ContentSource`, `ContentFile`, `ContentReasoningFile`) in `provider/language_model.go`; change `GenerateContentPart.Type` field to `GenerateContentType`
- [x] 1.8 Add `SourceType` type and constants (`SourceTypeURL`, `SourceTypeDocument`) in `provider/stream_part.go`; change `SourceInfo.SourceType` field to `SourceType`; change `GenerateContentPart.SourceType` field to `SourceType`
- [x] 1.9 Change `CallOptions.Reasoning` field from `*string` to `*ReasoningEffort` in `provider/language_model.go`

## 2. Root aisdk package type definitions

- [x] 2.1 Add `StepType` type and constants (`StepTypeInitial`, `StepTypeToolResult`) in `text.go`; change `StepResult.StepType` field to `StepType`
- [x] 2.2 Add `ToolInvocationState` type and constants (`ToolStateInputStreaming`, `ToolStateInputAvailable`, `ToolStateOutputAvailable`, `ToolStateOutputError`, `ToolStateOutputDenied`) in `message.go`; change `ToolInvocationPart.State` and `DynamicToolUIPart.State` fields to `ToolInvocationState`
- [x] 2.3 Change `Source.SourceType` field in `types.go` to `provider.SourceType`

## 3. Update usage sites in root aisdk package

- [x] 3.1 Update `streamtext.go`: replace inline `"initial"`, `"tool-result"` with `StepTypeInitial`, `StepTypeToolResult`; replace `"url"` SourceType comparison with `provider.SourceTypeURL`
- [x] 3.2 Update `stream.go`: replace inline `"input-available"`, `"output-available"`, `"output-error"` with `ToolStateInputAvailable`, `ToolStateOutputAvailable`, `ToolStateOutputError`
- [x] 3.3 Update `convert.go`: replace inline ToolInvocationState strings with constants; replace `"provider"`/`"function"` in tool conversion with `provider.ToolTypeProvider`/`provider.ToolTypeFunction`; replace Warning construction with typed constants

## 4. Update output package

- [x] 4.1 Update `output/text.go`, `output/object.go`, `output/json.go`, `output/choice.go`, `output/array.go`: replace `"text"`/`"json"` ResponseFormat Type with `provider.ResponseFormatText`/`provider.ResponseFormatJSON`

## 5. Update middleware package

- [x] 5.1 Update `middleware/simulate_streaming.go`: replace inline `GenerateContentPart.Type` strings and `SourceType` strings with typed constants
- [x] 5.2 Update `middleware/extract_reasoning.go`: replace inline `GenerateContentPart.Type` strings with typed constants

## 6. Update anthropic module

- [x] 6.1 Update `anthropic/convert_request.go`: replace `ToolChoice.Type` string comparisons with `ToolChoiceType` constants; replace `ResponseFormat.Type` strings with typed constants; replace `ToolResultContentValue.Type` and `ToolResultOutput.Type` strings with typed constants; replace `GenerateContentPart.Type` strings with typed constants
- [x] 6.2 Update `anthropic/convert_response.go`: replace all `GenerateContentPart{Type: "..."}` constructions with typed constants; replace `SourceType` strings with typed constants
- [x] 6.3 Update `anthropic/convert_stream.go`: replace `SourceType` strings and any `GenerateContentPart.Type` strings with typed constants
- [x] 6.4 Update `anthropic/convert_citations.go`: replace `SourceType` strings with typed `SourceTypeURL`/`SourceTypeDocument` constants
- [x] 6.5 Update `anthropic/reasoning.go`: change reasoning effort map key types from `string` to `ReasoningEffort`; replace `ReasoningNone`/`ReasoningProviderDefault` comparisons (already using constant names, types just change)
- [x] 6.6 Update `anthropic/cache_control.go`: warning constructions use typed constants (may already work since constant names are preserved)

## 7. Update tests

- [x] 7.1 Update `provider/types_test.go`: replace bare string `ToolResultContentValue.Type` values with typed constants
- [x] 7.2 Update `provider/stream_part_test.go`: replace bare string `SourceType` values with typed constants
- [x] 7.3 Update `streamtext_test.go`: replace bare string `StepType` assertions and `Warning.Type` comparisons with typed constants
- [x] 7.4 Update `convert_test.go`: replace bare string `ToolInvocationPart.State` values with typed constants; replace `Warning.Type` comparisons
- [x] 7.5 Update `message_json_test.go`: replace bare string `State` and `SourceType` values with typed constants
- [x] 7.6 Update `http_test.go`: replace bare string `State` and `StepType` values with typed constants
- [x] 7.7 Update `chunk_test.go`: no changes expected (already uses `ChunkType`)
- [x] 7.8 Update `streamtext_output_test.go`: replace bare string `ResponseFormat.Type` with typed constant
- [x] 7.9 Update `anthropic/convert_request_test.go`: replace all bare string `ToolChoice.Type`, `ResponseFormat.Type`, `FunctionTool.Type`, `ProviderTool.Type`, `GenerateContentPart.Type`, `Warning.Type`, and `ToolResultContentValue.Type` values with typed constants
- [x] 7.10 Update `anthropic/convert_response_test.go`: replace bare string `GenerateContentPart.Type` and `SourceType` values with typed constants
- [x] 7.11 Update `anthropic/convert_stream_test.go`: replace bare string `SourceType` and `GenerateContentPart.Type` values with typed constants
- [x] 7.12 Update `anthropic/convert_citations_test.go`: replace bare string `SourceType` values with typed constants
- [x] 7.13 Update `anthropic/reasoning_test.go`: reasoning constant references (may already work since names are preserved, but verify map key types)
- [x] 7.14 Update `anthropic/tool_name_mapping_test.go`: replace bare string `FunctionTool.Type`/`ProviderTool.Type` values with typed constants
- [x] 7.15 Update `middleware/simulate_streaming_test.go`: replace bare string `GenerateContentPart.Type`, `SourceType`, and `Warning.Type` values with typed constants
- [x] 7.16 Update `middleware/extract_reasoning_test.go`: replace bare string `GenerateContentPart.Type` values with typed constants
- [x] 7.17 Update `middleware/composition_test.go`: replace bare string `GenerateContentPart.Type` values with typed constants
- [x] 7.18 Update `middleware/middleware_test.go`: replace bare string `GenerateContentPart.Type` values with typed constants
- [x] 7.19 Update `test/conformance/runner.go`: replace bare string `ResponseFormat.Type` with typed constant

## 8. Verification

- [x] 8.1 Run `make build` to verify both modules compile cleanly
- [x] 8.2 Run `make test` to verify all tests pass
- [x] 8.3 Run `make lint` to verify no lint warnings
