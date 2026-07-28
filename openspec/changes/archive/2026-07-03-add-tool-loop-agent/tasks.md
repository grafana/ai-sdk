## 1. Upstream baseline and API shape

- [x] 1.1 Reconfirm `test/conformance/upstream.yaml` pins `ai@7.0.19` and `@ai-sdk/react@4.0.20`, and compare implementation targets against the exact upstream Agent files at those tags.
- [x] 1.2 Add root-package Agent version/identity types and the Agent interface using existing `GenerateTextResult`, `StreamTextResult`, `ToolSet`, and Go callback/result types.
- [x] 1.3 Define ToolLoopAgent construction API with required `provider.LanguageModel`, optional ID, tools, instructions/system messages, Agent-only runtime context, and reusable lower-level options without adding no-op unsupported upstream fields.
- [x] 1.4 Add compile-time interface checks and constructor tests proving construction does not call provider methods and exposes version, ID, and tools.

## 2. Settings merge, delegation, and call headers

- [x] 2.1 Implement internal config merging that applies reusable settings first and per-call options second without mutating Agent settings.
- [x] 2.2 Inject ToolLoopAgent's default `StepCountIs(20)` only when neither settings nor per-call options configured stop conditions.
- [x] 2.3 Implement Agent `Stream` by delegating to the existing `StreamText` engine and preserving retries, timeouts, message conversion, prepare-step, tools, approval, output, warnings, stream errors, and abort behavior.
- [x] 2.4 Implement Agent `Generate` by delegating through the existing `GenerateText` behavior so generated results inherit the stream-loop semantics.
- [x] 2.5 Add the `ai-sdk-agent/tool-loop` marker to `provider.CallOptions.Headers` at the root provider-call boundary, appending to any caller-supplied `User-Agent` and preserving unrelated custom headers.
- [x] 2.6 Document provider-specific header limitations in implementation docs/godoc: providers that honor call headers receive the marker, OpenAI Responses currently does not honor call headers, and provider default user-agent append semantics are not guaranteed without separate provider work.
- [x] 2.7 Add tests for settings application, per-call overrides, settings immutability across calls, Agent default stop count, explicit stop-condition overrides, unchanged direct `StreamText` one-step default, and provider `CallOptions.Headers` merge behavior.

## 3. Callbacks and runtime context

- [x] 3.1 Implement reusable-plus-per-call callback merging in reusable-then-call order for all lifecycle callbacks supported by the current Go API.
- [x] 3.2 Preserve `OnStepEnd` / `OnStepFinish` alias behavior when callbacks are merged.
- [x] 3.3 Add Agent-only runtime context setting and per-call override support with presence tracking so per-call context overrides reusable context only when supplied.
- [x] 3.4 Compose resolved Agent runtime context by wrapping the user `PrepareStep`: start with resolved Agent context, run user `PrepareStep`, preserve user errors and non-context overrides unchanged, and let a non-nil `PrepareStepResult.Context` override the Agent context for that step.
- [x] 3.5 Ensure nil `PrepareStepResult.Context` does not accidentally clear a resolved Agent runtime context; document that intentionally clearing to nil is not represented by the current Go result shape.
- [x] 3.6 Add tests for callback composition order, callback invocation counts, concurrent tool callback inheritance, reusable runtime context, per-call runtime context override, prepare-step context override, nil prepare-step context preserving Agent context, and settings immutability across calls.

## 4. Agent UI stream helpers and validation

- [x] 4.1 Implement a `createAgentUIStream`-style helper that accepts an Agent and UI message history, performs required pre-stream validation, converts messages before starting the provider stream, calls Agent `Stream`, and returns `UIMessageChunk` output from `ToUIMessageStream`.
- [x] 4.2 Implement an HTTP response helper that pipes the Agent UI stream through the existing `PipeUIMessageStreamToResponse` SSE writer.
- [x] 4.3 Validate static `ToolInvocationPart` tool names against the Agent tool set, validate known tool invocation states, and validate state-required fields for final-state `ToolInvocationPart` and `DynamicToolUIPart` values without requiring a separate prior input or approval-request part in the current Go UI model.
- [x] 4.4 Add tests proving conversion errors are returned before streaming, valid UI messages are passed as model messages, original messages are preserved, emitted chunks match the existing `StreamTextResult.ToUIMessageStream` path, invalid tool names are rejected, invalid tool states are rejected, final-state tool parts with missing required fields are rejected, and a persisted assistant message containing a single final-state tool invocation such as `ToolInvocationPart{State: ToolStateOutputAvailable}` is accepted and converted before streaming.
- [x] 4.5 Add HTTP tests proving Agent response helpers emit the existing SSE headers, chunk framing, and `[DONE]` sentinel.

## 5. Inherited orchestration behavior

- [x] 5.1 Add Agent stream tests proving pending local tool approval emits approval request events/chunks and stops the current invocation without another model step.
- [x] 5.2 Add Agent generate tests proving approved and denied approval responses resume through existing `GenerateText` approval collection semantics.
- [x] 5.3 Add Agent stream tests for unresolved external tools, provider-executed tools, structured output parsing, and stream error propagation to confirm delegation rather than reimplementation.
- [x] 5.4 Add focused UI chunk snapshot/unit coverage for Agent UI helpers when approval/tool chunks are present; add or update conformance fixtures only if the helper changes committed wire output expectations.

## 6. Upstream gap inventory, documentation, and parity coverage

- [x] 6.1 Update godoc for new Agent and ToolLoopAgent symbols, including supported settings, default stop condition, callback merge behavior, Agent runtime context semantics, call-header metadata boundary, minimum UI validation, and unsupported upstream-only gaps.
- [x] 6.2 Update `docs/guides/agent-loops.md` to explain lower-level `StreamText` loops versus reusable `ToolLoopAgent` and note that `StreamText` keeps the one-step default.
- [x] 6.3 Add or update a runnable example showing ToolLoopAgent with tools and an Agent UI stream/HTTP helper.
- [x] 6.4 Update `test/conformance/PARITY.md` to classify the new Agent core orchestration and Agent UI/frontend interop surfaces, including Go adaptations/gaps for `prepareCall`, `allowSystemInMessages`, `experimental_download`, `include`, `_internal` ID generators, `toolsContext`/call-options-template behavior, telemetry, transforms, sandbox, repair/refine hooks, `toolOrder`, TypeScript generic/schema inference, provider call-header behavior, and remaining `validateUIMessages` differences.
- [x] 6.5 Ensure unsupported upstream-only fields are documented as gaps/non-goals and are not exposed as silent no-op public options.

## 7. Verification

- [x] 7.1 Run `gofmt`/`mise run fmt` for changed Go files.
- [x] 7.2 Run focused root tests for Agent, StreamText delegation, callback merging, runtime context, UI message validation/conversion, and HTTP helpers.
- [x] 7.3 Run `go test ./...` or `mise run test-short` after implementation.
- [x] 7.4 Run `mise run validate-parity-baseline` and `mise run parity-check` if parity coverage, conformance fixtures, or UI wire behavior changed.
- [x] 7.5 Run `openspec validate add-tool-loop-agent --type change --strict --json` before marking implementation complete.
