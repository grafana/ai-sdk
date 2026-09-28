## Why

Issue #220 remains an implementation gap against the registered `@ai-sdk/openai` 4.0.72 baseline: the Go Responses provider forwards GPT-6 reasoning efforts without the upstream capability check and cannot express capability-gated async function settings, reasoning-effort configuration updates, or explicit compaction triggers. Its existing Astra sampling correction and compaction *output* support do not cover these contracts.

## What Changes

- Gate request-level reasoning effort, async function/custom-tool declarations, and reasoning-effort updates by model capabilities; preserve existing behavior for pre-GPT-6 models except for newly requested unsupported features.
- Propose typed OpenAI provider/tool options for the new controls, subject to explicit API approval before implementation. Insert a supported configuration update before converted prompt history and an explicit compaction trigger after it without mutating callers' messages.
- Carry present async metadata (including explicit `false`) on function/custom tool calls in generate and stream, reconstruct it during continuation, and reject unsupported execution-denied programmatic result continuations as the registered upstream does. Coordinate any required root orchestration changes against existing concurrent execution and approval paths rather than creating a new scheduler by assumption.
- Add deterministic provider request/response and core continuation tests, keeping provider-boundary fixture provenance honest.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `openai-responses-provider`: Extend model capability gating, typed provider/tool controls, Responses input ordering, call metadata, and programmatic continuation constraints for generate and stream.

## Impact

`providers/openai/models.go`, `options.go`, `convert_request.go`, `prepare_tools.go`, `convert_response.go`, `stream_adapter.go`, `convert_provider_tool_continuation.go`, and related tests are the principal surfaces; root `streamtext.go`/tool-message projection and `provider/` need verified coordination only if existing metadata/approval semantics cannot express the baseline behavior. The public Go option additions received API owner approval; no unrelated provider, frontend wire-format, or #164 reasoning-summary change is proposed. This is a candidate-source validated parity work package for `@ai-sdk/openai` 4.0.72 at `4e8c387622ee1bb0d55841664416d38754d5c9a3`, not a pin upgrade. Request snapshots and synthetic tests alone do not prove live OpenAI acceptance; do not synthesize `recorded/` or `upstream/` provider inputs.
