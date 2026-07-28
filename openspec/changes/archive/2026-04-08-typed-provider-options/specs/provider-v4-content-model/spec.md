## MODIFIED Requirements

### Requirement: CustomContentPart prompt-side type
The provider package SHALL define a `CustomContentPart` struct with fields `Kind string` (format convention `"provider.type"`) and `ProviderOptions map[string]provider.ProviderOption`. `CustomContentPart` SHALL implement the `AssistantContentPart` sealed interface.

#### Scenario: CustomContentPart in assistant message
- **WHEN** an `AssistantMessage` is constructed with a `CustomContentPart{Kind: "anthropic.cache-control"}`
- **THEN** the content part SHALL be accepted as a valid `AssistantContentPart`

#### Scenario: CustomContentPart does not implement UserContentPart
- **WHEN** a `CustomContentPart` is used
- **THEN** it SHALL NOT satisfy the `UserContentPart` interface (compile-time check)

### Requirement: ReasoningFileContentPart prompt-side type
The provider package SHALL define a `ReasoningFileContentPart` struct with fields `Data DataContent`, `MediaType string`, and `ProviderOptions map[string]provider.ProviderOption`. `ReasoningFileContentPart` SHALL implement the `AssistantContentPart` sealed interface.

#### Scenario: ReasoningFileContentPart in assistant message
- **WHEN** an `AssistantMessage` is constructed with a `ReasoningFileContentPart{MediaType: "image/png", Data: DataContent{Base64: "..."}}`
- **THEN** the content part SHALL be accepted as a valid `AssistantContentPart`

### Requirement: ToolApprovalResponseContentPart prompt-side type
The provider package SHALL define a `ToolApprovalResponseContentPart` struct with fields `ApprovalID string`, `Approved bool`, `Reason string` (optional), and `ProviderOptions map[string]provider.ProviderOption`. `ToolApprovalResponseContentPart` SHALL implement the `ToolMessageContentPart` sealed interface.

#### Scenario: ToolApprovalResponseContentPart in tool message
- **WHEN** a `ToolMessage` is constructed with a `ToolApprovalResponseContentPart{ApprovalID: "apr_123", Approved: true}`
- **THEN** the content part SHALL be accepted as a valid `ToolMessageContentPart`

#### Scenario: ToolApprovalResponseContentPart with denial reason
- **WHEN** a `ToolApprovalResponseContentPart{ApprovalID: "apr_123", Approved: false, Reason: "unsafe action"}` is constructed
- **THEN** the `Reason` field SHALL carry the denial explanation
