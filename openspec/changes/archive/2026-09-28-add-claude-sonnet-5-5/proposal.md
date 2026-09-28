## Why

Claude Sonnet 5.5 (`claude-sonnet-5-5`) always thinks and rejects forced tool use. The Claude API, Vertex AI and Bedrock Converse return a 400 for `thinking: {"type":"disabled"}`, budget-based `enabled` thinking, and `tool_choice` `any` or `tool`. On `main` the model falls into the `claude-sonnet-5` capability row, so root reasoning `none`, a `required` or named tool choice, and the forced JSON response tool all produce requests the provider rejects. Vertex also resolves the undated ID to `claude-sonnet-5-5@latest`.

The registered baseline (`@ai-sdk/anthropic` 4.0.59, `@ai-sdk/amazon-bedrock` 5.0.90) does not know this model. Upstream added it in `@ai-sdk/anthropic` 4.0.67 and `@ai-sdk/amazon-bedrock` 5.0.99. Waiting for the next pinned-version upgrade would leave the model unusable for common calls, so this change ports that behavior ahead of the baseline. The specs name the upstream version each behavior comes from, and an `upstream-sync` issue tracks the missing upstream-backed evidence, so the next pinned-version upgrade reassesses it instead of rediscovering it.

## What Changes

- Add `claude-sonnet-5-5` to the direct Anthropic and undated Vertex model IDs, and `anthropic.claude-sonnet-5-5` to the Bedrock known model IDs.
- Add three Anthropic model capabilities, as upstream 4.0.67: rejects disabled thinking, rejects forced tool use, and supports `between_tools` thinking. Only `claude-sonnet-5-5` sets them.
- Add the `between_tools` thinking type (`ThinkingBetweenTools`) to Anthropic provider options and the Bedrock `reasoningConfig.type`.
- Anthropic and Vertex: root reasoning `none` sends `between_tools` on this model. Explicit `disabled` becomes `between_tools` and budget `enabled` becomes `adaptive`, each with an unsupported warning. `between_tools` at `xhigh` or `max` effort is lowered to `high` with a warning. `between_tools` counts as active thinking for sampling-parameter removal.
- Anthropic and Vertex: a `required` tool choice is sent as `auto`, and a named tool choice is sent as `auto` with only that tool, each with a warning. `structuredOutputMode: jsonTool` becomes native `outputFormat` when the model and transport support it. Otherwise the JSON response tool uses `auto` instead of `any`.
- Bedrock: the same forced tool-choice fallback, and a JSON schema response always uses the system-prompt instruction path instead of the forced JSON tool.
- Bedrock root reasoning `none` sends `between_tools`. This is an **intentional deviation**: upstream 5.0.99 maps it to `disabled`, which omits `thinking` and lets the model run adaptive thinking at its default effort.
- Bump `anthropic-sdk-go` from v1.75.0 to v1.76.0 for `BetaThinkingConfigBetweenToolsParam`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `model-capabilities`: add the three capability flags and the `claude-sonnet-5-5` row ahead of `claude-sonnet-5`; specify the thinking rewrites, forced tool-choice fallback and `between_tools` effort limit for models with those flags.
- `effort-level`: root reasoning `none` resolves to `between_tools` on models that support it; `display` is not sent with `between_tools`.
- `anthropic-structured-output`: `jsonTool` mode falls back to native output, and the JSON response tool uses `auto`, on models that reject forced tool use.
- `anthropic-model-ids`: undated Vertex IDs such as `claude-sonnet-5-5` resolve without `@latest`.
- `bedrock-provider`: `between_tools` reasoning, reasoning `none` deviation, forced tool-choice fallback and JSON-instruction routing for `claude-sonnet-5-5`.

## Impact

`providers/anthropic` (`models.go`, `options.go`, `reasoning.go`, `convert_request.go`), `providers/bedrock` (`model_family.go`, `models.go`, `options.go`, `convert_request.go`, `convert_tools.go`), `anthropic-sdk-go` v1.76.0 across the modules that require it (`providers/anthropic`, `ai-gateway`, `middleware/agentobservability`, `test/conformance`, two examples), `test/conformance/upstream.yaml`, and `docs/providers/anthropic.md`. The registered baseline, recorded fixtures and upstream fixtures are unchanged.
