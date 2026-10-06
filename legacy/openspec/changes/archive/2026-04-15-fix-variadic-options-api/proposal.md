## Why

Three public functions (`ConvertToModelMessages`, `WriteUIMessageStream`, `ReadUIMessageStream`) accept variadic option structs but only use the first element, silently ignoring any extras. This makes the API signature misleading — variadic suggests multiple values are accepted — and creates a subtle bug trap for callers. Changing to a single optional pointer parameter makes the "zero or one" contract explicit at the type level. (ref: grafana/ai-sdk#100)

## What Changes

- **BREAKING**: Change `ConvertToModelMessages(messages []UIMessage, opts ...ConvertOptions)` to `ConvertToModelMessages(messages []UIMessage, opts *ConvertOptions)`
- **BREAKING**: Change `WriteUIMessageStream(w http.ResponseWriter, result *StreamTextResult, opts ...UIMessageStreamOptions)` to `WriteUIMessageStream(w http.ResponseWriter, result *StreamTextResult, opts *UIMessageStreamOptions)`
- **BREAKING**: Change `ReadUIMessageStream(stream <-chan UIMessageChunk, opts ...ReadStreamOption)` to `ReadUIMessageStream(stream <-chan UIMessageChunk, opts *ReadStreamOption)`
- Update all internal call sites (e.g., `streamtext.go`, test files) to pass `nil` instead of omitting the argument
- Update `README.md` examples to reflect new signatures

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

(none — these functions are not covered by existing specs; the `functional-options` spec covers `StreamText`/`GenerateText` which use a different sealed-interface pattern)

## Impact

- **Public API**: Three exported function signatures change (breaking for callers passing variadic args)
- **Files affected**: `convert.go`, `http.go`, `convert_test.go`, `http_test.go`, `streamtext.go`, `README.md`
- **Dependencies**: None — this is a pure signature refactor with no new dependencies
- **Wire format**: No change — SSE/chunk format is unaffected
