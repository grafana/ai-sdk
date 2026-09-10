## MODIFIED Requirements

### Requirement: Baseline validation covers all parity TypeScript consumers
The repository SHALL validate that every retained test package consuming `ai` or `@ai-sdk/*` packages uses versions compatible with the registered upstream parity baseline.

#### Scenario: Integration package consumes upstream AI SDK packages
- **WHEN** `test/integration/package.json` declares `ai` or `@ai-sdk/*` dependencies
- **THEN** baseline validation verifies those dependency pins match `test/conformance/upstream.yaml`

#### Scenario: CLI package consumes upstream AI SDK packages
- **WHEN** `test/cli/package.json` declares `ai` or `@ai-sdk/*` dependencies
- **THEN** baseline validation verifies those dependency pins match `test/conformance/upstream.yaml`

#### Scenario: Conformance tools consume upstream AI SDK packages
- **WHEN** `test/conformance/tools/package.json` declares `ai` or `@ai-sdk/*` dependencies
- **THEN** baseline validation verifies those dependency pins match `test/conformance/upstream.yaml`

#### Scenario: Parity upgrade updates every retained consumer
- **WHEN** the registered package set is upgraded
- **THEN** conformance, integration, and CLI package manifests that consume a tracked package are updated together
