## Why

Issue #221 remains valid against the registered Vercel AI SDK baseline: the Go OpenAI Responses adapter flattens generic multipart function results and loses scalar tool-result cache breakpoints. These request-conversion gaps discard file/text payloads and caller-specified caching hints on continuation.

## What Changes

- Convert generic function-result content arrays to ordered Responses `input_text`, `input_image`, and `input_file` output parts, including OpenAI/Azure-namespace uploaded references as `file_id`; retain per-content cache breakpoints and image detail.
- Preserve scalar output-option cache breakpoints, with result-part options as fallback, on generic and custom outputs; retain scalar JSON/error/denied and output-schema encoding rules.
- Preserve ordered parallel-wrapper child serialization and scalar child cache hints while converting multipart children with the same generic converter; do not replace the wrapper's existing newline-delimited serialized-output contract with native child blocks.
- Preserve the native hosted/client provider-tool dispatch, warnings, item/call identity, and supported custom multipart text/data/URL behavior. **Narrow issue #221's custom uploaded-reference acceptance** to generic function output: pinned `@ai-sdk/openai` 4.0.71 warns `unsupported custom tool content part type: file with data type: reference` and drops custom references. Custom reference conversion is **not** supported by this change. This is strict registered-upstream parity, not an approved permanent deviation.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `openai-responses-provider`: Specify generic and custom function-result output shapes, namespace-sensitive references, scalar/content cache handling, warning/drop semantics, and parallel-wrapper continuation.

## Impact

OpenAI provider request conversion (`providers/openai/convert_provider_tool_continuation.go`, `convert_messages.go`, `parallel_tool_call.go`) and focused request tests; existing request conformance snapshots only where an authentic input demonstrates the changed boundary. No public API, provider-contract, dependency, stream/SSE format, or baseline-version change.
