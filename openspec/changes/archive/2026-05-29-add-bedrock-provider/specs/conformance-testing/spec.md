## ADDED Requirements

### Requirement: Bedrock provider conformance directory

The conformance harness SHALL host Bedrock fixtures under `test/conformance/bedrock/` with the same `upstream/` and `recorded/` category split used for other providers. Each test case SHALL contain `config.yaml`, one or more `input*.chunks.txt` fixture files, and an `expected.jsonl` of UIMessageChunk output produced by the upstream TypeScript `@ai-sdk/amazon-bedrock` SDK.

#### Scenario: Bedrock upstream fixtures imported

- **WHEN** upstream Bedrock fixtures from `@ai-sdk/amazon-bedrock/src/__fixtures__/` are imported
- **THEN** they are placed under `test/conformance/bedrock/upstream/<name>/` with one fixture per test case directory and a corresponding `config.yaml`

#### Scenario: Bedrock upstream INDEX

- **WHEN** Bedrock upstream fixtures exist
- **THEN** `test/conformance/bedrock/upstream/INDEX.yaml` maps each upstream fixture filename (without extension) to its imported test case directory or `null` if not yet imported

#### Scenario: Bedrock recorded fixtures

- **WHEN** a fixture is captured via `record.mts` against a real Bedrock endpoint
- **THEN** it is placed under `test/conformance/bedrock/recorded/<name>/` with a `config.yaml` including a `prompt` field for re-recording

### Requirement: AWS event-stream replay framing

The replay server SHALL support a Bedrock framing mode that serves fixture lines as AWS Smithy event-stream binary frames instead of SSE. Each fixture line MUST be encoded as a single frame with a `:event-type` header set to the outer JSON key of the line and a payload equal to the inner JSON object. The HTTP response Content-Type MUST be `application/vnd.amazon.eventstream`.

#### Scenario: Bedrock replay encodes binary frames

- **WHEN** the replay server is in Bedrock mode and a fixture line is `{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":"hi"}}}`
- **THEN** the wire response contains a Smithy event-stream frame with `:event-type=contentBlockDelta` and JSON payload `{"contentBlockIndex":0,"delta":{"text":"hi"}}`

#### Scenario: Bedrock replay content type

- **WHEN** the replay server is in Bedrock mode
- **THEN** the HTTP response carries `Content-Type: application/vnd.amazon.eventstream`

#### Scenario: Multi-step Bedrock replay

- **WHEN** the replay server is in Bedrock mode for a multi-step case with `input-1.chunks.txt` and `input-2.chunks.txt`
- **THEN** sequential requests receive the corresponding fixture as separate event-stream binary responses

#### Scenario: Anthropic replay unaffected

- **WHEN** the replay server is in SSE mode (Anthropic, Grafana)
- **THEN** the SSE wire format and Content-Type are unchanged from the existing behavior

### Requirement: Bedrock conformance Go test runner

The system SHALL provide `test/conformance/bedrock/conformance_test.go` that discovers test cases under `test/conformance/bedrock/{upstream,recorded}/`, instantiates `bedrock.New(modelID, bedrock.WithBaseURL(replay.BaseURL))`, runs `StreamText` -> `ToUIMessageStream` with a deterministic ID generator, and compares the resulting UIMessageChunk sequence against `expected.jsonl`.

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

The TypeScript tools (`record.mts`, `generate.mts`) SHALL operate on Bedrock fixture directories. `record.mts` SHALL capture fixtures from real Bedrock APIs into `test/conformance/bedrock/recorded/`. `generate.mts` SHALL pipe fixtures from `test/conformance/bedrock/{upstream,recorded}/` through the upstream `@ai-sdk/amazon-bedrock` SDK to regenerate `expected.jsonl`.

#### Scenario: Record against Bedrock

- **WHEN** the recorder runs with `--provider bedrock --scenario simple-text`
- **THEN** it captures the raw event-stream JSON chunks as `input.chunks.txt` and the UIMessageChunk output as `expected.jsonl` under `test/conformance/bedrock/recorded/simple-text/`

#### Scenario: Generate Bedrock expected output

- **WHEN** the generator runs with `--provider bedrock`
- **THEN** it regenerates `expected.jsonl` for each test case under `test/conformance/bedrock/{upstream,recorded}/` using the upstream `@ai-sdk/amazon-bedrock` SDK with a deterministic ID generator

### Requirement: Bedrock recorded coverage of core paths

The recorded Bedrock fixtures SHALL cover at minimum: simple text generation, single tool call, parallel tool calls, and reasoning/thinking. Each fixture's `expected.jsonl` MUST be byte-identical to the upstream TypeScript SDK output for the same configuration.

#### Scenario: Simple text recorded fixture

- **WHEN** `make test-conformance` runs the `bedrock/recorded/simple-text` case
- **THEN** the Go UIMessageChunk sequence exactly matches the upstream TypeScript `expected.jsonl`

#### Scenario: Tool call recorded fixture

- **WHEN** `make test-conformance` runs the `bedrock/recorded/tool-call` case
- **THEN** the Go UIMessageChunk sequence exactly matches the upstream TypeScript `expected.jsonl` and includes a tool call chunk

#### Scenario: Parallel tool calls recorded fixture

- **WHEN** `make test-conformance` runs the `bedrock/recorded/parallel-tool-calls` case
- **THEN** the Go UIMessageChunk sequence exactly matches the upstream TypeScript `expected.jsonl` and contains two concurrent tool calls in the same step

#### Scenario: Thinking text recorded fixture

- **WHEN** `make test-conformance` runs the `bedrock/recorded/thinking-text` case
- **THEN** the Go UIMessageChunk sequence exactly matches the upstream TypeScript `expected.jsonl` and contains reasoning content parts
