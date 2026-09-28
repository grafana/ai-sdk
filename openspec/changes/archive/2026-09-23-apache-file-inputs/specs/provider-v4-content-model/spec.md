## MODIFIED Requirements

### Requirement: ContentPart is a flat discriminated struct

The `provider` package SHALL define `ContentPart` as a single flat struct discriminated by a typed `Type` field, mirroring how `provider.StreamPart` is already modeled:

```go
type ContentPart struct {
    Type             ContentPartType `json:"type"`
    Text             string          `json:"text,omitempty"`
    Data             *DataContent    `json:"data,omitempty"`
    Filename         *string         `json:"filename,omitempty"`
    MediaType        string          `json:"mediaType,omitempty"`
    Kind             string          `json:"kind,omitempty"`
    ToolCallID       string          `json:"toolCallId,omitempty"`
    ToolName         string          `json:"toolName,omitempty"`
    Input            json.RawMessage `json:"input,omitempty"`
    Output           *ToolResultOutput `json:"output,omitempty"`
    ProviderExecuted bool            `json:"providerExecuted,omitempty"`
    ApprovalID       string          `json:"approvalId,omitempty"`
    Approved         *bool           `json:"approved,omitempty"`
    Reason           string          `json:"reason,omitempty"`
    ProviderOptions  ProviderOptions `json:"providerOptions,omitempty"`
}
```

The previous sealed interfaces `UserContentPart`, `AssistantContentPart`, `ToolMessageContentPart` SHALL be removed. The previous concrete types `TextContentPart`, `FileContentPart`, `ReasoningContentPart`, `ToolCallContentPart`, `ToolResultContentPart`, `CustomContentPart`, `ReasoningFileContentPart`, and `ToolApprovalResponseContentPart` SHALL be removed.

For input files, nil `Filename` SHALL mean absent and a pointer to an empty string SHALL mean explicitly supplied empty. Source conversions using the shared flat struct SHALL retain existing absent/empty normalization; this field change SHALL NOT make `SourceInfo`, generated-output, stream, or UI `SourceDocumentPart` filenames presence-aware. Ordinary UI `FilePart` inputs SHALL instead preserve filename presence under the ui-message-conversion contract.

#### Scenario: Removed types
- **WHEN** the `provider` package is inspected
- **THEN** none of the listed concrete content-part types and none of the three `*ContentPart` interfaces SHALL exist as identifiers

#### Scenario: ContentPartType constants exist
- **WHEN** `ContentPartType` is inspected
- **THEN** it SHALL be a typed string with constants for at least: `text`, `file`, `reasoning`, `reasoning-file`, `tool-call`, `tool-result`, `custom`, `tool-approval-response`

#### Scenario: Round-trip every ContentPartType
- **WHEN** every defined `ContentPartType` value is constructed as a `ContentPart`, marshaled to JSON, and unmarshaled back
- **THEN** the decoded value SHALL equal the original for every type

#### Scenario: Input filename presence survives JSON
- **WHEN** file content with absent, explicit empty, and non-empty filenames is encoded and decoded
- **THEN** all three states SHALL remain distinct and only nil SHALL omit the filename member

#### Scenario: Source filename normalization remains stable
- **WHEN** source information with an absent or empty descriptive filename crosses the `ContentPart` boundary
- **THEN** it SHALL retain the existing no-filename semantics without changing source or UI wire representations
