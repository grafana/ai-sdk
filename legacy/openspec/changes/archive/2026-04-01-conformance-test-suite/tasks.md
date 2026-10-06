## 1. Go Module and Directory Setup

- [x] 1.1 Create `test/conformance/` directory structure: `tools/`, `anthropic/upstream/`, `anthropic/recorded/`
- [x] 1.2 Initialize separate Go module at `test/conformance/` with `replace` directives for `aisdk` and `anthropic`
- [x] 1.3 Initialize Node.js project at `test/conformance/tools/` with dependencies: `ai`, `@ai-sdk/anthropic`, `yaml`, TypeScript tooling

## 2. Go Replay Server

- [x] 2.1 Implement `runner.go`: `httptest.Server` that reads `.chunks.txt` files and serves them as SSE with `event: <type>\ndata: <json>` framing
- [x] 2.2 Add stateful multi-step support: request counter per test, serves `input-1.chunks.txt` for first request, `input-2.chunks.txt` for second, etc.
- [x] 2.3 Add single-step fallback: if `input.chunks.txt` exists (no number suffix), serve it for all requests

## 3. Go Config Loader

- [x] 3.1 Implement YAML config parser: load `config.yaml` into typed structs (model, prompt, maxSteps, providerOptions, tools with mockResults)
- [x] 3.2 Implement config-to-StreamText mapping: infer provider from directory path, build provider instance pointing at replay server, register tools with ordered mock `Execute` funcs, set `StopWhen` from maxSteps, pass providerOptions

## 4. Go Conformance Test Runner

- [x] 4.1 Implement shared test helpers in `runner.go`: auto-discover test case directories (walk `upstream/` and `recorded/`), run replay-and-compare cycle, report diffs
- [x] 4.2 Implement `anthropic/conformance_test.go` with `//go:build conformance` tag: use shared runner, instantiate Anthropic provider, table-driven test per discovered test case
- [x] 4.3 Implement test loop: parse config, start replay server, run StreamText -> ToUIMessageStream with deterministic ID generator, collect UIMessageChunk sequence
- [x] 4.4 Implement comparison: read `expected.jsonl`, compare line-by-line against actual output, report diff on failure with context
- [x] ~~4.5 ID-remapping fallback~~ Removed: all IDs are deterministic from fixtures, exact comparison works

## 5. TypeScript Generation Tool

- [x] 5.1 Implement `generate.mts`: discover test cases across all providers (both `upstream/` and `recorded/`), read `config.yaml` + `input-*.chunks.txt`, set up local test server serving fixtures in order, configure upstream SDK with deterministic ID generator
- [x] 5.2 Implement config-to-streamText mapping: infer provider from directory path, build provider config, register tools with ordered mock results, set maxSteps
- [x] 5.3 Implement expected output capture: run streamText -> toUIMessageStream, collect UIMessageChunk sequence, write to `expected.jsonl` (one JSON per line)
- [x] 5.4 Add `--scenario <name>` flag for selective regeneration, default to all test cases

## 6. TypeScript Recording Tool

- [x] 6.1 Implement recording proxy: `http.createServer` that forwards requests to real provider API, tees response bodies to `input-N.chunks.txt` files in order
- [x] 6.2 Implement `record.mts`: discover `recorded/` directories only, read `config.yaml`, configure upstream SDK to use recording proxy, run streamText, save fixtures + expected output
- [x] 6.3 Add `--scenario <name>` flag for selective recording

## 7. Initial Test Cases and Validation

- [x] 7.1 Copy upstream Anthropic fixtures from `packages/anthropic/src/__fixtures__/` into `anthropic/upstream/`, create `config.yaml` per test case, run `generate.mts` to produce `expected.jsonl`
- [x] 7.2 Record fresh fixtures using `record.mts` into `anthropic/recorded/` (simple-text, tool-call) to validate the recording pipeline
- [x] 7.3 Run Go conformance tests against both upstream and recorded test cases, verify pass/fail behavior
- [x] 7.4 Validate ID comparison strategy: confirm exact comparison works or add ID-remapping

## 8. Build and CI Integration

- [x] 8.1 Add Makefile target `test-conformance` that runs `go test -tags conformance ./...` from `test/conformance/`
- [x] 8.2 Add Makefile target `generate-conformance` that runs `generate.mts`
- [x] 8.3 Add CI job that runs conformance tests using committed fixtures (no API keys)
