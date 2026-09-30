## MODIFIED Requirements

### Requirement: Tool approval configuration

The `Tool` API SHALL expose a `NeedsApproval` approval configuration for local tool calls. When unset, the tool SHALL NOT require approval. The configuration SHALL support a static form that always requires approval and a dynamic function form that receives the tool input as `json.RawMessage` plus `ToolExecutionOptions` and returns whether the specific invocation requires user approval.

`StreamText` and `GenerateText` SHALL also expose a call-level tool approval policy equivalent to upstream `toolApproval`. The call-level policy SHALL support a generic function for all tool calls and a per-tool policy map. Policy results SHALL support the upstream statuses: not applicable, user approval, approved, and denied, with optional reasons for approved and denied statuses. When both call-level policy and tool-defined `NeedsApproval` are present, call-level policy SHALL take precedence.

New local approval configuration SHALL be evaluated only for steps that do not violate their effective required/named tool choice. The local processing scenarios below SHALL be subject to this condition; provider-originated events and approval responses from prior input messages SHALL retain their existing handling.

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

When a completed step does not violate its effective required/named tool choice and contains a non-provider-executed tool call whose tool exists, has an `Execute` function, and requires user approval, `StreamText` SHALL generate an approval ID, emit a tool approval request stream part, and record a tool approval request content part in the step content. The approval request SHALL reference the approval ID and the full tool call identity needed to resume later.

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

## ADDED Requirements

### Requirement: Choice violations create no new local approval effects

A completed model response violating effective required/named tool choice SHALL bypass the whole new local approval/execution phase for that step. Shared StreamText orchestration, including GenerateText and both ToolLoopAgent entry points, SHALL NOT invoke call-level approval policies or dynamic NeedsApproval, allocate or sign new locally generated approvals, create automatic approval responses or synthetic denial results, or dispatch local Execute/ExecuteStream and execution callbacks for that step. This exception SHALL take precedence over the normal local approval configuration, request emission and continuation rules.

Already received provider-originated calls, results and approval events SHALL retain their existing stream/content handling, including existing provider-event signing behavior. This gate SHALL NOT undo provider effects or alter approval resolution/execution from prior input messages before the current model call. Steps satisfying tool choice and other disallowed-finish approval behavior SHALL retain existing semantics.

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
