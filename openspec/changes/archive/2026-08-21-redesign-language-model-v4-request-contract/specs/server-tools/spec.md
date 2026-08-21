## MODIFIED Requirements

### Requirement: Citation document tracking

The Anthropic stream adapter and response converter SHALL track citation documents extracted from prompt user messages. Extraction SHALL collect file-typed `provider.ContentPart` values with `MediaType` of `application/pdf` or `text/plain` and `providerOptions["anthropic"]["citations"]["enabled"] == true`.

Because these values are provider requests, citation tracking SHALL read filename presence only from `FilePartFilename`; it SHALL NOT read the generated-response/source `Filename` field. A non-nil request filename supplies the tracked filename. Nil or explicit-empty filename uses the existing `"Untitled Document"` title fallback and an empty tracked filename. Mixed request/response filename state SHALL be rejected by the Anthropic request boundary before citation extraction is used.

Each tracked document SHALL record title, filename, and media type in prompt order to match Anthropic document indexing. Citation sources generated in responses SHALL continue to write `SourceInfo.Filename` and response/source `ContentPart.Filename`; this change SHALL NOT move source filenames into `FilePartFilename`.

#### Scenario: PDF file part with citations enabled

- **WHEN** the prompt contains a file `ContentPart` with `MediaType: "application/pdf"`, `FilePartFilename` pointing to `"report.pdf"`, and provider options `{"anthropic":{"citations":{"enabled":true}}}`
- **THEN** the citation documents list SHALL include `{title: "report.pdf", filename: "report.pdf", mediaType: "application/pdf"}`

#### Scenario: Text file part with citations enabled

- **WHEN** the prompt contains a file `ContentPart` with `MediaType: "text/plain"`, `FilePartFilename` pointing to `"notes.txt"`, and citations enabled
- **THEN** the citation documents list SHALL include `{title: "notes.txt", filename: "notes.txt", mediaType: "text/plain"}`

#### Scenario: File part without citations enabled is excluded

- **WHEN** a prompt file has `MediaType: "application/pdf"` but no enabled Anthropic citations option
- **THEN** it SHALL NOT be included in the citation documents list

#### Scenario: File part with non-citation media type is excluded

- **WHEN** a prompt file has `MediaType: "image/png"` and citations enabled
- **THEN** it SHALL NOT be included in the citation documents list

#### Scenario: File part without filename defaults title

- **WHEN** a citation-enabled prompt file has nil `FilePartFilename`
- **THEN** the citation document SHALL use title `"Untitled Document"` and an empty filename

#### Scenario: Explicit empty request filename uses fallback

- **WHEN** a citation-enabled prompt file has `FilePartFilename` pointing to `""`
- **THEN** the citation document SHALL use title `"Untitled Document"` and an empty filename
- **AND** request mapping SHALL still preserve the explicit-empty filename member

#### Scenario: Source filenames remain response-owned

- **WHEN** citation conversion emits a document source with a tracked non-empty filename
- **THEN** the source SHALL populate `SourceInfo.Filename` and response/source `ContentPart.Filename`
- **AND** it SHALL NOT populate `FilePartFilename`
