# Spec Delta

## MODIFIED Requirements

### Requirement: Constructed streaming limits
The strict ProviderWire V4 handler SHALL use the stream idle and post-cancellation drain durations of the execution runtime it is constructed from, which gateway-execution-runtime requires to be positive. It SHALL NOT take a total model duration, a provider stream-part count or a complete SSE frame byte limit. Complete frames SHALL consist of the `data: ` prefix, JSON payload, and terminating `\n\n`.

#### Scenario: Valid streaming limits
- **WHEN** a handler is constructed from a runtime with positive idle and drain durations
- **THEN** handler construction SHALL succeed and runtime streaming behavior SHALL be fixed by those values

#### Scenario: Invalid streaming limits
- **WHEN** a streaming duration is zero or negative
- **THEN** runtime construction SHALL fail before any handler serves a request

### Requirement: Single-owner stream setup and logical commitment

The request context's deadline SHALL cover `DoStream` setup and all later stream consumption. Setup SHALL run synchronously through the execution runtime with panic recovery. Every returned stream SHALL have one cleanup owner, the runtime: it SHALL cancel and drain exactly once a stream the handler consumed, a stream returned with an error, and a stream returned after the request context ended.

#### Scenario: Stream setup fails before commitment
- **WHEN** `DoStream` returns an error, panics, returns `nil, nil`, or returns a result with a nil stream channel
- **THEN** the handler SHALL return the corresponding non-2xx safe JSON response and SHALL not commit SSE

#### Scenario: Result and error include a stream
- **WHEN** `DoStream` returns both an error and a non-nil stream channel
- **THEN** the outcome SHALL remain pre-commit, the runtime SHALL cancel and start asynchronous bounded drain exactly once, and the handler SHALL return non-2xx JSON without waiting for drain duration

#### Scenario: Setup completion races termination
- **WHEN** `DoStream` returns a valid stream after the request context was canceled or its deadline expired
- **THEN** the handler SHALL return the pre-commit cancellation or timeout JSON failure, and the runtime SHALL cancel and drain the stream exactly once

#### Scenario: Non-nil stream commits streaming mode
- **WHEN** the handler receives a non-nil result and stream channel without error while the request context is live
- **THEN** the response mode SHALL irrevocably become HTTP 200 SSE before the first provider part is interpreted, and the runtime SHALL own cleanup

#### Scenario: Initial provider part never arrives
- **WHEN** setup establishes a stream but no first provider part arrives before cancellation or timeout
- **THEN** the handler SHALL remain committed to SSE and SHALL attempt one empty start followed by one corresponding terminal error frame when the writer remains usable

### Requirement: Cancellation and timeouts
The request context's deadline SHALL cover selection, setup, attempts and streaming without resets, and the handler SHALL add no total deadline of its own. Idle timeout SHALL follow represented provider parts; cancellation and timeout SHALL retain terminal authority, bounded cleanup and writer-failure behavior.

#### Scenario: Streaming deadline and terminal policy
- **WHEN** selection or streaming encounters cancellation, timeout or competing terminal conditions
- **THEN** The request context's deadline SHALL cover selection/construction, logical `DoStream` setup, all credential attempts and stream consumption.
- **AND** Logical invocation, credential attempts and stream establishment SHALL NOT reset or extend the request deadline.
- **AND** Expiry of the request context's deadline SHALL be a timeout, and cancellation of the request context SHALL be a cancellation.
- **AND** Selection and setup SHALL run synchronously under the request context; a selection or stream returned after it ended SHALL be discarded under gateway-execution-runtime.
- **AND** After stream commitment, the configured idle timeout SHALL restart after each accepted provider part is successfully represented, including a consumed start and a written provider error.
- **AND** Request cancellation, request deadline expiry, or idle timeout SHALL cancel selection/provider work before the corresponding safe terminal response or event is written.
- **AND** A valid finish written before another terminal outcome SHALL remain authoritative.
- **AND** When provider output, cancellation, and timeout become ready concurrently before terminal output, any applicable safe bounded outcome MAY win; the protocol does not define scheduler-level precedence.
- **AND** Writer failure SHALL terminate immediately without another write.

#### Scenario: Terminal conditions race
- **WHEN** provider output, request cancellation, the request deadline or the idle timeout become ready concurrently before terminal output
- **THEN** the handler SHALL produce at most one applicable bounded terminal outcome and SHALL not leak or clean up the stream twice

#### Scenario: Selection exhausts streaming time
- **WHEN** selection returns only after the request deadline expired
- **THEN** the handler SHALL return a pre-commit timeout JSON failure with zero logical model invocations

#### Scenario: Attempts inherit the remaining budget
- **WHEN** selection consumes part of the time before the request deadline and the chosen model then tries multiple eligible credentials
- **THEN** every attempt and subsequent stream consumption SHALL use only the remaining time without resetting it

#### Scenario: Total timeout cancels provider before output
- **WHEN** the request deadline expires during setup or stream consumption
- **THEN** the model context SHALL be canceled before the timeout response or event is encoded or written, and that response or event SHALL use the timeout category

#### Scenario: Idle timeout cancels provider before output
- **WHEN** the idle timeout expires after stream establishment
- **THEN** the model context SHALL be canceled before the idle-timeout event is encoded or written

#### Scenario: Provider finish arrives before termination
- **WHEN** a valid finish is written before cancellation or timeout terminates the stream
- **THEN** finish SHALL be the authoritative final event

#### Scenario: Accepted activity resets idle time
- **WHEN** accepted provider parts arrive within each idle interval before the request deadline
- **THEN** the idle timeout SHALL restart after each represented part and SHALL not expire solely because total stream age exceeds one idle interval

#### Scenario: Stream without a request deadline
- **WHEN** the request context carries no deadline and the provider keeps emitting parts within the idle duration
- **THEN** the stream SHALL continue until finish, caller cancellation or an idle timeout

### Requirement: Bounded stream ownership and drain

Every returned stream SHALL have one cleanup owner, the execution runtime. Terminal handling SHALL cancel provider work and start one asynchronous drain that reads only until channel close or the configured drain duration.

#### Scenario: Provider closes after cancellation
- **WHEN** terminal handling cancels provider work and the provider closes its stream within the drain duration
- **THEN** the bounded drain SHALL consume the remaining channel values and exit

#### Scenario: Provider ignores cancellation
- **WHEN** a provider channel does not close before the drain duration
- **THEN** handler latency and the drain lifetime SHALL remain bounded even though provider-owned work may remain defective

#### Scenario: Continuously ready drain reaches a hard bound
- **WHEN** a canceled provider channel remains permanently ready with values
- **THEN** the drain SHALL stop within the configured duration rather than consuming indefinitely

#### Scenario: Setup returns a late stream
- **WHEN** `DoStream` returns a non-nil stream after the request context ended
- **THEN** the runtime SHALL cancel and bounded-drain that channel exactly once instead of abandoning it unread

### Requirement: Bounded drain latency and provider lifecycle responsibility

Draining SHALL not extend handler latency and SHALL remain bounded even when the provider channel is continuously ready. Providers SHALL remain responsible for observing cancellation, returning from setup and closing their channels; a provider call that never returns holds its own request, and a producer that remains blocked after bounded drain SHALL be classified as a provider lifecycle defect rather than a bounded Gateway resource.

#### Scenario: Bounded drain latency and provider lifecycle responsibility
- **WHEN** a canceled provider continuously produces parts or never returns
- **THEN** Gateway drain latency SHALL remain bounded, a setup call that never returns SHALL hold only its own request, and non-cooperative provider-owned work SHALL be classified as a lifecycle defect
