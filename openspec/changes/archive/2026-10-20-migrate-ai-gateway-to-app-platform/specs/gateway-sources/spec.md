# Spec Delta

## MODIFIED Requirements

### Requirement: Source transport provenance and delivery evidence separation
Synthetic transports SHALL NOT be presented as provider recordings. Published dependency, candidate-workspace and plugin package evidence SHALL be reported separately.

#### Scenario: Source transport provenance and delivery evidence separation
- **WHEN** source acceptance uses a synthetic transport and candidate workspace build
- **THEN** the report SHALL NOT label the transport as a recording and SHALL separate published-dependency, candidate-workspace and plugin package evidence
