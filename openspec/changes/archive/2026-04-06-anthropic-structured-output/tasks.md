## 1. Model Capabilities

- [x] 1.1 Add `supportsStructuredOutput` field to `modelCapabilities` struct in `anthropic/models.go`
- [x] 1.2 Update `getModelCapabilities` to set `supportsStructuredOutput: true` for claude-*-4-5, claude-*-4-6, and claude-opus-4-1
- [x] 1.3 Update existing model capabilities tests to cover the new field

## 2. Request Building -- Native Mode

- [x] 2.1 Replace the `ResponseFormat` unsupported warning in `buildParams` with structured output handling logic that branches on `supportsStructuredOutput`
- [x] 2.2 Implement native JSON schema path: set `OutputConfig.Format` via `BetaJSONSchemaOutputFormat()` when model supports it and `ResponseFormat.Type == "json"` with non-nil `Schema`
- [x] 2.3 Ensure native mode composes correctly with existing `OutputConfig.Effort` from provider options
- [x] 2.4 Add warning for schemaless JSON mode (`Type: "json"`, nil `Schema`) and no-op for `Type: "text"`

## 3. Request Building -- Tool Fallback

- [x] 3.1 Implement synthetic `"json"` tool creation: build `BetaToolParam` with name `"json"`, description `"Respond with a JSON object."`, and schema as `InputSchema` via `BetaToolInputSchema()`
- [x] 3.2 Append the synthetic tool to the existing tools list and override `ToolChoice` to `OfAny` with `DisableParallelToolUse: true`
- [x] 3.3 Extend `buildParams` return signature to communicate `usesJsonResponseTool` state to the caller

## 4. Stream Remapping

- [x] 4.1 Add `usesJsonResponseTool` flag to `streamAdapter` and wire it from `DoStream`
- [x] 4.2 Implement content block remapping: suppress `PartToolInputStart`/`PartToolInputEnd`/`PartToolCall` for the `"json"` tool, emit `PartTextDelta` from `input_json_delta`
- [x] 4.3 Implement finish reason remapping: convert `tool_use` stop reason to `stop` when json response tool is active

## 5. Tests

- [x] 5.1 Add unit tests for `buildParams` native mode: verify `OutputConfig.Format` is set, no tool injection, tool choice unchanged
- [x] 5.2 Add unit tests for `buildParams` tool fallback: verify synthetic tool appended, tool choice overridden, `usesJsonResponseTool` returned
- [x] 5.3 Add unit tests for `buildParams` edge cases: schemaless JSON warning, text format no-op, combined with existing user tools
- [x] 5.4 Add unit tests for stream remapping: tool input -> text delta, finish reason remapping, non-json tools unaffected
- [x] 5.5 Add integration test for structured output end-to-end (if feasible with mock or real API)
