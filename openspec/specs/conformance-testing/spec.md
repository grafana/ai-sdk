# conformance-testing Specification

## Purpose

Define provider conformance fixture formats, provider-independent core UI lifecycle goldens, replay/generation tooling, and exact UI message chunk comparison behavior for validating Go SDK behavior against upstream TypeScript SDK output.
## Requirements
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
The system SHALL support two categories of fixtures organized under each provider directory: `upstream/` for fixtures copied from the Vercel AI SDK, and `recorded/` for locally controlled fixtures captured from real provider APIs or deterministically derived from captured provider payloads to reproduce transport failures that cannot be recorded reliably. Derived fixtures SHALL preserve valid provider event shapes and SHALL document their synthetic condition through the test case name and configuration. Both categories SHALL use the same fixture format and `config.yaml` schema. The `record.mts` tool SHALL only operate on `recorded/` fixtures. The `generate.mts` tool SHALL operate on both categories.

#### Scenario: Upstream fixture
- **WHEN** a fixture is copied from the Vercel AI SDK
- **THEN** it is placed under `<provider>/upstream/<test-name>/` with a `config.yaml` that does not include a `prompt` field

#### Scenario: Recorded fixture
- **WHEN** a fixture is captured by the recording tool
- **THEN** it is placed under `<provider>/recorded/<test-name>/` with a `config.yaml` that includes a `prompt` field for re-recording

#### Scenario: Derived transport-failure fixture
- **WHEN** a deterministic truncation or transport failure cannot be captured reliably from a live provider
- **THEN** a valid captured provider payload MAY be shortened or otherwise minimally derived under `<provider>/recorded/<test-name>/`
- **AND** upstream expected output is regenerated from that derived input

#### Scenario: Bulk-copy upstream fixtures
- **WHEN** upstream fixtures are copied from a Vercel SDK provider package
- **THEN** they are placed under `<provider>/upstream/` with each test case in its own subdirectory, and `config.yaml` files are added per test case

#### Scenario: Upstream fixture index
- **WHEN** upstream fixtures exist for a provider
- **THEN** `<provider>/upstream/INDEX.yaml` SHALL map each known upstream fixture name to its local test case directory (if imported) or `null` (if not yet imported), providing a single view of import coverage

### Requirement: Test case configuration
Each provider-backed test case directory SHALL contain a `config.yaml` file that declares the replay configuration. The YAML SHALL specify: `model` (string), and optionally: `system`, `prompt` (string, for recording and documentation), `messages` (ordered role/content entries), persisted `uiMessages`, `allowSystemInMessages`, `stopWhenStepCount` (integer, default 1), `providerOptions` (nested map), `tools` (map of tool name to tool definition with `description`, `inputSchema` JSON schema, `mockResults` list, optional declarative `modelOutput`, and optional approval configuration), `providerTools`, `responseFormat`, `assertOutputValue`, approval-resumption setup for scenarios that replay a second approval call, and `expectStreamError` (boolean, default false). Configured message content SHALL support scalar text or ordered `text`, `reasoning`, `file`, `tool-call`, and `tool-result` parts with part `providerOptions`. A configured file part SHALL carry `mediaType`, an optional `filename`, and either base64 `data` or a provider `reference` map. Tool-call/result entries SHALL support tool ids, names, inputs or structured outputs, provider-executed state where applicable, and per-part provider options. The provider SHALL be inferred from the parent directory structure, not from the YAML. When `providerOptions` is specified in YAML, the Go test SHALL marshal each provider namespace value to JSON and wrap it as `provider.RawProviderOption` for use with the typed provider options field on `StreamText`.

The `messages` configuration SHALL be supported equivalently by the Go runner and TypeScript tools. It SHALL represent system, user, assistant, and tool roles and allow provider request snapshots to exercise continuation conversion without relying on an additional live or replayed model step.

Tool approval configuration in conformance YAML SHALL be supported by both the Go runner and the TypeScript tools so recorded fixtures can exercise upstream-equivalent approval behavior. The approval configuration SHALL allow a tool to always require approval. Approval-resumption setup SHALL allow the test case to seed the conversation with an assistant tool call plus approval request and a tool approval response so approved and denied second-call fixtures can be replayed without manual code changes.

#### Scenario: Minimal config
- **WHEN** a config specifies only `model`
- **THEN** it is a valid single-step test case with no tools and no special provider options

#### Scenario: Tool call config
- **WHEN** a config specifies `tools` with `mockResults` and `stopWhenStepCount` > 1
- **THEN** the Go test SHALL register tools with `Execute` functions that return mock results from the list in order, one per tool execution

#### Scenario: Tool approval config
- **WHEN** a config specifies a tool with approval required
- **THEN** the Go runner and TypeScript tools SHALL configure that tool so approval is required before local execution

#### Scenario: Persisted UI tool output config
- **WHEN** a config supplies `uiMessages` containing a completed tool output
- **AND** the matching tool supplies a declarative `modelOutput`
- **THEN** the Go runner and TypeScript tools SHALL convert the UI messages with the configured tool set
- **AND** both paths SHALL use the configured model output in the provider request

#### Scenario: Approval resumption config
- **WHEN** a config specifies a prior tool call, approval request, and approval response
- **THEN** the Go runner and TypeScript tools SHALL seed equivalent model messages before replaying the provider fixture
- **AND** both SDKs SHALL use those messages to resolve the approval before the model call

#### Scenario: Configured provider-tool continuation history
- **WHEN** a config supplies `messages` containing prior assistant provider-tool calls and assistant/tool results with provider item metadata
- **THEN** the Go runner and TypeScript tools SHALL construct equivalent ordered model messages
- **AND** the captured request snapshot SHALL compare each SDK's provider-tool continuation items

#### Scenario: Provider options config
- **WHEN** a config specifies `providerOptions` with nested YAML map values
- **THEN** the Go test SHALL marshal each namespace value to JSON, wrap as `RawProviderOption`, and pass the resulting provider options to `StreamText`

#### Scenario: Configured file message part
- **WHEN** a config message contains a file part with `data: AAECAw==`, `mediaType: application/pdf`, and `filename: report.pdf`
- **THEN** the Go runner and TypeScript tools SHALL construct equivalent user file parts containing the same base64 data, media type, and filename
- **AND** generated request snapshots SHALL validate the provider-specific file request shape

#### Scenario: Provider inference
- **WHEN** a test case is located at `anthropic/upstream/tool-call/`
- **THEN** the Go test SHALL use the Anthropic provider without needing a `provider` field in `config.yaml`

#### Scenario: Expected stream error config
- **WHEN** a config sets `expectStreamError: true`
- **THEN** the Go runner SHALL require `StreamTextResult.Err()` to be non-nil while still comparing every emitted UI chunk against `expected.jsonl`

#### Scenario: Parsed output assertion config
- **WHEN** a config sets `assertOutputValue: true` and the test case contains `expected-object.json`
- **THEN** the Go runner SHALL compare `expected-object.json` directly with `StreamTextResult.OutputValue()`
- **AND** it SHALL NOT reconstruct a missing output value from text chunks

### Requirement: Expected output format
Each test case directory SHALL contain an `expected.jsonl` file with the expected UIMessageChunk sequence. The file SHALL contain one JSON object per line, each representing a single UIMessageChunk as produced by the upstream TypeScript SDK's `toUIMessageStream()`. For multi-step test cases, the file SHALL contain the full chunk sequence across all steps in a single file.

#### Scenario: Expected output is complete
- **WHEN** a multi-step test case produces chunks across 3 steps
- **THEN** `expected.jsonl` contains all chunks from all 3 steps in order, including `start-step`/`finish-step` boundaries

#### Scenario: Expected output uses deterministic IDs
- **WHEN** the TypeScript generator produces `expected.jsonl`
- **THEN** all SDK-generated IDs SHALL use a deterministic generator producing a sequential series (e.g., `id-0`, `id-1`, `id-2`)

### Requirement: Expected structured output format

A conformance test case MAY contain `expected-object.json` with the complete structured output produced by the upstream TypeScript SDK. The Go runner SHALL compare this value with the Go result. For legacy response-format-only fixtures that do not construct an SDK `Output`, the runner MAY reconstruct the comparison value from text-delta chunks. A fixture with `assertOutputValue: true` SHALL instead require the actual parsed `StreamTextResult.OutputValue()`.

#### Scenario: Parsed output matches upstream

- **WHEN** a test case contains `expected-object.json` and sets `assertOutputValue: true`
- **THEN** the Go test SHALL compare the decoded expected value with `StreamTextResult.OutputValue()`
- **AND** the test SHALL fail when `OutputValue()` is nil but the expected value is non-nil

#### Scenario: Legacy response-format fixture has no OutputValue

- **WHEN** a test case contains `expected-object.json`, does not set `assertOutputValue`, and does not configure an SDK `Output`
- **THEN** the Go runner MAY reconstruct the actual comparison value from emitted text-delta chunks

### Requirement: Expected request input format
Each provider-backed conformance test case SHALL contain an `expected-requests.jsonl` file that captures the upstream TypeScript provider request inputs for that test case. The file SHALL contain one JSON object per provider API request, in request order. Each request snapshot SHALL include the HTTP method, request path, normalized behavior-affecting headers, and decoded JSON request body.

#### Scenario: Single request fixture
- **WHEN** a test case performs one provider API request
- **THEN** `expected-requests.jsonl` contains exactly one request snapshot line

#### Scenario: Multi-step request fixture
- **WHEN** a test case performs multiple provider API requests
- **THEN** `expected-requests.jsonl` contains one request snapshot line per request in the same order as the requests occurred

#### Scenario: Request snapshot shape
- **WHEN** a request snapshot is parsed
- **THEN** it includes `method`, `path`, `headers`, and `body` fields
- **AND** `headers` is a JSON object of normalized header names to normalized values
- **AND** `body` is the decoded JSON request body as a JSON object

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

### Requirement: TypeScript request input capture
The TypeScript conformance generation and recording tools SHALL capture provider request inputs while producing fixture output. The generation tool SHALL capture requests sent to its replay server. The recording tool SHALL capture requests sent through its provider proxy and SHALL redact secrets before writing request snapshots.

#### Scenario: Generate request snapshots
- **WHEN** `test/conformance/tools/generate.mts` regenerates expected output for a test case
- **THEN** it writes `expected-requests.jsonl` from the upstream TypeScript provider requests observed during that run

#### Scenario: Record request snapshots
- **WHEN** `test/conformance/tools/record.mts` records a fixture from a real provider API
- **THEN** it writes `expected-requests.jsonl` from the upstream TypeScript provider requests observed during recording
- **AND** committed request snapshots do not contain API keys, bearer tokens, or other secret header values

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

### Requirement: Go request input comparison
The Go conformance runner SHALL capture actual Go provider requests during replay and compare them against `expected-requests.jsonl`. The runner SHALL fail when request counts differ, method or path differs, normalized headers differ, or decoded JSON bodies differ.

#### Scenario: Matching request input
- **WHEN** the Go provider sends the same request inputs as the upstream TypeScript provider
- **THEN** the conformance test passes the request input assertion

#### Scenario: Request count mismatch
- **WHEN** the Go provider sends fewer or more provider API requests than `expected-requests.jsonl` contains
- **THEN** the conformance test fails with a request count mismatch

#### Scenario: Request body mismatch
- **WHEN** the Go provider request body has a missing field, extra field, or different value compared with the expected request body
- **THEN** the conformance test fails and identifies the mismatched request index

#### Scenario: Request method or path mismatch
- **WHEN** the Go provider sends a different HTTP method or request path than the expected snapshot
- **THEN** the conformance test fails and identifies the mismatched request index

### Requirement: Order-insensitive JSON object comparison
Request body comparison SHALL ignore JSON object field ordering by comparing decoded JSON values instead of raw request body bytes. The comparison SHALL preserve exact ordering for JSON arrays and for the sequence of request snapshots in `expected-requests.jsonl`, except tool declaration arrays SHALL be normalized by tool identity before comparison.

#### Scenario: Same object fields in different order
- **WHEN** the expected and actual request bodies contain the same JSON object fields with the same values but in different serialized order
- **THEN** the request input assertion passes

#### Scenario: Ordered array differs
- **WHEN** the expected and actual request bodies contain an array with the same elements in different order
- **THEN** the request input assertion fails

#### Scenario: Tool declaration array differs only by order
- **WHEN** the expected and actual request bodies contain a `tools` array with the same tool declarations in different order
- **THEN** the request input assertion passes

#### Scenario: Multi-step request order differs
- **WHEN** the Go provider sends semantically matching requests in a different step order than the expected request snapshots
- **THEN** the request input assertion fails

### Requirement: Request header normalization
Request header comparison SHALL use provider-specific allowlists of behavior-affecting headers. Header names SHALL be normalized to lowercase, values SHALL be trimmed, secret values SHALL be redacted, and volatile transport headers SHALL be excluded from snapshots and comparisons.

#### Scenario: Header name casing differs
- **WHEN** the expected and actual requests use different casing for the same included header name
- **THEN** the request input assertion compares them as the same header

#### Scenario: Volatile header differs
- **WHEN** a volatile transport header such as `host`, `content-length`, `user-agent`, `accept-encoding`, or connection management differs
- **THEN** the request input assertion ignores that header

#### Scenario: Behavior-affecting header differs
- **WHEN** an included provider header such as a beta or version header differs
- **THEN** the request input assertion fails

#### Scenario: Beta header order differs
- **WHEN** expected and actual Anthropic beta headers contain the same comma-separated beta values in different order or with different whitespace
- **THEN** the request input assertion passes

#### Scenario: Secret header present
- **WHEN** an included auth header is present in a captured request
- **THEN** the snapshot records a redacted value rather than the secret header value
- **AND** the comparison verifies the normalized redacted representation

### Requirement: Provider-specific request value normalization
Provider-specific request value normalization SHALL be narrow and documented. For Anthropic, tool-result JSON content SHALL compare semantically whether it is serialized as a raw JSON string or as a single text content block containing JSON, and `web_search_result.page_age: null` SHALL compare the same as an omitted `page_age`.

#### Scenario: Anthropic tool result JSON serialization differs
- **WHEN** expected and actual Anthropic request bodies contain equivalent `tool_result` JSON content serialized with different JSON object field order or different raw-string versus text-block shape
- **THEN** the request input assertion passes

### Requirement: ID comparison strategy
The system SHALL use exact comparison for all IDs in the UIMessageChunk stream. Content block IDs (from the provider, e.g. stringified block index) and tool call IDs (from the API response) are deterministic for a given fixture. Message-level IDs SHALL be controlled via deterministic generators configured identically in both the TypeScript tools and Go tests.

#### Scenario: Exact comparison with deterministic IDs
- **WHEN** both SDKs process the same fixture with deterministic message ID generators
- **THEN** the UIMessageChunk sequences are compared exactly (byte-identical JSON per line)

### Requirement: Recorded tool approval conformance fixtures

The conformance suite SHALL include recorded fixtures for Anthropic tool approval flows that compare Go `StreamText` -> `ToUIMessageStream` output against upstream TypeScript SDK output. The recorded coverage SHALL include a pending approval request, an approved local tool execution resumed from an approval response, and a denied local tool execution resumed from an approval response.

#### Scenario: Recorded approval request fixture
- **WHEN** `mise run test-conformance` runs the recorded approval request fixture
- **THEN** the Go UI chunk sequence SHALL exactly match the upstream TypeScript `expected.jsonl`
- **AND** the sequence SHALL include a `tool-approval-request` chunk for the tool call

#### Scenario: Recorded approved execution fixture
- **WHEN** `mise run test-conformance` runs the recorded approved execution fixture
- **THEN** the Go UI chunk sequence SHALL exactly match the upstream TypeScript `expected.jsonl`
- **AND** the sequence SHALL show the approved tool execution result before the subsequent model response completes

#### Scenario: Recorded denied execution fixture
- **WHEN** `mise run test-conformance` runs the recorded denied execution fixture
- **THEN** the Go UI chunk sequence SHALL exactly match the upstream TypeScript `expected.jsonl`
- **AND** the sequence SHALL preserve the denied approval response and execution-denied behavior expected by upstream

#### Scenario: Conformance expected output is regenerated from local upstream beta
- **WHEN** approval conformance fixtures are added or refreshed
- **THEN** their `expected.jsonl` files SHALL be generated with `test/conformance/tools/generate.mts` using the local upstream TypeScript SDK clone/dependencies

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

- **WHEN** the replay server is in Anthropic SSE mode
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

- **WHEN** `mise run test-conformance` runs the `bedrock/recorded/simple-text` case
- **THEN** the Go UIMessageChunk sequence exactly matches the upstream TypeScript `expected.jsonl`

#### Scenario: Tool call recorded fixture

- **WHEN** `mise run test-conformance` runs the `bedrock/recorded/tool-call` case
- **THEN** the Go UIMessageChunk sequence exactly matches the upstream TypeScript `expected.jsonl` and includes a tool call chunk

#### Scenario: Parallel tool calls recorded fixture

- **WHEN** `mise run test-conformance` runs the `bedrock/recorded/parallel-tool-calls` case
- **THEN** the Go UIMessageChunk sequence exactly matches the upstream TypeScript `expected.jsonl` and contains two concurrent tool calls in the same step

#### Scenario: Thinking text recorded fixture

- **WHEN** `mise run test-conformance` runs the `bedrock/recorded/thinking-text` case
- **THEN** the Go UIMessageChunk sequence exactly matches the upstream TypeScript `expected.jsonl` and contains reasoning content parts

#### Scenario: Guardrail provider options request fixture

- **WHEN** `mise run test-conformance` runs the `bedrock/recorded/guardrail` case
- **THEN** the Go request matches the upstream `expected-requests.jsonl`
- **AND** the top-level `guardrailConfig` preserves the configured identifier, version, trace, and stream processing mode fields

### Requirement: Conformance baseline consistency

The conformance tooling SHALL use the registered upstream parity baseline as the declared source of truth for TypeScript package versions used to generate expected outputs and request snapshots. A validation check SHALL compare the baseline manifest against the conformance tools dependency pins and fail when the declared versions differ.

#### Scenario: Conformance dependencies match baseline

- **WHEN** the conformance TypeScript dependency pins match the upstream parity baseline manifest
- **THEN** baseline validation passes

#### Scenario: Conformance dependencies drift from baseline

- **WHEN** a conformance TypeScript dependency pin differs from the upstream parity baseline manifest
- **THEN** baseline validation fails and identifies the mismatched dependency

### Requirement: Snapshot generation declares upstream baseline

The TypeScript conformance generation and recording workflow SHALL be traceable to the registered upstream parity baseline. Regenerating `expected.jsonl` and `expected-requests.jsonl` SHALL use the package versions declared by the baseline, and upgrade workflows SHALL update the baseline metadata alongside regenerated snapshots.

#### Scenario: Expected output is regenerated

- **WHEN** a contributor regenerates conformance expected outputs and request snapshots
- **THEN** the generated artifacts are produced using TypeScript package versions that match the registered upstream parity baseline

#### Scenario: Baseline upgrade regenerates snapshots

- **WHEN** a contributor bumps upstream TypeScript package versions for a parity upgrade
- **THEN** regenerated expected outputs and request snapshots are reviewed together with the baseline manifest update

### Requirement: Parity check runs conformance signal

The repository parity check command SHALL run the conformance test signal required by the upstream parity baseline. The command MAY run the full conformance suite or a documented stable subset, but the selected scope SHALL be recorded so contributors know which conformance coverage was enforced.

#### Scenario: Full conformance is configured

- **WHEN** the upstream parity baseline requires full conformance
- **THEN** the parity check command runs the full conformance test suite

#### Scenario: Stable subset is configured

- **WHEN** the upstream parity baseline requires only a stable conformance subset
- **THEN** the parity check command runs that subset and documents that full conformance remains advisory

### Requirement: Truncated provider stream coverage

The conformance suite SHALL include deterministic fixtures for provider streams that close without a finish part. Coverage SHALL distinguish an incomplete stream with no model output from an incomplete stream with partial model output and SHALL compare direct provider replay against the registered upstream UI chunk sequence.

#### Scenario: Empty truncated provider stream

- **WHEN** a provider response emits only administrative metadata and closes without a finish part
- **THEN** upstream expected output contains an error chunk and no finish chunk
- **AND** the Go result reports a stream error

#### Scenario: Partial truncated provider stream

- **WHEN** a provider response emits model output and closes without a finish part
- **THEN** upstream expected output retains the partial chunks, emits `finish-step`, and finishes with reason `other`
- **AND** the Go result does not report a stream error

### Requirement: Conformance as confidence suite

The conformance suite SHALL be treated as both an upstream parity checker and an executable confidence suite for provider-boundary and UI-boundary behavior. When reported bugs or new features can be represented through recorded provider chunks, provider request snapshots, or structured output snapshots, contributors SHOULD add or update conformance coverage before or alongside implementation changes.

#### Scenario: Bug is reproducible through replay

- **WHEN** a reported bug can be expressed as provider fixture input and expected upstream output
- **THEN** the conformance fixture is added or updated before the implementation fix is considered complete

#### Scenario: Provider behavior changes

- **WHEN** provider request conversion, response parsing, provider-defined tools, or provider options change
- **THEN** the conformance evidence includes request snapshots, stream output snapshots, or a documented reason existing coverage is sufficient

#### Scenario: Core stream behavior changes

- **WHEN** core orchestration, stream part conversion, UI chunk output, tools, or structured output behavior changes
- **THEN** the conformance evidence includes UI chunk snapshots, structured output snapshots, or a documented reason existing coverage is sufficient

### Requirement: Provider-independent core UI conformance

The conformance suite SHALL support deterministic provider-independent cases for core orchestration and UI lifecycle behavior that cannot be represented reliably by timed provider replay. These cases SHALL live under `test/conformance/ui/<capability>/<scenario>/`, SHALL contain an `expected.jsonl` traced to the registered upstream baseline, and SHALL run the actual Go `StreamText` to `ToUIMessageStream` path with a controlled mock language model. They SHALL NOT require `config.yaml`, provider input chunks, or `expected-requests.jsonl`, and provider fixture discovery and inventory SHALL exclude them.

#### Scenario: Generated-file UI chunk parity

- **WHEN** a controlled model replays URL-valued reasoning-file and inline-data file stream parts
- **THEN** Go SHALL emit the same `reasoning-file` and `file` UI chunks, fields, metadata, ordering, and lifecycle chunks as the registered upstream `ai` package
- **AND** the expected sequence SHALL be reproducible from the fixture's stream-part input and exact-baseline TypeScript generator

#### Scenario: Cancellation before provider output

- **WHEN** context cancellation occurs after the provider stream starts but before it emits output
- **THEN** the UI sequence SHALL end with exactly one `abort` chunk containing the cancellation reason
- **AND** the UI finish callback SHALL report `IsAborted` as true

#### Scenario: Cancellation after partial output

- **WHEN** text start and partial text delta parts are emitted before context cancellation
- **THEN** the UI sequence SHALL preserve those emitted text chunks in order and end with exactly one `abort` chunk
- **AND** the UI finish callback SHALL report `IsAborted` as true

#### Scenario: Provider output is pending when cancellation is observed

- **WHEN** provider parts are available but cancellation is already observable before orchestration processes them
- **THEN** the pending provider parts SHALL NOT appear in the UI sequence
- **AND** the UI sequence SHALL end with exactly one `abort` chunk

#### Scenario: Core UI cases remain outside provider fixture inventory

- **WHEN** parity coverage inventory scans conformance cases
- **THEN** provider-independent core UI cases without `config.yaml` SHALL NOT be treated as incomplete provider fixtures

### Requirement: OpenAI conformance provider directory
The conformance suite SHALL include an OpenAI provider directory at
`test/conformance/openai/` containing `upstream/` and `recorded/` subdirectories
and a `conformance_test.go` (gated by the `conformance` build tag) that
auto-discovers test case directories and runs each through the shared runner. The
test SHALL instantiate the OpenAI Responses provider pointing at the replay
server via a base-URL request option and a deterministic ID generator, then
compare the produced UIMessageChunk sequence against `expected.jsonl` and the
captured requests against `expected-requests.jsonl`.

#### Scenario: OpenAI case auto-discovery
- **WHEN** a new test case directory is added under `test/conformance/openai/upstream/` or `test/conformance/openai/recorded/`
- **THEN** the OpenAI conformance test discovers and runs it without Go code changes

#### Scenario: OpenAI conformance passes for matching output
- **WHEN** the Go OpenAI provider produces a UIMessageChunk sequence identical to `expected.jsonl` and requests matching `expected-requests.jsonl`
- **THEN** the OpenAI conformance test passes

#### Scenario: Build tag gating for OpenAI cases
- **WHEN** Go tests run without the `conformance` build tag
- **THEN** the OpenAI conformance tests are not compiled or executed

### Requirement: OpenAI request header normalization
The conformance runner SHALL define an OpenAI request-header allowlist of
behavior-affecting headers, normalizing header names to lowercase, trimming
values, redacting the OpenAI authorization secret, and excluding volatile
transport headers from snapshots and comparisons.

#### Scenario: OpenAI auth header redacted
- **WHEN** an OpenAI request carries an `Authorization` header
- **THEN** the snapshot records a redacted value rather than the secret
- **AND** the comparison verifies the normalized redacted representation

#### Scenario: OpenAI behavior-affecting header differs
- **WHEN** an included OpenAI header such as a beta or version header differs between expected and actual requests
- **THEN** the request input assertion fails

### Requirement: OpenAI request value normalization
OpenAI request-body normalization SHALL preserve the order of the `input` item
array and treat object field ordering as insensitive, consistent with the
order-insensitive JSON object comparison requirement. Any OpenAI-specific
semantic equivalences SHALL be narrow and documented.

#### Scenario: OpenAI input array order is significant
- **WHEN** expected and actual OpenAI request bodies contain an `input` array with the same items in different order
- **THEN** the request input assertion fails

#### Scenario: OpenAI object field order is ignored
- **WHEN** expected and actual OpenAI request bodies contain the same object fields with the same values in different serialized order
- **THEN** the request input assertion passes

### Requirement: OpenAI TypeScript tooling support
The TypeScript recording and generation tools SHALL support the OpenAI provider:
`createModel` SHALL construct an `@ai-sdk/openai` Responses model, the recording
tool SHALL define the OpenAI base URL and require `OPENAI_API_KEY`, and the
generation tool SHALL regenerate `expected.jsonl` and `expected-requests.jsonl`
for OpenAI fixtures from committed inputs without API keys.

#### Scenario: Generate OpenAI goldens offline
- **WHEN** `mise run generate-conformance` runs against committed OpenAI fixtures
- **THEN** OpenAI `expected.jsonl` and `expected-requests.jsonl` are regenerated without any API key

#### Scenario: Record OpenAI fixtures requires key
- **WHEN** the recording tool runs an OpenAI scenario without `OPENAI_API_KEY`
- **THEN** the tool reports the missing key and does not record

### Requirement: OpenAI conformance fixture coverage
The conformance suite SHALL include OpenAI fixtures covering at minimum: simple
text generation, function tool calling, reasoning with summaries, structured
(JSON schema) output, a provider-executed built-in tool (e.g. web search) with
source citations, conversation continuation via `previous_response_id`, and
provider-tool continuation request taxonomy for shell, local-shell, tool-search,
apply-patch, and custom tools. Upstream fixtures SHALL be tracked in an
`upstream/INDEX.yaml` mapping upstream Vercel fixture names to local directories.

#### Scenario: Text generation fixture
- **WHEN** `mise run test-conformance` runs the OpenAI simple-text fixture
- **THEN** the Go UIMessageChunk sequence exactly matches the upstream TypeScript `expected.jsonl`

#### Scenario: Provider-executed tool fixture
- **WHEN** `mise run test-conformance` runs the OpenAI web-search fixture
- **THEN** the Go output includes the provider-executed tool-call, tool-result, and source chunks matching upstream

#### Scenario: Provider-tool continuation fixtures
- **WHEN** `mise run test-conformance` runs the OpenAI provider-tool continuation fixtures
- **THEN** the Go requests preserve upstream item-reference, native call/output taxonomy, tool-name mapping, and call/result ids
- **AND** the requests exactly match the registered upstream TypeScript snapshots

### Requirement: Parity coverage inventory

The repository SHALL provide a parity coverage inventory command that validates local conformance fixture completeness and provider upstream fixture index coverage.

#### Scenario: Fixture artifacts are complete

- **WHEN** a conformance test case has a `config.yaml`
- **THEN** the inventory verifies the test case has `expected.jsonl` and `expected-requests.jsonl`

#### Scenario: Upstream fixture is intentionally missing

- **WHEN** an upstream streaming fixture exists in the local upstream clone but is not imported
- **THEN** the provider `INDEX.yaml` records the fixture as `null`

### Requirement: Expanded conformance configuration

The conformance harness SHALL support parity-sensitive `streamText`, `convertToModelMessages`, and `toUIMessageStream` options needed to reproduce upstream behavior. The supported config SHALL include configured `messages` with text, reasoning, base64 file, and provider-reference file parts; `toolChoice`; `activeTools`; `streamOptions`; persisted `uiMessages`; declarative tool `modelOutput`; tool `providerOptions`; and tool error simulation.

#### Scenario: Provider-reference file message is configured

- **WHEN** a fixture config declares a file message part with `mediaType`, optional `filename`, and a provider `reference` map
- **THEN** the Go conformance path SHALL build a provider file part with canonical reference data
- **AND** the TypeScript path SHALL build the equivalent tagged reference file part

#### Scenario: Provider-reference request is snapshotted

- **WHEN** an OpenAI fixture supplies a provider-reference file message
- **THEN** `expected-requests.jsonl` SHALL assert the resolved provider file ID in the outgoing request

#### Scenario: Tool choice is configured

- **WHEN** a fixture config declares `toolChoice`
- **THEN** the Go and TypeScript conformance paths pass the same tool choice to the SDK

#### Scenario: Active tools are configured

- **WHEN** a fixture config declares `activeTools`
- **THEN** the Go and TypeScript conformance paths pass the same active tool filter to the SDK

#### Scenario: UI stream options are configured

- **WHEN** a fixture config declares `streamOptions`
- **THEN** the Go and TypeScript conformance paths apply equivalent UI message stream options

#### Scenario: Tool execution error is configured

- **WHEN** a function tool config declares a mock error
- **THEN** the Go and TypeScript conformance paths make the tool execution fail with the configured message

#### Scenario: Persisted UI messages are configured

- **WHEN** a fixture config declares `uiMessages`
- **THEN** the Go and TypeScript conformance paths convert those messages with the configured tools before invoking `streamText`

#### Scenario: Tool model output is configured

- **WHEN** a function tool config declares `modelOutput`
- **THEN** the Go and TypeScript conformance paths expose equivalent `ToModelOutput` behavior for persisted successful tool results
