## MODIFIED Requirements

### Requirement: First-chunk error detection in DoStream
`fallback.Model.DoStream` SHALL wait for each candidate's first stream result before returning. A synchronous setup error, invalid nil result/channel, or channel close before any part SHALL remain a pre-commit candidate failure and SHALL be evaluated by the fallback decider. Receipt of any `provider.StreamPart`, including `PartError`, SHALL irrevocably select that candidate and SHALL NOT be evaluated by the decider.

#### Scenario: Primary streams PartError, secondary succeeds
- **WHEN** the primary candidate's first part is `PartError` and a secondary candidate would succeed if invoked
- **THEN** fallback SHALL return the primary stream beginning with that exact error part and SHALL NOT invoke the secondary candidate

#### Scenario: Primary streams PartError, decider rejects fallback
- **WHEN** the primary candidate's first part is `PartError` and the configured decider would reject its error
- **THEN** fallback SHALL return the primary stream without invoking the decider or any further candidate

#### Scenario: All candidates stream PartError
- **WHEN** every configured candidate would emit `PartError` as its first part
- **THEN** fallback SHALL select only the first candidate and return its error part without invoking later candidates

#### Scenario: Primary streams PartError
- **WHEN** the primary candidate returns a valid stream whose first part is `PartError`
- **THEN** fallback SHALL return a stream beginning with that exact part and SHALL NOT invoke the secondary candidate

#### Scenario: Primary ends before its first part
- **WHEN** the primary candidate returns a stream channel that closes before yielding any part
- **THEN** fallback SHALL classify the premature end as a pre-commit error and SHALL invoke the secondary candidate only when the decider accepts it and the request context remains live

#### Scenario: Setup returns an invalid stream
- **WHEN** a candidate returns nil without error, a nil result, or a result with a nil stream channel
- **THEN** fallback SHALL treat the outcome as a pre-commit failure rather than panic or return a successful stream

### Requirement: Valid first chunk replay
After a candidate yields its first part, `fallback.Model.DoStream` SHALL return a stream that emits the buffered part exactly once followed by all later parts from that candidate in original order. It SHALL never combine parts from different candidates.

#### Scenario: Primary succeeds with valid first chunk
- **WHEN** the primary candidate's stream emits a valid `PartTextDelta` as its first part
- **THEN** fallback SHALL return that part exactly once followed by all subsequent parts in original order, without invoking a secondary candidate

#### Scenario: Primary stream is empty
- **WHEN** the primary candidate returns a stream channel that closes before yielding any part
- **THEN** fallback SHALL treat the premature end as a pre-commit failure rather than successful empty output, and SHALL invoke the next candidate only if the decider accepts and the request context remains live

#### Scenario: Primary yields normal text parts
- **WHEN** the primary stream yields a text start followed by deltas and finish
- **THEN** the returned stream SHALL yield every part exactly once in original order and no secondary SHALL be invoked

#### Scenario: Provider errors remain ordered and non-terminal
- **WHEN** a selected candidate yields one or more `PartError` values followed by valid later parts
- **THEN** the returned stream SHALL preserve every error and later part in order without changing candidates or prematurely closing the stream

### Requirement: Failed stream cleanup
Fallback SHALL create a cancelable context per physical stream attempt. When an attempt is abandoned before commitment, it SHALL cancel that context immediately and perform only asynchronous cleanup bounded by a positive package-owned duration and part budget. The successful relay SHALL select on cancellation for every send and receive. Cleanup or relay SHALL close only fallback-owned output channels and SHALL not require an uncooperative provider channel to close.

#### Scenario: Stream drained after PartError
- **WHEN** a selected candidate emits `PartError` followed by additional parts before closing
- **THEN** fallback SHALL relay those parts to the caller in order rather than discard them as failed-attempt cleanup, and SHALL cancel the selected candidate if the caller cancels

#### Scenario: Abandoned producer cooperates
- **WHEN** a pre-commit candidate is abandoned and its producer closes after observing cancellation
- **THEN** fallback SHALL consume available cleanup parts within the bound and every fallback-owned cleanup goroutine SHALL exit

#### Scenario: Abandoned producer ignores cancellation
- **WHEN** an abandoned provider channel remains silent or continuously ready beyond the cleanup bound
- **THEN** every fallback-owned cleanup goroutine SHALL exit within the time or part bound without delaying the next candidate or caller response

#### Scenario: Downstream consumer stops reading
- **WHEN** the returned stream consumer cancels its context while the relay is blocked sending a part
- **THEN** the relay SHALL observe cancellation, cancel the selected candidate context, close its output, and exit

### Requirement: Decider applied to stream errors
The configured decider SHALL run exactly once for each synchronous setup error, invalid stream result, or premature EOF while the request context is live. It SHALL NOT inspect or decide on a received provider `PartError`. Fallback SHALL invoke a later candidate only when the decider returns true and a later candidate exists.

#### Scenario: Stream error with context length message
- **WHEN** a candidate's first part is `PartError` whose error message contains "context length"
- **THEN** fallback SHALL select that candidate, relay the error part, and SHALL NOT invoke the decider or a later candidate

#### Scenario: Stream error with generic message
- **WHEN** a candidate's first part is `PartError` whose error message is "model not found"
- **THEN** fallback SHALL select that candidate, relay the error part, and SHALL NOT invoke the decider or a later candidate

#### Scenario: Retryable setup API error
- **WHEN** a candidate's `DoStream` returns a retryable `APICallError` before exposing a stream
- **THEN** the default decider SHALL allow the next configured candidate

#### Scenario: Non-retryable setup API error
- **WHEN** a candidate's `DoStream` returns a non-retryable `APICallError`
- **THEN** fallback SHALL return the accumulated failure without invoking a later candidate

#### Scenario: Leading retryable PartError
- **WHEN** a candidate stream's first value is `PartError` with `IsRetryable` true
- **THEN** the part SHALL commit that candidate and the decider SHALL not be called

### Requirement: Context cancellation during stream peek
If the request context ends while fallback waits for a first part, fallback SHALL cancel the current candidate, SHALL NOT invoke a later candidate, and SHALL return the context error or the accumulated failure preserving that cause. Cleanup SHALL remain asynchronous and bounded.

#### Scenario: Context cancelled during first chunk read
- **WHEN** the request context is cancelled while `DoStream` waits for a candidate's first part
- **THEN** fallback SHALL promptly return an error preserving the context cause, cancel the candidate, and SHALL NOT invoke another candidate

#### Scenario: Context canceled during first-part wait
- **WHEN** the request context is canceled while `DoStream` waits for a candidate's first part
- **THEN** fallback SHALL return promptly without invoking another candidate and every fallback-owned cleanup goroutine SHALL terminate within its bound

#### Scenario: Deadline and candidate result become ready together
- **WHEN** request expiry and a candidate result or first part are concurrently observable
- **THEN** one synchronization decision SHALL give the outcome a single owner and SHALL not duplicate observation, candidate invocation, relay, or cleanup

## ADDED Requirements

### Requirement: Closed physical attempt decisions
The reusable fallback hook SHALL emit exactly one decision event for every candidate invocation. The event SHALL preserve one-based candidate index, provider and backend model identity, start and decision timestamps, optional error, an outcome from the closed set selected/failed/canceled, and `WillFallback` as the live decision-time intent to advance. This field SHALL be true only when the failed attempt is eligible, another candidate exists, and the request context is live in the snapshot immediately after the decider returns. Later cancellation MAY prevent that invocation without rewriting the event; subsequent attempt records SHALL establish which candidates were actually invoked. Retry SHALL NOT be a separate outcome. A streamed candidate SHALL be selected at its first received part rather than at stream completion.

Cancellation observable in that post-decider snapshot SHALL normalize the event outcome to canceled, its error and the returned error to the context cause, and `WillFallback` to false, regardless of the decider's answer. A failure entered while the request is live SHALL still receive exactly one decider invocation, including on the final candidate. Cancellation after a failed decision to advance and before the next invocation SHALL stop progression and remain in the returned error chain together with earlier failures, for unary and streaming calls.

#### Scenario: Unary primary fails and secondary wins
- **WHEN** a retry-eligible unary primary fails and the secondary succeeds
- **THEN** the hook SHALL receive one failed primary event with fallback selected and one selected secondary event in invocation order

#### Scenario: Stream commits on an error part
- **WHEN** a candidate's first part is `PartError`
- **THEN** the hook SHALL receive one selected event for that candidate with no fallback decision

#### Scenario: Final candidate fails
- **WHEN** the final candidate fails before commitment with an otherwise retry-eligible error
- **THEN** its event SHALL report failed with `WillFallback=false` because no later candidate exists

#### Scenario: Observer cancels after an eligible failure
- **WHEN** a unary or streaming primary fails with a live request and the synchronous observer cancels the request after receiving its failed event with `WillFallback=true`
- **THEN** fallback SHALL retain that decision-time event, SHALL NOT invoke another candidate, and SHALL return an error preserving both the failure and cancellation cause

#### Scenario: Request ends during the decider
- **WHEN** a decider cancels the request or a deadline expires while the decider is running, and the decider then returns either true or false
- **THEN** fallback SHALL emit one canceled event preserving the context cause with `WillFallback=false`, SHALL return that cause, and SHALL NOT invoke another candidate

### Requirement: Observer isolation contract
Fallback SHALL preserve model selection and returned errors if an observer panics. Observer callbacks SHALL be invoked synchronously at decision boundaries and SHALL be documented to return promptly; fallback SHALL NOT create an unbounded goroutine to isolate a blocking observer.

#### Scenario: Observer panics
- **WHEN** an attempt observer panics while receiving a decision
- **THEN** fallback SHALL recover the panic and continue with the same candidate selection and model result that would have occurred without the observer
