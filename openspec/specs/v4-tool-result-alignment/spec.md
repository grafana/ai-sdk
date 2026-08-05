## Purpose

Add preliminary tool result support, expand tool result content types, and verify stream part ID population, completing the V4 tool result alignment.

## Requirements

### Requirement: StreamPart Preliminary field

The `StreamPart` struct SHALL have a `Preliminary *bool` field. When set to `true`, it indicates the tool result is intermediate and will be replaced by a subsequent result (e.g., image previews). A final non-preliminary result SHALL always follow. If not set or `false`, the result is final.

This field applies to stream parts of type `PartToolResult`.

#### Scenario: Preliminary tool result
- **WHEN** a `StreamPart` of type `PartToolResult` has `Preliminary` set to `true`
- **THEN** it SHALL indicate an intermediate result that will be replaced

#### Scenario: Final tool result
- **WHEN** a `StreamPart` of type `PartToolResult` has `Preliminary` as `nil` or `false`
- **THEN** it SHALL indicate a final result

### Requirement: ToolResultContentValue expanded types

The `ToolResultContentValue` struct SHALL support the following `Type` values:
- `"text"` -- text content
- `"file"` -- file content with `Data *DataContent`, `MediaType`, and optional `Filename`
- `"custom"` -- custom provider-specific content with `ProviderOptions` only

File content SHALL use the LanguageModelV4 tagged `DataContent` union for inline data, URLs, provider references, and inline text. Images SHALL use `"file"` with an image media type (e.g. `image/png`).

The legacy `"file-data"`, `"file-url"`, and `"file-reference"` wire discriminators SHALL remain accepted during decoding and SHALL normalize to `"file"`. Marshaling SHALL emit the canonical `"file"` discriminator and tagged data union.

#### Scenario: inline file data content value
- **WHEN** a `ToolResultContentValue` is constructed with `Type: "file"`, `Data: &DataContent{Base64: "<base64>"}`, `MediaType: "application/pdf"`, and `Filename: "report.pdf"`
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

### Requirement: Stream part ID verification for Anthropic provider

The Anthropic provider SHALL populate `StreamPart.ID` for all of the following stream part types:
- `PartTextStart`, `PartTextDelta`, `PartTextEnd`
- `PartReasoningStart`, `PartReasoningDelta`, `PartReasoningEnd`
- `PartToolInputStart`, `PartToolInputDelta`, `PartToolInputEnd`

The ID SHALL be derived from the content block's identifier in the Anthropic response.

#### Scenario: Text stream parts carry ID
- **WHEN** the Anthropic provider emits `PartTextStart`, `PartTextDelta`, and `PartTextEnd` for a content block
- **THEN** each part SHALL have `ID` set to the content block's identifier

#### Scenario: Reasoning stream parts carry ID
- **WHEN** the Anthropic provider emits `PartReasoningStart`, `PartReasoningDelta`, and `PartReasoningEnd` for a thinking block
- **THEN** each part SHALL have `ID` set to the content block's identifier

#### Scenario: Tool input stream parts carry ID
- **WHEN** the Anthropic provider emits `PartToolInputStart`, `PartToolInputDelta`, and `PartToolInputEnd` for a tool_use block
- **THEN** each part SHALL have `ID` set to the content block's identifier
