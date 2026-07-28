## ADDED Requirements

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
