## ADDED Requirements

### Requirement: Legacy streamed execution-denied results preserve historical output

Changing `ToolResultOutput.Reason` to `*string` SHALL NOT change the legacy Go provider-wire stream projection. The internal `legacyToolResult` compatibility conversion MAY be updated only as needed to dereference the pointer while preserving historical output:

- nil reason SHALL project to the JSON string `""` and `isError: true`;
- a pointer to `""` SHALL project to the same JSON string `""` and `isError: true`;
- a pointer to a non-empty reason SHALL project to that JSON string and `isError: true`.

This legacy flat stream result cannot represent reason presence and SHALL NOT be used as evidence that nil and explicit empty remain distinct across the historical stream dialect. Generic request compatibility JSON and the explicit request adapter SHALL preserve those states separately.

#### Scenario: Absent legacy streamed denial reason remains empty string
- **WHEN** a legacy streamed tool-result output has type `execution-denied` and nil `Reason`
- **THEN** compatibility conversion SHALL emit `result: ""` with `isError: true`
- **AND** it SHALL NOT emit JSON null

#### Scenario: Explicit empty legacy streamed denial reason remains empty string
- **WHEN** a legacy streamed tool-result output has `Reason` pointing to `""`
- **THEN** compatibility conversion SHALL emit `result: ""` with `isError: true`

#### Scenario: Non-empty legacy streamed denial reason is preserved
- **WHEN** a legacy streamed tool-result output has `Reason` pointing to `"not allowed"`
- **THEN** compatibility conversion SHALL emit `result: "not allowed"` with `isError: true`

## MODIFIED Requirements

### Requirement: ToolResultContentValue expanded types

The `ToolResultContentValue` struct SHALL support the following `Type` values:
- `"text"` -- text content
- `"file"` -- file content with `Data *DataContent`, `MediaType`, and optional `Filename *string`
- `"custom"` -- custom provider-specific content with `ProviderOptions` only

File content SHALL require both `Data` and `MediaType` and use the LanguageModelV4 tagged `DataContent` union for inline data, URLs, provider references, and inline text. `MediaType` SHALL accept a full IANA media type, a top-level segment, or an equivalent `*`-subtype wildcard. Images SHALL use `"file"` with an image media type such as `image/png`. A nil `Filename` SHALL mean absent; a non-nil pointer to `""` SHALL preserve an explicitly present empty filename.

The legacy `"file-data"`, `"file-url"`, and `"file-reference"` wire discriminators SHALL remain accepted during decoding and SHALL normalize to `"file"`. Marshaling SHALL emit the canonical `"file"` discriminator and tagged data union. Existing non-empty untagged `DataContent` payload literals SHALL remain valid through deterministic type inference.

#### Scenario: inline file data content value
- **WHEN** a `ToolResultContentValue` is constructed with `Type: "file"`, `Data: &DataContent{Base64: "<base64>"}`, `MediaType: "application/pdf"`, and `Filename` pointing to `"report.pdf"`
- **THEN** it SHALL marshal with `type: "file"`, `data: {type: "data", data: "<base64>"}`, and `filename: "report.pdf"`

#### Scenario: Explicit empty filename is preserved
- **WHEN** a file content value has `Filename` pointing to `""`
- **THEN** compatibility and explicit request encoding SHALL include `filename: ""`
- **AND** decoding SHALL recover a non-nil pointer to `""`

#### Scenario: Absent filename remains absent
- **WHEN** a file content value has `Filename == nil`
- **THEN** request encoding SHALL omit `filename`
- **AND** decoding an omitted filename SHALL recover nil

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
- **THEN** it SHALL normalize to `ToolContentFile` with `DataContent.Base64` populated and an inferred data type

#### Scenario: custom content value
- **WHEN** a `ToolResultContentValue` is constructed with `Type: "custom"` and `ProviderOptions` containing provider-specific data
- **THEN** only the Type and ProviderOptions fields SHALL be present in the marshaled output
