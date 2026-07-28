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
- `"text"` -- text content (existing)
- `"file-data"` -- base64-encoded file data with `Data`, `MediaType`, and optional `Filename`
- `"file-url"` -- file referenced by URL with `URL`
- `"file-reference"` -- file referenced by provider-specific reference with `ProviderReference`
- `"custom"` -- custom provider-specific content with `ProviderOptions` only

Images are represented as `"file-data"` with an image media type (e.g. `image/png`).

The `FileID json.RawMessage` field SHALL be renamed to `ProviderReference map[string]string` (json tag `"providerReference,omitempty"`) to match upstream V4's `SharedV4ProviderReference` semantics where keys are provider names and values are provider-specific identifiers.

#### Scenario: file-data content value
- **WHEN** a `ToolResultContentValue` is constructed with `Type: "file-data"`, `Data: "<base64>"`, `MediaType: "application/pdf"`, and `Filename: "report.pdf"`
- **THEN** all fields SHALL be accessible and the struct SHALL marshal correctly

#### Scenario: image content value uses file-data
- **WHEN** a `ToolResultContentValue` holds image content with `Type: "file-data"`, `Data: "<base64>"`, and `MediaType: "image/png"`
- **THEN** the data and media type SHALL be preserved

#### Scenario: file-url content value
- **WHEN** a `ToolResultContentValue` is constructed with `Type: "file-url"` and `URL: "https://example.com/file.pdf"`
- **THEN** the URL field SHALL be present in the marshaled output

#### Scenario: file-reference content value
- **WHEN** a `ToolResultContentValue` is constructed with `Type: "file-reference"` and `ProviderReference: map[string]string{"openai": "file-abc123"}`
- **THEN** the ProviderReference SHALL serialize as `{"providerReference": {"openai": "file-abc123"}}`

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
