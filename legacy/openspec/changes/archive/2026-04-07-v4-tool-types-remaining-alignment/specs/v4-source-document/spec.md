## ADDED Requirements

### Requirement: SourceInfo document variant

The `SourceInfo` struct SHALL support a `sourceType: "document"` variant alongside the existing `"url"` variant. The document variant uses the following fields:
- `SourceType` -- `"document"` (required)
- `ID` -- source identifier (required)
- `MediaType` -- IANA media type of the document, e.g. `"application/pdf"` (required for document)
- `Title` -- document title (required for document)
- `Filename` -- optional filename

The existing fields `URL` and `ProviderMetadata` remain available for both variants.

#### Scenario: Document source construction
- **WHEN** a `SourceInfo` is constructed with `SourceType: "document"`, `ID: "doc_1"`, `MediaType: "application/pdf"`, `Title: "Research Paper"`, and `Filename: "paper.pdf"`
- **THEN** all fields SHALL be accessible and the struct SHALL be valid

#### Scenario: URL source unchanged
- **WHEN** a `SourceInfo` is constructed with `SourceType: "url"`, `ID: "src_1"`, `URL: "https://example.com"`, and `Title: "Example"`
- **THEN** it SHALL continue to work as before with no changes required

#### Scenario: Document source in stream part
- **WHEN** a `StreamPart` with `Type: PartSource` carries a `SourceInfo` with `SourceType: "document"`
- **THEN** the source SHALL be emitted through the stream with the document fields preserved
