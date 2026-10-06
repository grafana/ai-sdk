## MODIFIED Requirements

### Requirement: Runnable examples are external and linked

Complete, runnable example programs SHALL live under a top-level `/examples` Go directory as self-contained modules. The collection SHALL be curated around recognizable application outcomes rather than providing one runnable directory for every individual API call. Each example SHALL compile via `go build`, SHALL provide deterministic credential-free behavioral tests, and those tests SHALL run in blocking CI. `docs/` pages MAY include short illustrative snippets but SHALL link to the full program in `/examples` for non-trivial end-to-end scenarios rather than embedding the complete program.

#### Scenario: Example programs are buildable

- **WHEN** an example program is added or changed under `/examples`
- **THEN** it SHALL compile via the repository's example build task
- **AND** the example build task SHALL run in blocking CI

#### Scenario: Example behavior is tested

- **WHEN** an example module is added or changed
- **THEN** it SHALL include deterministic tests for its application-visible success path and important boundary failures
- **AND** the tests SHALL NOT require provider credentials or external network access
- **AND** the repository's example test task SHALL discover and execute the module in blocking CI

#### Scenario: Frontend example behavior is checked across languages

- **WHEN** an example demonstrates integration with an upstream frontend hook
- **THEN** the cross-language integration suite SHALL exercise the representative stream through the registered upstream frontend package version
- **AND** it SHALL assert the frontend-visible state central to the example

#### Scenario: Examples represent application outcomes

- **WHEN** the runnable example collection is reviewed
- **THEN** each top-level example SHALL correspond to a distinct application outcome or integration boundary
- **AND** focused API techniques that do not require a complete application SHALL remain in the README, guides, or godoc rather than requiring another runnable module

#### Scenario: Guides link to runnable code

- **WHEN** a guide demonstrates a non-trivial end-to-end scenario
- **THEN** it SHALL link to the corresponding runnable program under `/examples` rather than embedding the complete program inline
