## Why

Issue #20 asks for current research before planning because the original request is old and likely stale. The registered upstream baseline now includes a first-class `Agent` / `ToolLoopAgent` API in `ai@7.0.19` plus `createAgentUIStream` helpers for `@ai-sdk/react@4.0.20`, while the Go port currently exposes the underlying loop only through `StreamText` / `GenerateText`; this creates upstream API drift and forces users to repeat reusable agent settings and UI bridging code.

## What Changes

- Add a first-class Go `Agent` abstraction with upstream-visible version identity (`agent-v1`), optional ID, tools, and `Generate` / `Stream` methods returning existing `GenerateTextResult` and `StreamTextResult` types.
- Add `ToolLoopAgent` as a reusable settings wrapper over the existing `GenerateText` / `StreamText` orchestration engine rather than a second tool-loop implementation.
- Give `ToolLoopAgent` the upstream default loop stop condition `StepCountIs(20)` when neither agent settings nor per-call options supply `WithStopWhen`, while preserving the existing lower-level `StreamText` default of one step.
- Support settings and per-call overrides for Go primitives that already exist: model, instructions/system messages, prompt/model/UI messages, tools, tool choice, active tools, stop conditions, tool approval, prepare-step, output, provider options, headers, timeouts, retry/model parameters, and callbacks.
- Add concrete Agent-only runtime context settings/call options. The Agent wrapper resolves reusable/per-call runtime context and composes it with existing `PrepareStep` and tool execution context paths without adding a lower-level `StreamText` runtime-context option in this change.
- Merge agent-level and per-call callbacks so both are invoked for the same lifecycle event, matching upstream intent while retaining existing Go callback state types and concurrency behavior.
- Pass the upstream Agent user-agent marker `ai-sdk-agent/tool-loop` through `provider.CallOptions.Headers` where providers honor call headers. This change does not modify provider modules, so actual outgoing network header behavior remains provider-dependent; providers such as OpenAI Responses that ignore call headers are documented as parity gaps unless later provider work is scoped.
- Add Agent UI stream helpers that accept an `Agent` and `UIMessage` history, perform the minimum pre-stream validation supported by the current Go UI data model, convert to model messages using existing conversion paths, call `Agent.Stream`, and return or pipe existing `UIMessageChunk` SSE output compatible with `@ai-sdk/react`.
- Document upstream-only fields that lack current Go equivalents as explicit Go adaptations, intentional deviations, or gaps, including `prepareCall`, `allowSystemInMessages`, `experimental_download`, `include`, `_internal` ID generators, tools-context/call-options-template behavior, telemetry, transforms, sandbox, repair/refine hooks, tool order, and TypeScript generic/schema inference.
- Update docs, examples, and parity coverage notes for the new Agent surface without changing provider interfaces or provider wire behavior.

## Capabilities

### New Capabilities

- `agent-tool-loop`: First-class Agent and ToolLoopAgent API, settings/per-call merge semantics, agent-specific loop defaults, callback merging, Agent-only runtime context propagation, provider call-header metadata insertion, and Agent UI stream helpers that preserve existing UI chunk/SSE compatibility.

### Modified Capabilities

- None.

## Impact

- **Root API**: new root-package types, constructor/options, helper functions, and godoc for `Agent`, `ToolLoopAgent`, Agent runtime context, and Agent UI stream helpers.
- **Core orchestration**: `ToolLoopAgent` delegates to current `StreamText` / `GenerateText`; no separate provider call loop and no change to `StreamText` defaults.
- **Runtime context**: Agent options wrap existing `PrepareStep` behavior so resolved runtime context reaches tool execution unless an explicit per-step context override is returned.
- **Provider call metadata**: Agent inserts the user-agent marker into `provider.CallOptions.Headers` only; no provider interface or provider implementation changes are included, and provider-specific header gaps must be documented.
- **UI/SSE**: helpers reuse `ConvertToModelMessages`, `StreamTextResult.ToUIMessageStream`, and `PipeUIMessageStreamToResponse`; emitted `UIMessageChunk` JSON and `[DONE]` framing remain unchanged.
- **Tests**: root unit tests for option merge/defaults/callbacks/runtime context/header metadata and UI helper validation behavior; parity-sensitive UI helper coverage using existing chunk output paths.
- **Docs/examples**: update agent-loop guidance to explain when to use lower-level `StreamText` versus reusable `ToolLoopAgent`, and add a minimal runnable example.
- **No provider changes**: provider interfaces, provider modules, provider wire protocol, and conformance fixture schema are unaffected except for parity coverage documentation of provider-specific call-header gaps.
