## ADDED Requirements

### Requirement: ProviderWire V4 contract is a registered parity consumer

The repository SHALL register `test/providerwire-v4` as a parity TypeScript consumer governed by `test/conformance/upstream.yaml`. Baseline validation SHALL compare every `ai` and `@ai-sdk/*` dependency in that workspace with the manifest. The standard parity check SHALL run the workspace's non-mutating compile-time surface, production-schema, semantic-golden, and registered-client consumption checks. The parity coverage map SHALL classify this evidence separately from provider conformance, frontend hook state-machine coverage, and future Go ProviderWire runtime coverage.

#### Scenario: ProviderWire workspace matches the baseline
- **WHEN** `test/providerwire-v4/package.json` pins the registered AI SDK package versions
- **THEN** baseline validation SHALL pass for that consumer

#### Scenario: ProviderWire workspace drifts from the baseline
- **WHEN** an `ai` or `@ai-sdk/*` dependency in `test/providerwire-v4/package.json` differs from or is absent in `test/conformance/upstream.yaml`
- **THEN** baseline validation SHALL fail and identify the workspace, package, declared version, and baseline version or omission

#### Scenario: Standard parity check includes ProviderWire contract evidence
- **WHEN** a contributor runs the repository parity check
- **THEN** it SHALL typecheck the exhaustive finite surface witnesses
- **AND** it SHALL compile and test the production request schema
- **AND** it SHALL compare in-memory real-client request captures with committed semantic goldens
- **AND** it SHALL run focused unary, SSE, and non-2xx registered-client consumption probes
- **AND** none of those checks SHALL rewrite tracked files

#### Scenario: Baseline upgrade includes the ProviderWire consumer
- **WHEN** the registered upstream package set is upgraded
- **THEN** the ProviderWire workspace dependency pins SHALL be updated with every other parity consumer
- **AND** compile-time drift, schema drift, request-golden drift, and client-consumption drift SHALL be reviewed before the upgrade is complete
- **AND** request goldens SHALL change only through the explicit ProviderWire golden update workflow

#### Scenario: Coverage map records the evidence boundary
- **WHEN** ProviderWire V4 contract coverage is added or changed
- **THEN** `test/conformance/PARITY.md` SHALL identify the registered-client HTTP projection and consumption behavior that is automated
- **AND** it SHALL state that strict Go request replay, server response correctness, runtime lifecycle, privacy, and resource bounds are not established by this contract workspace
