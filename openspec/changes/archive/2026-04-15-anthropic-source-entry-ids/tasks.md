## 1. Provider Struct Change

- [x] 1.1 Add `ID string` field with JSON tag `"id,omitempty"` to `GenerateContentPart` in `provider/language_model.go`

## 2. Streaming Fix

- [x] 2.1 Set `ID: a.generateID()` on each `SourceInfo` in the web search results loop of `emitWebSearchResult` in `anthropic/convert_stream.go`

## 3. Non-Streaming Fix

- [x] 3.1 Copy `src.ID` into `GenerateContentPart.ID` for citation source entries in `convertResponse` in `anthropic/convert_response.go`
- [x] 3.2 Set `ID: generateID()` on web search source `GenerateContentPart` entries in `convertResponse` in `anthropic/convert_response.go`

## 4. Tests

- [x] 4.1 Update or add test assertions for `emitWebSearchResult` verifying `SourceInfo.ID` is non-empty
- [x] 4.2 Update or add test assertions for `convertResponse` citation sources verifying `GenerateContentPart.ID` is non-empty
- [x] 4.3 Update or add test assertions for `convertResponse` web search sources verifying `GenerateContentPart.ID` is non-empty
