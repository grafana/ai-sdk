## Why

`StreamTextParams` is a 25+ field flat struct mixing model parameters, lifecycle callbacks, and behavioral flags. Optional scalars require pointer indirection (`*int`, `*float64`) and temporary variables for construction. `GenerateText` and the `output` package wrappers accept the same struct, silently ignoring streaming-only fields (`OnChunk`, `IncludeRawChunks`). This makes the API awkward to use and impossible to catch misuse at compile time.

## What Changes

- **BREAKING** Remove `StreamTextParams` struct; replace with `StreamOption` functional options for `StreamText`
- **BREAKING** Change `StreamText` signature from `(ctx, StreamTextParams)` to `(ctx, model, ...StreamOption)`
- **BREAKING** Change `GenerateText` signature from `(ctx, StreamTextParams)` to `(ctx, model, ...GenerateOption)`
- **BREAKING** Change `output.GenerateObject` and `output.StreamObject` signatures to use the new option types
- Add `StreamOption` type and ~25 option constructor functions for streaming
- Add `GenerateOption` type and ~22 option constructor functions for generation (excludes streaming-only options like `OnChunk`, `WithIncludeRawChunks`)
- Add unexported `streamConfig` and `generateConfig` structs as internal option accumulators
- `provider.CallOptions` remains unchanged -- the provider boundary is not affected

## Capabilities

### New Capabilities

- `functional-options`: Defines the functional options API pattern for `StreamText` and `GenerateText`, including option types, constructor functions, internal config structs, and the translation to `provider.CallOptions`

### Modified Capabilities

_(none -- existing specs cover provider behavior, structured output schemas, and tool mechanics, none of which change at the requirements level)_

## Impact

- **Public API**: Every callsite constructing `StreamTextParams` must migrate (~49 struct literals across production code, tests, and examples)
- **`output` package**: `GenerateObject` and `StreamObject` signatures change to accept the new option types
- **Tests**: ~45 test constructions in `streamtext_test.go`, `streamtext_output_test.go`, `integration_test.go`, `output/value_test.go`, `http_test.go`
- **Conformance/integration harness**: `test/conformance/runner.go` and `test/integration/testserver/` need updates
- **Documentation**: `doc.go` and `README.md` examples need updating
- **Provider boundary**: No change -- `provider.CallOptions` struct is unchanged
- **Result/event types**: No change -- `StreamTextResult`, `GenerateTextResult`, all stream part types unchanged
