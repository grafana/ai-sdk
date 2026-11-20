## MODIFIED Requirements

### Requirement: Observation once per call
Observe SHALL run once per call for each applicable extension that implements it, after the outcome is decided, including denied, failed and cancelled calls. Its record SHALL carry the outcome (success, denied, error, cancelled or invalid output), the denying extension and phase when denied, usage as the logical observers report it, the physical attempts, start time and duration.

#### Scenario: Observation is off the response path
- **WHEN** an observer takes longer than the call itself
- **THEN** a unary result SHALL be returned, and a stream SHALL close, without waiting for the observer

#### Scenario: Denied at admit
- **WHEN** extension `b` denies at admit
- **THEN** every applicable observer SHALL receive one record with outcome denied, extension `b` and phase admit, no usage and no attempts

#### Scenario: Provider error and cancellation
- **WHEN** one call fails with a provider error and another stream is cancelled by the caller
- **THEN** each observer SHALL receive exactly one record per call, with outcome error and cancelled respectively

#### Scenario: In-band error around a finish
- **WHEN** a model stream delivers an error part, before or after its finish
- **THEN** the outcome SHALL be error, also when the call is then cancelled, unless it is denied or its output is invalid

#### Scenario: Caller cancellation ends an unfinished stream
- **WHEN** the model stream closes before the current response segment has finished and the caller's context has already ended
- **THEN** the outcome SHALL be cancelled even if the host observes the close before it handles the cancellation, unless the call is denied, the output is invalid, or the host has observed an in-band error part

#### Scenario: Stream closes unexpectedly
- **WHEN** the host observes the model stream close before the current response segment has finished and the caller's context is still active at that point
- **THEN** the outcome SHALL be error unless the call is denied or its output is invalid

#### Scenario: Observer fails
- **WHEN** observe returns an error or panics
- **THEN** the call's result, error and stream SHALL be unchanged, other observers SHALL still run, and the failure SHALL be counted
