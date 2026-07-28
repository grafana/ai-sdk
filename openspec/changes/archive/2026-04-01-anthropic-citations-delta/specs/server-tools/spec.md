## ADDED Requirements

### Requirement: citations_delta streaming

The Anthropic stream adapter SHALL handle `citations_delta` events in `BetaRawContentBlockDeltaEvent` by converting each citation to a `PartSource` stream part with appropriate `SourceInfo`. The conversion SHALL dispatch on the citation's concrete type via `.AsAny()` and handle `web_search_result_location`, `page_location`, and `char_location` variants. Unknown citation types SHALL be silently skipped.

#### Scenario: web_search_result_location citation delta

- **WHEN** a `citations_delta` event arrives with a `BetaCitationsWebSearchResultLocation` citation containing `URL`, `Title`, `CitedText`, and `EncryptedIndex`
- **THEN** the adapter emits a `PartSource` with `SourceInfo{SourceType: "url", URL: citation.URL, Title: citation.Title}` and `ProviderMetadata{"anthropic": {"citedText": citation.CitedText, "encryptedIndex": citation.EncryptedIndex}}`

#### Scenario: page_location citation delta with tracked document

- **WHEN** a `citations_delta` event arrives with a `BetaCitationPageLocation` citation referencing `DocumentIndex: 0` and the adapter's `citationDocuments` has a document at index 0 with `Title: "Report"`, `MediaType: "application/pdf"`, `Filename: "report.pdf"`
- **THEN** the adapter emits a `PartSource` with `SourceInfo{SourceType: "document", MediaType: "application/pdf", Title: "Report", Filename: "report.pdf"}` and `ProviderMetadata{"anthropic": {"citedText": citation.CitedText, "startPageNumber": citation.StartPageNumber, "endPageNumber": citation.EndPageNumber}}`

#### Scenario: page_location citation delta uses document title from citation when available

- **WHEN** a `citations_delta` event arrives with a `BetaCitationPageLocation` citation that has a non-empty `DocumentTitle` field
- **THEN** the adapter uses `citation.DocumentTitle` as the source `Title`, overriding the tracked document's title

#### Scenario: char_location citation delta with tracked document

- **WHEN** a `citations_delta` event arrives with a `BetaCitationCharLocation` citation referencing a valid `DocumentIndex` in the adapter's `citationDocuments`
- **THEN** the adapter emits a `PartSource` with `SourceInfo{SourceType: "document"}` using the tracked document's `MediaType` and `Filename`, and `ProviderMetadata{"anthropic": {"citedText": citation.CitedText, "startCharIndex": citation.StartCharIndex, "endCharIndex": citation.EndCharIndex}}`

#### Scenario: document-based citation with out-of-range index

- **WHEN** a `citations_delta` event arrives with a `page_location` or `char_location` citation whose `DocumentIndex` exceeds the length of `citationDocuments`
- **THEN** the adapter silently skips the citation without emitting a `PartSource`

#### Scenario: unknown citation type silently skipped

- **WHEN** a `citations_delta` event arrives with a citation type not matching `web_search_result_location`, `page_location`, or `char_location` (e.g., `content_block_location` or `search_result_location`)
- **THEN** no `PartSource` is emitted and no error is produced

### Requirement: Citation document tracking

The Anthropic stream adapter and response converter SHALL track citation documents extracted from the prompt's user messages. The extraction SHALL collect file content parts with `MediaType` of `application/pdf` or `text/plain` that have `providerOptions["anthropic"]["citations"]["enabled"]` set to `true`. Each tracked document SHALL record `title` (from filename, defaulting to `"Untitled Document"`), `filename`, and `mediaType`. The documents SHALL be tracked in prompt order to match Anthropic's document indexing.

#### Scenario: PDF file part with citations enabled

- **WHEN** the prompt contains a user message with a `FileContentPart` having `MediaType: "application/pdf"`, `Filename: "report.pdf"`, and provider metadata `{"anthropic": {"citations": {"enabled": true}}}`
- **THEN** the citation documents list includes `{title: "report.pdf", filename: "report.pdf", mediaType: "application/pdf"}`

#### Scenario: Text file part with citations enabled

- **WHEN** the prompt contains a user message with a `FileContentPart` having `MediaType: "text/plain"`, `Filename: "notes.txt"`, and provider metadata `{"anthropic": {"citations": {"enabled": true}}}`
- **THEN** the citation documents list includes `{title: "notes.txt", filename: "notes.txt", mediaType: "text/plain"}`

#### Scenario: File part without citations enabled is excluded

- **WHEN** the prompt contains a user message with a `FileContentPart` having `MediaType: "application/pdf"` but no `citations.enabled` in provider metadata
- **THEN** the file is NOT included in the citation documents list

#### Scenario: File part with non-citation media type is excluded

- **WHEN** the prompt contains a user message with a `FileContentPart` having `MediaType: "image/png"` and `citations.enabled: true`
- **THEN** the file is NOT included in the citation documents list

#### Scenario: File part without filename defaults title

- **WHEN** the prompt contains a user message with a `FileContentPart` having `MediaType: "application/pdf"`, no `Filename`, and `citations.enabled: true`
- **THEN** the citation documents list includes an entry with `title: "Untitled Document"` and empty `filename`

### Requirement: Text block citation handling in non-streaming responses

The Anthropic response converter SHALL process the `Citations` array on text content blocks in non-streaming responses. For each citation, it SHALL append a `GenerateContentPart` with `Type: "source"` using the same citation-to-source conversion logic as the streaming path. Unknown citation types SHALL be silently skipped.

#### Scenario: Non-streaming text block with web search citations

- **WHEN** `convertResponse()` encounters a text content block with a `Citations` array containing `web_search_result_location` entries
- **THEN** it appends a text `GenerateContentPart` followed by source `GenerateContentPart` entries with `Type: "source"`, `SourceType: "url"`, `URL`, and `Title` from each citation

#### Scenario: Non-streaming text block with document citations

- **WHEN** `convertResponse()` encounters a text content block with `page_location` or `char_location` citations and `citationDocuments` is populated
- **THEN** it appends source `GenerateContentPart` entries with `Type: "source"`, `SourceType: "document"`, resolved `MediaType`, `Title`, and `Filename` from the tracked documents

#### Scenario: Non-streaming text block with no citations

- **WHEN** `convertResponse()` encounters a text content block with an empty `Citations` array
- **THEN** only the text `GenerateContentPart` is appended, no source entries

### Requirement: Citation source ID generation

Each `PartSource` (streaming) and source `GenerateContentPart` (non-streaming) emitted from citation conversion SHALL include a unique `ID` generated by the model's ID generator. This ID SHALL be set on `SourceInfo.ID` (streaming) or `GenerateContentPart.SourceID` (non-streaming).

#### Scenario: Each citation source gets a unique ID

- **WHEN** two `citations_delta` events arrive in sequence
- **THEN** each emitted `PartSource` has a distinct `SourceInfo.ID` value
