# Spec Delta

## ADDED Requirements

### Requirement: Unary provider-result golden

For Bedrock `operation: generate` fixtures, Go and TypeScript replay SHALL serve the unchanged `input.response.json` as `application/json`. The registered upstream provider's `doGenerate` SHALL produce `expected-generate.json` offline. The Go runner SHALL compare ordered content, finish reason, usage, all provider metadata namespaces and warnings from `DoGenerate` against the golden. It SHALL compare request snapshots separately.

#### Scenario: Unary result matches upstream
- **WHEN** a unary fixture replays successfully
- **THEN** the Go provider result matches the upstream golden, including both metadata namespaces when the provider emits both

#### Scenario: Unary golden is missing or unreadable
- **WHEN** replay discovers a unary fixture without a readable result golden
- **THEN** the test fails before calling the provider and identifies the expected file and regeneration command instead of skipping the fixture

#### Scenario: Unary golden differs from the result
- **WHEN** the fixture's golden is invalid or differs from the Go provider result
- **THEN** the test fails even if the request snapshot matches

## MODIFIED Requirements

### Requirement: Fixture format
Streaming provider response fixtures SHALL use `.chunks.txt` files with one JSON object per provider event, without SSE framing (`data:`, `event:`, blank lines). The replay server SHALL add transport framing at serve time. The same streaming fixture format SHALL be used for both upstream and recorded fixtures.

#### Scenario: Single-step fixture
- **WHEN** a streaming test case has one provider response
- **THEN** the fixture is stored as `input.chunks.txt` in the test case directory

#### Scenario: Multi-step fixture
- **WHEN** a test case involves multiple `DoStream` calls (multi-step tool calling)
- **THEN** each step's provider response is stored as `input-1.chunks.txt`, `input-2.chunks.txt`, etc. in the test case directory

#### Scenario: Fixture line format
- **WHEN** a `.chunks.txt` fixture is read
- **THEN** each non-empty line parses as a valid JSON object representing a provider streaming event (e.g., Anthropic's `message_start`, `content_block_delta`, etc.)

### Requirement: Expected output format
Each streaming test case directory SHALL contain an `expected.jsonl` file with the expected UIMessageChunk sequence. The file SHALL contain one JSON object per line, each representing a single UIMessageChunk as produced by the upstream TypeScript SDK's `toUIMessageStream()`. For multi-step test cases, the file SHALL contain the full chunk sequence across all steps in a single file.

#### Scenario: Expected output is complete
- **WHEN** a multi-step test case produces chunks across 3 steps
- **THEN** `expected.jsonl` contains all chunks from all 3 steps in order, including `start-step`/`finish-step` boundaries

#### Scenario: Expected output uses deterministic IDs
- **WHEN** the TypeScript generator produces `expected.jsonl`
- **THEN** all SDK-generated IDs SHALL use a deterministic generator producing a sequential series (e.g., `id-0`, `id-1`, `id-2`)

### Requirement: Go replay server

The Go replay server in `test/conformance/runner.go` SHALL use `httptest.Server` to serve the fixture in the provider's transport format. For SSE streams, Go and TypeScript replay SHALL use `Content-Type: text/event-stream` and frame each event as `event: <type>\ndata: <json>\n\n`, taking the event type from its JSON `type`.

#### Scenario: Single-step SSE replay
- **WHEN** the replay server receives a request for a single-step SSE streaming test case
- **THEN** it serves `input.chunks.txt` with SSE framing and `Content-Type: text/event-stream`

#### Scenario: Multi-step replay
- **WHEN** the replay server receives sequential requests for a multi-step test case
- **THEN** it serves `input-1.chunks.txt` for the first request, `input-2.chunks.txt` for the second, etc.

#### Scenario: SSE framing
- **WHEN** the replay server serves a fixture line `{"type":"message_start",...}`
- **THEN** it sends `event: message_start\ndata: {"type":"message_start",...}\n\n` on the wire

#### Scenario: Fixtures exhausted
- **WHEN** the replay server receives more requests than available fixture files
- **THEN** it returns an HTTP error

### Requirement: TypeScript generation tool
`test/conformance/tools/generate.mts` SHALL regenerate expectations from existing `upstream/` and `recorded/` fixtures without live API calls. For streaming cases it SHALL read `config.yaml` and `input-*.chunks.txt`, serve the input through a local test server, run the upstream TypeScript SDK pipeline, and write the resulting UIMessageChunk sequence to `expected.jsonl`.

#### Scenario: Regenerating expected output
- **WHEN** the generation tool runs for a streaming test case with existing fixtures
- **THEN** it produces `expected.jsonl` by piping the fixtures through the upstream TypeScript SDK with a deterministic ID generator

#### Scenario: Regenerating all test cases
- **WHEN** the generation tool is invoked without a `--scenario` flag
- **THEN** it regenerates `expected.jsonl` for streaming cases and `expected-generate.json` for supported unary cases

#### Scenario: Selective regeneration
- **WHEN** the generation tool is invoked with `--scenario <name>`
- **THEN** only matching test cases' expected outputs are regenerated

### Requirement: Go conformance test runner

The system SHALL provide per-provider Go tests (e.g. `test/conformance/anthropic/conformance_test.go`) auto-discovering cases in provider `upstream/` and `recorded/`. For streaming cases the runner SHALL compare deterministic Go `StreamText` -> `ToUIMessageStream` output with `expected.jsonl`. Shared replay, comparison and helpers SHALL live in `test/conformance/runner.go`.

#### Scenario: Discovered provider case execution
- **WHEN** a provider test discovers a streaming test case directory
- **THEN** it SHALL parse `config.yaml`, infer provider from directories, start fixture replay, instantiate the provider pointing at replay, configure mock execute functions for defined tools, run `StreamText` -> `ToUIMessageStream` with a deterministic ID generator, collect chunks and compare them with `expected.jsonl`

#### Scenario: Conformance test passes for matching output
- **WHEN** the Go pipeline produces a UIMessageChunk sequence identical to `expected.jsonl`
- **THEN** the test passes

#### Scenario: Conformance test fails with diff
- **WHEN** the Go pipeline produces a UIMessageChunk sequence that differs from `expected.jsonl`
- **THEN** the test fails and reports a line-by-line diff showing which chunks differ

#### Scenario: Expected stream failure is compared
- **WHEN** a test case declares `expectStreamError: true`
- **THEN** the runner requires a non-nil result error
- **AND** it compares the emitted error-path UI chunks against `expected.jsonl` instead of failing solely because the result contains an error

#### Scenario: Auto-discovery across categories
- **WHEN** a new test case directory is added to either `upstream/` or `recorded/` under a provider
- **THEN** the Go test automatically discovers and runs it without any Go code changes

#### Scenario: Build tag gating
- **WHEN** Go tests run without the `conformance` build tag
- **THEN** conformance tests are not compiled or executed

### Requirement: Bedrock conformance Go test runner

The system SHALL provide `test/conformance/bedrock/conformance_test.go` that discovers test cases under `test/conformance/bedrock/{upstream,recorded}/`, instantiates `bedrock.New(modelID, bedrock.WithBaseURL(replay.BaseURL))`, and, for streaming cases, runs `StreamText` -> `ToUIMessageStream` with a deterministic ID generator and compares the resulting UIMessageChunk sequence against `expected.jsonl`.

#### Scenario: Bedrock conformance pass

- **WHEN** the Go Bedrock pipeline produces a UIMessageChunk sequence identical to the recorded `expected.jsonl`
- **THEN** the test passes

#### Scenario: Bedrock conformance fails with diff

- **WHEN** the Go Bedrock pipeline diverges from `expected.jsonl`
- **THEN** the test fails with a line-by-line diff showing which chunks differ

#### Scenario: Auto-discovery for Bedrock

- **WHEN** a new test case directory is added under `test/conformance/bedrock/upstream/` or `test/conformance/bedrock/recorded/`
- **THEN** the Go Bedrock test discovers and runs it without any Go code changes

#### Scenario: Build tag gating

- **WHEN** Go tests run without the `conformance` build tag
- **THEN** the Bedrock conformance test is not compiled or executed

### Requirement: TypeScript recording and generation support Bedrock

The TypeScript tools (`record.mts`, `generate.mts`) SHALL operate on Bedrock fixture directories. `record.mts` SHALL capture fixtures from real Bedrock APIs into `test/conformance/bedrock/recorded/`. `generate.mts` SHALL pipe fixtures from `test/conformance/bedrock/{upstream,recorded}/` through the upstream `@ai-sdk/amazon-bedrock` SDK to regenerate the expected output for each fixture's operation.

#### Scenario: Record against Bedrock

- **WHEN** the recorder runs with `--provider bedrock --scenario simple-text`
- **THEN** it captures the raw event-stream JSON chunks as `input.chunks.txt` and the UIMessageChunk output as `expected.jsonl` under `test/conformance/bedrock/recorded/simple-text/`

#### Scenario: Generate Bedrock expected output

- **WHEN** the generator runs with `--scenario bedrock/`
- **THEN** it regenerates `expected.jsonl` for streaming cases and `expected-generate.json` for unary cases under `test/conformance/bedrock/{upstream,recorded}/` using the registered `@ai-sdk/amazon-bedrock` SDK

### Requirement: Provider-parts golden scope

Anthropic, Bedrock, OpenAI and OpenAI-compatible SHALL be enabled. Fixture inputs SHALL remain unmodified, and non-streaming operations SHALL NOT have a provider-parts golden because they produce no stream parts.

#### Scenario: Generate operation
- **WHEN** a Bedrock case uses the non-streaming generate operation
- **THEN** it SHALL have no `expected-provider-parts.jsonl` and the runner SHALL NOT require one
