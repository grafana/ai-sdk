## 1. Provider options

- [x] 1.1 Add `ToolStreaming *bool \`json:"toolStreaming,omitempty"\`` to `AnthropicOptions` in `providers/anthropic/options.go`
- [x] 1.2 Add a small helper (or inline expression) in `convert_request.go` that resolves the effective tool-streaming flag (`nil` → `true`)

## 2. Plumb the `stream` flag through tool conversion

- [x] 2.1 Update `buildParams` signature in `providers/anthropic/convert_request.go` to accept a `stream bool` parameter
- [x] 2.2 Update `convertTools` signature to accept a `defaultEagerInputStreaming bool` parameter
- [x] 2.3 In `buildParams`, compute `defaultEagerInputStreaming = stream && resolveToolStreaming(anthropicOpts)` after option extraction and pass it to `convertTools`
- [x] 2.4 In `convertTools`, resolve the effective `eager_input_streaming` value (per-tool explicit value wins, otherwise fall back to `defaultEagerInputStreaming`) and set `tp.EagerInputStreaming = anthropic.Bool(true)` only when the resolved value is truthy. An explicit per-tool `false` SHALL omit the field rather than emit `eager_input_streaming: false`, matching upstream `...(eagerInputStreaming ? { eager_input_streaming: true } : {})`.
- [x] 2.5 Update `model.DoStream` in `providers/anthropic/model.go` to call `buildParams(..., true)` and `model.DoGenerate` to call `buildParams(..., false)`
- [x] 2.6 Thread `defaultEagerInputStreaming` into `applyResponseFormat` so the synthetic JSON fallback tool (used on non-native-structured-output models with `ResponseFormat.Type = "json"`) receives the same per-tool default as user-supplied function tools. Matches upstream calling `prepareTools` once with `[...tools, jsonResponseTool]` and the same default flag (anthropic-language-model.ts:519-545).

## 3. Update callers and tests

- [x] 3.1 Update every existing call to `buildParams(...)` in tests (`convert_request_test.go`, `reasoning_test.go`, etc.) to pass the new `stream` argument; default existing call sites to `false` (matches today's "do nothing" semantics for these assertions, except where a test explicitly exercises streaming behavior — adjust those individually)
- [x] 3.2 Adjust any existing assertion that read `EagerInputStreaming.Valid()` as `false` in a streaming context (no adjustments needed: all existing tests pass `stream=false` so `EagerInputStreaming` remains unset for tools that don't explicitly opt in)

## 4. New tests covering defaulting behavior

- [x] 4.1 Add a unit test asserting that `buildParams(..., true)` (streaming) with a function tool lacking provider options sets `EagerInputStreaming.Valid() == true && .Value == true`
- [x] 4.2 Add a unit test asserting that `buildParams(..., false)` (non-streaming) does NOT set `EagerInputStreaming` for the same input
- [x] 4.3 Add a unit test asserting that `AnthropicOptions.ToolStreaming = anthropic.Bool(false)` disables the streaming default
- [x] 4.4 Add a unit test asserting that an explicit per-tool `EagerInputStreaming: false` suppresses the model-level default `true` — the resulting `BetaToolParam.EagerInputStreaming` MUST be unset (`Valid() == false`), not `Bool(false)`, so the field is omitted from the wire payload.
- [x] 4.5 Add a unit test asserting that an explicit per-tool `EagerInputStreaming: true` wins when `ToolStreaming = false`
- [x] 4.6 Add a unit test asserting that provider-defined tools never receive an `EagerInputStreaming` setting from the model-level default
- [x] 4.7 Add unit tests under `TestBuildParams_StructuredOutput` covering the JSON response-format fallback tool: streaming with default `ToolStreaming` defaults `eager_input_streaming: true` on the `"json"` tool; `ToolStreaming = false` suppresses it; `DoGenerate` never defaults it.

## 5. Verification

- [x] 5.1 Run `make fmt vet test` and ensure all checks pass
- [x] 5.2 Run `openspec validate anthropic-default-eager-tool-streaming --strict`
- [x] 5.3 Confirm GitHub issue #186 item `ad0b376` can be checked off; mention the broader scope in the verification notes when archiving
