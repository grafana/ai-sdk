## ADDED Requirements

### Requirement: Results expose the final recorded step

StreamTextResult and GenerateTextResult SHALL expose `FinalStep() (StepResult, bool)`. It SHALL return the last recorded step with true, or the zero step with false if no step was recorded. Agent results SHALL expose the same accessor. The accessor SHALL NOT change aggregate fields, step recording, or generation errors.

#### Scenario: Multi-step generation

- **WHEN** StreamText, GenerateText, or an Agent records multiple steps
- **THEN** the accessor SHALL return the final entry of the step history by value rather than an all-step projection
- **AND** the stream accessor SHALL wait for result completion without consuming events on behalf of the caller

#### Scenario: Error or cancellation interrupts a stream

- **WHEN** a stream fails or is canceled
- **THEN** the accessor SHALL return the last retained recorded step, or false if none was recorded
- **AND** an interrupted step not present in the recorded history SHALL NOT become the final step

#### Scenario: Assigning fields on the returned step

- **WHEN** a caller reassigns fields on the returned value
- **THEN** the recorded step SHALL be unaffected
- **AND** nested data SHALL remain shared as with the existing step history; callers SHALL copy nested data before modifying it
