## MODIFIED Requirements

### Requirement: ContentPart constructor helpers

The `provider` package SHALL provide per-variant constructor helpers that return a `ContentPart` with `Type` set to the matching `ContentPartType` constant and the relevant fields populated:

- `TextPart(text string) ContentPart`
- `FilePart(mediaType string, data DataContent) ContentPart`
- `ReasoningPart(text string) ContentPart`
- `ReasoningFilePart(mediaType string, data DataContent) ContentPart`
- `ToolCallPart(toolCallID, toolName string, input json.RawMessage) ContentPart`
- `ToolResultPart(toolCallID, toolName string, output *ToolResultOutput) ContentPart`
- `CustomPart(kind string) ContentPart`
- `ToolApprovalRequestPart(approvalID, toolCallID string, isAutomatic bool) ContentPart`
- `ToolApprovalResponsePart(approvalID string, approved bool, reason string) ContentPart`
- `ProviderExecutedToolApprovalResponsePart(approvalID string, approved bool, reason string) ContentPart`

These helpers exist to keep producer call sites readable; the underlying flat `ContentPart` struct remains usable as a literal where needed.

#### Scenario: TextPart shape
- **WHEN** `TextPart("hello")` is called
- **THEN** it SHALL return `ContentPart{Type: ContentPartTypeText, Text: "hello"}`

#### Scenario: FilePart shape
- **WHEN** `FilePart("image/png", DataContent{URL: "..."})` is called
- **THEN** it SHALL return `ContentPart{Type: ContentPartTypeFile, MediaType: "image/png", Data: <pointer to the given DataContent>}`

#### Scenario: ToolCallPart shape
- **WHEN** `ToolCallPart("call_1", "fetch", json.RawMessage("{\"q\":\"x\"}"))` is called
- **THEN** it SHALL return a `ContentPart` with `Type: ContentPartTypeToolCall`, `ToolCallID: "call_1"`, `ToolName: "fetch"`, and `Input` carrying the given JSON

#### Scenario: ToolResultPart shape
- **WHEN** `ToolResultPart("call_1", "fetch", &ToolResultOutput{Type: ToolOutputText, Text: "sunny"})` is called
- **THEN** it SHALL return a `ContentPart` with `Type: ContentPartTypeToolResult`, `ToolCallID`, `ToolName`, and `Output` populated

#### Scenario: ToolApprovalRequestPart shape
- **WHEN** `ToolApprovalRequestPart("apr_1", "call_1", false)` is called
- **THEN** it SHALL return a `ContentPart` with `Type: ContentPartTypeToolApprovalRequest`, `ApprovalID: "apr_1"`, `ToolCallID: "call_1"`, and no approval decision fields populated

### Requirement: StreamPart tool approval fields

`StreamPart` SHALL include `ApprovalID string` for `PartToolApprovalRequest` parts. `PartToolApprovalRequest` SHALL carry the approval request ID and the tool call ID that needs approval, matching the current upstream V4 stream part. The provider package SHALL NOT define `PartToolApprovalResult`, because current upstream V4 does not define a `tool-approval-result` stream part.

User decisions SHALL be represented as tool-message content parts with type `ContentPartTypeToolApprovalResponse`, not as provider stream parts.

#### Scenario: Tool approval request in stream
- **WHEN** a provider emits `StreamPart{Type: PartToolApprovalRequest, ApprovalID: "apr_123", ToolCallID: "call_456"}`
- **THEN** the stream part SHALL carry both the approval ID and the tool call ID

#### Scenario: Tool approval result stream part is absent
- **WHEN** the provider stream part constants are inspected
- **THEN** there SHALL NOT be a `PartToolApprovalResult` constant

#### Scenario: Approval decision uses tool message content
- **WHEN** a caller records an approval decision for an approval ID
- **THEN** the decision SHALL be represented as `ContentPart{Type: ContentPartTypeToolApprovalResponse, ApprovalID: <id>, Approved: <bool>}` in a tool-role message

## ADDED Requirements

### Requirement: Assistant approval request content part

The `provider` package SHALL define `ContentPartTypeToolApprovalRequest = "tool-approval-request"` for assistant-role model messages. A tool approval request content part SHALL carry `ApprovalID`, `ToolCallID`, and optional automatic-approval metadata. It SHALL be valid in assistant-role messages so orchestration can persist approval requests across the stateless two-call flow.

Provider prompt conversion SHALL use assistant approval request parts only for local correlation and missing-tool-result checks. Local approval request parts SHALL NOT be forwarded to provider APIs that do not accept them.

#### Scenario: Approval request content round-trips
- **WHEN** a `ContentPart` with `Type: ContentPartTypeToolApprovalRequest`, approval ID, and tool call ID is JSON round-tripped
- **THEN** the decoded value SHALL preserve the type, approval ID, and tool call ID

#### Scenario: Assistant message can carry approval request
- **WHEN** an assistant-role `provider.Message` is constructed with a tool call part followed by a tool approval request part
- **THEN** both parts SHALL be valid in the message content slice

#### Scenario: Local approval request is stripped before provider call
- **WHEN** provider request conversion receives an assistant message containing `ContentPartTypeToolApprovalRequest`
- **THEN** the converted provider API prompt SHALL omit that approval request part

#### Scenario: Provider-executed approval response is preserved
- **WHEN** provider request conversion receives a tool-role message containing a provider-executed `ContentPartTypeToolApprovalResponse`
- **THEN** the converted provider API prompt SHALL preserve the approval response when the provider supports it

### Requirement: Root content includes approval requests

The root SDK content model used by `StepResult.Content`, `StreamTextResult.Content`, and `GenerateTextResult.Content` SHALL include a tool approval request content variant. The variant SHALL carry approval ID, tool call ID, tool name, input, dynamic flag, title, provider-executed flag, and automatic-approval metadata when available.

#### Scenario: Step content records approval request
- **WHEN** a step emits an approval request for a tool call
- **THEN** `StepResult.Content` SHALL contain a tool approval request content value correlated with the same tool call ID

#### Scenario: Approval request content serializes for consumers
- **WHEN** a result containing approval request content is marshaled to JSON by a consumer
- **THEN** the approval ID and tool call ID SHALL be available in the serialized representation

### Requirement: Provider-emitted approval requests are surfaced by orchestration

When a provider emits `PartToolApprovalRequest`, orchestration SHALL surface it as a stream approval request and SHALL record it in step content. The request SHALL be associated with the previously emitted provider-executed tool call for the same tool call ID when one exists.

#### Scenario: Provider approval request becomes stream event
- **WHEN** a provider stream includes `PartToolApprovalRequest` for a provider-executed tool call ID
- **THEN** `StreamText` SHALL emit a tool approval request stream part with the same approval ID and tool call ID

#### Scenario: Provider approval request becomes content
- **WHEN** a provider approval request is processed in a completed step
- **THEN** the step content SHALL include a tool approval request content value for that approval ID

#### Scenario: Unknown provider approval tool call is still surfaced
- **WHEN** a provider emits `PartToolApprovalRequest` before orchestration has seen the matching tool call
- **THEN** `StreamText` SHALL still emit the approval request with the provided approval ID and tool call ID
- **AND** it SHALL NOT synthesize a local tool execution for that provider-executed request
