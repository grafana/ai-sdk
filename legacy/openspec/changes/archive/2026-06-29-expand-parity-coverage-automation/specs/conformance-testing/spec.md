## ADDED Requirements

### Requirement: Parity coverage inventory
The repository SHALL provide a parity coverage inventory command that validates local conformance fixture completeness and provider upstream fixture index coverage.

#### Scenario: Fixture artifacts are complete
- **WHEN** a conformance test case has a `config.yaml`
- **THEN** the inventory verifies the test case has `expected.jsonl` and `expected-requests.jsonl`

#### Scenario: Upstream fixture is intentionally missing
- **WHEN** an upstream streaming fixture exists in the local upstream clone but is not imported
- **THEN** the provider `INDEX.yaml` records the fixture as `null`

### Requirement: Expanded conformance configuration
The conformance harness SHALL support parity-sensitive `streamText` and `toUIMessageStream` options needed to reproduce upstream behavior. The supported config SHALL include `toolChoice`, `activeTools`, `streamOptions`, tool `providerOptions`, and tool error simulation.

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
