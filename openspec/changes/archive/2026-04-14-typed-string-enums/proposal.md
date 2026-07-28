## Why

Multiple fields across `provider` and `aisdk` use bare `string` types where the upstream TypeScript SDK uses string literal unions. This loses compile-time safety that Go's type system can provide via typed string enums. The port already follows this pattern correctly for `StreamPartType` and `ChunkType`, but nine other discriminator groups and one set of reasoning effort constants remain untyped.

## What Changes

- **BREAKING**: Add typed string enum types for all untyped discriminator fields:
  - `ToolChoiceType` for `ToolChoice.Type` (values: auto, none, required, tool)
  - `WarningType` for `Warning.Type` (values: unsupported, compatibility, other)
  - `ResponseFormatType` for `ResponseFormat.Type` (values: text, json)
  - `ToolType` for `FunctionTool.Type` and `ProviderTool.Type` (values: function, provider)
  - `StepType` for `StepResult.StepType` (values: initial, tool-result)
  - `ToolInvocationState` for `ToolInvocationPart.State` and `DynamicToolUIPart.State` (values: input-streaming, input-available, output-available, output-error, output-denied)
  - `SourceType` for `Source.SourceType`, `SourceInfo.SourceType`, and `GenerateContentPart.SourceType` (values: url, document)
  - `ToolResultContentType` for `ToolResultContentValue.Type` (values: text, file-data, file-url, file-reference, image-data, image-url, image-file-reference, custom)
  - `GenerateContentType` for `GenerateContentPart.Type` (values: text, reasoning, tool-call, tool-result, source, file, reasoning-file)
  - `ReasoningEffort` for reasoning effort constants and `CallOptions.Reasoning` field (values: provider-default, none, minimal, low, medium, high, xhigh)
- Existing untyped warning constants (`WarnUnsupported`, `WarnCompatibility`, `WarnOther`) become typed `WarningType`
- All inline string literals at usage sites replaced with named constants
- `CallOptions.Reasoning` field changes from `*string` to `*ReasoningEffort`

## Capabilities

### New Capabilities

- `typed-string-enums`: Typed string enum definitions, constants, and migration of all discriminator fields across `provider` and `aisdk` packages

### Modified Capabilities

- `provider-v4-core-types`: `ToolChoice`, `Warning`, `ResponseFormat`, `ToolResultContentValue`, `GenerateContentPart`, and reasoning effort constants gain typed discriminator fields
- `v4-tool-type-split`: `FunctionTool.Type` and `ProviderTool.Type` change from bare `string` to `ToolType`

## Impact

- **provider package**: Type definitions for `ToolChoice`, `Warning`, `ResponseFormat`, `FunctionTool`, `ProviderTool`, `ToolResultContentValue`, `GenerateContentPart`, `SourceInfo`, `CallOptions` all change
- **aisdk (root) package**: `StepResult`, `Source`, `ToolInvocationPart`, `DynamicToolUIPart` struct fields change type
- **anthropic module**: All usage sites of these fields in `convert_request.go`, `convert_response.go`, `convert_stream.go`, `convert_citations.go`, `reasoning.go`, `cache_control.go`, and their tests need updating
- **middleware package**: `simulate_streaming.go`, `extract_reasoning.go` and tests
- **output package**: `text.go`, `object.go`, `json.go`, `choice.go`, `array.go` ResponseFormat construction
- **test/conformance**: `runner.go` ResponseFormat construction
- **Wire compatibility**: JSON serialization is unaffected -- typed strings marshal identically to bare strings
- **Breaking for callers**: Code using raw string literals (e.g., `ToolChoice{Type: "auto"}`) must switch to constants. Migration is mechanical.
