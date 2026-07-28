## Context

The issue describes the original human-in-the-loop plan, but the local Vercel AI SDK beta has moved on in a few important ways. Current upstream uses `resolveToolApproval` and `collectToolApprovals`, represents approval requests in assistant model messages, represents user decisions as `tool-approval-response` parts in tool messages, emits UI chunks for both request and response, and has no `tool-approval-result` stream part.

The Go SDK already has several pieces in place: UI tool parts can carry approval state, `ConvertToModelMessages` emits approval responses and synthetic execution-denied results for denied UI responses, provider tool output includes `execution-denied`, and provider streams have a `tool-approval-request` type. The missing part is orchestration: `StreamText` does not decide whether a local tool needs approval, does not emit approval request content, does not resume approved tools before the next model call, and still carries the obsolete `PartToolApprovalResult` constant.

## Goals / Non-Goals

**Goals:**
- Align the Go SDK with current upstream approval semantics for `StreamText` and `GenerateText` through `StreamText`.
- Support tool-defined approval checks with static and dynamic forms using Go-native types.
- Model approvals as a stateless two-call flow: first call records a request, second call carries responses and resolved tool results.
- Preserve `@ai-sdk/react` UI wire compatibility for approval request and response chunks.
- Remove provider API and wire drift caused by the obsolete `tool-approval-result` stream part.
- Add recorded conformance coverage that proves Go UI chunks match the current upstream TypeScript SDK for approval request, approved execution, and denied execution flows.

**Non-Goals:**
- Blocking on an in-process approval callback while a stream is open.
- Persisting approval state inside the SDK between calls.
- Adding a new provider-specific approval backend beyond forwarding provider-executed approval responses that existing providers support.
- Reworking unrelated tool execution, retry, timeout, or provider tool mapping behavior.

## Decisions

### Use upstream's status-based approval model

The public API should support both tool-defined `NeedsApproval` and call-level approval policy equivalent to upstream `toolApproval`. Internally, all approval checks normalize to the upstream status model: not applicable, user approval, approved, or denied, with optional reasons for automatic approval/denial.

Call-level policy takes precedence over tool-defined `NeedsApproval`. This lets callers reuse the same tool set across contexts with different safety policy, and keeps the Go SDK aligned with upstream's current capability rather than only the older issue-level API.

Alternative considered: only pass booleans through the execution path. That is simpler initially but misses upstream capabilities such as automatic approval/denial, reasons, and caller-owned approval policy.

### Treat approval request as conversation content, not provider execution state

Approval requests should be stored in result content and response messages alongside the tool call that triggered them. On the next call, orchestration scans the incoming messages for assistant approval requests and tool approval responses, correlates by `approvalId`, and executes or denies before the next model call.

Alternative considered: keep approval requests only in `StreamTextResult` and require callers to pass a side-channel token back. That would not match upstream, would be harder to persist, and would not work naturally with UI messages.

### Resolve approval responses before the first model call

At the beginning of `StreamText`, collect approvals from the standardized model messages. Approved local tool calls execute immediately and append tool-result messages to the prompt for the upcoming model call. Denied local tool calls become synthetic `execution-denied` tool results. Provider-executed denied approvals keep their provider-executed approval response available for providers that can consume it.

Alternative considered: make the model call first and then execute approved tools as a normal tool step. That would send the model an unresolved approval response and delay the actual tool result by another turn.

### Do not continue the multi-step loop for unresolved approvals

The continuation condition should only continue when all local client tool calls have either a concrete tool output or an approval denial response. Tool calls waiting for user approval deliberately do not have results, so the current call finishes after emitting the approval request.

Alternative considered: continue when there are any client tool calls, even if some are approval-pending. That would produce follow-up model calls without required tool results and violates upstream's missing-result checks.

### Align provider types with upstream current beta

Keep `PartToolApprovalRequest` as a provider stream part and remove `PartToolApprovalResult`. Add assistant-side approval request content so orchestration can persist requests in model messages, while keeping provider-facing conversion responsible for stripping local approval bookkeeping before provider calls. `tool-approval-response` remains the tool-message prompt part used for provider-executed approvals.

Alternative considered: preserve `PartToolApprovalResult` for backward compatibility. It is not in upstream V4, is unused in orchestration, and keeping it would continue to encode a protocol that current clients and providers do not understand.

### Use recorded conformance fixtures for approval UI parity

Approval behavior affects stream part ordering, generated approval IDs, UI chunk shapes, and assembled tool invocation states. Unit tests should cover internal decisions, but recorded conformance cases should lock the externally visible `StreamText` -> `ToUIMessageStream` behavior against the local upstream TypeScript SDK. The conformance config needs to support approval-specific tool metadata and second-call approval response setup so the same fixture replay can cover pending request, approved execution, and denied execution scenarios.

Alternative considered: rely only on hand-written Go tests. That would miss subtle upstream drift in chunk ordering and field mapping, which is exactly what the conformance suite is designed to catch.

## Risks / Trade-offs

- Approval request content in provider messages could leak to providers that do not strip it -> Add provider conversion tests and ensure local approval requests are skipped before outbound provider prompts.
- Tool execution during approval resumption happens before a model call -> Reuse existing tool execution callbacks, timeouts, and result construction so behavior matches normal local tool execution as closely as possible.
- Removing `PartToolApprovalResult` is a provider API break -> It is intentional upstream alignment; update wire tests and fail compile-time references during implementation.
- Dynamic approval functions can fail -> Surface failures as stream errors using existing error propagation rather than silently executing or approving tools.
- UI chunk shape must match upstream -> Add tests for request and response chunk translation and response message assembly.
- Conformance approval fixtures require config support not present today -> Extend `test/conformance` YAML, Go runner, and TypeScript generate/record tools before adding the recorded cases.

## Migration Plan

This change is source-breaking only for code that referenced `provider.PartToolApprovalResult`. Remove those references and use `PartToolApprovalRequest` for provider-emitted approval requests plus tool-message `ContentPartTypeToolApprovalResponse` for user decisions. Existing tools without approval configuration keep executing as before.

## Open Questions

- None. This change exposes both tool-defined `NeedsApproval` and call-level `WithToolApproval` while structuring orchestration around upstream's status model.
