## MODIFIED Requirements

### Requirement: Test case configuration

Each test case directory SHALL contain a `config.yaml` file that declares the replay configuration. The YAML SHALL specify: `model` (string), and optionally: `prompt` (string, for recording and documentation), `stopWhenStepCount` (integer, default 1), `providerOptions` (nested map), `tools` (map of tool name to tool definition with `description`, `inputSchema` JSON schema, `mockResults` list, and optional approval configuration), `providerTools`, `responseFormat`, and approval-resumption setup for scenarios that replay a second approval call. The provider SHALL be inferred from the parent directory structure, not from the YAML. When `providerOptions` is specified in YAML, the Go test SHALL marshal each provider namespace value to JSON and wrap it as `provider.RawProviderOption` for use with the typed provider options field on `StreamText`.

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

#### Scenario: Approval resumption config
- **WHEN** a config specifies a prior tool call, approval request, and approval response
- **THEN** the Go runner and TypeScript tools SHALL seed equivalent model messages before replaying the provider fixture
- **AND** both SDKs SHALL use those messages to resolve the approval before the model call

#### Scenario: Provider options config
- **WHEN** a config specifies `providerOptions` with nested YAML map values
- **THEN** the Go test SHALL marshal each namespace value to JSON, wrap as `RawProviderOption`, and pass the resulting provider options to `StreamText`

#### Scenario: Provider inference
- **WHEN** a test case is located at `anthropic/upstream/tool-call/`
- **THEN** the Go test SHALL use the Anthropic provider without needing a `provider` field in `config.yaml`

## ADDED Requirements

### Requirement: Recorded tool approval conformance fixtures

The conformance suite SHALL include recorded fixtures for Anthropic tool approval flows that compare Go `StreamText` -> `ToUIMessageStream` output against upstream TypeScript SDK output. The recorded coverage SHALL include a pending approval request, an approved local tool execution resumed from an approval response, and a denied local tool execution resumed from an approval response.

#### Scenario: Recorded approval request fixture
- **WHEN** `make test-conformance` runs the recorded approval request fixture
- **THEN** the Go UI chunk sequence SHALL exactly match the upstream TypeScript `expected.jsonl`
- **AND** the sequence SHALL include a `tool-approval-request` chunk for the tool call

#### Scenario: Recorded approved execution fixture
- **WHEN** `make test-conformance` runs the recorded approved execution fixture
- **THEN** the Go UI chunk sequence SHALL exactly match the upstream TypeScript `expected.jsonl`
- **AND** the sequence SHALL show the approved tool execution result before the subsequent model response completes

#### Scenario: Recorded denied execution fixture
- **WHEN** `make test-conformance` runs the recorded denied execution fixture
- **THEN** the Go UI chunk sequence SHALL exactly match the upstream TypeScript `expected.jsonl`
- **AND** the sequence SHALL preserve the denied approval response and execution-denied behavior expected by upstream

#### Scenario: Conformance expected output is regenerated from local upstream beta
- **WHEN** approval conformance fixtures are added or refreshed
- **THEN** their `expected.jsonl` files SHALL be generated with `test/conformance/tools/generate.mts` using the local upstream TypeScript SDK clone/dependencies
