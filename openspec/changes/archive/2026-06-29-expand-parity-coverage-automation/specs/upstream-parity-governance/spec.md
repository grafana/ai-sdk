## ADDED Requirements

### Requirement: Baseline validation covers all parity TypeScript consumers
The repository SHALL validate that every test package consuming `ai` or `@ai-sdk/*` packages uses versions compatible with the registered upstream parity baseline.

#### Scenario: Integration package consumes upstream AI SDK packages
- **WHEN** `test/integration/package.json` declares `ai` or `@ai-sdk/*` dependencies
- **THEN** baseline validation verifies those dependency pins match `test/conformance/upstream.yaml`

#### Scenario: CLI package consumes upstream AI SDK packages
- **WHEN** `test/cli/package.json` declares `ai` or `@ai-sdk/*` dependencies
- **THEN** baseline validation verifies those dependency pins match `test/conformance/upstream.yaml`

### Requirement: Provider API-shape drift report
The repository SHALL provide a provider V4 API-shape drift report that compares upstream LanguageModelV4 discriminator values with Go provider constants.

#### Scenario: Upstream adds a stream part type
- **WHEN** upstream LanguageModelV4 declares a stream part discriminator missing from Go provider constants
- **THEN** the drift report identifies the missing value as parity review input

### Requirement: Frontend hook interop coverage
The repository SHALL include hook-level frontend interop tests for upstream AI SDK UI consumers.

#### Scenario: React chat hook consumes Go UI stream
- **WHEN** integration tests run
- **THEN** a `useChat`-level test verifies the Go SSE stream can be consumed through the upstream React hook surface

#### Scenario: React object and completion surfaces consume Go streams
- **WHEN** integration tests run
- **THEN** hook-level or equivalent upstream UI tests cover object and completion stream consumption
