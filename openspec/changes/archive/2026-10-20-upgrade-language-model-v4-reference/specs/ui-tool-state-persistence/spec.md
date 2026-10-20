# Spec Delta

## ADDED Requirements

### Requirement: Unavailable static-tool history uses an inert successful-output placeholder

Agent normalization SHALL mark terminal static-tool parts whose tool is unavailable without changing persisted public data. Model-message conversion of a successfully completed marked part without a configured tool SHALL replace its output with the registered unavailable-tool placeholder. Arbitrary dynamic parts and failed outputs SHALL retain their existing conversion.

#### Scenario: Removed tool has successful persisted output
- **WHEN** Agent history contains an output-available static tool that is no longer configured
- **THEN** copied UI history SHALL normalize to a dynamic part while model history uses `Tool output omitted because the tool is no longer available.`
- **AND** original caller history SHALL remain unchanged
