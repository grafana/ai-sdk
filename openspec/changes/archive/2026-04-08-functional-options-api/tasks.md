## 1. Core option types and infrastructure

- [x] 1.1 Create `options.go` with `StreamOption` and `GenerateOption` sealed interfaces, unexported marker methods, and concrete option types (`sharedOption`, `streamOnlyOption`, `generateOnlyOption`)
- [x] 1.2 Create `baseConfig`, `streamConfig`, and `generateConfig` unexported structs in `options.go` with all fields mirroring current `StreamTextParams` (shared fields in `baseConfig`, stream-only fields in `streamConfig`)
- [x] 1.3 Implement `apply*` methods on each concrete option type to satisfy the interfaces

## 2. Option constructor functions

- [x] 2.1 Implement message options: `WithMessages`, `WithModelMessages`, `WithSystem`, `WithSystemMessages` (shared)
- [x] 2.2 Implement model parameter options: `WithTemperature`, `WithMaxOutputTokens`, `WithTopP`, `WithTopK`, `WithSeed`, `WithPresencePenalty`, `WithFrequencyPenalty`, `WithStopSequences` (shared)
- [x] 2.3 Implement tool options: `WithTools`, `WithToolChoice`, `WithActiveTools`, `WithStopWhen` (shared)
- [x] 2.4 Implement provider integration options: `WithProviderOptions`, `WithHeaders`, `WithResponseFormat` (shared)
- [x] 2.5 Implement shared callback options: `OnStart`, `OnStepStart`, `OnStepFinish`, `OnError`, `OnToolCallStart`, `OnToolCallFinish`
- [x] 2.6 Implement stream-only options: `OnChunk`, `WithIncludeRawChunks`
- [x] 2.7 Implement advanced options: `WithPrepareStep`, `WithOutput`

## 3. Migrate StreamText

- [x] 3.1 Change `StreamText` signature to `(ctx, model, ...StreamOption)` and add internal config-building logic that applies options to `streamConfig`
- [x] 3.2 Update `run` method and `executeSingleStep` to read from `streamConfig` instead of `StreamTextParams`
- [x] 3.3 Verify `CallOptions` construction from `streamConfig` produces identical output to the previous `StreamTextParams` path

## 4. Migrate GenerateText

- [x] 4.1 Change `GenerateText` signature to `(ctx, model, ...GenerateOption)` and add internal conversion from `generateConfig` to `streamConfig` for delegation to the streaming path

## 5. Migrate output package

- [x] 5.1 Change `output.GenerateObject` signature to `(ctx, model, out, ...GenerateOption)` and update internal delegation
- [x] 5.2 Change `output.StreamObject` signature to `(ctx, model, out, ...StreamOption)` and update internal delegation

## 6. Remove StreamTextParams

- [x] 6.1 Remove `StreamTextParams` struct from `text.go`
- [x] 6.2 Remove any helper functions that only existed to support the params struct pattern

## 7. Migrate tests

- [x] 7.1 Migrate `streamtext_test.go` (~20 callsites) to functional options
- [x] 7.2 Migrate `streamtext_output_test.go` (~12 callsites) to functional options
- [x] 7.3 Migrate `integration_test.go` (~6 callsites) to functional options
- [x] 7.4 Migrate `output/value_test.go` (~5 callsites) to functional options
- [x] 7.5 Migrate `http_test.go` (~2 callsites) to functional options
- [x] 7.6 Migrate `test/conformance/runner.go` and `test/integration/testserver/` callsites to functional options

## 8. Migrate documentation

- [x] 8.1 Update `doc.go` examples to use functional options
- [x] 8.2 Update `README.md` examples to use functional options

## 9. Verification

- [x] 9.1 Run `make check` (fmt + vet + test) and fix any failures
- [x] 9.2 Add compile-time interface satisfaction checks for option types in test files
