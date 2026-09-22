## Context

Apply only after WP11. Planning baseline is `c055bec` (WP8 `a2177d9`, WP7 `e0e6c01`); Nara bundle `e8f443d88763e2b96f615c2f3a9ff348accfce2d` and #106 define scope. PR #177 is an unaccepted reference, not a substitute for separate WP11/12 contracts.

Exact upstream `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e` supplies `language-model-v4-stream-part.ts`, `language-model-v4-tool-call.ts`, `language-model-v4-tool-result.ts`, Gateway `doStream`, and ai `generate-text/stream-text.ts` plus its tests. Gateway forwards parsed V4 parts; higher-level ai code executes tools and prepares later prompts. In particular, provider output tool-call input is stringified JSON, while next-request assistant history contains JSON input.

Current `ai-gateway/providerwire/v4/stream.go` has bounded text state, frame encoding and terminal ownership. WP7 already has independent broad request mapping; its response decoder must be tested for the added events. Existing provider and core tool-stream fixtures are evidence for their own layer, not proof that Gateway HTTP supports tools. WP8's fixed chain and metadata-only client remain the observation foundation.

## Goals / Non-Goals

**Goals:** Incremental input, supported calls/results, two-client real-handler loops, per-request logical observation and unchanged bounds/terminal semantics.

**Non-Goals:** Provider-defined tools, providerExecuted true, dynamic/preliminary true (WP13); approvals (WP14); richer result media/options; Gateway-side function execution, workflow persistence, trusted workflow-parent context, or effectful fallback.

## Decisions

### Bounded independent tool state

Extend the existing state machine, not a second stream engine. Track each tool ID through input-open → input-closed → call-emitted → optional result-emitted. IDs/names must be valid UTF-8 and non-empty; input IDs cannot be reopened or reused. Deltas/end require the matching open input; a call after an input sequence must use that ID and tool name and may appear only after input-end. A complete standalone tool-call without preceding input events is valid. Calls/results must not duplicate completed IDs; results require a matching call in the current stream. Support interleaved independent tool-input IDs and existing text events; do not require exactly one tool-input block globally. Use the request part-count limit to bound maps and retain IDs/names only, not accumulated input; the complete call carries authoritative input. Empty delta remains an explicit empty string. Metadata must precede the first text or tool event; finish requires no open text or tool-input block. A client-executed call need not have a result before finish.

### Basic results without provider-tool expansion

WP12 implements the registered basic tool-result wire envelope (toolCallId, toolName, non-null JSON result, optional isError) and explicit correlation/lifecycle checks. This is transport capability, not authorization for Gateway tool execution. Normal client-executed loops deliver local results in the next request, not as fabricated provider events. A deterministic recording model exercises basic result transport separately. Provider-defined tool configuration, providerExecuted true, dynamic/preliminary variants and provider-native execution integration belong to WP13. False/absent optional boolean markers normalize to disabled. Do not reinterpret a result as proof that the Gateway executed anything.

Use private allowlisted event DTOs. Preserve raw JSON result semantics within frame bounds without exposing arbitrary provider metadata. Input/schema/result content must not enter logs, metrics or metadata-only AO exports. Unsafe output or deferred families fail safely via existing synthetic-terminal rules.

### Stateless client orchestration

Use registered ai streamText with the Gateway model, deterministic local function and explicit step limit; pair with existing Go StreamText/tool-loop orchestration using providers/grafana. Assert two or more HTTP calls, subsequent assistant call and tool result history, one local execution per emitted call, final text and finish. No server state or hidden model loop. Test cancellation while input is open and between steps; do not add new automatic replay of tool execution. Existing SDK retry behavior is controlled in deterministic tests and documented, not rewritten.

The Go Gateway client decodes provider stream parts only; root orchestration converts them into UI chunks. If implementation changes UI behavior, add the mandated test/integration scenario parsed with parseJsonEventStream and uiMessageChunkSchema. Do not send UIMessageChunk over the ProviderWire endpoint.

### Observability and fallback

One HTTP model generation gets one canonical logical observation. A two-step tool loop gets two generations, not a fabricated server workflow. Retain approved per-request correlation; do not infer parentage from IDs or trust arbitrary headers. Extend normalized SDK tool event mapping as needed; Gateway keeps metadata-only exports and safe finish/error/timing. Tool names/IDs/data are content, never metric labels.

WP9's guard rejects streaming function tools/history before any fallback candidate. Direct routes execute normally. No replay/idempotency contract is introduced here; fallback remains effect-disabled even before a tool call has been emitted.

## Risks / Trade-offs

- Interleaved inputs versus text-only state → explicit independent tool-ID map bounded by existing part limits, table-driven valid/invalid transition tests.
- Basic result versus provider execution scope → separate transport fixture from client-local execution scenario and leave execution markers unsupported.
- Untrusted JSON and escaped strings → preflight complete-frame bounds before write, test exact and one-byte-over limits.
- Cancellation with active tools → reuse setup/cleanup ownership and deadline machinery; no extra unbounded goroutine.

## Migration Plan

Apply WP11 first. Commit any Apache fixes before pinning isolated Gateway changes; committed-pin GOWORK=off validation is required. Enable the streaming subset on direct routes, retain unary cases, and extend existing deployment smoke only when internally activated after WP10. Rollback rejects streaming tools while retaining WP11 unary support.

## Open Questions

No product decision blocks implementation. Validate exact source/tests again if accepted WP11 or registered versions change. Production activation remains outside this planning stage.
