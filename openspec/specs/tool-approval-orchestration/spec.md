# tool-approval-orchestration Specification

## Purpose

Define local and provider-executed tool approval orchestration for streaming and generate flows, including approval configuration, pending approval emission, resumed approval responses, provider prompt validation, UI message chunks, and multi-step behavior.

## Requirements

### Requirement: Tool approval configuration

The `Tool` API SHALL expose a `NeedsApproval` approval configuration for local tool calls. When unset, the tool SHALL NOT require approval. The configuration SHALL support a static form that always requires approval and a dynamic function form that receives the tool input as `json.RawMessage` plus `ToolExecutionOptions` and returns whether the specific invocation requires user approval.

`StreamText` and `GenerateText` SHALL also expose a call-level tool approval policy equivalent to upstream `toolApproval`. The call-level policy SHALL support a generic function for all tool calls and a per-tool policy map. Policy results SHALL support the upstream statuses: not applicable, user approval, approved, and denied, with optional reasons for approved and denied statuses. When both call-level policy and tool-defined `NeedsApproval` are present, call-level policy SHALL take precedence.

If the dynamic approval function returns an error, orchestration SHALL surface that error through the normal stream error path and SHALL NOT execute the tool.

#### Scenario: Tool without approval executes normally
- **WHEN** a tool has no approval configuration and the model calls it
- **THEN** `StreamText` SHALL execute the tool using the existing local tool execution path
- **AND** no approval request SHALL be emitted

#### Scenario: Static approval blocks execution
- **WHEN** a tool has static approval enabled and the model calls it
- **THEN** `StreamText` SHALL emit an approval request for that tool call
- **AND** `StreamText` SHALL NOT call the tool's `Execute` function in that SDK call

#### Scenario: Dynamic approval receives execution options
- **WHEN** a tool has a dynamic approval function and the model calls it
- **THEN** `StreamText` SHALL call the approval function with the parsed input and `ToolExecutionOptions` containing the tool call ID, current messages, and step context

#### Scenario: Dynamic approval error stops the stream
- **WHEN** a dynamic approval function returns an error
- **THEN** `StreamText` SHALL emit a `StreamError` carrying that error
- **AND** the tool SHALL NOT be executed

#### Scenario: Call-level per-tool policy takes precedence
- **WHEN** a tool has `NeedsApproval` enabled but the call-level per-tool approval policy returns not applicable
- **THEN** `StreamText` SHALL execute the tool without emitting an approval request

#### Scenario: Call-level generic policy applies to all tools
- **WHEN** `StreamText` is configured with a generic approval policy function
- **THEN** the function SHALL receive the tool call, tools, input, and execution options for each local tool call

#### Scenario: Automatic approved policy executes tool
- **WHEN** call-level approval policy returns approved for a local tool call
- **THEN** `StreamText` SHALL emit automatic approval request and approval response events
- **AND** it SHALL execute the tool in the same invocation

#### Scenario: Automatic denied policy blocks tool
- **WHEN** call-level approval policy returns denied for a local tool call
- **THEN** `StreamText` SHALL emit automatic approval request and approval response events
- **AND** it SHALL NOT execute the tool
- **AND** the synthetic execution-denied result SHALL preserve the denial reason

### Requirement: Approval request emission for blocked local tools

When a completed step contains a non-provider-executed tool call whose tool exists, has an `Execute` function, and requires user approval, `StreamText` SHALL generate an approval ID, emit a tool approval request stream part, and record a tool approval request content part in the step content. The approval request SHALL reference the approval ID and the full tool call identity needed to resume later.

Blocked local tools SHALL NOT produce a tool result during the request-emitting call. Non-blocked local tools in the same step SHALL continue to execute normally.

#### Scenario: Approval request is emitted after tool input is available
- **WHEN** a model emits a local tool call for a tool that requires approval
- **THEN** the stream SHALL include the normal tool input available event for that tool call
- **AND** the stream SHALL include a tool approval request event with a non-empty approval ID and the same tool call ID

#### Scenario: Blocked tool is not executed
- **WHEN** a tool approval request is emitted for a local tool call
- **THEN** the tool's `Execute` function SHALL NOT be called during that `StreamText` invocation
- **AND** the step SHALL NOT contain a `ToolResult` for that tool call

#### Scenario: Mixed blocked and unblocked tools
- **WHEN** a step contains one tool call that requires approval and one tool call that does not
- **THEN** `StreamText` SHALL emit an approval request for the blocked tool
- **AND** `StreamText` SHALL execute the unblocked tool and emit its tool result normally

#### Scenario: Approval request appears in result content
- **WHEN** `StreamTextResult.Content()` or `GenerateTextResult.Content` is read after a blocked tool call
- **THEN** the content SHALL include the original tool call content and a tool approval request content part correlated by approval ID

### Requirement: Approval response collection before model calls

At the start of `StreamText`, orchestration SHALL inspect the supplied model messages for approval responses in the last tool-role message. Each approval response SHALL be correlated with an earlier assistant-side approval request by `approvalId` and with the original tool call by `toolCallId`.

For approved local tool approvals, orchestration SHALL execute the original local tool before making the next model call and append the resulting tool-result message to the prompt sent to the model. For denied local tool approvals, orchestration SHALL append a synthetic tool result whose model output type is `execution-denied` and whose reason is the approval response reason.

#### Scenario: Approved tool executes before next model call
- **WHEN** input messages contain an assistant approval request and a tool approval response with `approved: true`
- **THEN** `StreamText` SHALL execute the correlated local tool before calling the language model
- **AND** the prompt for the language model SHALL include a tool-result part for that execution

#### Scenario: Denied tool creates execution-denied result
- **WHEN** input messages contain an assistant approval request and a tool approval response with `approved: false`
- **THEN** `StreamText` SHALL NOT execute the correlated local tool
- **AND** the prompt for the language model SHALL include a tool-result part with `ToolOutputExecutionDenied`
- **AND** the denial reason SHALL be preserved when present

#### Scenario: Mixed approval responses preserve upstream event order
- **WHEN** input messages contain one approved local approval response and one denied local approval response
- **THEN** `StreamText` SHALL emit the denied output event before the approved tool output event
- **AND** the prompt for the language model SHALL append the approved tool result before the synthetic execution-denied result

#### Scenario: Existing tool result is not duplicated
- **WHEN** the last tool message already contains a tool-result for the approval request's tool call ID
- **THEN** approval response collection SHALL NOT execute the tool again
- **AND** it SHALL NOT append a duplicate synthetic result

#### Scenario: Unknown approval ID is an error
- **WHEN** a tool approval response refers to an approval ID with no prior approval request in the messages
- **THEN** `StreamText` SHALL surface an error and SHALL NOT call the language model

#### Scenario: Missing original tool call is an error
- **WHEN** an approval request exists but its referenced tool call cannot be found in prior assistant messages
- **THEN** `StreamText` SHALL surface an error and SHALL NOT call the language model

### Requirement: Provider prompts reject missing tool results

Before each language model call, after approval resolution and any per-step message override, orchestration SHALL validate the sanitized provider prompt. Every non-provider-executed assistant tool call SHALL have a corresponding tool result in a subsequent tool-role message before the next user or system message and before the end of the prompt, unless a correlated tool approval response resolves that call. Provider-executed tool calls SHALL NOT require a client tool result.

When one or more tool calls remain unresolved, orchestration SHALL surface a `MissingToolResultsError` through the normal stream error path and SHALL NOT invoke the language model. The error's `ToolCallIDs` SHALL contain every unresolved ID in prompt encounter order. `GenerateText` SHALL inherit this behavior through its shared `StreamText` orchestration path.

#### Scenario: All missing tool results are reported before the provider call
- **WHEN** a provider prompt contains multiple non-provider-executed assistant tool calls without corresponding tool results
- **THEN** `StreamText` SHALL surface a `MissingToolResultsError` before invoking the language model
- **AND** `ToolCallIDs` SHALL contain every missing tool call ID in prompt encounter order

#### Scenario: Tool result resolves a client tool call
- **WHEN** a non-provider-executed assistant tool call is followed by a tool-role result with the same tool call ID
- **THEN** prompt validation SHALL accept that tool call as resolved

#### Scenario: Provider-executed tool call does not require a client result
- **WHEN** an assistant tool call has `ProviderExecuted` set to true and no client tool result follows
- **THEN** prompt validation SHALL accept the provider-executed tool call

#### Scenario: Approval response resolves a pending tool call
- **WHEN** an assistant tool call and approval request are followed by a correlated tool approval response
- **THEN** prompt validation SHALL accept that tool call as approval-resolved
- **AND** existing approved and denied approval orchestration SHALL remain unchanged

#### Scenario: User or system message cannot precede a missing result
- **WHEN** a user or system message follows an unresolved non-provider-executed tool call
- **THEN** prompt validation SHALL surface `MissingToolResultsError` at that message boundary
- **AND** a tool result appearing later in the prompt SHALL NOT repair the malformed history

#### Scenario: Per-step message override is validated
- **WHEN** `PrepareStep` supplies messages containing an unresolved non-provider-executed tool call
- **THEN** `StreamText` SHALL surface `MissingToolResultsError`
- **AND** it SHALL NOT invoke the per-step language model

### Requirement: Approval-aware multi-step continuation

The multi-step loop SHALL continue only when all non-provider-executed tool calls in the step have corresponding tool outputs or denied approval responses. A tool call waiting for user approval SHALL be treated as unresolved and SHALL cause the current `StreamText` invocation to finish instead of starting another model step.

#### Scenario: Pending approval stops the current invocation
- **WHEN** a step emits a local tool call and an approval request but no tool result for that tool call
- **THEN** `StreamText` SHALL finish after that step
- **AND** it SHALL NOT start a follow-up model call in the same invocation

#### Scenario: Approved resumed tool can continue
- **WHEN** an approved tool is executed during approval response collection and stop conditions allow another step
- **THEN** `StreamText` SHALL include the tool result in the next model prompt
- **AND** normal multi-step continuation SHALL apply after the model response

### Requirement: UI message stream approval chunks

`ToUIMessageStream` SHALL translate approval request and approval response stream parts into UI message chunks compatible with the current upstream AI SDK UI protocol. The approval request chunk SHALL have type `tool-approval-request`, `approvalId`, `toolCallId`, and optional `isAutomatic`. The approval response chunk SHALL have type `tool-approval-response`, `approvalId`, `approved`, optional `reason`, and optional `providerExecuted`.

`assembleResponseMessage` SHALL apply these chunks to the corresponding tool invocation part so pending approvals use state `approval-requested` with `Approval.Approved` unset, and responded approvals use state `approval-responded` with the approved value and reason populated.

#### Scenario: Approval request chunk updates tool state
- **WHEN** a UI message stream contains a tool input available chunk followed by a tool approval request chunk for the same tool call ID
- **THEN** the assembled assistant message SHALL contain one tool invocation part in state `approval-requested`
- **AND** that part SHALL contain the approval ID with no approved value

#### Scenario: Approval response chunk updates tool state
- **WHEN** a UI message stream contains a tool approval response chunk for a prior approval request
- **THEN** the assembled assistant message SHALL mark the tool invocation state as `approval-responded`
- **AND** the approval approved value and reason SHALL be preserved

#### Scenario: Tool output replaces approval state
- **WHEN** a tool invocation receives a tool output chunk after approval response handling
- **THEN** the assembled assistant message SHALL move the invocation to `output-available`
- **AND** it SHALL preserve the approval metadata on the tool part

### Requirement: GenerateText inherits approval orchestration

`GenerateText` SHALL use the same approval orchestration semantics as `StreamText` because it is implemented by consuming `StreamText`. Approval requests, approval-resolved tool results, denied synthetic results, and content accessors SHALL match the streaming path.

#### Scenario: GenerateText returns pending approval content
- **WHEN** `GenerateText` receives a model response with a tool call requiring approval
- **THEN** the returned result SHALL include the tool call and approval request in `Content`
- **AND** the tool SHALL NOT have executed

#### Scenario: GenerateText resumes approved approval
- **WHEN** `GenerateText` is called with messages containing an approved approval response
- **THEN** it SHALL execute the approved local tool before the model call
- **AND** the returned steps SHALL include the resumed tool result before any subsequent model response content
