## 1. Provider Tool Type Split

- [x] 1.1 Define `Tool` sealed interface with `tool()` marker method, `FunctionTool` struct, and `ProviderTool` struct in `provider/language_model.go`. Remove the old `Tool` struct. Add `InputExample` wrapper type.
- [x] 1.2 Update `CallOptions.Tools` from `[]Tool` (struct) to `[]Tool` (interface)
- [x] 1.3 Add compile-time interface satisfaction checks in `provider/language_model_test.go`
- [x] 1.4 Update `toolSetToProviderTools` in `convert.go` to construct `provider.FunctionTool` and `provider.ProviderTool` values, wrapping `InputExamples` in `InputExample` structs
- [x] 1.5 Update `convert_test.go` -- all `provider.Tool{...}` literals to use `provider.FunctionTool{...}` or `provider.ProviderTool{...}`

## 2. Anthropic Provider Tool Handling

- [x] 2.1 Update `convertTools()` in `anthropic/convert_request.go` to accept `[]provider.Tool` (interface) and type-switch on `provider.FunctionTool`/`provider.ProviderTool`
- [x] 2.2 Update `convertProviderDefinedTool()` signature to accept `provider.ProviderTool` instead of `provider.Tool`
- [x] 2.3 Update `hasFunctionTools()` to use type switch on `provider.FunctionTool`
- [x] 2.4 Update `newToolNameMapping()` in `anthropic/tool_name_mapping.go` to accept `[]provider.Tool` (interface)
- [x] 2.5 Update InputExamples handling in `convertTools` to unwrap `InputExample.Input` field
- [x] 2.6 Remove `"provider-defined"` string comparisons throughout `anthropic/` -- replace with type switches
- [x] 2.7 Update all `anthropic/convert_request_test.go` tool literals from `provider.Tool{...}` to appropriate concrete types
- [x] 2.8 Update `anthropic/convert_response_test.go` and `anthropic/convert_stream_test.go` tool literals
- [x] 2.9 Update `anthropic/tool_name_mapping_test.go` tool literals

## 3. Source Document Variant

- [x] 3.1 Verify `SourceInfo` struct in `provider/stream_part.go` already has the required fields (`MediaType`, `Title`, `Filename`) for the document variant -- no struct changes needed, just document the new `sourceType: "document"` usage
- [x] 3.2 Add tests for document source variant construction and serialization

## 4. StreamPart Preliminary Field

- [x] 4.1 Add `Preliminary *bool` field to `StreamPart` in `provider/stream_part.go`
- [x] 4.2 Add tests verifying Preliminary field on PartToolResult stream parts

## 5. ToolResultContentValue Expansion

- [x] 5.1 Rename `FileID json.RawMessage` to `ProviderReference map[string]string` in `ToolResultContentValue` in `provider/types.go`, update json tag to `"providerReference,omitempty"`
- [x] 5.2 Add type constant comments or documentation for the new type values: `file-data`, `file-url`, `file-reference`, `image-data`, `image-url`, `image-file-reference`, `custom`
- [x] 5.3 Update any existing references to `FileID` across the codebase
- [x] 5.4 Add tests for new content type construction and serialization

## 6. Stream Part ID Verification

- [x] 6.1 Audit Anthropic provider stream conversion (`anthropic/convert_stream.go`) to verify `StreamPart.ID` is set for text start/delta/end, reasoning start/delta/end, and tool-input start/delta/end parts
- [x] 6.2 Add or update tests in `anthropic/convert_stream_test.go` verifying ID population for all relevant part types

## 7. Integration & Orchestration

- [x] 7.1 Update `streamtext.go` `filterProviderTools` to handle `provider.Tool` interface
- [x] 7.2 Update `integration_test.go` tool references from `"provider-defined"` to use new types
- [x] 7.3 Run `make check` -- fix any remaining compilation errors and test failures
