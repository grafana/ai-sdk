# Spec Delta

## ADDED Requirements

### Requirement: Unary provider-result golden

Every supported provider fixture with `operation: generate` SHALL have an `expected-generate.json` produced offline from the existing input by the registered upstream provider's `doGenerate`. The Go runner SHALL replay the same input through `DoGenerate` and compare the ordered content, finish reason, usage, all provider metadata namespaces and warnings against that golden. Request snapshots SHALL remain independently compared.

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

### Requirement: TypeScript generation tool
`test/conformance/tools/generate.mts` SHALL regenerate expectations from existing `upstream/` and `recorded/` fixtures without live API calls. For streaming cases it SHALL read `config.yaml` and `input-*.chunks.txt`, serve the input through a local test server, run the upstream TypeScript SDK pipeline, and write the resulting UIMessageChunk sequence to `expected.jsonl`.

#### Scenario: Regenerating expected output
- **WHEN** the generation tool runs for a streaming test case with existing fixtures
- **THEN** it produces `expected.jsonl` by piping the fixtures through the upstream TypeScript SDK with a deterministic ID generator

#### Scenario: Regenerating all test cases
- **WHEN** the generation tool is invoked without a `--scenario` flag
- **THEN** it regenerates `expected.jsonl` for all streaming test case directories across all providers

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
