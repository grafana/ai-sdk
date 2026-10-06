## ADDED Requirements

### Requirement: Message is a flat discriminated struct

The `provider` package SHALL define `Message` as a single struct, not a sealed interface:

```go
type Message struct {
    Role            Role            `json:"role"`
    Content         []ContentPart   `json:"content"`
    ProviderOptions ProviderOptions `json:"providerOptions,omitempty"`
}
```

The `Role` field SHALL discriminate the message variant (`"system"`, `"user"`, `"assistant"`, `"tool"`). The previous `Message` sealed interface and the four concrete variants (`SystemMessage`, `UserMessage`, `AssistantMessage`, `ToolMessage`) SHALL be removed.

#### Scenario: Message is a struct
- **WHEN** the `provider.Message` type is inspected
- **THEN** it SHALL be a Go struct exported as `provider.Message` with public fields `Role`, `Content`, `ProviderOptions`

#### Scenario: Removed types
- **WHEN** the `provider` package is inspected
- **THEN** `provider.SystemMessage`, `provider.UserMessage`, `provider.AssistantMessage`, and `provider.ToolMessage` SHALL NOT exist as identifiers

#### Scenario: Round-trip via encoding/json
- **WHEN** a `Message` carrying every role is marshaled to JSON and unmarshaled back
- **THEN** the decoded value SHALL equal the original (using `reflect.DeepEqual`) with no field loss

### Requirement: Message constructor helpers preserved

The provider package SHALL preserve constructor helpers for ergonomics: `NewSystemMessage(text string) Message`, `NewUserMessage(parts ...ContentPart) Message`, `NewAssistantMessage(parts ...ContentPart) Message`, and `NewToolMessage(parts ...ContentPart) Message`. Each SHALL set the appropriate `Role` and pack the arguments into `Content`.

#### Scenario: NewSystemMessage shape
- **WHEN** `NewSystemMessage("hello")` is called
- **THEN** it SHALL return `Message{Role: RoleSystem, Content: []ContentPart{{Type: ContentPartTypeText, Text: "hello"}}}`

#### Scenario: NewUserMessage shape
- **WHEN** `NewUserMessage(ContentPart{Type: ContentPartTypeText, Text: "hi"})` is called
- **THEN** it SHALL return `Message{Role: RoleUser, Content: [...]}` with the given parts

#### Scenario: NewAssistantMessage shape
- **WHEN** `NewAssistantMessage(part1, part2)` is called
- **THEN** it SHALL return `Message{Role: RoleAssistant, Content: [...]}` with both parts in order

#### Scenario: NewToolMessage shape
- **WHEN** `NewToolMessage(ContentPart{Type: ContentPartTypeToolResult, ToolCallID: "1", ToolName: "t"})` is called
- **THEN** it SHALL return `Message{Role: RoleTool, Content: [...]}` with the given part

### Requirement: ContentPart constructor helpers

The `provider` package SHALL provide per-variant constructor helpers that return a `ContentPart` with `Type` set to the matching `ContentPartType` constant and the relevant fields populated:

- `TextPart(text string) ContentPart`
- `FilePart(mediaType string, data DataContent) ContentPart`
- `ReasoningPart(text string) ContentPart`
- `ReasoningFilePart(mediaType string, data DataContent) ContentPart`
- `ToolCallPart(toolCallID, toolName string, input json.RawMessage) ContentPart`
- `ToolResultPart(toolCallID, toolName string, output *ToolResultOutput) ContentPart`
- `CustomPart(kind string) ContentPart`
- `ToolApprovalResponsePart(approvalID string, approved *bool, reason string) ContentPart`

These helpers exist to keep producer call sites readable; the underlying flat `ContentPart` struct remains usable as a literal where needed.

#### Scenario: TextPart shape
- **WHEN** `TextPart("hello")` is called
- **THEN** it SHALL return `ContentPart{Type: ContentPartTypeText, Text: "hello"}`

#### Scenario: FilePart shape
- **WHEN** `FilePart("image/png", DataContent{URL: "..."})` is called
- **THEN** it SHALL return `ContentPart{Type: ContentPartTypeFile, MediaType: "image/png", Data: <pointer to the given DataContent>}`

#### Scenario: ToolCallPart shape
- **WHEN** `ToolCallPart("call_1", "fetch", json.RawMessage(`{"q":"x"}`))` is called
- **THEN** it SHALL return a `ContentPart` with `Type: ContentPartTypeToolCall`, `ToolCallID: "call_1"`, `ToolName: "fetch"`, and `Input` carrying the given JSON

#### Scenario: ToolResultPart shape
- **WHEN** `ToolResultPart("call_1", "fetch", &ToolResultOutput{Type: ToolOutputText, Text: "sunny"})` is called
- **THEN** it SHALL return a `ContentPart` with `Type: ContentPartTypeToolResult`, `ToolCallID`, `ToolName`, and `Output` populated

### Requirement: Role-text shortcut helpers

The `provider` package SHALL provide ergonomic shortcuts for the common case of a role-only message carrying a single text part:

- `UserText(text string) Message` — equivalent to `NewUserMessage(TextPart(text))`
- `AssistantText(text string) Message` — equivalent to `NewAssistantMessage(TextPart(text))`

`NewSystemMessage` already serves the system-role text shortcut role.

#### Scenario: UserText shape
- **WHEN** `UserText("hello")` is called
- **THEN** it SHALL return `Message{Role: RoleUser, Content: []ContentPart{TextPart("hello")}}`

#### Scenario: AssistantText shape
- **WHEN** `AssistantText("hi back")` is called
- **THEN** it SHALL return `Message{Role: RoleAssistant, Content: []ContentPart{TextPart("hi back")}}`

### Requirement: ContentPart is a flat discriminated struct

The `provider` package SHALL define `ContentPart` as a single flat struct discriminated by a typed `Type` field, mirroring how `provider.StreamPart` is already modeled:

```go
type ContentPart struct {
    Type             ContentPartType `json:"type"`
    Text             string          `json:"text,omitempty"`
    Data             *DataContent    `json:"data,omitempty"`
    Filename         string          `json:"filename,omitempty"`
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

#### Scenario: Removed types
- **WHEN** the `provider` package is inspected
- **THEN** none of the listed concrete content-part types and none of the three `*ContentPart` interfaces SHALL exist as identifiers

#### Scenario: ContentPartType constants exist
- **WHEN** `ContentPartType` is inspected
- **THEN** it SHALL be a typed string with constants for at least: `text`, `file`, `reasoning`, `reasoning-file`, `tool-call`, `tool-result`, `custom`, `tool-approval-response`

#### Scenario: Round-trip every ContentPartType
- **WHEN** every defined `ContentPartType` value is constructed as a `ContentPart`, marshaled to JSON, and unmarshaled back
- **THEN** the decoded value SHALL equal the original for every type

### Requirement: CallOptions.Prompt is wire-serializable

`CallOptions.Prompt` SHALL be tagged `json:"prompt,omitempty"` (not `json:"-"`). Its element type SHALL be the flat `Message` struct. The field SHALL round-trip losslessly through `encoding/json`.

#### Scenario: Prompt JSON tag
- **WHEN** the `CallOptions` struct is inspected
- **THEN** the `Prompt` field SHALL carry `json:"prompt,omitempty"` (not `json:"-"`)

#### Scenario: Prompt round-trip
- **WHEN** a `CallOptions` with a non-empty `Prompt` carrying every role is marshaled and unmarshaled
- **THEN** every message and content part SHALL be preserved

### Requirement: StreamPart carries APICallError directly

`StreamPart` SHALL include a field `APICallError *APICallError` (`json:"apiCallError,omitempty"`) populated only when `Type == PartError`. The previous `Error error` field SHALL be removed. Producers SHALL wrap any error into an `*APICallError` before emitting a `PartError` event so the retryability bit and HTTP status reach consumers across any boundary (including the wire).

#### Scenario: Error event carries APICallError
- **WHEN** a provider emits a stream-error event
- **THEN** it SHALL emit `StreamPart{Type: PartError, APICallError: &APICallError{...}}` with `IsRetryable`, `StatusCode`, `Message`, `ResponseBody`, and `Data` populated as appropriate

#### Scenario: Removed Error field
- **WHEN** the `StreamPart` struct is inspected
- **THEN** it SHALL NOT have an `Error error` field

### Requirement: StreamPart.FileData is JSON-serialized as base64

`StreamPart.FileData` SHALL be tagged `json:"fileData,omitempty"`. Go's `encoding/json` natively encodes `[]byte` as base64; the wire SHALL rely on that representation.

#### Scenario: FileData JSON tag
- **WHEN** the `StreamPart` struct is inspected
- **THEN** the `FileData` field SHALL carry `json:"fileData,omitempty"` (not `json:"-"`)

#### Scenario: FileData round-trip
- **WHEN** a `StreamPart{Type: PartReasoningFile, FileData: bytes, MediaType: "image/png"}` is marshaled and unmarshaled
- **THEN** the decoded value SHALL have equal `FileData`, `MediaType`, and `Type`

## MODIFIED Requirements

### Requirement: ToolMessageContentPart sealed interface

The `ToolMessageContentPart` sealed interface SHALL no longer exist. Tool messages carry `[]ContentPart` like every other role; producers SHALL populate only `ContentPartTypeToolResult` and `ContentPartTypeToolApprovalResponse` parts inside a tool-role `Message.Content`. Producer-side validation lives in the orchestration layer; the wire MUST trust the `Type` discriminator on each part.

#### Scenario: Tool message content uses flat ContentPart
- **WHEN** a tool-role `Message` is constructed
- **THEN** its `Content` SHALL be `[]ContentPart`, with each part's `Type` set to `ContentPartTypeToolResult` or `ContentPartTypeToolApprovalResponse`

#### Scenario: ToolMessageContentPart removed
- **WHEN** the `provider` package is inspected
- **THEN** the identifier `ToolMessageContentPart` SHALL NOT exist

### Requirement: ToolMessage content type expansion

`ToolMessage` SHALL no longer exist as a distinct type. Tool-role messages SHALL be expressed as `Message{Role: RoleTool, Content: []ContentPart}`. The flat `ContentPart` MUST carry any of the previously-allowed shapes (tool result, tool approval response) discriminated by `Type`. Mixed content (both tool-result and tool-approval-response parts) in the same message SHALL be valid.

#### Scenario: Tool-role message with mixed content
- **WHEN** a tool-role `Message` is constructed with both a tool-result `ContentPart` and a tool-approval-response `ContentPart`
- **THEN** both parts SHALL be valid in the same `Content` slice

#### Scenario: Tool-role message with only tool results
- **WHEN** a tool-role `Message` is constructed with only tool-result `ContentPart` entries
- **THEN** the message SHALL be valid

### Requirement: ImageContentPart removal

The provider package SHALL NOT define `ImageContentPart` (already removed in the prior content-model expansion). With this change the `UserContentPart` interface SHALL also no longer exist. Image content MUST be expressed as `ContentPart{Type: ContentPartTypeFile, MediaType: "image/...", Data: ...}` in user-role messages.

#### Scenario: Image as file-typed ContentPart
- **WHEN** image content appears in a user-role message
- **THEN** it SHALL be expressed as `ContentPart{Type: ContentPartTypeFile, Data: &DataContent{...}, MediaType: "image/png"}`

### Requirement: PartCustom stream part constant

The provider package SHALL define `PartCustom StreamPartType = "custom"` as a stream part type constant. `StreamPart` SHALL include a `Kind string` field populated when `Type` is `PartCustom`. With this change the constant and field are unchanged; the stream part as a whole MUST round-trip losslessly through JSON.

#### Scenario: Custom content in stream
- **WHEN** a provider emits a `StreamPart{Type: PartCustom, Kind: "anthropic.cache-control"}`
- **THEN** the stream part SHALL carry the `Kind` field with the custom content identifier

### Requirement: PartReasoningFile stream part constant

The provider package SHALL define `PartReasoningFile StreamPartType = "reasoning-file"` as a stream part type constant. When `Type` is `PartReasoningFile`, `StreamPart` SHALL use the existing `FileData` and `MediaType` fields. With this change `StreamPart.FileData` MUST be JSON-tagged (`json:"fileData,omitempty"`) and the part MUST round-trip losslessly through the wire.

#### Scenario: Reasoning file in stream round-trips
- **WHEN** a provider emits `StreamPart{Type: PartReasoningFile, FileData: data, MediaType: "image/png"}` and the part is JSON round-tripped
- **THEN** the decoded part SHALL preserve `Type`, `FileData`, and `MediaType`

### Requirement: GenerateContentPart Kind field

`GenerateContentPart` SHALL include a `Kind string` field for `Type: "custom"` content in non-streaming generate results. The `Kind` field SHALL also be used for `Type: "reasoning-file"` content. The field MUST be JSON-serializable (`json:"kind,omitempty"`) and round-trip losslessly through the wire.

#### Scenario: Custom content in generate result
- **WHEN** a non-streaming generate result contains custom content
- **THEN** the `GenerateContentPart` SHALL have `Type: "custom"` and `Kind` set to the custom content identifier

### Requirement: StreamPart tool approval fields

`StreamPart` SHALL include `ApprovalID string` for `PartToolApprovalRequest` parts, and `Approved *bool` plus `Reason string` for `PartToolApprovalResult` parts. With this change all three fields MUST be JSON-tagged and round-trip losslessly through the wire.

#### Scenario: Tool approval request in stream
- **WHEN** a provider emits `StreamPart{Type: PartToolApprovalRequest, ApprovalID: "apr_123", ToolCallID: "call_456"}`
- **THEN** the stream part SHALL carry both the approval ID and the tool call ID

### Requirement: Anthropic provider ImageContentPart removal

The Anthropic provider's user-content conversion SHALL handle image media types on file-typed `ContentPart` (base64 or URL sources). The previous `case provider.FileContentPart` type-switch branch SHALL be replaced with discriminator dispatch on `cp.Type == ContentPartTypeFile`.

#### Scenario: Image via file-typed ContentPart in user message
- **WHEN** the Anthropic conversion receives a `ContentPart{Type: ContentPartTypeFile, MediaType: "image/jpeg", Data: &DataContent{Base64: "..."}}`
- **THEN** the provider SHALL produce a `BetaImageBlockParam` with the image source

#### Scenario: Image via URL ContentPart
- **WHEN** the Anthropic conversion receives a `ContentPart{Type: ContentPartTypeFile, MediaType: "image/png", Data: &DataContent{URL: "..."}}`
- **THEN** the provider SHALL produce a `BetaImageBlockParam` with a URL source

#### Scenario: Non-image file ContentPart unchanged
- **WHEN** the Anthropic conversion receives a `ContentPart{Type: ContentPartTypeFile, MediaType: "application/pdf"}`
- **THEN** the provider SHALL produce a `BetaRequestDocumentBlockParam`

### Requirement: Anthropic provider unsupported content warnings

The Anthropic provider's assistant-content conversion SHALL produce a warning and skip `ContentPart` values whose `Type` it does not support natively. Specifically, `ContentPartTypeCustom` and `ContentPartTypeReasoningFile` parts MUST emit a `Warning{Type: "unsupported"}` and be omitted from the converted assistant content.

#### Scenario: Custom-typed ContentPart in assistant message to Anthropic
- **WHEN** the conversion encounters a `ContentPart{Type: ContentPartTypeCustom}`
- **THEN** the provider SHALL add a warning with `Type: "unsupported"` and skip the part

#### Scenario: Reasoning-file-typed ContentPart in assistant message to Anthropic
- **WHEN** the conversion encounters a `ContentPart{Type: ContentPartTypeReasoningFile}`
- **THEN** the provider SHALL add a warning with `Type: "unsupported"` and skip the part

### Requirement: ToolResultContentPart implements AssistantContentPart

Neither `ToolResultContentPart` nor `AssistantContentPart` SHALL exist after the flatten. The equivalent rule under the flat model: a `ContentPart` with `Type: ContentPartTypeToolResult` MUST be valid in an assistant-role `Message.Content` slice. This shape is used for provider-executed tool results carried back into multi-turn conversations; tooling SHALL accept it without warning.

#### Scenario: Tool-result ContentPart in assistant message
- **WHEN** an assistant-role `Message` is constructed with `ContentPart{Type: ContentPartTypeToolResult}`
- **THEN** the message SHALL be valid; tooling SHALL accept the assistant-role tool-result form

## REMOVED Requirements

### Requirement: CustomContentPart prompt-side type

**Reason**: Replaced by the unified flat `ContentPart{Type: ContentPartTypeCustom, Kind, ProviderOptions}`.

**Migration**: Replace `provider.CustomContentPart{Kind: "...", ProviderOptions: ...}` with `provider.ContentPart{Type: provider.ContentPartTypeCustom, Kind: "...", ProviderOptions: ...}`.

### Requirement: ReasoningFileContentPart prompt-side type

**Reason**: Replaced by the unified flat `ContentPart{Type: ContentPartTypeReasoningFile, Data, MediaType, ProviderOptions}`.

**Migration**: Replace `provider.ReasoningFileContentPart{Data: ..., MediaType: ...}` with `provider.ContentPart{Type: provider.ContentPartTypeReasoningFile, Data: &..., MediaType: "..."}`.

### Requirement: ToolApprovalResponseContentPart prompt-side type

**Reason**: Replaced by the unified flat `ContentPart{Type: ContentPartTypeToolApprovalResponse, ApprovalID, Approved, Reason, ProviderOptions}`.

**Migration**: Replace `provider.ToolApprovalResponseContentPart{ApprovalID: "...", Approved: true}` with `provider.ContentPart{Type: provider.ContentPartTypeToolApprovalResponse, ApprovalID: "...", Approved: ptr(true)}`.
