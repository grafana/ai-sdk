# Spec Delta

## MODIFIED Requirements

### Requirement: Request information
Each phase SHALL receive request information: the route namespace, key ownership, selection source, caller type (user, service account or internal service), the identity actor's typed UID when present, correlation ID, the requested model ID as sent, the canonical model ID when the selection has one, and the planned candidates in order. It SHALL NOT carry credentials or other user claims.

#### Scenario: BYOK call
- **WHEN** a service account calls with request BYOK for `openai/gpt-5` with one account
- **THEN** admit SHALL see source `request-byok`, ownership `customer`, caller type service account, its `service-account:<uid>`, the route namespace, the call's correlation ID, requested model `openai/gpt-5`, no canonical model, and one planned candidate with model `openai/gpt-5`, provider `openai`, provider model `gpt-5` and no provider instance

#### Scenario: Alias of a configured model
- **WHEN** an internal service calls a configured model by its alias, and the model has a primary and one fallback
- **THEN** admit SHALL see caller type internal service, no identity UID, the alias as requested model, the canonical ID, and the two configured candidates, each naming the alias, the canonical ID, its provider instance, provider and provider model

#### Scenario: Routed call
- **WHEN** a configured call names fallback models with `models`
- **THEN** the canonical model SHALL be the requested model's canonical ID, and the planned candidates SHALL be the composed chain, each naming the requested model or `models` entry and canonical ID it belongs to

#### Scenario: Request information matches the access decision
- **WHEN** an internal service calls on behalf of a user in `cloud` mode
- **THEN** the caller type SHALL be internal service and the identity UID SHALL be that user's `user:<uid>`, as the access decision recorded

#### Scenario: Own copies
- **WHEN** one extension changes the candidates of its request information
- **THEN** other extensions' request information and the call's execution SHALL be unchanged

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

#### Scenario: Observer fails
- **WHEN** observe returns an error or panics
- **THEN** the call's result, error and stream SHALL be unchanged, other observers SHALL still run, and the failure SHALL be counted

## ADDED Requirements

### Requirement: Physical attempts in the observation record
The record's attempts SHALL be the call's physical attempts, projected as the execution overview projects them: in order, each with provider, provider instance when configured, provider model, outcome (selected, failed or canceled) and status code when known, the i-th attempt being the i-th planned candidate. They SHALL carry no error text, credentials or other provider-originated values.

#### Scenario: Fallback
- **WHEN** a configured call's primary fails with HTTP 503 and its fallback is selected
- **THEN** the record SHALL carry a failed attempt with status 503 for the primary, then a selected attempt for the fallback

#### Scenario: Single candidate
- **WHEN** a configured model with one candidate answers
- **THEN** the record SHALL carry one selected attempt for that candidate

#### Scenario: Output refused after the model answered
- **WHEN** a response inspector denies the output of a single-candidate call
- **THEN** the record SHALL carry one selected attempt, as the overview does

#### Scenario: Refused before the model
- **WHEN** an extension denies at admit or request inspection
- **THEN** the record SHALL carry no attempts

#### Scenario: Stream
- **WHEN** a stream's primary fails before its first part, its fallback streams, the adapter publishes the overview at the first part, and the stream then closes
- **THEN** the record dispatched at stream end SHALL carry both attempts

#### Scenario: Single-candidate stream without output
- **WHEN** a single-candidate stream closes before its first part
- **THEN** the record SHALL carry one failed attempt, as the overview does

#### Scenario: Attempts without attribution
- **WHEN** the observed attempts cannot be attributed to the planned candidates, so the execution overview is omitted
- **THEN** the record SHALL carry no attempts rather than invented ones

### Requirement: Unreplaceable attempt capture
Extensions SHALL NOT be able to replace, wrap or extend the runtime's attempt capture, or observe physical attempts other than through the record. The execution overview and the record SHALL come from the same capture, and each observer SHALL receive its own copy of the attempts.

#### Scenario: Extension derives its own attempt observer
- **WHEN** an extension derives a context with its own fallback attempt observer in a request phase
- **THEN** that observer SHALL receive no attempt of the call, and the overview and records SHALL be unchanged

#### Scenario: Observer changes its attempts
- **WHEN** one observer changes the attempts in its record
- **THEN** other observers' records and the call's overview SHALL be unchanged
