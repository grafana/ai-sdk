## MODIFIED Requirements

### Requirement: React hook lifecycle and failure integration coverage
The integration suite SHALL exercise Go HTTP responses through the public hooks from the registered `@ai-sdk/react` package and assert intermediate state, terminal state, callback arguments, message identity and retained partial data for the covered lifecycle and failure contracts. Reconnect coverage SHALL use deterministic test transport responses and MUST NOT claim to validate durable server-side stream recovery.

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

#### Scenario: Chat regeneration replaces assistant response
- **WHEN** `useChat` regenerates a completed assistant response through the Go testserver
- **THEN** message snapshots retain the relevant user message ID while replacing the earlier assistant response with the new response and observable assistant ID
- **AND** the hook reaches its pinned terminal status with the expected finish callback arguments

#### Scenario: Chat reconnect sees a deterministic available stream
- **WHEN** `useChat.resumeStream()` reconnects to a test-only Go GET transport returning UI SSE for a fixed chat ID and known message IDs
- **THEN** the hook shows the pinned submitted/streaming/ready sequence and the response's observable message identity and content
- **AND** the test makes no claim that the server stored or recovered an earlier response

#### Scenario: Chat reconnect finds no stream or fails
- **WHEN** the test transport returns 204 or a non-success HTTP response to `resumeStream()`
- **THEN** the hook follows the pinned no-stream ready path without a spurious submitted transition, or the error path with one matching `onError` callback, respectively
- **AND** unrelated messages retain their identity

#### Scenario: Chat abort and error callbacks are distinguishable
- **WHEN** a partial chat response is stopped or an HTTP/UI stream response fails
- **THEN** callback history records the pinned `onFinish` abort/error flags and the expected `onError` counts and values
- **AND** the retained message/status state is asserted for each path

#### Scenario: Chat metadata and data updates reach the hook
- **WHEN** `useChat` consumes Go SSE with updated assistant message metadata and named data parts including transient data
- **THEN** snapshots expose the expected metadata and retained parts in order; `onData` receives both retained and transient payloads, but transient data does not enter message parts
- **AND** the same stream parses through the registered `uiMessageChunkSchema` and assembles with the expected message fields

#### Scenario: Overlapping completions retain observable request ownership
- **WHEN** two distinguishable `useCompletion.complete()` calls overlap and the earlier request completes or fails while the later one is active
- **THEN** an earlier request's `onFinish` still receives its prompt and result, or its distinguishable failure still reaches `onError`, even when a newer request is active
- **AND** the earlier request's settlement cannot overwrite the newer displayed completion or error state or clear its active loading state; assertions identify each callback's request by prompt or distinguishable error

#### Scenario: Object HTTP failure reports an error
- **WHEN** `useObject` receives a Go non-success HTTP response
- **THEN** it exposes the expected `Error`, resets loading, calls `onError` once and does not call `onFinish`

#### Scenario: Object body read failure retains the partial value
- **WHEN** `useObject` reads valid partial JSON from a Go text response and the response body then fails during reading
- **THEN** the hook retains the observed partial object, resets loading, invokes `onError` with a transport error and does not invoke `onFinish`

#### Scenario: Stopped object retains partial value
- **WHEN** `stop()` is called after `useObject` renders a partial object from an abortable Go stream
- **THEN** loading clears and the partial object remains without later updates
- **AND** neither `onError` nor `onFinish` is invoked for the stopped stream

#### Scenario: Valid final object finishes successfully
- **WHEN** `useObject` completes a valid Go text response
- **THEN** `onFinish` receives the validated object once without an error and `onError` is not called
