## ADDED Requirements

### Requirement: CustomContentPart prompt-side type
The provider package SHALL define a `CustomContentPart` struct with fields `Kind string` (format convention `"provider.type"`) and `ProviderOptions map[string]json.RawMessage`. `CustomContentPart` SHALL implement the `AssistantContentPart` sealed interface.

#### Scenario: CustomContentPart in assistant message
- **WHEN** an `AssistantMessage` is constructed with a `CustomContentPart{Kind: "anthropic.cache-control"}`
- **THEN** the content part SHALL be accepted as a valid `AssistantContentPart`

#### Scenario: CustomContentPart does not implement UserContentPart
- **WHEN** a `CustomContentPart` is used
- **THEN** it SHALL NOT satisfy the `UserContentPart` interface (compile-time check)

### Requirement: ReasoningFileContentPart prompt-side type
The provider package SHALL define a `ReasoningFileContentPart` struct with fields `Data DataContent`, `MediaType string`, and `ProviderOptions map[string]json.RawMessage`. `ReasoningFileContentPart` SHALL implement the `AssistantContentPart` sealed interface.

#### Scenario: ReasoningFileContentPart in assistant message
- **WHEN** an `AssistantMessage` is constructed with a `ReasoningFileContentPart{MediaType: "image/png", Data: DataContent{Base64: "..."}}`
- **THEN** the content part SHALL be accepted as a valid `AssistantContentPart`

### Requirement: ToolApprovalResponseContentPart prompt-side type
The provider package SHALL define a `ToolApprovalResponseContentPart` struct with fields `ApprovalID string`, `Approved bool`, `Reason string` (optional), and `ProviderOptions map[string]json.RawMessage`. `ToolApprovalResponseContentPart` SHALL implement the `ToolMessageContentPart` sealed interface.

#### Scenario: ToolApprovalResponseContentPart in tool message
- **WHEN** a `ToolMessage` is constructed with a `ToolApprovalResponseContentPart{ApprovalID: "apr_123", Approved: true}`
- **THEN** the content part SHALL be accepted as a valid `ToolMessageContentPart`

#### Scenario: ToolApprovalResponseContentPart with denial reason
- **WHEN** a `ToolApprovalResponseContentPart{ApprovalID: "apr_123", Approved: false, Reason: "unsafe action"}` is constructed
- **THEN** the `Reason` field SHALL carry the denial explanation

### Requirement: ToolMessageContentPart sealed interface
The provider package SHALL define a `ToolMessageContentPart` sealed interface with an unexported marker method `toolMessageContentPart()`. Both `ToolResultContentPart` and `ToolApprovalResponseContentPart` SHALL implement this interface.

#### Scenario: ToolResultContentPart implements ToolMessageContentPart
- **WHEN** a `ToolResultContentPart` is used
- **THEN** it SHALL satisfy the `ToolMessageContentPart` interface

#### Scenario: ToolApprovalResponseContentPart implements ToolMessageContentPart
- **WHEN** a `ToolApprovalResponseContentPart` is used
- **THEN** it SHALL satisfy the `ToolMessageContentPart` interface

### Requirement: ToolMessage content type expansion
`ToolMessage.Content` SHALL be typed as `[]ToolMessageContentPart` (the sealed interface), replacing the previous `[]ToolResultContentPart`. This allows tool messages to carry both tool results and tool approval responses.

#### Scenario: ToolMessage with mixed content
- **WHEN** a `ToolMessage` is constructed with both a `ToolResultContentPart` and a `ToolApprovalResponseContentPart`
- **THEN** both parts SHALL be accepted in the `Content` slice

#### Scenario: ToolMessage with only tool results
- **WHEN** a `ToolMessage` is constructed with only `ToolResultContentPart` entries
- **THEN** the message SHALL be valid (backward-compatible construction pattern)

### Requirement: ImageContentPart removal
The provider package SHALL NOT define `ImageContentPart`. The type and its `userContentPart()` marker method SHALL be removed. All image content SHALL be represented as `FileContentPart` with an image media type.

#### Scenario: Image as FileContentPart
- **WHEN** image content is included in a user message
- **THEN** it SHALL be expressed as `FileContentPart{Data: DataContent{...}, MediaType: "image/png"}`

#### Scenario: UserContentPart implementations
- **WHEN** the `UserContentPart` sealed interface is checked
- **THEN** only `TextContentPart` and `FileContentPart` SHALL implement it (no `ImageContentPart`)

### Requirement: PartCustom stream part constant
The provider package SHALL define `PartCustom StreamPartType = "custom"` as a stream part type constant. `StreamPart` SHALL include a `Kind string` field populated when `Type` is `PartCustom`.

#### Scenario: Custom content in stream
- **WHEN** a provider emits a `StreamPart{Type: PartCustom, Kind: "anthropic.cache-control"}`
- **THEN** the stream part SHALL carry the `Kind` field with the custom content identifier

### Requirement: PartReasoningFile stream part constant
The provider package SHALL define `PartReasoningFile StreamPartType = "reasoning-file"` as a stream part type constant. When `Type` is `PartReasoningFile`, the `StreamPart` SHALL use the existing `FileData` and `MediaType` fields for the file content.

#### Scenario: Reasoning file in stream
- **WHEN** a provider emits a `StreamPart{Type: PartReasoningFile, FileData: data, MediaType: "image/png"}`
- **THEN** the stream part SHALL carry the reasoning file data in the existing file fields

### Requirement: GenerateContentPart Kind field
`GenerateContentPart` SHALL include a `Kind string` field for `type: "custom"` content in non-streaming generate results. The `Kind` field SHALL also be used for `type: "reasoning-file"` content.

#### Scenario: Custom content in generate result
- **WHEN** a non-streaming generate result contains custom content
- **THEN** the `GenerateContentPart` SHALL have `Type: "custom"` and `Kind` set to the custom content identifier

#### Scenario: Reasoning file in generate result
- **WHEN** a non-streaming generate result contains a reasoning file
- **THEN** the `GenerateContentPart` SHALL have `Type: "reasoning-file"` with `MediaType` and `Data` fields populated

### Requirement: StreamPart tool approval fields
`StreamPart` SHALL include `ApprovalID string` for `PartToolApprovalRequest` parts, and `Approved *bool` plus `Reason string` for `PartToolApprovalResult` parts.

#### Scenario: Tool approval request in stream
- **WHEN** a provider emits a `StreamPart{Type: PartToolApprovalRequest, ApprovalID: "apr_123", ToolCallID: "call_456"}`
- **THEN** the stream part SHALL carry both the approval ID and the tool call ID

#### Scenario: Tool approval result in stream
- **WHEN** a provider emits a `StreamPart{Type: PartToolApprovalResult, ApprovalID: "apr_123", Approved: ptr(true)}`
- **THEN** the stream part SHALL carry the approval decision

### Requirement: Warning type rename
The `Warning.Type` field SHALL use `"unsupported"` (not `"unsupported-setting"`) for features the model does not support. All existing code producing `"unsupported-setting"` warnings SHALL be updated.

#### Scenario: Warning type value
- **WHEN** a provider emits a warning for an unsupported feature
- **THEN** `Warning.Type` SHALL be `"unsupported"` (not `"unsupported-setting"`)

### Requirement: Warning compatibility variant
The `Warning` type SHALL support a `"compatibility"` type value for features used in a degraded/compatibility mode. The `"compatibility"` variant uses the same fields as `"unsupported"` (`Feature` and optional `Details`).

#### Scenario: Compatibility warning
- **WHEN** a provider uses a feature in compatibility mode
- **THEN** it SHALL emit a `Warning{Type: "compatibility", Feature: "featureName", Details: "explanation"}`

### Requirement: Warning type constants
The provider package SHALL define string constants for warning types: `WarnUnsupported = "unsupported"`, `WarnCompatibility = "compatibility"`, `WarnOther = "other"`.

#### Scenario: Warning constants used instead of string literals
- **WHEN** code creates a `Warning`
- **THEN** it SHALL use the defined constants for the `Type` field

### Requirement: CallOptions.Reasoning field
`CallOptions` SHALL include a `Reasoning *string` field for controlling model reasoning effort. The valid values SHALL be: `"provider-default"`, `"none"`, `"minimal"`, `"low"`, `"medium"`, `"high"`, `"xhigh"`. The provider package SHALL define string constants for these values.

#### Scenario: Reasoning field set
- **WHEN** `CallOptions` is constructed with `Reasoning` set to a pointer to `"high"`
- **THEN** the field SHALL carry the reasoning effort level for the provider to interpret

#### Scenario: Reasoning field nil
- **WHEN** `CallOptions` is constructed without setting `Reasoning`
- **THEN** the field SHALL be `nil`, indicating no reasoning preference

### Requirement: Anthropic provider ImageContentPart removal
The Anthropic provider's `convertUserContent` SHALL handle image media types on `FileContentPart` (base64 or URL sources). The `case provider.ImageContentPart` branch SHALL be removed.

#### Scenario: Image via FileContentPart in user message
- **WHEN** `convertUserContent` receives a `FileContentPart` with `MediaType: "image/jpeg"` and base64 data
- **THEN** the provider SHALL produce a `BetaImageBlockParam` with the image source

#### Scenario: Image via FileContentPart with URL
- **WHEN** `convertUserContent` receives a `FileContentPart` with `MediaType: "image/png"` and a URL
- **THEN** the provider SHALL produce a `BetaImageBlockParam` with a URL source

#### Scenario: Non-image FileContentPart unchanged
- **WHEN** `convertUserContent` receives a `FileContentPart` with `MediaType: "application/pdf"`
- **THEN** the provider SHALL produce a `BetaRequestDocumentBlockParam` (existing behavior)

### Requirement: ToolResultContentPart implements AssistantContentPart
`ToolResultContentPart` SHALL implement the `AssistantContentPart` sealed interface (pre-existing behavior). This allows provider-executed tool results to appear in assistant message content for multi-turn conversations.

#### Scenario: ToolResultContentPart in assistant message
- **WHEN** an `AssistantMessage` is constructed with a `ToolResultContentPart`
- **THEN** the content part SHALL be accepted as a valid `AssistantContentPart`

#### Scenario: Compile-time interface check
- **WHEN** a compile-time check `var _ AssistantContentPart = ToolResultContentPart{}` exists
- **THEN** it SHALL compile successfully

### Requirement: Anthropic provider unsupported content warnings
The Anthropic provider's `convertAssistantContent` SHALL produce a warning and skip content parts it does not support natively (`CustomContentPart`, `ReasoningFileContentPart`).

#### Scenario: CustomContentPart in assistant message to Anthropic
- **WHEN** `convertAssistantContent` encounters a `CustomContentPart`
- **THEN** the provider SHALL add a warning with `Type: "unsupported"` and skip the content part

#### Scenario: ReasoningFileContentPart in assistant message to Anthropic
- **WHEN** `convertAssistantContent` encounters a `ReasoningFileContentPart`
- **THEN** the provider SHALL add a warning with `Type: "unsupported"` and skip the content part
