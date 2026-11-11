## MODIFIED Requirements

### Requirement: Constructed streaming limits
The strict ProviderWire V4 handler SHALL require a positive stream idle duration and a positive post-cancellation drain duration. It SHALL NOT take a total model duration, a provider stream-part count or a complete SSE frame byte limit. Complete frames SHALL consist of the `data: ` prefix, JSON payload, and terminating `\n\n`.

#### Scenario: Valid streaming limits
- **WHEN** a caller supplies positive idle and drain durations
- **THEN** handler construction SHALL succeed and runtime streaming behavior SHALL be fixed by those values

#### Scenario: Invalid streaming limits
- **WHEN** a streaming duration is zero or negative
- **THEN** construction SHALL fail before the handler serves a request

### Requirement: Single-owner stream setup and logical commitment

The request context's deadline SHALL cover `DoStream` setup and all later stream consumption. Setup SHALL execute with panic recovery so request cancellation or timeout can end handler latency even if `DoStream` does not return. Every returned stream SHALL have one cleanup owner: an outcome received by the handler belongs to the handler, while setup work SHALL clean up a stream returned after the request has ended.

#### Scenario: Stream setup fails before commitment
- **WHEN** `DoStream` returns an error, panics, returns `nil, nil`, or returns a result with a nil stream channel
- **THEN** the handler SHALL return the corresponding bounded non-2xx safe JSON response and SHALL not commit SSE

#### Scenario: Result and error include a stream
- **WHEN** `DoStream` returns both an error and a non-nil stream channel
- **THEN** the outcome SHALL remain pre-commit, its single owner SHALL cancel and start asynchronous bounded drain exactly once, and the handler SHALL return bounded non-2xx JSON without waiting for drain duration

#### Scenario: Setup completion races termination
- **WHEN** a valid setup outcome and request cancellation or timeout become ready concurrently
- **THEN** either the handler SHALL receive and own the stream or the setup worker SHALL retain and clean up the late stream, with no unowned channel or duplicate cleanup

#### Scenario: Non-nil stream commits streaming mode
- **WHEN** the handler receives a non-nil result and stream channel without error
- **THEN** the response mode SHALL irrevocably become HTTP 200 SSE before the first provider part is interpreted and the handler SHALL own cleanup

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
- **AND** Selection cancellation and late-result cleanup SHALL follow the shared bounded invocation contract.
- **AND** After stream commitment, the configured idle timeout SHALL restart after each accepted provider part is successfully represented, including a consumed start and a written provider error.
- **AND** Request cancellation, request deadline expiry, or idle timeout SHALL cancel selection/provider work before the corresponding safe terminal response or event is written.
- **AND** A valid finish written before another terminal outcome SHALL remain authoritative.
- **AND** When provider output, cancellation, and timeout become ready concurrently before terminal output, any applicable safe bounded outcome MAY win; the protocol does not define scheduler-level precedence.
- **AND** Writer failure SHALL terminate immediately without another write.

#### Scenario: Terminal conditions race
- **WHEN** provider output, request cancellation, the request deadline or the idle timeout become ready concurrently before terminal output
- **THEN** the handler SHALL produce at most one applicable bounded terminal outcome and SHALL not leak or clean up the stream twice

#### Scenario: Selection exhausts streaming time
- **WHEN** the request deadline expires during delayed or non-cooperative selection
- **THEN** handler latency SHALL remain bounded with a pre-commit JSON failure, zero logical model invocations and bounded cleanup of late selection results

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
