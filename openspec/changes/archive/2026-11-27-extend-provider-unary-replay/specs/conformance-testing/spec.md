## MODIFIED Requirements

### Requirement: OpenAI conformance provider directory

The suite SHALL include `test/conformance/openai/` with `upstream/`, `recorded/` and conformance-build-tagged `conformance_test.go` auto-discovering cases through the shared runner. Tests SHALL instantiate OpenAI Responses pointed at replay via a base-URL request option with a deterministic ID generator, compare the output against the golden for the fixture's operation and captured requests with `expected-requests.jsonl`.

#### Scenario: OpenAI case auto-discovery
- **WHEN** a new test case directory is added under `test/conformance/openai/upstream/` or `test/conformance/openai/recorded/`
- **THEN** the OpenAI conformance test discovers and runs it without Go code changes

#### Scenario: OpenAI conformance passes for matching output
- **WHEN** the Go OpenAI provider produces output matching `expected.jsonl` for streaming or `expected-generate.json` for unary, and requests matching `expected-requests.jsonl`
- **THEN** the OpenAI conformance test passes

#### Scenario: Build tag gating for OpenAI cases
- **WHEN** Go tests run without the `conformance` build tag
- **THEN** the OpenAI conformance tests are not compiled or executed

### Requirement: OpenAI TypeScript tooling support
The TypeScript recording and generation tools SHALL support the OpenAI provider:
`createModel` SHALL construct an `@ai-sdk/openai` Responses model, the recording
tool SHALL define the OpenAI base URL and require `OPENAI_API_KEY`, and the
generation tool SHALL regenerate `expected.jsonl` for streaming cases or
`expected-generate.json` for unary cases, plus `expected-requests.jsonl`, from
committed inputs without API keys.

#### Scenario: Generate OpenAI goldens offline
- **WHEN** `mise run generate-conformance` runs against committed OpenAI fixtures
- **THEN** the request and operation-specific output goldens are regenerated without any API key

#### Scenario: Record OpenAI fixtures requires key
- **WHEN** the recording tool runs an OpenAI scenario without `OPENAI_API_KEY`
- **THEN** the tool reports the missing key and does not record


[276 more lines in file. Use offset=740 to continue.]

### Requirement: Unary provider-result golden

For Anthropic, Bedrock, OpenAI Responses and OpenAI-compatible Chat Completions, unary replay SHALL serve unchanged `input.response.json` as `application/json`. The registered `doGenerate` SHALL produce `expected-generate.json` offline. Go `DoGenerate` SHALL match ordered content, finish reason, usage, all provider metadata namespaces and warnings. The runner SHALL compare the single captured request with `expected-requests.jsonl`.

#### Scenario: Unary result matches upstream
- **WHEN** a unary fixture replays successfully
- **THEN** the Go provider result matches the upstream golden, including both metadata namespaces when the provider emits both
- **AND** the single captured provider request matches the pinned request snapshot

#### Scenario: Unary input is missing
- **WHEN** a unary fixture has no `input.response.json`
- **THEN** replay fails instead of falling back to stream input

#### Scenario: Unary golden is missing or unreadable
- **WHEN** replay discovers a unary fixture without a readable result golden
- **THEN** the test fails before calling the provider and identifies the expected file and regeneration command instead of skipping the fixture

#### Scenario: Unary golden differs from the result
- **WHEN** the fixture's golden is invalid or differs from the Go provider result
- **THEN** the test fails even if the request snapshot matches

#### Scenario: Unary request differs from its snapshot
- **WHEN** the actual provider request differs from the snapshot or the request count is not one
- **THEN** the test fails even if the result matches

#### Scenario: Unary behavior is not representable
- **WHEN** the registered upstream adapter returns a structured-output text selection not represented by the golden projection
- **THEN** generation fails and identifies the unsupported result field

#### Scenario: Live recorder refuses unary cases
- **WHEN** the recorder is pointed at a fixture with `operation: generate`
- **THEN** it rejects the operation without making a live request
