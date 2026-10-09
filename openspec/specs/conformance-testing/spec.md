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

The system SHALL support provider `upstream/` fixtures copied from Vercel AI SDK and locally controlled `recorded/` fixtures captured from real APIs or deterministically derived from captured payloads for transport failures that cannot be recorded reliably. Both SHALL share fixture format and `config.yaml` schema. `record.mts` SHALL operate only on `recorded/`; `generate.mts` SHALL operate on both.

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

### Requirement: Derived transport-failure provenance

Derived transport-failure fixtures SHALL preserve valid provider event shapes and SHALL document their synthetic condition through the test case name and configuration.

#### Scenario: Derived transport-failure provenance
- **WHEN** a captured payload is minimally derived to reproduce a transport failure
- **THEN** its event shapes SHALL remain valid and its name and configuration SHALL document the synthetic condition

### Requirement: Test case configuration

Each provider-backed test case SHALL contain `config.yaml` declaring replay configuration with required string `model`. Provider SHALL be inferred from parent directories, not YAML. Go SHALL marshal each YAML provider namespace value to JSON and wrap it as `provider.RawProviderOption` for typed `StreamText` options. Go and TypeScript SHALL support equivalent ordered `messages` for system, user, assistant and tool roles, enabling continuation snapshots without another model step.

#### Scenario: Replay configuration keys and defaults
- **WHEN** the replay configuration schema is inspected
- **THEN** it SHALL require `model` (string) and optionally support:
- `system`
- `prompt` (string, for recording and documentation)
- `messages` (ordered role/content entries) and persisted `uiMessages`
- `allowSystemInMessages`
- `stopWhenStepCount` (integer, default 1)
- `providerOptions` (nested map)
- `tools` (map of tool name to definition with `description`, `inputSchema` JSON schema, `mockResults` list, optional declarative `modelOutput`, and optional approval configuration)
- `providerTools`, `responseFormat`, `assertOutputValue`
- approval-resumption setup for scenarios replaying a second approval call
- `expectStreamError` (boolean, default false)

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

### Requirement: Configured message content shapes

Configured message content SHALL support scalar text or ordered `text`, `reasoning`, `file`, `tool-call` and `tool-result` parts with part `providerOptions`. File parts SHALL carry `mediaType`, optional `filename`, and base64 `data` or a provider `reference` map. Tool-call/result entries SHALL support tool ids, names, inputs or structured outputs, provider-executed state where applicable and per-part provider options.

#### Scenario: Configured message content shapes
- **WHEN** a configured assistant message contains ordered tool-call and tool-result parts
- **THEN** both loaders SHALL preserve ids, names, inputs/structured outputs, applicable provider-executed state and per-part provider options

### Requirement: Configured tool approval resumption

Both Go and TypeScript SHALL support conformance YAML tool approval for upstream-equivalent recorded flows, including tools always requiring approval. Approval-resumption setup SHALL allow seeding an assistant tool call plus approval request and a tool approval response, allowing approved and denied second-call fixtures to replay without manual code changes.

#### Scenario: Configured tool approval resumption
- **WHEN** a second-call fixture seeds a denied tool approval response
- **THEN** both paths SHALL seed the assistant call/request and tool response and replay the denied flow without manual code changes

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

Each provider-backed case SHALL contain `expected-requests.jsonl` capturing upstream TypeScript provider inputs: one JSON object per provider API request in request order. Each snapshot SHALL include HTTP method, normalized escaped target in `path`, normalized behavior-affecting headers and decoded JSON body. The target SHALL include any nonempty URL query with the escaped pathname.

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
- **AND** `path` contains the escaped pathname followed by the nonempty query, if present, without scheme, authority or fragment

### Requirement: TypeScript recording tool

The system SHALL provide `test/conformance/tools/record.mts` to capture real API fixtures only in `recorded/`. It SHALL read `config.yaml`, proxy between upstream TypeScript SDK and the real API, run configured `streamText`, and save raw responses as `input-N.chunks.txt` and UIMessageChunk output as `expected.jsonl` in the case directory.

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

The system SHALL provide `test/conformance/runner.go` using `httptest.Server` to serve `input.chunks.txt` or `input-N.chunks.txt` as SSE with `Content-Type: text/event-stream`. Each line SHALL use framing `event: <type>\ndata: <json>\n\n`, taking event type from its JSON `type`. Go and TypeScript replay SHALL use the same SSE format as real Anthropic.

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

### Requirement: Replay request sequencing

For multi-step cases, the replay server SHALL be stateful: serve `input-1.chunks.txt` for the first request, `input-2.chunks.txt` for the second, and so on. It SHALL return an HTTP error when fixtures are exhausted.

#### Scenario: Replay request sequencing
- **WHEN** a third request arrives when a case contains only two numbered input files
- **THEN** the server SHALL return an HTTP error after serving those files in request order

### Requirement: Go conformance test runner

The system SHALL provide per-provider Go tests (e.g. `test/conformance/anthropic/conformance_test.go`) auto-discovering cases in provider `upstream/` and `recorded/`. Each SHALL compare deterministic Go `StreamText` -> `ToUIMessageStream` output with `expected.jsonl`. Shared replay, comparison and helpers SHALL live in `test/conformance/runner.go`.

#### Scenario: Discovered provider case execution
- **WHEN** a provider test discovers a test case directory
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

### Requirement: Go request input comparison
The Go conformance runner SHALL capture actual Go provider requests during replay and compare them against `expected-requests.jsonl`. The runner SHALL fail when request counts differ, method or normalized escaped request target differs, normalized headers differ, or decoded JSON bodies differ. Comparison of the `path` field SHALL be exact, including query spelling and pair order; it SHALL NOT accept a query-bearing request as matching a legacy path-only expectation.

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
- **WHEN** the Go provider sends a different HTTP method or escaped request target than the expected snapshot
- **THEN** the conformance test fails and identifies the mismatched request index

#### Scenario: API version differs only in query
- **WHEN** the expected request target is `/v1/messages?api-version=2024-10-21&feature=a&feature=b` and the actual target differs only in the `api-version` value
- **THEN** the request comparison fails with a path mismatch and the request index

#### Scenario: Repeated values differ only in order
- **WHEN** otherwise identical expected and actual requests contain `feature=a&feature=b` and `feature=b&feature=a` respectively
- **THEN** the request comparison fails with a path mismatch and the request index

#### Scenario: Query is missing
- **WHEN** otherwise identical expected and actual requests differ only by presence of a nonempty query
- **THEN** the request comparison fails with a path mismatch and the request index

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

The system SHALL compare UIMessageChunk IDs exactly. Provider content block IDs (e.g. stringified block index) and API tool call IDs are deterministic for a fixture. Message IDs SHALL use deterministic generators configured identically in Go and TypeScript. Sequences SHALL compare positionally except adjacent locally-executed tool output runs, whose completion order is normalized.

#### Scenario: Exact comparison with deterministic IDs
- **WHEN** both SDKs process the same fixture with deterministic message ID generators
- **THEN** the UIMessageChunk sequences are compared exactly (byte-identical JSON per line), except for the order within a run of adjacent locally-executed tool output chunks

#### Scenario: Concurrent tool outputs compare regardless of arrival order

- **WHEN** a step contains two locally-executed tools whose output chunks are adjacent in the expected sequence
- **AND** the Go implementation emits them in the opposite order because the second tool completed first
- **THEN** the comparison passes

#### Scenario: Success and error outputs normalize as one run

- **WHEN** a step contains one tool that succeeds and one that fails, and their `tool-output-available` and `tool-output-error` chunks are adjacent
- **THEN** the two chunks are compared as an unordered pair

#### Scenario: Provider-executed output order is still exact

- **WHEN** two adjacent `tool-output-available` chunks carry `providerExecuted: true` and are swapped relative to the fixture
- **THEN** the comparison fails

#### Scenario: Rejected tool call output order is still exact

- **WHEN** a rejected call's `tool-output-error` chunk is adjacent to an executed tool's output chunk and the two are swapped relative to the fixture
- **THEN** the comparison fails

### Requirement: Local tool output run normalization

In expected and actual sequences, the comparator SHALL sort each maximal adjacent run of `tool-output-available` and `tool-output-error` chunks by `toolCallId` before comparison, because local tools emit in completion order. A run SHALL NOT cross any other chunk type or step boundary; order relative to all other chunks SHALL stay exact.

#### Scenario: Local tool output run normalization
- **WHEN** adjacent local success and error outputs arrive in opposite completion order
- **THEN** the comparator SHALL sort the maximal run by `toolCallId` in both sequences without crossing other chunk types or step boundaries

### Requirement: Rejected and provider-executed output ordering

An output whose `toolCallId` has a `tool-input-error` in the sequence is a rejected call emitted during model-stream reading; it SHALL be excluded from and end local-output runs. A chunk with `providerExecuted: true` SHALL also be excluded from and end runs: provider-executed outputs SHALL retain exact recorded provider order.

#### Scenario: Rejected and provider-executed output ordering
- **WHEN** a provider-executed output separates two local outputs
- **THEN** it SHALL end the normalization run and stay exactly ordered; the local outputs SHALL NOT be regrouped across it

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

The suite SHALL support deterministic provider-independent core orchestration/UI lifecycle cases unsuitable for timed provider replay, under `test/conformance/ui/<capability>/<scenario>/`. Each SHALL contain `expected.jsonl` traced to the registered baseline and run actual Go `StreamText` -> `ToUIMessageStream` with a controlled mock model. They SHALL NOT require `config.yaml`, provider chunks or `expected-requests.jsonl`; provider discovery/inventory SHALL exclude them.

#### Scenario: Generated-file UI chunk parity

- **WHEN** a controlled model replays a `data:` URL-valued reasoning-file and inline-data file stream parts
- **THEN** Go SHALL emit the same resolved `reasoning-file` and `file` UI chunks, fields, metadata, ordering, and lifecycle chunks as the registered upstream `ai` package
- **AND** the expected sequence SHALL be reproducible from the fixture's stream-part input and exact-baseline TypeScript generator without external network access

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

The suite SHALL include `test/conformance/openai/` with `upstream/`, `recorded/` and conformance-build-tagged `conformance_test.go` auto-discovering cases through the shared runner. Tests SHALL instantiate OpenAI Responses pointed at replay via a base-URL request option with a deterministic ID generator, compare UIMessageChunk output with `expected.jsonl` and captured requests with `expected-requests.jsonl`.

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

OpenAI fixtures SHALL cover at least simple text, function calls, reasoning summaries, JSON-schema output, provider-executed built-ins (e.g. web search) with citations, `previous_response_id` continuation, and provider-tool continuation taxonomy for shell, local-shell, tool-search, apply-patch and custom tools. `upstream/INDEX.yaml` SHALL map upstream Vercel fixture names to local directories.

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

### Requirement: Escaped request target normalization

TypeScript and Go capture SHALL use escaped pathname plus nonempty query, omitting scheme, authority, fragment and empty query marker; empty pathname SHALL become `/`. They SHALL preserve query order, repeated keys/values, percent escaping, literal plus, key-only parameters and empty values without decoding, re-encoding, sorting or collapsing pairs. Queryless escaped paths SHALL remain unchanged.

#### Scenario: Behavior-affecting query and repeated values
- **WHEN** either implementation captures `/v1/messages?api-version=2024-10-21&feature=a&feature=b`
- **THEN** `path` is `/v1/messages?api-version=2024-10-21&feature=a&feature=b`

#### Scenario: Escaped path and query delimiters
- **WHEN** either implementation captures `/v1/a%2Fb?q=a%26b%3Dc&space=a+b&space=a%20b`
- **THEN** `path` is `/v1/a%2Fb?q=a%26b%3Dc&space=a+b&space=a%20b`
- **AND** no escaped delimiter is interpreted as a path separator or query pair separator

#### Scenario: Interleaved duplicates and empty values
- **WHEN** either implementation captures `/v1/messages?feature=a&flag&feature=b&empty=&feature=a`
- **THEN** `path` preserves that complete target without regrouping, deduplicating or adding an equals sign to `flag`

#### Scenario: Percent-encoded UTF-8 and escape spelling
- **WHEN** either implementation captures `/v1/%E2%9C%93?q=%e2%9c%93`
- **THEN** `path` retains `/v1/%E2%9C%93?q=%e2%9c%93` without changing escape spelling

#### Scenario: Queryless request
- **WHEN** either implementation captures `/v1/messages`
- **THEN** `path` remains `/v1/messages`

#### Scenario: Empty query marker
- **WHEN** either implementation captures `/v1/messages?`
- **THEN** `path` is `/v1/messages`

#### Scenario: Absolute URL with empty path and fragment
- **WHEN** either implementation captures `https://example.test?x=1#ignored`
- **THEN** `path` is `/?x=1`

#### Scenario: Literal apostrophe in a serialized query
- **WHEN** either implementation captures `/v1/messages?q=O'Reilly` or `https://example.test/v1/messages?q=O'Reilly`
- **THEN** `path` is `/v1/messages?q=O'Reilly`, not `/v1/messages?q=O%27Reilly`

#### Scenario: Query delimiter appears only in a fragment
- **WHEN** either implementation captures `/v1/messages#ignored?not=a-query`
- **THEN** `path` is `/v1/messages`, without the fragment's apparent query

### Requirement: Cross-language request target regression evidence

The harness SHALL maintain shared synthetic request cases and committed TypeScript-generated `expected-requests.jsonl` in `test/conformance/testdata/request-snapshots/`, outside provider inputs. At least one snapshot SHALL include nonsecret behavior-affecting API-version query and ordered repeated values. Evidence SHALL be identified as harness-only sensitivity testing, not recorded provider behavior or new provider support.

#### Scenario: Matching cross-language snapshot
- **WHEN** Go captures the requests represented by the shared synthetic cases
- **THEN** the production comparator accepts them against the committed TypeScript-generated request snapshots
- **AND** both implementations' target assertions retain the API version and repeated values

#### Scenario: Stale TypeScript expectation
- **WHEN** recomputing snapshots from the shared cases differs from the committed JSONL
- **THEN** the normal TypeScript test/check fails without changing the committed file

#### Scenario: Executable mismatch witness
- **WHEN** a focused test captures a request differing only in API version, repeated-value order or query presence from the committed TypeScript expectation
- **THEN** the production comparator reports a path mismatch and fails its isolated test invocation
- **AND** the enclosing regression test verifies that rejection without failing the normal suite

#### Scenario: Explicit harness regeneration
- **WHEN** a contributor explicitly regenerates the harness request expectations
- **THEN** the registered conformance tools' request snapshot normalizer and JSONL writer produce the expectations from controlled nonsecret cases
- **AND** no provider response inputs, upstream pins or fixture provenance are modified

### Requirement: TypeScript request-target regression checks

TypeScript tests SHALL assert independently declared target values and verify committed expectations are current without rewriting them during normal tests.

#### Scenario: TypeScript request-target regression checks
- **WHEN** recomputed shared-case request snapshots differ from committed JSONL
- **THEN** the TypeScript test SHALL fail without rewriting the expectation

### Requirement: Go production request-target regression checks

Go tests SHALL load shared TypeScript expectations through the production loader, capture corresponding requests through the production snapshot function and exercise the production comparator for matching and mismatching queries.

#### Scenario: Go production request-target regression checks
- **WHEN** a Go test changes only API version in a captured shared request
- **THEN** the production loader, capture and comparator path SHALL reject the mismatch against the TypeScript expectation

### Requirement: Fixture configuration rejects unknown keys

TypeScript generation/recording and Go replay SHALL reject keys undeclared by any config type at top level and every structured nested value. Errors SHALL name the key and location and stop generation, recording or replay before snapshots are produced or consumed. Payloads SHALL remain open. A shared fixture listing every key at every level SHALL be accepted by both loaders to enforce key alignment.

#### Scenario: Nested configuration key boundaries
- **WHEN** unknown keys occur in structured config values or arbitrary payload maps
- **THEN** unknown-key rejection SHALL cover `tools.<name>`, `providerTools.<name>`, message entries/content parts, model-output items, approvals, stream options, tool choice and response format. Payload keys SHALL remain arbitrary in provider options, JSON schemas, tool inputs/outputs, provider tool arguments, headers and UI message parts

#### Scenario: Misspelled top-level key

- **WHEN** a fixture sets `stopWhenStepCoun: 2`
- **THEN** both loaders reject it and name `stopWhenStepCoun`

#### Scenario: Misspelled key in a message part

- **WHEN** a configured tool-call content part sets `toolCallID` instead of `toolCallId`
- **THEN** both loaders reject it and name `toolCallID`, including the Go path that decodes message content in a custom unmarshaler

#### Scenario: Payload maps stay open

- **WHEN** a fixture puts arbitrary keys inside `providerOptions`, a tool `inputSchema`, a tool-call `input` or a UI message part
- **THEN** both loaders accept it

#### Scenario: Key sets stay aligned

- **WHEN** one loader declares a key the shared all-keys fixture does not contain, or the other loader rejects a key the fixture contains
- **THEN** that language's alignment test fails

### Requirement: Provider-parts golden

Every streaming case of an enabled provider SHALL contain `expected-provider-parts.jsonl`: the parts the registered upstream model emitted for each model call, raw chunks included, one entry per call in emission order. The Go runner SHALL compare normalized provider parts of each call against it.

#### Scenario: Finish count per message stop
- **WHEN** the unchanged `programmatic-tool-calling` upstream fixture is replayed
- **THEN** the golden SHALL contain one finish part per `message_stop` and the Go parts SHALL match it, including part count and order

#### Scenario: Raw ordering
- **WHEN** a case enables raw chunks
- **THEN** the golden SHALL record raw parts in the order upstream emits them relative to the parts their frames produce, and Go SHALL match that order

#### Scenario: Narrow normalization
- **WHEN** Go and upstream parts are compared
- **THEN** only these SHALL be normalized: generated source and MCP tool-call IDs; the Go-only `provider`, `responseHeaders` and `usage` fields on `response-metadata`; the response timestamp for providers that derive it from the HTTP `Date` header; absent versus false `isError`; and the content of errors that carry no provider status

#### Scenario: Provider errors are compared in full
- **WHEN** an error part carries a status code
- **THEN** its message, status code and retryability SHALL be compared with upstream's

#### Scenario: Every enabled provider is green
- **WHEN** the conformance suite runs
- **THEN** every streaming case of every enabled provider SHALL match its golden without an allowlist entry, or the case SHALL appear on the allowlist with a tracking issue

#### Scenario: Multi-step case
- **WHEN** a case replays `input-1.chunks.txt` and `input-2.chunks.txt` as two model calls
- **THEN** the golden SHALL contain two call entries and Go SHALL match each in order

#### Scenario: Unrelated difference is visible
- **WHEN** a case differs from upstream for a reason outside the change under test
- **THEN** it SHALL be listed on an explicit allowlist naming a tracking issue, and SHALL NOT be skipped silently

### Requirement: Provider-parts golden scope

Anthropic, Bedrock, OpenAI and OpenAI-compatible SHALL be enabled. Fixture inputs SHALL remain unmodified, and non-streaming operations SHALL NOT have a golden because they produce no stream parts.

#### Scenario: Generate operation
- **WHEN** a Bedrock case uses the non-streaming generate operation
- **THEN** it SHALL have no `expected-provider-parts.jsonl` and the runner SHALL NOT require one

### Requirement: Synthetic provider unary cases

Unary behavior that no recorded or upstream fixture exhibits MAY be covered by labeled synthetic responses under `test/conformance/testdata/`. Expected results SHALL be generated by running the registered upstream `doGenerate`, and the Go result SHALL match the upstream content, finish reason, usage and provider metadata, or both SHALL fail the call.

#### Scenario: Input transformations
- **WHEN** a synthetic response carries input transformations, an empty array, null, nothing, or a malformed entry
- **THEN** Go SHALL match the upstream-generated result, or fail the call where upstream does

### Requirement: Synthetic provider stream cases

Scenarios that no recorded or upstream fixture exhibits MAY be covered by synthetic provider stream inputs under `test/conformance/testdata/`, never under `<provider>/recorded/` or `<provider>/upstream/`. Each input SHALL be labeled synthetic, and its expected parts SHALL be generated by running the registered upstream package. They SHALL NOT be presented as recorded or upstream evidence.

#### Scenario: Multiple deltas in one message
- **WHEN** a synthetic Anthropic stream has three `message_delta` events before one `message_stop`
- **THEN** the upstream-generated expectation and the Go parts SHALL both contain exactly one finish

#### Scenario: Error and incomplete-stream shapes
- **WHEN** synthetic streams contain an error between delta and stop, an error without stop, an error as first frame, or end after a delta
- **THEN** Go provider parts SHALL match the upstream-generated expectation for each

#### Scenario: Expectations are regenerated on baseline upgrade
- **WHEN** the generation workflow runs against a new registered baseline
- **THEN** synthetic case expectations SHALL be regenerated from the upstream packages declared by that baseline
