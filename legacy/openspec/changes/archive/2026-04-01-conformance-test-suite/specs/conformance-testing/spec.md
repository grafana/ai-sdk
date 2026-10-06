## ADDED Requirements

### Requirement: Fixture format
The system SHALL use `.chunks.txt` files as provider response fixtures, where each file contains one JSON object per line representing a single provider streaming event. The fixture files SHALL NOT contain SSE framing (`data:`, `event:`, blank lines) -- framing SHALL be added by the replay server at serve time. The same fixture format SHALL be used for both upstream and recorded fixtures.

#### Scenario: Single-step fixture
- **WHEN** a test case has one provider response
- **THEN** the fixture is stored as `input.chunks.txt` in the test case directory

#### Scenario: Multi-step fixture
- **WHEN** a test case involves multiple `DoStream` calls (multi-step tool calling)
- **THEN** each step's provider response is stored as `input-1.chunks.txt`, `input-2.chunks.txt`, etc. in the test case directory

#### Scenario: Fixture line format
- **WHEN** a fixture file is read
- **THEN** each non-empty line parses as a valid JSON object representing a provider streaming event (e.g., Anthropic's `message_start`, `content_block_delta`, etc.)

### Requirement: Fixture categories
The system SHALL support two categories of fixtures organized under each provider directory: `upstream/` for fixtures copied from the Vercel AI SDK, and `recorded/` for fixtures captured by the recording tool from real provider APIs. Both categories SHALL use the same fixture format and `config.yaml` schema. The `record.mts` tool SHALL only operate on `recorded/` fixtures. The `generate.mts` tool SHALL operate on both categories.

#### Scenario: Upstream fixture
- **WHEN** a fixture is copied from the Vercel AI SDK
- **THEN** it is placed under `<provider>/upstream/<test-name>/` with a `config.yaml` that does not include a `prompt` field

#### Scenario: Recorded fixture
- **WHEN** a fixture is captured by the recording tool
- **THEN** it is placed under `<provider>/recorded/<test-name>/` with a `config.yaml` that includes a `prompt` field for re-recording

#### Scenario: Bulk-copy upstream fixtures
- **WHEN** upstream fixtures are copied from a Vercel SDK provider package
- **THEN** they are placed under `<provider>/upstream/` with each test case in its own subdirectory, and `config.yaml` files are added per test case

#### Scenario: Upstream fixture index
- **WHEN** upstream fixtures exist for a provider
- **THEN** `<provider>/upstream/INDEX.yaml` SHALL map each known upstream fixture name to its local test case directory (if imported) or `null` (if not yet imported), providing a single view of import coverage

### Requirement: Test case configuration
Each test case directory SHALL contain a `config.yaml` file that declares the replay configuration. The YAML SHALL specify: `model` (string), and optionally: `prompt` (string, for recording and documentation), `stopWhenStepCount` (integer, default 1), `providerOptions` (nested map), and `tools` (map of tool name to tool definition with `description`, `inputSchema` JSON schema, and `mockResults` list). The provider SHALL be inferred from the parent directory structure, not from the YAML.

#### Scenario: Minimal config
- **WHEN** a config specifies only `model`
- **THEN** it is a valid single-step test case with no tools and no special provider options

#### Scenario: Tool call config
- **WHEN** a config specifies `tools` with `mockResults` and `stopWhenStepCount` > 1
- **THEN** the Go test SHALL register tools with `Execute` functions that return mock results from the list in order, one per tool execution

#### Scenario: Provider options config
- **WHEN** a config specifies `providerOptions`
- **THEN** the Go test SHALL pass those options when configuring the provider and calling `StreamText`

#### Scenario: Provider inference
- **WHEN** a test case is located at `anthropic/upstream/tool-call/`
- **THEN** the Go test SHALL use the Anthropic provider without needing a `provider` field in `config.yaml`

### Requirement: Expected output format
Each test case directory SHALL contain an `expected.jsonl` file with the expected UIMessageChunk sequence. The file SHALL contain one JSON object per line, each representing a single UIMessageChunk as produced by the upstream TypeScript SDK's `toUIMessageStream()`. For multi-step test cases, the file SHALL contain the full chunk sequence across all steps in a single file.

#### Scenario: Expected output is complete
- **WHEN** a multi-step test case produces chunks across 3 steps
- **THEN** `expected.jsonl` contains all chunks from all 3 steps in order, including `start-step`/`finish-step` boundaries

#### Scenario: Expected output uses deterministic IDs
- **WHEN** the TypeScript generator produces `expected.jsonl`
- **THEN** all SDK-generated IDs SHALL use a deterministic generator producing a sequential series (e.g., `id-0`, `id-1`, `id-2`)

### Requirement: TypeScript recording tool
The system SHALL provide a TypeScript script (`test/conformance/tools/record.mts`) that captures new conformance fixtures from real provider APIs. The script SHALL only operate on `recorded/` directories. It SHALL read `config.yaml`, set up a recording proxy between the upstream TypeScript SDK and the real provider API, run `streamText` with the configuration, and save both the raw provider responses (as `input-N.chunks.txt`) and the UIMessageChunk output (as `expected.jsonl`) to the test case directory.

#### Scenario: Recording a single-step test case
- **WHEN** the recording tool runs a config with `maxSteps: 1`
- **THEN** it captures one provider response as `input.chunks.txt` and the UIMessageChunk sequence as `expected.jsonl`

#### Scenario: Recording a multi-step test case
- **WHEN** the recording tool runs a config with tools and `maxSteps` > 1
- **THEN** it captures each provider response in order as `input-1.chunks.txt`, `input-2.chunks.txt`, etc., and the full UIMessageChunk sequence as `expected.jsonl`

#### Scenario: Recording handles API errors
- **WHEN** a provider API call fails during recording (auth error, rate limit, network error)
- **THEN** the recording tool SHALL report the error with the test case name and continue to the next test case

#### Scenario: Selective recording
- **WHEN** the recording tool is invoked with a `--scenario <name>` flag
- **THEN** only the named test case is recorded

### Requirement: TypeScript generation tool
The system SHALL provide a TypeScript script (`test/conformance/tools/generate.mts`) that regenerates expected output from existing fixture files without making real API calls. The script SHALL operate on both `upstream/` and `recorded/` directories. It SHALL read `config.yaml` and `input-*.chunks.txt` files, serve them through a local test server, run the upstream TypeScript SDK's full pipeline against the test server, and write the resulting UIMessageChunk sequence to `expected.jsonl`.

#### Scenario: Regenerating expected output
- **WHEN** the generation tool runs for a test case with existing fixtures
- **THEN** it produces `expected.jsonl` by piping the fixtures through the upstream TypeScript SDK with a deterministic ID generator

#### Scenario: Regenerating all test cases
- **WHEN** the generation tool is invoked without a `--scenario` flag
- **THEN** it regenerates `expected.jsonl` for all test case directories across all providers

#### Scenario: Selective regeneration
- **WHEN** the generation tool is invoked with `--scenario <name>`
- **THEN** only the named test case's `expected.jsonl` is regenerated

### Requirement: Go replay server
The system SHALL provide a replay server (`test/conformance/runner.go`) that uses `httptest.Server` to serve fixture files as SSE responses. The server SHALL read `input.chunks.txt` or `input-N.chunks.txt` files, wrap each line with SSE framing (`event: <type>\ndata: <json>\n\n`), and serve with `Content-Type: text/event-stream`. The event type is extracted from the JSON `type` field of each fixture line. Both the Go and TypeScript replay servers SHALL use the same SSE format, matching what the real Anthropic API sends. For multi-step test cases, the server SHALL be stateful, serving `input-1.chunks.txt` for the first request, `input-2.chunks.txt` for the second, and so on. The server SHALL return an HTTP error if fixtures are exhausted.

#### Scenario: Single-step replay
- **WHEN** the replay server receives a request for a single-step test case
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

### Requirement: Go conformance test runner
The system SHALL provide per-provider Go test files (e.g., `test/conformance/anthropic/conformance_test.go`) that auto-discover test case directories under both `upstream/` and `recorded/` within their provider directory. For each test case: parse `config.yaml`, infer the provider from the directory structure, start the replay server with the fixture files, instantiate the Go provider pointing at the replay server, configure tools with mock execute functions (if defined), run `StreamText` -> `ToUIMessageStream` with a deterministic ID generator, collect the UIMessageChunk sequence, and compare it against `expected.jsonl`. Shared test infrastructure (replay, comparison, helpers) SHALL live in `test/conformance/runner.go`.

#### Scenario: Conformance test passes for matching output
- **WHEN** the Go pipeline produces a UIMessageChunk sequence identical to `expected.jsonl`
- **THEN** the test passes

#### Scenario: Conformance test fails with diff
- **WHEN** the Go pipeline produces a UIMessageChunk sequence that differs from `expected.jsonl`
- **THEN** the test fails and reports a line-by-line diff showing which chunks differ

#### Scenario: Auto-discovery across categories
- **WHEN** a new test case directory is added to either `upstream/` or `recorded/` under a provider
- **THEN** the Go test automatically discovers and runs it without any Go code changes

#### Scenario: Build tag gating
- **WHEN** Go tests run without the `conformance` build tag
- **THEN** conformance tests are not compiled or executed

### Requirement: ID comparison strategy
The system SHALL use exact comparison for all IDs in the UIMessageChunk stream. Content block IDs (from the provider, e.g. stringified block index) and tool call IDs (from the API response) are deterministic for a given fixture. Message-level IDs SHALL be controlled via deterministic generators configured identically in both the TypeScript tools and Go tests.

#### Scenario: Exact comparison with deterministic IDs
- **WHEN** both SDKs process the same fixture with deterministic message ID generators
- **THEN** the UIMessageChunk sequences are compared exactly (byte-identical JSON per line)
