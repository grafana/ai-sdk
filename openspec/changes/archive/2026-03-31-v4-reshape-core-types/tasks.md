## 1. Usage type restructure

- [x] 1.1 Replace `Usage`, `InputTokenDetails`, `OutputTokenDetails` in `provider/types.go` with `Usage`, `InputTokenUsage`, `OutputTokenUsage` matching V4 shape. Delete `InputTokenDetails` and `OutputTokenDetails`. Remove `TotalTokens`.
- [x] 1.2 Update `provider/types_test.go` for the new Usage shape (zero-value assertions, JSON serialization).
- [x] 1.3 Update Anthropic non-streaming Usage construction in `anthropic/convert_response.go` to use nested `InputTokenUsage`/`OutputTokenUsage`.
- [x] 1.4 Update Anthropic streaming Usage construction in `anthropic/convert_stream.go` (message_start and message_delta handlers) to use nested structs. Migrate `InputTokenDetails` cache fields to `InputTokenUsage.CacheRead`/`CacheWrite`.
- [x] 1.5 Update `anthropic/convert_stream_test.go` assertions for the new Usage/InputTokenUsage field names.
- [x] 1.6 Update `aggregateUsage` in `streamtext.go` to sum `InputTokens.Total` and `OutputTokens.Total` across steps, handling nil `*int` fields.
- [x] 1.7 Update all root-package references to `Usage` fields (streamtext.go step handling, StepResult, StreamFinishStep, GenerateTextResult accessors).

## 2. FinishReason from string to struct

- [x] 2.1 Add `UnifiedFinishReason` type and constants in `provider/types.go`. Replace `type FinishReason string` with `type FinishReason struct { Unified UnifiedFinishReason; Raw string }` with appropriate JSON tags.
- [x] 2.2 Remove `RawFinishReason` field from `StreamPart` in `provider/stream_part.go`.
- [x] 2.3 Update `mapFinishReason` in `anthropic/convert_response.go` to return `FinishReason` struct (accepting raw string as parameter). Update `convert_stream.go` PartFinish construction to use the struct and drop `RawFinishReason`.
- [x] 2.4 Update `anthropic/convert_response_test.go` table test for `mapFinishReason` to assert struct values.
- [x] 2.5 Remove `RawFinishReason` field from `StreamFinishStep`, `StreamFinish`, and `StepResult` in root package. Update `streamtext.go` to read `part.FinishReason.Raw` instead of `part.RawFinishReason`.
- [x] 2.6 Update `streamtext_test.go` `TestStreamTextRawFinishReasonPropagation` to verify raw reason flows through `FinishReason.Raw`.
- [x] 2.7 Ensure `UIMessageChunk.FinishReason` (string) is populated from `FinishReason.Unified` in `streamtext.go` chunk conversion. Verify SSE wire format is unchanged.

## 3. Metadata to ProviderMetadata rename

- [x] 3.1 Rename `type Metadata map[string]json.RawMessage` to `type ProviderMetadata map[string]json.RawMessage` in `provider/types.go`.
- [x] 3.2 Update all references to the `Metadata` type across `provider/` package (stream_part.go, language_model.go fields).
- [x] 3.3 Rename `GenerateResult.Metadata` field to `GenerateResult.ProviderMetadata` in `provider/language_model.go`.
- [x] 3.4 Update all references to `provider.Metadata` type in the root package (textstream.go, streamtext.go, text.go, tool.go, types.go, message.go, chunk.go, convert.go, stream.go).
- [x] 3.5 Update all references to `provider.Metadata` type in `anthropic/` module.
- [x] 3.6 Update test files referencing the `Metadata` type (streamtext_test.go, convert_test.go, anthropic test files).

## 4. ResponseMetadata slimmed + GenerateResponse

- [x] 4.1 Slim `provider.ResponseMetadata` in `provider/language_model.go` to `{ID, ModelID, Timestamp}`. Remove `Headers` and `Body` fields.
- [x] 4.2 Create `GenerateResponse` type in `provider/language_model.go` that embeds `ResponseMetadata` and adds `Headers map[string]string` and `Body json.RawMessage`.
- [x] 4.3 Change `GenerateResult.Response` from `*ResponseMetadata` to `*GenerateResponse`.
- [x] 4.4 Update Anthropic `convert_response.go` to construct `GenerateResponse` for `GenerateResult.Response`.
- [x] 4.5 Update `streamtext.go` response metadata handling -- the provider-level `ResponseMetadata` is now slim; ensure headers flow into the right place.
- [x] 4.6 Update any test mocks constructing `GenerateResult` with `Response` field.

## 5. specificationVersion bump

- [x] 5.1 Change `const specVersion = "v3"` to `"v4"` in `anthropic/model.go`.
- [x] 5.2 Update all test mocks returning `"v3"` from `SpecificationVersion()` to return `"v4"` (streamtext_test.go, streamtext_output_test.go, output/value_test.go, fallback/fallback_test.go, integration test servers).
- [x] 5.3 Update the `fallback_test.go` assertion that checks `SpecificationVersion() == "v3"` to check `"v4"`.

## 6. Verification

- [x] 6.1 Run `make build` -- both modules must compile.
- [x] 6.2 Run `make test` -- all tests must pass.
- [x] 6.3 Run `make lint` -- no lint errors.
