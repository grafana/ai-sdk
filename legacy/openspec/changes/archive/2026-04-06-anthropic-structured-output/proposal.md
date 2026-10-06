## Why

The Anthropic provider discards `CallOptions.ResponseFormat` with a warning, making `output.Object[T]` and all other structured output modes non-functional. The orchestration layer correctly wires `Output.ResponseFormat()` into `CallOptions`, but the provider ignores it. Since Anthropic is the only provider in this codebase, structured output is effectively broken for all users.

## What Changes

- Replace the "unsupported" warning for `ResponseFormat` in `buildParams` with actual structured output handling
- Add a `supportsStructuredOutput` field to `modelCapabilities` to distinguish models that support native `output_config.format` from those that need a tool-based fallback
- Implement native JSON schema mode via `output_config.format` for newer models (claude-*-4-5, claude-*-4-6, claude-opus-4-1)
- Implement tool-based JSON fallback for older models by synthesizing a `"json"` tool with the schema as input, forcing `toolChoice: required`
- Handle the synthetic tool response in the stream adapter, remapping the tool call output back to text content so the orchestration layer receives it transparently

## Capabilities

### New Capabilities
- `anthropic-structured-output`: Provider-level handling of `ResponseFormat` in the Anthropic module, including native JSON schema mode and tool-based fallback

### Modified Capabilities
- `model-capabilities`: Add `supportsStructuredOutput` field to the capabilities struct and per-model lookup

## Impact

- **Code**: `anthropic/convert_request.go` (buildParams, tool injection), `anthropic/convert_stream.go` (synthetic tool response remapping), `anthropic/models.go` (capabilities struct and lookup)
- **APIs**: No public API changes -- this enables existing `Output`/`ResponseFormat` fields that were already part of the public surface
- **Dependencies**: No new dependencies -- uses existing `anthropic-sdk-go` types for tools and output config
- **Wire format**: No changes to SSE/UIMessageChunk -- the structured output is handled at the provider boundary and surfaces through the existing `Output.ParseComplete()` flow
