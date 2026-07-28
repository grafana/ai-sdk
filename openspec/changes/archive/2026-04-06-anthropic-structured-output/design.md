## Context

The Anthropic provider currently discards `CallOptions.ResponseFormat` with a warning at `convert_request.go:109-115`. The orchestration layer (`StreamText`) correctly wires `Output.ResponseFormat()` into `CallOptions`, and `Output.ParseComplete()` validates the response on completion. The gap is entirely at the provider boundary.

The Anthropic API supports two mechanisms for structured output:
- **Native `output_config.format`**: Sends `{type: "json_schema", schema: ...}` in the request body. Available on newer models only.
- **Tool-based fallback**: Synthesizes a function tool with the schema as `inputSchema`, forcing the model to produce structured output via tool calling. Works on all models.

The Go SDK (`anthropic-sdk-go@v1.27.1`) supports both: `BetaOutputConfigParam.Format` for native mode, and `BetaToolParam` + `BetaToolChoiceUnionParam` for tool-based mode. The SDK also provides `BetaJSONSchemaOutputFormat()` which applies Anthropic-compatible schema transformations.

## Goals / Non-Goals

**Goals:**
- Enable structured output (`output.Object`, `output.Array`, `output.Choice`) for the Anthropic provider
- Support both native JSON schema mode and tool-based fallback, matching upstream behavior
- Handle the synthetic tool response transparently so the orchestration layer receives text content unchanged
- Work correctly when user tools and structured output are combined

**Non-Goals:**
- Schemaless JSON mode (`output.JSON()`) -- the Anthropic API does not support schemaless JSON mode; neither path (native or tool) works without a schema. This remains a warning.
- Provider option to override mode selection (e.g. `structuredOutputMode: "outputFormat"`) -- YAGNI for now; auto-detection covers all current models
- Changes to the orchestration layer or `Output` interface

## Decisions

### Decision 1: Dual-mode with auto-detection

Use native `output_config.format` when the model supports it, fall back to tool-based approach otherwise. Mode selection is driven by a new `supportsStructuredOutput` field on `modelCapabilities`.

**Why not tool-only?** Native mode is simpler (no tool injection, no stream remapping), more reliable (guaranteed schema compliance from the API), and better supported on newer models. Tool-based is a compatibility fallback for older models.

**Why not native-only?** Older Claude models (pre-4.5) don't support `output_config.format`. Dropping support for these models is not acceptable.

**Why auto-detect over a config option?** The model ID already determines capabilities in this codebase (max tokens, adaptive thinking). Adding structured output follows the same pattern. A provider option override can be added later if needed.

### Decision 2: Tool-based fallback mechanics

Synthesize a tool named `"json"` with `description: "Respond with a JSON object."` and the `ResponseFormat.Schema` as `inputSchema`. Override tool choice to `required` (OfAny) with `DisableParallelToolUse: true`.

This matches the upstream TypeScript implementation exactly. The json tool is appended to any existing user tools, so structured output + tool calling works: the model can call user tools in earlier steps and the json tool for final output.

**Alternative considered: OfTool instead of OfAny** -- Using `OfTool{Name: "json"}` would force exclusively the json tool, preventing user tools from being called. `OfAny` lets the model choose which tool to call, which is necessary for multi-step flows where real tools run before structured output.

### Decision 3: Stream remapping for tool fallback

When the json response tool is active, the `streamAdapter` intercepts events for the `"json"` tool and remaps them:
- `tool_use` content block start for `"json"` -> suppress (no `PartToolInputStart`)
- `input_json_delta` for the json tool -> emit `PartTextDelta` instead of `PartToolInputDelta`
- `content_block_stop` for the json tool -> suppress (no `PartToolCall`)
- `stop_reason: tool_use` -> remap to `stop` finish reason

This makes the tool-based path transparent to the orchestration layer. `StreamText` sees text deltas and a `stop` finish reason, so `Output.ParseComplete(step.Text)` works identically to the native path.

**Alternative considered: handle remapping in the orchestration layer** -- This would require `StreamText` to know about provider-specific tool conventions, breaking the abstraction boundary. Provider-level remapping keeps the concern where it belongs.

### Decision 4: State passing from request to stream

`buildParams` needs to signal to `streamAdapter` whether a json response tool was injected. Extend `buildParams` to return a `requestConfig` struct (or add a bool return) indicating `usesJsonResponseTool`. Pass this to the `streamAdapter` at construction.

The adapter uses this flag plus the tool name `"json"` to identify which content blocks to remap. This avoids relying solely on tool name matching, which could collide with a user-defined tool named `"json"` (unlikely but defensive).

### Decision 5: Tool choice override scope

When injecting the json response tool in fallback mode:
- If the user explicitly set `ToolChoice`, override it to `required` (OfAny). This matches upstream behavior.
- If the user set `ToolChoice` to `none`, the structured output request takes precedence -- the json tool must be callable.
- `DisableParallelToolUse` is set unconditionally to prevent the model from calling multiple tools in one turn, which would complicate the response.

### Decision 6: Model capability mapping

Add `supportsStructuredOutput bool` to `modelCapabilities`:

| Model | supportsStructuredOutput |
|---|---|
| claude-sonnet-4-6 / claude-opus-4-6 | true |
| claude-sonnet-4-5 / claude-opus-4-5 / claude-haiku-4-5 | true |
| claude-opus-4-1 | true |
| claude-sonnet-4-* (older) | false |
| claude-opus-4-* (older) | false |
| claude-3-haiku | false |
| unknown | false |

This matches the upstream TypeScript `getModelCapabilities()` mapping.

## Risks / Trade-offs

**[Risk] User tool named "json" collides with synthetic tool** -> Mitigation: The `usesJsonResponseTool` flag from `buildParams` gates remapping. If we want to be extra defensive, check for name collision and rename the synthetic tool or warn. Low probability, acceptable for now.

**[Risk] Tool-based fallback produces less reliable output than native mode** -> Mitigation: This is inherent to the approach -- tool-based output depends on the model following tool-calling conventions. Schema validation in `Output.ParseComplete()` catches malformed responses. Upstream has shipped this approach for a year+.

**[Risk] `DisableParallelToolUse` override may surprise users who set their own tool choice** -> Mitigation: This is necessary for correctness and matches upstream behavior. When structured output is requested, the tool choice override is expected. This only affects the tool fallback path; native mode doesn't touch tool choice.

**[Trade-off] Schema transformation** -> The SDK's `BetaJSONSchemaOutputFormat()` and `BetaToolInputSchema()` apply Anthropic-specific schema transforms (strip unsupported keys, force `additionalProperties: false`, convert `oneOf` to `anyOf`). We should use these helpers rather than passing raw schemas, accepting that some edge-case schemas may be transformed in unexpected ways. This matches how the upstream TypeScript SDK handles it.
