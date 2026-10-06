## Context

The current Go root package already has the tool-loop engine that upstream `ToolLoopAgent` uses. `StreamText` starts an asynchronous multi-step loop, converts UI messages with `ConvertToModelMessages`, prepends system messages, resolves approval responses, applies `PrepareStep`, executes local tools, stops for external tools or pending approvals, and emits `TextStreamPart` events. `GenerateText` delegates to `StreamText` and drains the stream, so generated results already inherit stream-loop semantics. The lower-level loop defaults to `StepCountIs(1)` when `WithStopWhen` is not supplied.

The UI bridge also already exists. `StreamTextResult.ToUIMessageStream` translates text stream parts to `UIMessageChunk`, and `PipeUIMessageStreamToResponse` writes `text/event-stream` with the `x-vercel-ai-ui-message-stream: v1` header and `[DONE]` sentinel. These chunks are the compatibility boundary for `@ai-sdk/react`.

Pinned upstream `ai@7.0.19` defines an `Agent` interface with `version: 'agent-v1'`, optional `id`, `tools`, `generate`, and `stream`. Upstream `ToolLoopAgent` is not a separate orchestrator; it prepares settings, defaults `stopWhen` to `isStepCount(20)`, merges reusable and per-call callbacks, appends the `ai-sdk-agent/tool-loop` user-agent suffix, and calls `generateText` / `streamText`. Upstream `createAgentUIStream` validates UI messages against `agent.tools`, converts them to model messages, calls `agent.stream`, and passes the resulting stream through `toUIMessageStream` with the original UI history.

## Goals / Non-Goals

**Goals:**

- Add a root-package Agent surface that is recognizable against upstream `agent-v1` while staying idiomatic Go.
- Implement `ToolLoopAgent` as a reusable wrapper over `GenerateText` and `StreamText`.
- Preserve existing lower-level defaults, especially `StreamText` defaulting to one step.
- Match upstream observable Agent semantics where local primitives exist: settings reuse, per-call overrides, default `StepCountIs(20)`, callback merging, tool approval and prepare-step passthrough, Agent-only runtime context passthrough, and user-agent marker insertion into provider call headers.
- Provide Agent UI helpers that reuse existing UI message conversion and UI chunk/SSE serialization, preserving wire compatibility.
- Make upstream-only gaps explicit and test their omission as non-goals rather than silently adding placeholder APIs.

**Non-Goals:**

- Reimplementing tool orchestration, approval handling, structured output parsing, or UI chunk translation outside the existing `StreamText` pipeline.
- Changing provider interfaces, provider request/response wire protocol, provider modules, or provider conformance fixture formats.
- Guaranteeing that every provider's actual outbound network request contains the Agent user-agent marker. This change only passes the marker through `provider.CallOptions.Headers`; provider implementations that ignore or replace call headers remain documented parity gaps.
- Changing existing `StreamText` / `GenerateText` option behavior or one-step default.
- Providing TypeScript-level generic inference, call option schema validation, upstream `prepareCall`, sandbox session support, telemetry, stream transforms, tool ordering, repair-tool-call hooks, or refine-tool-input hooks unless separate Go primitives are added by future changes.
- Adding a new frontend wire format or changing emitted `UIMessageChunk` discriminators.

## Decisions

### Decision 1: Root package Agent interface and concrete ToolLoopAgent

**Choice:** Add the Agent surface to the root `aisdk` package, alongside `StreamText`, `GenerateText`, `Tool`, and UI helpers. The interface exposes version identity, optional ID, tools, `Generate(ctx, ...options)` and `Stream(ctx, ...options)`. `ToolLoopAgent` is a concrete implementation constructed with a required `provider.LanguageModel` and functional settings/options.

**Rationale:** Existing orchestration APIs are root-package APIs, and users already compose tools, messages, and HTTP helpers from that package. A subpackage would create import cycles or force wrappers around root result types. Keeping Agent in root preserves the flat API style used by the repo.

**Alternatives considered:**

- Subpackage `agent`: cleaner namespace, but it would depend on root types and could not be imported by root helpers without cycles.
- Generic-heavy Go API mirroring TypeScript: not idiomatic and does not map well to existing untyped `ToolSet`, `Output`, and callback option patterns.

### Decision 2: Reuse existing call options with a narrow Agent merge layer

**Choice:** Agent settings and per-call arguments reuse the functional option families already used by `StreamText` / `GenerateText` wherever a lower-level primitive exists. Agent-only settings are limited to the required model, optional ID, Agent runtime context, and Agent-specific call-header marker injection. Internally, `ToolLoopAgent` builds base stream/generate configs, applies settings first, applies call options second, injects the Agent default stop condition only when neither layer set one, composes runtime context through `PrepareStep`, and delegates to `streamTextWithConfig` / `GenerateText` equivalents.

**Rationale:** The current option surface already covers model messages, UI messages, system/instructions, tools, tool choice, active tools, stop conditions, tool approval, prepare-step, output, provider options, headers, timeouts, retry/model parameters, and callbacks. Reusing it avoids a parallel Agent option hierarchy that would immediately drift. The only new Agent-only options are for concepts that do not have a current lower-level option but can be implemented entirely in the wrapper.

**Alternatives considered:**

- Store settings as `[]StreamOption` / `[]GenerateOption` only and replay them for every call. This is simple but makes it hard to inspect whether `WithStopWhen` was set, merge callbacks, or compose runtime context without mutating stored settings.
- Add a broad `ToolLoopAgentSettings` struct mirroring upstream exactly. This would expose unsupported fields and create placeholders for features the Go port does not implement.

### Decision 3: ToolLoopAgent default stop condition is local to Agent

**Choice:** `ToolLoopAgent` applies `StepCountIs(20)` when no stop condition exists in either reusable settings or per-call options. The lower-level `StreamText` path remains unchanged and continues to default to `StepCountIs(1)`.

**Rationale:** This matches upstream `ToolLoopAgent` without a breaking change for existing `StreamText` users. It also preserves current docs/examples that show explicit `WithStopWhen` when users want multi-step lower-level loops.

**Alternatives considered:**

- Change `StreamText` default to 20. This would be surprising and potentially expensive for existing users.
- Require every Agent caller to pass `WithStopWhen`. That would miss upstream's default Agent behavior.

### Decision 4: Callback merge composes settings and per-call callbacks

**Choice:** When both Agent settings and per-call options supply the same lifecycle callback, `ToolLoopAgent` invokes both in settings-then-call order. The merged callback uses existing Go callback state types. Callback concurrency remains inherited from `StreamText`, including concurrent tool execution callbacks.

**Rationale:** Upstream `ToolLoopAgent` uses `mergeCallbacks` so reusable and call-specific observers both run. Settings-then-call is deterministic and lets call-level code observe effects after reusable instrumentation. Avoiding internal synchronization preserves existing concurrency semantics and performance.

**Alternatives considered:**

- Let per-call callbacks replace settings callbacks. This would be simpler but diverges from upstream and makes reusable instrumentation fragile.
- Add locks around callback invocation. This would serialize concurrent tool callbacks and diverge from current documented behavior.

### Decision 5: Runtime context is Agent-only and composes through PrepareStep

**Choice:** Add concrete Agent-only runtime context settings and per-call options. Exact exported names should follow existing option conventions during implementation, but the semantics are fixed:

1. Resolve runtime context before delegation: per-call runtime context wins when present; otherwise reusable Agent runtime context is used when present; otherwise no runtime context is injected.
2. Wrap the user-supplied `PrepareStep` callback, if any, instead of adding a lower-level `StreamText` runtime-context option in this change.
3. Start from the resolved Agent runtime context for the step.
4. Run the user `PrepareStep` with the same state and preserve its error behavior unchanged.
5. Preserve all non-context `PrepareStepResult` overrides exactly as direct `StreamText` would.
6. Override the step context with `PrepareStepResult.Context` only when the result intentionally supplies a context. With the current Go shape, a non-nil `Context` is the explicit signal; a nil `Context` means "do not override" so unrelated prepare-step overrides do not accidentally erase the Agent runtime context.
7. Pass the resulting step context to tool execution through the existing `ToolExecutionOptions.Context` path.

**Rationale:** Go already has `ToolExecutionOptions.Context` and `PrepareStepResult.Context`; Agent should not invent another propagation path. Wrapping `PrepareStep` gives parity for upstream `runtimeContext` where the current Go engine can support it, while keeping lower-level `StreamText` API scope unchanged.

**Go adaptation:** The current `PrepareStepResult.Context any` field cannot distinguish an omitted context from an intentional nil override. This change treats nil as omitted for Agent runtime context composition; intentionally clearing a non-nil Agent runtime context to nil is not represented and must be documented as a Go adaptation/gap if needed later.

**Alternatives considered:**

- Add a lower-level `WithRuntimeContext` primitive to `StreamText` / `GenerateText`. That would widen scope beyond the wrapper and require lower-level API design.
- Store runtime context only on Agent and never allow per-call override. This would be too rigid for shared Agent instances.
- Add provider-level metadata for runtime context. That would leak orchestration data into provider wire.

### Decision 6: Agent user-agent marker is CallOptions metadata, not a provider guarantee

**Choice:** `ToolLoopAgent` appends the upstream marker `ai-sdk-agent/tool-loop` to the `User-Agent` entry in `provider.CallOptions.Headers` during config merge. It must preserve caller-supplied headers and append to any caller-supplied `User-Agent` value rather than replacing it. The requirement stops at the provider call boundary: providers that honor call headers will see the marker, and providers that ignore or replace call headers remain parity gaps.

**Rationale:** The proposal explicitly keeps provider interfaces and modules out of scope. The root orchestration layer can populate `provider.CallOptions.Headers`, but it cannot guarantee physical outbound HTTP headers for providers that do not consume those headers. Narrowing the requirement avoids silently adding unplanned provider work.

**Known provider gaps to document:**

- OpenAI Responses currently does not honor `provider.CallOptions.Headers` for SDK calls, so actual outgoing requests will not contain the Agent marker without provider work.
- OpenAI-compatible providers may apply call headers after setting their own defaults; if a call-level `User-Agent` replaces rather than appends to provider defaults, this does not fully match upstream suffix chaining.

**Alternatives considered:**

- Add provider changes now to guarantee actual outgoing headers. This would conflict with the approved no-provider-changes scope and require provider-specific request tests.
- Omit the marker entirely. This would miss a visible upstream `ToolLoopAgent` behavior where providers already honor call headers.

### Decision 7: Agent UI helpers wrap existing conversion and add minimum pre-stream validation

**Choice:** Add helpers equivalent in intent to upstream `createAgentUIStream` and response piping: validate UI messages before starting the provider stream, convert UI messages to model messages, call `Agent.Stream`, then return `result.ToUIMessageStream` with `OriginalMessages` set to the validated UI history. HTTP helpers pipe the returned `UIMessageChunk` stream with the existing `PipeUIMessageStreamToResponse` implementation.

Minimum in-scope validation for this change is limited to what the current Go UI data model represents:

- Static `ToolInvocationPart` tool names must be non-empty and must exist in the Agent tool set unless they are explicitly represented as dynamic/provider-executed tool parts.
- Tool invocation states must be one of the known `ToolInvocationState` constants.
- A static `ToolInvocationPart` or `DynamicToolUIPart` in a final state (`output-available`, `output-error`, `output-denied`, or `approval-responded`) represents the tool call itself when it has a non-empty `ToolCallID`, a non-empty tool name, and the fields required by that state. The current model must not require a separate prior input or approval-request part for those final states.
- If a future Go UI model introduces separate result or approval-response parts that do not themselves represent the tool call, only those separate parts must cross-reference a prior represented tool call or approval request with the same ID and tool name.
- Existing `ConvertToModelMessages` conversion errors remain pre-stream errors.

Remaining upstream `validateUIMessages` differences are explicit parity gaps for this change, including deeper ordering rules, schema validation, provider-specific part validation, separate result-part cross-reference validation for any future UI model that represents those parts separately, and any upstream UI message features not represented by current Go `UIMessage` parts.

**Rationale:** UI compatibility lives in `UIMessageChunk` serialization and SSE framing. Reusing the current path means Agent helpers cannot accidentally introduce a divergent chunk schema. Adding the minimum validation prevents the most important upstream-observable invalid tool history cases from reaching the model while avoiding a broad UI validator rewrite.

**Alternatives considered:**

- Make Agent write SSE directly. This duplicates filtering, metadata, headers, and `[DONE]` handling.
- Return `StreamTextResult` only and ask users to call `ToUIMessageStream`. This leaves the upstream `createAgentUIStream` gap unresolved and repeats boilerplate in chat handlers.
- Port upstream `validateUIMessages` wholesale. This would exceed the narrow wrapper change because the Go UI data model does not represent every upstream validation dimension.

### Decision 8: Upstream-only settings are documented gaps, not no-op options

**Choice:** Do not add no-op fields/options for unsupported upstream Agent settings. The implementation and parity docs must classify the pinned `ai@7.0.19` inventory as follows:

| Upstream field/behavior | Classification for this change |
| --- | --- |
| `model`, `instructions`, `tools`, `toolChoice`, `activeTools`, `stopWhen`, `toolApproval`, `prepareStep`, `output`, callbacks, provider options, headers, timeout/retry/model parameters | Supported through existing Go primitives or Agent merge layer. |
| `runtimeContext` | Supported as an Agent-only Go adaptation composed through wrapped `PrepareStep` and tool execution context. |
| Agent user-agent suffix | Supported at `provider.CallOptions.Headers` boundary only; provider network-header behavior remains provider-specific gap. |
| `prepareCall` | Unsupported gap. It templates/overrides call parameters before `generateText`/`streamText`; Go `PrepareStep` is per-step and not equivalent. |
| `allowSystemInMessages` | Intentional deviation/gap. This change keeps existing Go message conversion behavior and does not add Agent-specific rejection of system messages. |
| `experimental_download` | Unsupported gap; no current Go Agent/download primitive. |
| `include` | Unsupported gap unless a lower-level provider option already represents the same behavior explicitly. No silent Agent option. |
| `_internal` ID generators | Go adaptation. Use existing ID/message generation hooks where already exposed; do not expose an upstream `_internal` namespace. |
| `toolsContext` and call-options-template behavior | Partial Go adaptation via Agent runtime context; upstream tools-context templating and call-options templates remain unsupported gaps. |
| `telemetry` | Unsupported gap; no current Go telemetry primitive in scope. |
| stream transforms | Unsupported gap; no Agent stream transform option in scope. |
| sandbox sessions | Unsupported gap; no sandbox primitive in scope. |
| `repairToolCall` / repair hooks | Unsupported gap; no repair hook primitive in scope. |
| `refineToolInput` / refine hooks | Unsupported gap; no refine hook primitive in scope. |
| `toolOrder` | Unsupported gap; current Go tool execution/order behavior is inherited from `StreamText`. |
| TypeScript generic/schema inference and `callOptionsSchema` | Go adaptation/non-goal; use idiomatic Go types and implemented runtime validation only. |

**Rationale:** No-op API surface is worse than missing API surface because users may believe behavior exists. Separate changes can add real Go primitives later.

**Alternatives considered:**

- Add fields that are ignored with warnings. This creates noisy configuration with no behavior and complicates tests.
- Block the Agent API until every upstream field exists. This withholds useful parity over already-supported primitives.

## Risks / Trade-offs

- **[API naming drift]** → Mitigation: keep upstream-identifiable concepts (`Agent`, `ToolLoopAgent`, `agent-v1`, `Generate`, `Stream`, UI stream helpers), but finalize exact Go signatures in implementation using existing root package option conventions and godoc.
- **[Option merge bugs]** → Mitigation: add focused tests proving settings are applied, per-call values override settings, callbacks compose, explicit stop conditions suppress the Agent default, and runtime context composition does not mutate stored settings.
- **[Callback race expectations]** → Mitigation: document that Agent callbacks inherit `StreamText` callback concurrency, especially tool execution callbacks.
- **[UI validation gap]** → Mitigation: implement the minimum pre-stream validation listed in Decision 7 and classify remaining upstream `validateUIMessages` differences in parity coverage.
- **[User-agent provider gap]** → Mitigation: test `provider.CallOptions.Headers` merge at the root boundary, document OpenAI Responses and provider default replacement behavior as gaps, and avoid claiming universal outgoing request headers.
- **[Parity source mismatch]** → Mitigation: implementation must compare against `ai@7.0.19` / `@ai-sdk/react@4.0.20` from `test/conformance/upstream.yaml`, not upstream main or latest docs.

## Migration Plan

This is additive. Existing `StreamText`, `GenerateText`, options, provider interfaces, and UI/SSE helpers remain valid. Rollback is deleting the new Agent files/docs/tests because lower-level orchestration remains unchanged.

## Open Questions

- Exact exported option type names should be finalized during implementation to minimize churn with the existing `Option`, `StreamOption`, and `GenerateOption` interfaces.
