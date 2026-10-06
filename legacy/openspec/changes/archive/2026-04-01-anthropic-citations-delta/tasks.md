## 1. Citation document extraction

- [x] 1.1 Add `citationDocument` struct and `extractCitationDocuments` function in the anthropic module. Scans prompt user messages for `FileContentPart` with `MediaType` of `application/pdf` or `text/plain` and `providerOptions.anthropic.citations.enabled = true`. Returns `[]citationDocument` in prompt order.
- [x] 1.2 Add unit tests for `extractCitationDocuments`: PDF with citations enabled, text with citations enabled, file without citations excluded, non-citation media type excluded, missing filename defaults to "Untitled Document", multiple documents preserve order.

## 2. Citation-to-source conversion

- [x] 2.1 Add `createCitationSource` helper function that takes a citation (via `AsAny()` result from either `BetaCitationsDeltaCitationUnion` or `BetaTextCitationUnion`), `citationDocuments` slice, and an ID generator. Returns `(*provider.SourceInfo, bool)`. Handles `web_search_result_location`, `page_location`, `char_location`; skips unknown types.
- [x] 2.2 Add unit tests for `createCitationSource`: web search citation with URL/title/metadata, page location with valid document index, char location with valid document index, document title override from citation, out-of-range document index returns false, unknown citation type returns false.

## 3. Streaming citations_delta handling

- [x] 3.1 Add `citationDocuments` field and `idGenerator` to `streamAdapter` struct. Wire them through from the model's `DoStream` call, calling `extractCitationDocuments` on the prompt before constructing the adapter.
- [x] 3.2 Add `citations_delta` case in the `BetaRawContentBlockDeltaEvent` switch in `handleEvent`. Extract citation via `delta.AsCitationsDelta()`, call `createCitationSource`, emit `PartSource` if conversion succeeds.
- [x] 3.3 Add streaming integration tests: web search result location citation emits `PartSource` with `SourceType: "url"`, page/char location citations emit `PartSource` with `SourceType: "document"`, unknown citation type produces no output, out-of-range document index produces no output.

## 4. Non-streaming citation handling

- [x] 4.1 Update `convertResponse` to accept `citationDocuments` slice and ID generator. After appending a text `GenerateContentPart`, iterate over `block.Citations` and append source `GenerateContentPart` entries using `createCitationSource`.
- [x] 4.2 Wire citation document extraction in `DoGenerate`, passing the extracted documents and ID generator to `convertResponse`.
- [x] 4.3 Add non-streaming tests: text block with web search citations produces source entries, text block with document citations produces source entries, text block with no citations produces no extra entries.

## 5. Verification

- [x] 5.1 Run full test suite (`make test`) and verify all existing tests still pass.
- [x] 5.2 Run `make lint` and fix any issues.
