## ADDED Requirements

### Requirement: Step finish reason follows the last terminal part

Within one model call, each provider error part SHALL set the step's unified finish reason to `error` and keep the raw finish reason of any earlier finish. Each finish part SHALL set both the unified and raw finish reasons. The recorded step, the step-finish and stream-finish parts and the UI finish chunk SHALL carry the reason set by the last terminal part.

#### Scenario: Error after a finish

- **WHEN** a provider stream emits text, a finish with reason `stop` and raw reason `end_turn`, then an error, and closes
- **THEN** the step finish reason SHALL be `error` with raw reason `end_turn`
- **AND** the UI finish chunk SHALL report `error`
- **AND** the text SHALL be retained and the error surfaced

#### Scenario: Finish after an error after a finish

- **WHEN** a provider stream emits a finish, an error and then another finish with reason `stop`
- **THEN** the step finish reason SHALL be the last finish's reason

#### Scenario: Finish after an error

- **WHEN** a provider stream emits an error and then a finish with reason `stop`
- **THEN** the step finish reason SHALL be `stop`
