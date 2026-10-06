## Why

The upstream Vercel AI SDK LanguageModelV4 splits the single `Tool` type into distinct `FunctionTool` and `ProviderTool` types, adds a `document` source variant, expands tool result content types, and introduces a `preliminary` field for tool results. Our Go port still uses a single `Tool` struct with a mixed bag of fields for both tool kinds, lacks the document source variant, and is missing the newer tool result content sub-types. This is the final PR (3 of 3) in the V3-to-V4 migration tracked in #32.

## What Changes

- **BREAKING** -- Split `provider.Tool` into `FunctionTool` and `ProviderTool` types behind a sealed `Tool` interface. The `"provider-defined"` type value is renamed to `"provider"`. `ProviderOptions` is removed from provider tools (only function tools keep it). `InputExamples` changes from `[]json.RawMessage` to `[]InputExample` (wrapping input in objects).
- Add `sourceType: "document"` variant to `SourceInfo` with `MediaType`, `Title` (required), and optional `Filename` fields.
- Add `Preliminary *bool` field to `StreamPart` for intermediate tool results that get replaced.
- Expand `ToolResultContentValue` with new sub-types: `file-data`, `file-url`, `file-reference`, `image-data`, `image-url`, `image-file-reference`, and `custom`.
- Verify `StreamPart.ID` is populated for all text/reasoning/tool-input start/delta/end parts in the Anthropic provider.

## Capabilities

### New Capabilities
- `v4-tool-type-split`: Split single Tool struct into FunctionTool and ProviderTool with sealed interface, including type value rename and field restructuring
- `v4-source-document`: Add document variant to SourceInfo alongside existing URL variant
- `v4-tool-result-alignment`: Add preliminary field to StreamPart and expand ToolResultContentValue with new content sub-types; verify stream part ID population

### Modified Capabilities
- `server-tools`: Tool type value changes from `"provider-defined"` to `"provider"`, and the Anthropic provider's tool conversion must handle the new FunctionTool/ProviderTool types instead of a single Tool struct

## Impact

- `provider/language_model.go` -- Tool type split: remove `Tool` struct, add `FunctionTool`, `ProviderTool`, sealed `Tool` interface
- `provider/stream_part.go` -- SourceInfo document variant, StreamPart.Preliminary field
- `provider/types.go` -- ToolResultContentValue sub-type expansion
- `anthropic/convert_request.go` -- Update tool conversion from single struct to interface dispatch; rename `"provider-defined"` checks to `"provider"`
- `anthropic/convert_request_test.go` -- Update all tool construction and assertions
- Root orchestration (`tool.go`, `convert.go`, `convert_test.go`) -- Update tool construction to use new types
- `integration_test.go` -- Update provider tool references
- Tests across all affected packages
