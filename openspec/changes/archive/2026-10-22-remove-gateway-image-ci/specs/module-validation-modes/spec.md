## MODIFIED Requirements

### Requirement: No premature source-gate switch

Boundary, parity, integration and merged-pin checks SHALL remain blocking candidate-source checks. All-module standalone compilation SHALL NOT block an otherwise valid source PR. Source checks SHALL NOT require a Gateway image build or publisher. Independent library publication SHALL retain its separate standalone validation and authorization requirements.

#### Scenario: Source PR with incomplete standalone dependencies
- **WHEN** a source PR passes required candidate-source checks but cannot build against its older merged published pins
- **THEN** it SHALL remain mergeable without Gateway image validation or native smoke
- **AND** source success SHALL NOT authorize independent library publication
