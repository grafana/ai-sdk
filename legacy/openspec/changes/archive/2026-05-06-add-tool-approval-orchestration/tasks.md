## 1. Provider Protocol Alignment

- [x] 1.1 Remove `provider.PartToolApprovalResult` and update provider stream part tests, wire SSE tests, and all no-op orchestration references.
- [x] 1.2 Add `ContentPartTypeToolApprovalRequest`, approval request fields/helpers, and JSON round-trip tests for assistant approval request content.
- [x] 1.3 Update provider request conversion so local assistant approval request parts are stripped before provider API calls while provider-executed approval responses remain supported.
- [x] 1.4 Update provider wire request/SSE tests to cover `tool-approval-request` and remove `tool-approval-result` coverage.

## 2. Root Approval Data Model

- [x] 2.1 Add root stream/content part types for tool approval request and approval response events, including approval ID, tool call ID, automatic flag, provider-executed flag, and reason fields.
- [x] 2.2 Add root result content support so `StepResult.Content`, `StreamTextResult.Content`, and `GenerateTextResult.Content` can expose approval requests.
- [x] 2.3 Add `Tool.NeedsApproval` configuration with static and dynamic forms and unit tests for default, static, dynamic true, dynamic false, and dynamic error cases.

## 3. Approval Request Orchestration

- [x] 3.1 Normalize tool approval decisions during tool execution and prevent execution for local tool calls that require user approval.
- [x] 3.2 Emit approval request stream parts and record approval request content for blocked local tools.
- [x] 3.3 Keep unblocked local tool calls executing normally when mixed with approval-blocked calls in the same step.
- [x] 3.4 Surface provider-emitted `PartToolApprovalRequest` stream parts as root approval request events and step content.

## 4. Approval Response Resumption

- [x] 4.1 Implement approval collection from incoming messages by matching last tool-message responses to prior assistant approval requests and tool calls.
- [x] 4.2 Execute approved local tools before the first model call and append their tool results to the prompt sent to the provider.
- [x] 4.3 Convert denied local approvals into synthetic `ToolOutputExecutionDenied` tool results with denial reasons preserved.
- [x] 4.4 Skip already-resolved approval responses and return errors for unknown approval IDs or missing original tool calls.

## 5. Multi-Step And UI Wire Behavior

- [x] 5.1 Update the multi-step continuation condition so pending approvals stop the current invocation and resolved approvals continue normally.
- [x] 5.2 Add `tool-approval-request` and `tool-approval-response` UI chunk types, JSON marshaling, and `translateToChunks` coverage.
- [x] 5.3 Update `assembleResponseMessage` so approval request/response chunks update tool invocation approval state without losing later tool outputs.
- [x] 5.4 Verify `ConvertToModelMessages` emits approval request/response content needed for the two-call UI flow.

## 6. End-To-End Verification

- [x] 6.1 Add `StreamText` tests for first-call approval request emission, mixed blocked/unblocked tools, and dynamic approval errors.
- [x] 6.2 Add `StreamText` tests for second-call approved execution, denied synthetic result, duplicate-result skipping, and invalid approval references.
- [x] 6.3 Add `GenerateText` tests proving approval request and approval resumption behavior matches `StreamText`.
- [x] 6.4 Extend `test/conformance` config parsing/builders in Go and TypeScript to support tool approval configuration and approval-resumption message setup.
- [x] 6.5 Add recorded conformance fixtures for approval request, approved tool execution, and denied tool execution flows, with `expected.jsonl` generated from the local upstream TypeScript SDK.
- [x] 6.6 Run `make fmt`, `make test`, targeted `anthropic` module tests covering request conversion changes, and `make test-conformance`.

## 7. Upstream Approval Policy Parity

- [x] 7.1 Add call-level `WithToolApproval` API supporting generic policy functions and per-tool policy maps.
- [x] 7.2 Normalize approval statuses to upstream semantics: not applicable, user approval, approved, and denied with optional reasons.
- [x] 7.3 Make call-level approval policy take precedence over tool-defined `NeedsApproval`.
- [x] 7.4 Implement automatic approved and denied policy behavior, including automatic approval request/response events and execution-denied results.
- [x] 7.5 Add unit tests for precedence, generic policy, automatic approved, automatic denied, and reason propagation.
- [x] 7.6 Run `make fmt`, `make test`, and `make test-conformance` after upstream parity changes.
