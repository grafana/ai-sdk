## ADDED Requirements

### Requirement: Provider wire excludes obsolete tool approval result stream part

The provider wire package SHALL round-trip `PartToolApprovalRequest` stream parts and SHALL NOT include tests, fixtures, or compatibility aliases for the obsolete `tool-approval-result` stream part. Approval decisions SHALL cross the provider wire as prompt content parts of type `tool-approval-response` when included in `CallOptions.Prompt`.

#### Scenario: Tool approval request stream part round-trips
- **WHEN** a `provider.StreamPart{Type: PartToolApprovalRequest, ApprovalID: "apr_1", ToolCallID: "call_1"}` is encoded and decoded through the provider SSE wire helpers
- **THEN** the decoded stream part SHALL preserve `Type`, `ApprovalID`, and `ToolCallID`

#### Scenario: Tool approval result is absent from stream part coverage
- **WHEN** the provider wire round-trip test enumerates every defined `provider.StreamPartType`
- **THEN** it SHALL NOT include `tool-approval-result`

#### Scenario: Approval response prompt content round-trips
- **WHEN** `provider.CallOptions` contains a tool-role message with `ContentPartTypeToolApprovalResponse`
- **THEN** provider wire request encoding and decoding SHALL preserve the approval ID, approved value, reason, and provider-executed flag
