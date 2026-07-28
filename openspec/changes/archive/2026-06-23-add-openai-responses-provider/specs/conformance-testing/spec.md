## ADDED Requirements

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
- **WHEN** `make generate-conformance` runs against committed OpenAI fixtures
- **THEN** OpenAI `expected.jsonl` and `expected-requests.jsonl` are regenerated without any API key

#### Scenario: Record OpenAI fixtures requires key
- **WHEN** the recording tool runs an OpenAI scenario without `OPENAI_API_KEY`
- **THEN** the tool reports the missing key and does not record

### Requirement: OpenAI conformance fixture coverage
The conformance suite SHALL include OpenAI fixtures covering at minimum: simple
text generation, function tool calling, reasoning with summaries, structured
(JSON schema) output, a provider-executed built-in tool (e.g. web search) with
source citations, and conversation continuation via `previous_response_id`.
Upstream fixtures SHALL be tracked in an `upstream/INDEX.yaml` mapping upstream
Vercel fixture names to local directories.

#### Scenario: Text generation fixture
- **WHEN** `make test-conformance` runs the OpenAI simple-text fixture
- **THEN** the Go UIMessageChunk sequence exactly matches the upstream TypeScript `expected.jsonl`

#### Scenario: Provider-executed tool fixture
- **WHEN** `make test-conformance` runs the OpenAI web-search fixture
- **THEN** the Go output includes the provider-executed tool-call, tool-result, and source chunks matching upstream
