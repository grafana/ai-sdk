# tool-approval-orchestration Specification

## Purpose

Define local and provider-executed tool approval orchestration for streaming and generate flows, including approval configuration, pending approval emission, resumed approval responses, provider prompt validation, UI message chunks, and multi-step behavior.

## Requirements

### Requirement: Tool approval configuration

Tool.NeedsApproval SHALL configure local approval: unset requires none; static always requires it; dynamic receives json.RawMessage input and ToolExecutionOptions and returns whether user approval is needed. Dynamic errors SHALL surface via normal stream error and SHALL prevent execution.

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

### Requirement: Call-level approval policy overrides tool approval

StreamText/GenerateText SHALL expose upstream-equivalent toolApproval as generic all-call function or per-tool map. Results SHALL support not applicable, user approval, approved and denied, with optional approved/denied reasons. Call-level policy SHALL take precedence over NeedsApproval.

#### Scenario: Not-applicable call policy suppresses static approval
- **WHEN** a per-tool call policy returns not applicable for a NeedsApproval tool
- **THEN** that call SHALL not require user approval under the overridden tool setting.

### Requirement: Approval configuration respects completed choice gates

New local approval configuration SHALL evaluate only steps without effective required/named choice violations. All local processing scenarios SHALL be subject to this condition. Provider-originated events and prior-input approval responses SHALL retain existing handling.

#### Scenario: Choice gate precedes approval configuration
- **WHEN** a completed response violates required choice
- **THEN** new local approval configuration SHALL not evaluate, while prior-input responses and provider-originated handling remain unchanged.

### Requirement: Approval request emission for blocked local tools

When a completed step has no effective required/named choice violation, a non-provider-executed call to an existing Execute tool requiring approval SHALL generate an approval ID, emit request stream part and record request content. The request SHALL reference approval ID and full resumable call identity. Blocked tools SHALL produce no result in that invocation; non-blocked siblings SHALL execute normally.

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

At StreamText start, orchestration SHALL inspect the last tool-role message for approval responses, correlate approvalId with earlier assistant request and toolCallId with original call. Approved local tools SHALL execute before next model call and append resulting tool-result message to prompt. Denied local approvals SHALL append synthetic execution-denied output with response reason.

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

Before each model call after approval resolution/per-step message overrides, orchestration SHALL validate the sanitized prompt. Non-provider-executed assistant calls SHALL have a subsequent tool-role result before next user/system message or prompt end, unless correlated approval response resolves them. Provider-executed calls SHALL NOT need client results.

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

### Requirement: Missing tool results report every unresolved identity

Unresolved calls SHALL produce MissingToolResultsError via normal stream error and SHALL prevent model invocation. ToolCallIDs SHALL include every unresolved ID in prompt encounter order. GenerateText SHALL inherit this through shared StreamText orchestration.

#### Scenario: Unresolved generated prompt never invokes the model
- **WHEN** GenerateText receives a prompt with several unresolved client tool calls
- **THEN** it SHALL surface MissingToolResultsError listing every unresolved ID in encounter order before model invocation.

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

ToUIMessageStream SHALL map approval stream parts to upstream-compatible chunks: tool-approval-request with approvalId/toolCallId and optional isAutomatic; tool-approval-response with approvalId/approved and optional reason/providerExecuted.

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

### Requirement: Approval chunks update the matching invocation state

assembleResponseMessage SHALL apply approval chunks to the corresponding invocation: pending state approval-requested with Approval.Approved unset; responded state approval-responded with approved value and reason populated.

#### Scenario: Pending request retains an unset decision
- **WHEN** a matching invocation receives an approval request then a response
- **THEN** assembly SHALL update the same part from approval-requested with unset Approved to approval-responded with supplied approved value and reason.

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

### Requirement: Approved streaming local tools use final results for approval continuation

For approval-requiring streaming local tools, automatic same-invocation approval and later approved-response resume SHALL use unblocked streamed execution semantics. Pending/denied requests SHALL NOT execute. Approved preliminary outputs SHALL stream but NOT enter prompts; only final output/error SHALL resolve the call. Existing response correlation/provider-executed handling SHALL remain unchanged.

#### Scenario: Pending streaming tool waits for user approval
- **WHEN** a streaming local tool requires user approval
- **THEN** its execution function SHALL not run during the request-emitting invocation
- **AND** no preliminary or final tool result SHALL be emitted for it

#### Scenario: Automatically approved streaming tool
- **WHEN** a streaming local tool receives automatic approval
- **THEN** preliminary outputs SHALL be visible after the approval events
- **AND** only its final result SHALL satisfy tool-result completion

#### Scenario: Resumed approved streaming tool
- **WHEN** an approved response to a prior streaming-tool approval is supplied in the input messages
- **THEN** preliminary outputs SHALL be visible before the next model call
- **AND** only the final result SHALL be appended to the continuation prompt

#### Scenario: Denied streaming tool
- **WHEN** approval is denied for a streaming local tool
- **THEN** the streaming execution function SHALL not run
- **AND** existing execution-denied prompt behavior SHALL apply

### Requirement: Choice violations create no new local approval effects

Completed effective required/named violations SHALL bypass the new local approval/execution phase in shared StreamText, GenerateText and both ToolLoopAgent paths. No call-level policy, dynamic NeedsApproval, new approval allocation/signing, automatic response/synthetic denial, local Execute/ExecuteStream or execution callback SHALL run for that step. This exception SHALL override ordinary local configuration/request/continuation rules.

#### Scenario: Unrelated tool requiring user approval
- **WHEN** named choice selects lookup but a completed response contains only another configured local call requiring static or dynamic user approval
- **THEN** its NeedsApproval function SHALL not run and no new local approval ID/request SHALL be created
- **AND** no local execution/result SHALL occur
- **AND** the received call SHALL remain visible alongside the terminal semantic failure

#### Scenario: Automatic approval or denial policy on a violating step
- **WHEN** a completed named-choice violation contains an unrelated tool with a generic or per-tool policy returning approved or denied
- **THEN** no approval policy SHALL run for that step
- **AND** no locally generated approval request/response, signing work or synthetic denial result SHALL occur
- **AND** no Execute, ExecuteStream or execution callback SHALL run

#### Scenario: Provider-originated events remain visible
- **WHEN** provider-executed unrelated calls, results or approval requests were received before a completed named-choice violation
- **THEN** their existing stream and recorded content handling SHALL remain observable
- **AND** the SDK SHALL not claim to undo those provider effects
- **AND** no additional local approval phase SHALL be entered

#### Scenario: Prior-message approvals are unaffected
- **WHEN** a submitted prior-message approval response resolves a local tool before the provider call and that call later violates tool choice
- **THEN** the prior-message approval execution and generated prompt results SHALL remain unchanged
- **AND** the new gate SHALL apply only to the violating response's new local approval/execution phase

#### Scenario: Valid choice retains ordinary approval behavior
- **WHEN** a completed response satisfies its effective named or required choice and contains eligible approval-requiring calls
- **THEN** ordinary pending, automatic approved and automatic denied handling SHALL remain unchanged

### Requirement: Choice gates do not undo provider or historical approval effects

Received provider calls/results/approval events SHALL retain stream/content handling including provider-event signing. The choice gate SHALL NOT undo provider effects or alter prior-input approval resolution/execution before the current model call. Choice-satisfying steps and other disallowed-finish approval behavior SHALL retain existing semantics.

#### Scenario: Historical execution remains before a violating model call
- **WHEN** a prior approved tool executes before a model call that later violates choice
- **THEN** its execution and prompt results SHALL remain unchanged, and received provider events SHALL retain existing handling.
