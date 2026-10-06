## Why

The registered AI SDK `ai@7.0.109` supports experimental tool callers that expose different tools to the model and the execution runtime, and local tools that stream preliminary outputs. Go currently exposes the same tool set to both and accepts only one output from `Tool.Execute`, so it cannot represent these upstream behaviors.

## What Changes

- Add an opt-in caller configuration for local and provider caller tools, including direct-call eligibility, per-step model visibility, local late binding, optional caller messages, and provider-option preparation. Preserve the existing behavior when unconfigured.
- Add an opt-in local tool execution stream that delivers preliminary outputs and a final output while retaining the existing single-result execution API.
- Apply the same preparation/execution semantics to `StreamText`, `GenerateText`, and Agent delegation, including active tools, approval handling, multi-step continuation, and cancellation.
- Test provider requests, stream/UI output, approval resumes, and manual provider options against the registered upstream reference. No JavaScript sandbox or new sandbox-provider packages.

## Capabilities

### New Capabilities

- `caller-aware-tool-routing`: Configuration, local/provider callers, model-visible versus executable tools, and per-step routing.
- `streamed-tool-results`: Preliminary and final local tool output lifecycle and stream/UI representation.

### Modified Capabilities

- `concurrent-tool-execution`: Extend executable tool eligibility and serialized event emission to streaming local tools without changing concurrent completion behavior.
- `tool-approval-orchestration`: Execute approved streaming tools, including approved resumptions, with preliminary outputs excluded from continuation.

## Impact

Root `Tool` and functional options API, tool preparation and execution in `streamtext.go`, approval resume, UI chunks, unit/conformance/integration tests, and docs/godoc. Provider contracts and adapters should remain unchanged where their existing provider-option passthrough already supports manually configured `allowedCallers`; automatic provider caller preparation remains an explicitly configured root-layer behavior. The registered upstream baseline is `ai@7.0.109` at `4e8c387622ee1bb0d55841664416d38754d5c9a3`; the issue's `7.0.107` references have no relevant implementation differences in the cited files. No breaking change to existing `Tool.Execute` is intended.
