# fallback-attempt-capture Specification

## Purpose

Define reusable request-scoped fallback decision observation, candidate-local source errors and independent observer delivery without changing selection or lifecycle policy.

## Requirements

### Requirement: Request-scoped fallback observation

The SDK SHALL allow a caller to register an attempt observer in its context without mutating a shared fallback model. The nearest registration SHALL replace an inherited request observer, including allowing a nil callback to disable it. Independent request contexts SHALL NOT share callback registrations. Callbacks SHALL remain synchronous and SHALL be documented to return promptly. Candidate indices SHALL retain their existing invocation-local meaning; registration SHALL NOT introduce automatic trace grouping or metadata serialization.

#### Scenario: Shared model with independent requests
- **WHEN** concurrent callers use one fallback model with different request-scoped observers
- **THEN** each observer SHALL receive only decisions made using its own context

#### Scenario: Replaced or disabled inherited observer
- **WHEN** a child context registers a different observer or a nil observer
- **THEN** only that child registration SHALL apply to request observation without changing the model observer

### Requirement: Candidate source errors remain separate from decision errors

Every observed attempt SHALL preserve the candidate setup, validation or first-part-wait error owned by fallback before decision-time cancellation substitution as `SourceErr`. The existing `Err`, outcome, fallback intent, decider inputs and returned errors SHALL remain unchanged. Selected error parts SHALL remain stream content rather than pre-selection failure summaries. Unowned late setup results SHALL NOT be captured.

#### Scenario: Cancellation during decision
- **WHEN** a candidate fails and the decider cancels the request
- **THEN** `SourceErr` SHALL preserve that candidate failure while `Err` and the returned error SHALL preserve the context cause and no later candidate SHALL be invoked

#### Scenario: Selected leading error part
- **WHEN** a candidate's first accepted part is an error part
- **THEN** the attempt SHALL remain selected with no setup source error and the original part SHALL remain ordered stream content

#### Scenario: Cancellation owns setup
- **WHEN** request cancellation wins setup ownership and the provider later returns a native failure
- **THEN** observation SHALL retain the owned context error and SHALL NOT be rewritten by the late native result

### Requirement: Independent observer delivery

The request observer SHALL run before the model observer at the existing decision boundary. Both SHALL receive the same decision snapshot. A panic in either SHALL NOT suppress the other or change fallback selection, returned errors, first-part commitment or cleanup ownership.

#### Scenario: Either observer panics
- **WHEN** either observer panics while receiving a decision
- **THEN** the other observer SHALL still receive that decision and fallback SHALL preserve its result and candidate ordering
