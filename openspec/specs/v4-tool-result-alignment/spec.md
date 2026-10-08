## Purpose

Add preliminary tool result support, expand tool result content types, and verify stream part ID population, completing the V4 tool result alignment.

## Requirements

### Requirement: StreamPart Preliminary field

The `StreamPart` struct SHALL have a presence-aware `Preliminary *bool` field. True SHALL indicate an intermediate tool result that will be replaced by a subsequent result, such as a preview. A final non-preliminary result SHALL follow a preliminary series. Absent and explicit false SHALL both indicate a final result while retaining their provider-domain presence. This field applies to `PartToolResult`. `GenerateContentPart.Preliminary` SHALL use the same boolean semantics for `ContentToolResult`.

#### Scenario: Preliminary tool result
- **WHEN** a `StreamPart` of type `PartToolResult` has `Preliminary` set to true
- **THEN** it SHALL indicate an intermediate result that will be replaced

#### Scenario: Final tool result
- **WHEN** a tool-result marker is omitted or explicitly false
- **THEN** decoding SHALL preserve nil or an explicit false pointer respectively and indicate a final result

### Requirement: Provider-domain dynamic marker normalization

`provider.GenerateContentPart.Dynamic` and `provider.StreamPart.Dynamic` SHALL retain `*bool` fields. On input-start, absence, false and true SHALL remain distinct through provider output and Go client decoding. In the text stream, explicit values SHALL take precedence; only absence SHALL infer dynamic classification from the application tool definition. UI conversion SHALL derive classification from the generation's original application tool types, independently of per-step execution registries and without storing a second UI classification on text events. A known dynamic application tool SHALL produce dynamic true, a known non-dynamic tool SHALL omit dynamic, and an unknown tool SHALL use the text-stream value. These are distinct behaviors in the registered upstream version. Absent and false call/result markers SHALL both mean disabled without requiring a provider-domain type migration. Function-tool Strict presence and unrelated core/UI APIs SHALL remain unchanged.

#### Scenario: Equivalent unary disabled markers
- **WHEN** unary provider output is decoded with absent and false dynamic markers
- **THEN** both SHALL mean dynamic disabled while unary decoding preserves nil versus an explicit false pointer and enabled true remains distinct

#### Scenario: Absent input-start marker permits inference
- **WHEN** an input-start omits dynamic and its application tool is dynamic
- **THEN** the transport SHALL retain absence and actual Go and pinned upstream text-stream and UI output SHALL infer dynamic true

#### Scenario: Explicit input-start marker overrides inference
- **WHEN** an input-start has dynamic false or true and its application tool is dynamic
- **THEN** the transport and text stream SHALL preserve the explicit value, while the UI chunk SHALL use true for the known dynamic tool in both cases

#### Scenario: Known ordinary tool omits UI dynamic
- **WHEN** a known function or provider tool's input-start infers false from an absent marker
- **THEN** its text-stream part SHALL carry false and its UI chunk SHALL omit dynamic

#### Scenario: Unknown tool retains UI marker
- **WHEN** an unknown tool's input-start carries explicit false or true
- **THEN** its UI chunk SHALL preserve the explicit value

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
