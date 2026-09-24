## ADDED Requirements

### Requirement: Native route and authentication
The Gateway SHALL own POST `/v1/chat/completions` with native JSON errors, strict one-token Bearer authentication in access-token mode, existing credential-stripped cloud authentication, canonical model resolution and one shared middleware stack.

#### Scenario: Conflicting native credentials
- **WHEN** Authorization is accompanied by X-Access-Token, X-Grafana-Id or a proxy assertion in access-token mode
- **THEN** the request fails with native 401 before body parsing or provider invocation.

#### Scenario: Unsupported native route or method
- **WHEN** a caller uses `/v1/models` or a non-POST Chat method
- **THEN** the Gateway returns native 404 or 405 respectively, including Allow for 405.

### Requirement: Explicit request capability matrix
The adapter SHALL accept only the documented finite text/function/schema/reasoning subset and SHALL reject unsupported fields or backend combinations before model invocation. Responses SHALL receive explicit store false and omitted/null Chat strict SHALL translate to false.

#### Scenario: Defaults differ from Responses
- **WHEN** a Chat request omits store and function strict
- **THEN** the Responses upstream sees store false and function strict false.

#### Scenario: Fallback cannot preserve storage policy
- **WHEN** a configured fallback contains a Responses candidate
- **THEN** native requests are rejected without weakening the shared text-only fallback guard.

### Requirement: Valid native results
Unary and streaming outputs SHALL carry generated opaque IDs, canonical model names, one choice, validated finish reasons and truthful optional usage. Unsupported content SHALL fail safely. Strict schema output SHALL be checked on successful stop, never on length/filter partial output.

#### Scenario: Fragmented function call
- **WHEN** a stream supplies function start, argument deltas, end and a matching final call
- **THEN** the native stream emits one stable tool index, initial ID/name/type once and argument fragments once.

#### Scenario: Invalid or premature stream termination
- **WHEN** a stream closes before finish, exceeds limits or contradicts function fragments
- **THEN** the adapter emits a safe native stream error where writable and does not emit DONE.

### Requirement: Bounded lifecycle
The adapter SHALL enforce request/response/frame/part and total/idle/drain bounds, preserve request cancellation and process shutdown, and avoid resets of idle time for discarded metadata/reasoning. Documentation SHALL distinguish per-request bounds from global admission.

#### Scenario: No visible progress
- **WHEN** a provider continuously emits discarded parts without native progress
- **THEN** the idle deadline still cancels execution.

#### Scenario: Provider returns after cancellation
- **WHEN** a cooperative delayed setup returns a stream after cancellation
- **THEN** a single owner drains it only for the configured bounded interval.

#### Scenario: Claimed stream fails without closing
- **WHEN** setup returns a nonnil stream plus an error or a committed stream fails
- **THEN** response completion does not wait for the drain budget, the provider context is cancelled, and exactly one asynchronous cleanup consumer exits on channel closure or drain expiry.

#### Scenario: Process shutdown with native SSE active
- **WHEN** shutdown begins with a committed native SSE response and unary requests active
- **THEN** their upstream contexts are cancelled, clients terminate, and the real command exits within its shutdown bound.
