## MODIFIED Requirements

### Requirement: Constructed language-model handler

The `ai-gateway/providerwire/v4` package SHALL provide one HTTP handler for relative `POST /language-model` unary and streaming requests. Construction SHALL require a non-nil host-owned request selector independent of catalog interfaces and positive limits for stream idle duration and bounded post-cancellation drain duration. Construction SHALL NOT accept a total model duration, or request, response, frame or stream-part size limits.

#### Scenario: Valid construction
- **WHEN** a caller supplies a request selector and positive duration limits
- **THEN** construction SHALL return an immutable handler

#### Scenario: Invalid construction
- **WHEN** the selector is nil or a duration limit is non-positive
- **THEN** construction SHALL fail before serving traffic

### Requirement: Resolution and bounded model invocation
The handler SHALL select and invoke one logical model under authenticated request policy. Configured access SHALL resolve its catalog once; BYOK SHALL use request accounts without a catalog. Both SHALL retain logical identity and run selection and execution under the request context's deadline.

#### Scenario: Selection and shared execution policy
- **WHEN** a supported request enters host selection and invocation
- **THEN** For a supported request, the handler SHALL select execution once under authenticated request policy and invoke the resulting logical model once.
- **AND** Configured selection SHALL resolve the exact catalog ID/alias; BYOK selection SHALL use only request provider/model/credentials.
- **AND** Selection SHALL return a nonempty valid-UTF-8 logical identity and a non-nil V4 model.
- **AND** Logical identity SHALL NOT substitute for native response identity, including when a native modelId equals it.
- **AND** The request context's deadline, set by the host, SHALL be the one execution deadline. It SHALL cover selection/construction, logical invocation and all underlying credential attempts, and streaming consumption SHALL use the same deadline.
- **AND** The handler SHALL NOT add a deadline of its own, and selection, invocation and attempt boundaries SHALL NOT reset or extend the request deadline.
- **AND** When the request context's deadline expires, the handler SHALL classify the outcome as a timeout; when the request context is canceled, it SHALL classify it as a cancellation.
- **AND** Selection and invocation SHALL retain context cancellation, panic containment, buffered completion and bounded handler latency when work ignores cancellation.
- **AND** Late selection completion SHALL NOT invoke a model and SHALL retain bounded request-owned cleanup.
- **AND** A permanently blocked selection or native function may retain its worker; no exactly-once or guaranteed-zeroization claim SHALL be made.

#### Scenario: Supported execution
- **WHEN** configured selection returns a valid model
- **THEN** catalog resolution and logical DoGenerate SHALL each run once

#### Scenario: BYOK execution
- **WHEN** BYOK selection returns a valid credential-attempt model
- **THEN** the logical invocation SHALL run once, the catalog SHALL not run and bounded physical attempts SHALL follow the BYOK contract

#### Scenario: Invalid resolution
- **WHEN** selection fails, panics or returns invalid identity/model values
- **THEN** the handler SHALL return a safe error without invoking an invalid model

#### Scenario: Cancellation or timeout
- **WHEN** cancellation or the request deadline becomes observable during selection or any attempt
- **THEN** handler latency SHALL remain bounded and no later credential attempt SHALL begin

#### Scenario: Request deadline is a timeout
- **WHEN** the request context's deadline expires during a unary attempt
- **THEN** the handler SHALL return the fixed timeout error, not the cancellation error

#### Scenario: Selector consumes the execution budget
- **WHEN** selection ignores cancellation or completes only after the request deadline expires
- **THEN** unary and streaming handler latency SHALL remain bounded, with a pre-commit JSON failure, zero model invocations and bounded cleanup of any late selection result

#### Scenario: Selection leaves only a partial budget
- **WHEN** selection consumes part of the time before the request deadline and returns a credential-attempt model
- **THEN** logical invocation, every credential attempt and streaming consumption SHALL receive only the remaining time, without a fresh deadline

#### Scenario: Request without a deadline
- **WHEN** the request context carries no deadline
- **THEN** the handler SHALL run selection and invocation without a deadline of its own, and caller cancellation SHALL still end them
