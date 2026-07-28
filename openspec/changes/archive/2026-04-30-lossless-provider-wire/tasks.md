## 1. Provider package: typed enums and flat structs

- [x] 1.1 Define `ContentPartType` typed string in `provider/content.go` with constants for every part variant (`text`, `file`, `reasoning`, `reasoning-file`, `tool-call`, `tool-result`, `custom`, `tool-approval-response`).
- [x] 1.2 Replace the three sealed interfaces (`UserContentPart`, `AssistantContentPart`, `ToolMessageContentPart`) and all eight concrete content-part types with a single flat `ContentPart` struct in `provider/content.go` per the design's D1 shape.
- [x] 1.3 Replace `Message` sealed interface plus `SystemMessage`/`UserMessage`/`AssistantMessage`/`ToolMessage` with a flat `Message{Role, Content []ContentPart, ProviderOptions}` struct in `provider/message.go`.
- [x] 1.4 Update `NewSystemMessage`/`NewUserMessage`/`NewAssistantMessage`/`NewToolMessage` constructors to return the flat `Message`. `NewSystemMessage(text)` packs the string into `Content: []ContentPart{{Type: ContentPartTypeText, Text: text}}`.
- [x] 1.5 Replace `Tool` sealed interface plus `FunctionTool`/`ProviderTool` concrete types with a flat `Tool` struct in `provider/language_model.go` carrying both function-tool and provider-tool fields (Name, Description, InputSchema, InputExamples, Strict, ID, Args, ProviderOptions) discriminated by `Type`.
- [x] 1.6 Promote `ProviderOptions` to a named alias `type ProviderOptions = map[string]ProviderOption` (or a non-alias named type if needed for method receiver) in `provider/provider_option.go`.
- [x] 1.7 Implement `MarshalJSON` and `UnmarshalJSON` on the named `ProviderOptions` type. Marshal serializes each `ProviderOption` value via `json.Marshal`; Unmarshal wraps every entry as `RawProviderOption{Key, Raw}`.
- [x] 1.8 Update every `ProviderOptions` field across the provider package to use the named alias and the JSON tag `json:"providerOptions,omitempty"` (replacing `json:"-"`).
- [x] 1.9 Update `CallOptions.Prompt` to use `json:"prompt,omitempty"` (replacing `json:"-"`); confirm `Tools` uses `json:"tools,omitempty"` with `[]Tool` (the flat struct).
- [x] 1.10 Drop `StreamPart.Error error` field; add `APICallError *APICallError` field with `json:"apiCallError,omitempty"`. Add `json:"fileData,omitempty"` tag to `StreamPart.FileData`.
- [x] 1.11 Refactor `APICallError` in `provider/api_call_error.go`: promote `message`/`cause` to exported `Message string` (with `json:"message"`); keep `cause error` unexported for in-process Unwrap; add JSON tags to all wire fields per design D3; change `RequestBodyValues` from `any` to `json.RawMessage`.
- [x] 1.12 Update `NewAPICallError` and `APICallErrorOptions` to accept `RequestBodyValues json.RawMessage` (or add a typed-value helper that marshals).
- [x] 1.13 Update or add compile-time interface checks where applicable (e.g. `var _ error = (*APICallError)(nil)`).
- [x] 1.14 Add per-variant `ContentPart` constructor helpers in `provider/content.go`: `TextPart`, `FilePart`, `ReasoningPart`, `ReasoningFilePart`, `ToolCallPart`, `ToolResultPart`, `CustomPart`, `ToolApprovalResponsePart`.
- [x] 1.15 Add role-text shortcut helpers in `provider/message.go`: `UserText(text) Message`, `AssistantText(text) Message`. Mark the existing `TextParts` helper as deprecated in favor of `TextPart`.

## 2. Provider package tests

- [x] 2.1 Update `provider/message_test.go`: remove sealed-interface assertions; add Message struct construction and round-trip JSON tests for every role.
- [x] 2.2 Update `provider/stream_part_test.go`: cover the new `APICallError` field on PartError; cover `FileData` JSON round-trip.
- [x] 2.3 Update `provider/types_test.go`: cover `ToolResultOutput` and `ToolResultContentValue` `ProviderOptions` JSON round-trip.
- [x] 2.4 Update `provider/api_call_error_test.go`: cover JSON marshal/unmarshal preserves `IsRetryable`, `StatusCode`, `Message`, `ResponseBody`, `Data`, `RequestBodyValues`; cover `cause` lost across the wire but `Unwrap` still works in-process.
- [x] 2.5 Update `provider/provider_option_test.go`: cover named `ProviderOptions` MarshalJSON for typed values, MarshalJSON for `RawProviderOption`, UnmarshalJSON wraps every entry as `RawProviderOption`, `ResolveOption[T]` round-trips.
- [x] 2.6 Add `provider/content_test.go` (if not present) covering flat `ContentPart` round-trip for every `ContentPartType` value.
- [x] 2.7 Add a wire-shape table-driven test asserting `CallOptions` JSON round-trip preserves every populated field for a representative sample (text + file + reasoning + tool-call + tool-result content; tools; reasoning effort; response format; provider options).
- [x] 2.8 Add `provider/helpers_test.go` covering each new constructor helper (`TextPart`, `FilePart`, `ReasoningPart`, `ReasoningFilePart`, `ToolCallPart`, `ToolResultPart`, `CustomPart`, `ToolApprovalResponsePart`, `UserText`, `AssistantText`) plus a compose-and-round-trip test that wires several helpers through `NewAssistantMessage` and `encoding/json`.

## 3. Root aisdk package: orchestration callers

- [x] 3.1 Update `convert.go` (`ConvertToModelMessages` and helpers) to construct flat `provider.Message` and `provider.ContentPart` values instead of the removed concrete types.
- [x] 3.2 Update `streamtext.go` everywhere it pattern-matches on `provider.Message` variants or `*ContentPart` types — switch dispatch to `msg.Role` and `cp.Type`.
- [x] 3.3 Update `text.go` and `tool.go` for the unified `provider.Tool` and `provider.ContentPart`; replace `provider.FunctionTool{...}` constructors with `provider.Tool{Type: ToolTypeFunction, ...}`.
- [x] 3.4 Update `chunk.go` if it constructs or matches on provider messages/content parts.
- [x] 3.5 Update `message.go` (UI message types) where it bridges to `provider.Message` via `ConvertToModelMessages`; the UI layer types themselves are unaffected, only the conversion output changes.
- [x] 3.6 Update `message_json.go` only where it touches provider-layer types (UI-layer JSON unaffected).
- [x] 3.7 Update `tool.go` (`toolSetToProviderTools`) per the orchestration-layer-tool-conversion requirement in `v4-tool-type-split` spec delta.
- [x] 3.8 Update `output/`, `middleware/`, `fallback/`, `registry/`, `schema/` files for new types (mostly mechanical).

## 4. Root aisdk package: tests

- [x] 4.1 Update `convert_test.go` for flat `Message`/`ContentPart` construction.
- [x] 4.2 Update `streamtext_test.go` and `streamtext_output_test.go` for new types in stream parts and tool calls.
- [x] 4.3 Update `stream_test.go` for the `APICallError` field on PartError.
- [x] 4.4 Update `text.go` test files for the unified `Tool` and `ContentPart`.
- [x] 4.5 Update `chunk_test.go`, `message_json_test.go`, `typed_tool_test.go`, `http_test.go` as needed.
- [x] 4.6 Update `integration_test.go` for new types.
- [x] 4.7 Update `retry_test.go` and `retry_integration_test.go` for the `APICallError` shape.

## 5. Anthropic provider

- [x] 5.1 Update `anthropic/convert_request.go`: `convertSystemContent`/`convertUserContent`/`convertAssistantContent`/`convertToolContent` switch to dispatching on `cp.Type` instead of type-asserting concrete `*ContentPart` types.
- [x] 5.2 Update `anthropic/convert_response.go` for new `provider.GenerateContentPart` construction (already flat) and new flat `Message`/`ContentPart` construction in any helpers.
- [x] 5.3 Update `anthropic/convert_stream.go`: emit `StreamPart{Type: PartError, APICallError: ...}` instead of populating the removed `Error error` field.
- [x] 5.4 Update `anthropic/wrap_api_error.go` for the new exported `APICallError` fields and the `RequestBodyValues json.RawMessage` change.
- [x] 5.5 Update `anthropic/cache_control.go`, `anthropic/reasoning.go`, `anthropic/options.go`, `anthropic/tool_name_mapping.go`, `anthropic/convert_citations.go` for new types where they construct/inspect provider types.
- [x] 5.6 Update all anthropic test files (`*_test.go`) for the flat type shapes.
- [x] 5.7 Run `make test` (root + anthropic) and confirm green.

## 6. Wire package

- [x] 6.1 Delete the empty `provider/wire/proto/` and `provider/wire/wirepb/wirepbconnect/` placeholder directories. Remove any references in `Makefile` or scripts.
- [x] 6.2 Create `provider/wire/doc.go` with package-level overview describing the JSON+SSE wire and its goal.
- [x] 6.3 Create `provider/wire/routes.go` with exported constants: `PathLanguageModel`, `HeaderModelID`, `HeaderStreaming`, `HeaderSpecVersion`.
- [x] 6.4 Create `provider/wire/request.go` with `EncodeCallOptions` and `DecodeCallOptions` helpers.
- [x] 6.5 Create `provider/wire/response.go` with `EncodeGenerateResult` and `DecodeGenerateResult` helpers.
- [x] 6.6 Create `provider/wire/sse.go` with `WriteSSEStreamPart(io.Writer, provider.StreamPart) error` and `ReadSSEStreamPart(*bufio.Reader) (provider.StreamPart, error)` (or scanner equivalent), implementing the `data: <JSON>\n\n` framing.
- [x] 6.7 Create `provider/wire/errors.go` with `EncodeAPICallError`, `DecodeAPICallError`, and a small helper for writing/reading HTTP error envelopes.

## 7. Wire round-trip tests

- [x] 7.1 Add `provider/wire/sse_test.go` with a round-trip test per `StreamPartType` (every constant in `provider/stream_part.go`).
- [x] 7.2 Add `provider/wire/request_test.go` with round-trip tests per notable `CallOptions` field (`Prompt`, `Tools`, `ToolChoice`, `MaxOutputTokens`, `Temperature`, `TopP`, `TopK`, `PresencePenalty`, `FrequencyPenalty`, `StopSequences`, `ResponseFormat`, `Seed`, `Reasoning`, `IncludeRawChunks`, `Headers`, `ProviderOptions`).
- [x] 7.3 Add `provider/wire/response_test.go` with round-trip tests for `GenerateResult` carrying every `ContentPartType` and full `Usage`/`FinishReason`/`ProviderMetadata`/`Warnings`/`Request`/`Response` populated.
- [x] 7.4 Add `provider/wire/errors_test.go` with round-trip tests for every `APICallError` field, including retryable and non-retryable variants.
- [x] 7.5 Add per-`ContentPartType` round-trip test verifying the flat `ContentPart` survives JSON for every defined type.
- [x] 7.6 Add per-`ToolType` round-trip test (`ToolTypeFunction`, `ToolTypeProvider`).

## 8. Conformance suite

- [x] 8.1 Run `test/conformance/anthropic/conformance_test.go` (and any other conformance harness in `test/conformance/`) end-to-end.
- [x] 8.2 Confirm every fixture's output is byte-identical against `expected.jsonl` files (no UI-layer SSE format change is intended).
- [x] 8.3 If any fixture diverges, investigate and fix the regression — divergence indicates a behavioral bug introduced by the refactor, not a fixture-update need.

## 9. Cross-package verification

- [x] 9.1 Run `make fmt` and confirm no formatting churn introduced beyond intended changes.
- [x] 9.2 Run `make vet` (root + anthropic) and confirm clean.
- [x] 9.3 Run `make lint` and confirm clean.
- [x] 9.4 Run `make test` and confirm all unit + integration tests pass.
- [x] 9.5 Run `make test-short` to confirm short tests still split correctly.
- [x] 9.6 Run `make check` for the full fmt + vet + test sweep.

## 10. Documentation

- [x] 10.1 Update `provider/doc.go` if it references the removed sealed-interface types.
- [x] 10.2 Update `README.md` provider section if it shows `SystemMessage{...}` / `FunctionTool{...}` construction examples.
- [x] 10.3 Add a short note in `provider/wire/doc.go` linking to upstream Vercel `gateway-language-model.ts` as the reference shape.
- [x] 10.4 Document the typed-vs-`RawProviderOption` round-trip asymmetry on the `ProviderOptions` named type's doc comment.
