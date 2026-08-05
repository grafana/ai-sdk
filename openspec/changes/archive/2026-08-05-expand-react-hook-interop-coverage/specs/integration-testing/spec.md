## ADDED Requirements

### Requirement: Controlled and abortable integration streams
The Go integration test server SHALL provide deterministic request-scoped stream scenarios that expose an observable partial response and honor request cancellation. Controlled stream handlers and mock-model producers MUST stop blocked work when the request context is canceled and MUST NOT depend on mutable state shared between tests.

#### Scenario: Controlled UI stream exposes lifecycle phases
- **WHEN** a React chat hook requests the controlled UI-stream scenario
- **THEN** the server flushes an initial response phase before holding the stream open
- **AND** the server closes the stream in a separate bounded phase when it has not been canceled

#### Scenario: Controlled text stream exposes partial completion
- **WHEN** a React completion hook requests the controlled text-stream scenario
- **THEN** the server flushes a deterministic partial completion before holding the response open
- **AND** content scheduled after the hold is not part of the partial completion

#### Scenario: Cancellation releases server work
- **WHEN** a hook aborts a controlled stream after observing its partial response
- **THEN** the handler and any mock-model producer stop waiting when the request context is canceled
- **AND** the scenario does not require a global controller or cross-request execution flag

### Requirement: React hook lifecycle and failure integration coverage
The integration suite SHALL exercise Go HTTP responses through the public hooks from the registered `@ai-sdk/react` package and assert intermediate state, terminal state, callback arguments, and retained partial data for the covered lifecycle and failure contracts.

#### Scenario: Chat status ordering succeeds
- **WHEN** `useChat` consumes a successful phased Go UI stream
- **THEN** its observed status history contains `submitted`, `streaming`, and `ready` in that order

#### Scenario: Chat HTTP and stream errors surface through the hook
- **WHEN** `useChat` receives either a non-success HTTP response or a Go UI stream error chunk
- **THEN** the hook exposes the expected error
- **AND** its terminal status is `error`

#### Scenario: Stopped chat retains partial output
- **WHEN** `stop()` is called after `useChat` has rendered partial assistant text
- **THEN** the hook returns to `ready`
- **AND** the partial text remains present without later server text

#### Scenario: Chat history exposes a step boundary
- **WHEN** `useChat` consumes the deterministic multi-step tool scenario
- **THEN** immutable message snapshots expose the completed first-step state before the next-step output
- **AND** the final message preserves the expected tool output and final text

#### Scenario: Approved tool response resumes chat
- **WHEN** a pending tool part is approved through `addToolApprovalResponse` and automatically resubmitted
- **THEN** message history transitions that part from `approval-requested` to `approval-responded`
- **AND** the response preserves `approved: true` and its reason
- **AND** the resumed Go flow produces the expected `output-available` tool state

#### Scenario: Denied tool response resumes without execution
- **WHEN** a pending tool part is denied through `addToolApprovalResponse` and automatically resubmitted
- **THEN** message history transitions that part from `approval-requested` to `approval-responded`
- **AND** the response preserves `approved: false` and its reason
- **AND** the resumed Go flow exposes `output-denied` without a successful tool output

#### Scenario: Completion error resets lifecycle state
- **WHEN** `useCompletion` receives a non-success HTTP response
- **THEN** `onError` is called exactly once with the expected `Error`
- **AND** `onFinish` is not called
- **AND** the hook exposes the error and resets `isLoading` to false

#### Scenario: Stopped completion retains partial output
- **WHEN** `stop()` is called after `useCompletion` has rendered a partial completion
- **THEN** the abort does not invoke `onError`
- **AND** `isLoading` becomes false
- **AND** the partial completion remains present without later server text

#### Scenario: Final object fails schema validation
- **WHEN** `useObject` finishes consuming valid JSON that does not match its configured schema
- **THEN** `onFinish` is called exactly once with `object: undefined`
- **AND** the same callback result contains an `Error`
