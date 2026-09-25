## MODIFIED Requirements

### Requirement: ToolResultContentValue expanded types

The `ToolResultContentValue` struct SHALL support the following `Type` values:
- `"text"` -- text content
- `"file"` -- file content with `Data *DataContent`, `MediaType`, and optional `Filename *string`
- `"custom"` -- custom provider-specific content with `ProviderOptions` only

File content SHALL require both `Data` and `MediaType` and use the LanguageModelV4 tagged `DataContent` union for inline data, URLs, provider references, and inline text. `MediaType` SHALL accept a full IANA media type, a top-level segment, or an equivalent `*`-subtype wildcard. Images SHALL use `"file"` with an image media type (e.g. `image/png`). File filenames SHALL preserve absence as nil and explicit empty as a pointer to an empty string through JSON and provider conversion. Data selection SHALL survive an empty selected payload.

The legacy `"file-data"`, `"file-url"`, and `"file-reference"` wire discriminators SHALL remain accepted during decoding and SHALL normalize to `"file"`. Marshaling SHALL emit the canonical `"file"` discriminator and tagged data union.

#### Scenario: inline file data content value
- **WHEN** a `ToolResultContentValue` is constructed with `Type: "file"`, `Data: &DataContent{Base64: "<base64>"}`, `MediaType: "application/pdf"`, and `Filename` pointing to `"report.pdf"`
- **THEN** it SHALL marshal with `type: "file"` and `data: {type: "data", data: "<base64>"}`

#### Scenario: image content value uses file
- **WHEN** a `ToolResultContentValue` holds image content with `Type: "file"`, base64 `DataContent`, and `MediaType: "image/png"`
- **THEN** the tagged data and media type SHALL be preserved

#### Scenario: file URL content value
- **WHEN** a `ToolResultContentValue` is constructed with `Type: "file"` and `Data: &DataContent{URL: "https://example.com/file.pdf"}`
- **THEN** the URL SHALL serialize through the tagged data union

#### Scenario: file provider-reference content value
- **WHEN** a `ToolResultContentValue` is constructed with `Type: "file"` and `Data.Reference` containing `{"openai":"file-abc123"}`
- **THEN** the reference SHALL serialize through the tagged data union

#### Scenario: legacy raw base64 content value
- **WHEN** a legacy `{"type":"file-data","data":"<base64>"}` value is decoded
- **THEN** it SHALL normalize to `ToolContentFile` with `DataContent.Base64` populated

#### Scenario: custom content value
- **WHEN** a `ToolResultContentValue` is constructed with `Type: "custom"` and `ProviderOptions` containing provider-specific data
- **THEN** only the Type and ProviderOptions fields SHALL be present in the marshaled output

#### Scenario: Empty filename and empty selected payload
- **WHEN** a tool-result file carries explicitly selected empty text or data and a present empty filename
- **THEN** JSON round-trip and input conversion SHALL preserve both selections, distinct from an absent filename or unselected data
