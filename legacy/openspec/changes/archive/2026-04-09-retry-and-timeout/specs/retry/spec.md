## ADDED Requirements

### Requirement: MaxRetries option
The system SHALL accept a `MaxRetries` option on both `StreamText` and `GenerateText` that controls how many retry attempts are made after the initial call fails. The default value SHALL be 2 (up to 3 total attempts). A value of 0 SHALL disable retry entirely.

#### Scenario: Default retry behavior
- **WHEN** `StreamText` is called without specifying `MaxRetries`
- **THEN** the system SHALL retry retryable errors up to 2 times (3 total attempts)

#### Scenario: Retry disabled
- **WHEN** `MaxRetries` is set to 0
- **THEN** the system SHALL NOT retry any errors and SHALL return the original error unwrapped

#### Scenario: Custom retry count
- **WHEN** `MaxRetries` is set to 5
- **THEN** the system SHALL retry retryable errors up to 5 times (6 total attempts)

### Requirement: Exponential backoff delay
The system SHALL use exponential backoff between retry attempts with an initial delay of 2 seconds and a backoff factor of 2. The delay sequence SHALL be: 2s, 4s, 8s, 16s, etc.

#### Scenario: Backoff progression
- **WHEN** a retryable error occurs on each attempt with `MaxRetries=3`
- **THEN** the system SHALL wait approximately 2s before attempt 2, 4s before attempt 3, and 8s before attempt 4

### Requirement: Retryable error classification
The system SHALL only retry errors that are `*provider.APICallError` with `IsRetryable` set to `true`. Errors that are not `APICallError` or have `IsRetryable` set to `false` SHALL NOT be retried.

#### Scenario: Retryable error triggers retry
- **WHEN** `DoStream` returns a `*provider.APICallError` with `IsRetryable` set to `true`
- **THEN** the system SHALL retry the call after the computed backoff delay

#### Scenario: Non-retryable error stops immediately
- **WHEN** `DoStream` returns an error that is not a `*provider.APICallError`
- **THEN** the system SHALL NOT retry and SHALL return the error

#### Scenario: Retryable false stops immediately
- **WHEN** `DoStream` returns a `*provider.APICallError` with `IsRetryable` set to `false`
- **THEN** the system SHALL NOT retry and SHALL return the error

### Requirement: Context cancellation never retried
The system SHALL never retry errors caused by context cancellation or deadline exceeded. If the context is cancelled during a retry delay, the system SHALL return the context error immediately.

#### Scenario: Context cancelled during call
- **WHEN** the context is cancelled while `DoStream` is in progress
- **THEN** the system SHALL return the context error without retrying

#### Scenario: Context cancelled during backoff delay
- **WHEN** the context is cancelled while waiting for a retry backoff delay
- **THEN** the system SHALL return the context error immediately without waiting for the delay to complete

### Requirement: Rate-limit header respect
The system SHALL check response headers from retryable errors for rate-limit delay hints. The system SHALL check `retry-after-ms` first (milliseconds, OpenAI convention), then `retry-after` (seconds or HTTP date, RFC standard). The header-derived delay SHALL be used only when it is non-negative AND either less than 60 seconds or less than the computed exponential delay. Otherwise, the system SHALL fall back to the exponential backoff delay.

#### Scenario: retry-after-ms header present
- **WHEN** a retryable error includes a `retry-after-ms` header with value `500`
- **THEN** the system SHALL wait 500ms before the next attempt instead of the exponential delay

#### Scenario: retry-after header with seconds
- **WHEN** a retryable error includes a `retry-after` header with value `3` (seconds)
- **THEN** the system SHALL wait 3s before the next attempt instead of the exponential delay

#### Scenario: Unreasonable header value ignored
- **WHEN** a retryable error includes a `retry-after-ms` header with value `120000` and the exponential delay is 4s
- **THEN** the system SHALL use the 4s exponential delay instead of the header value

### Requirement: RetryError on exhaustion
When all retry attempts are exhausted, the system SHALL return a `RetryError` that carries: a reason indicating why retries stopped (`MaxRetriesExceeded` or `ErrorNotRetryable`), the list of all errors from each attempt, and access to the last error.

#### Scenario: All retries exhausted
- **WHEN** `MaxRetries=2` and all 3 attempts fail with retryable errors
- **THEN** the system SHALL return a `RetryError` with reason `MaxRetriesExceeded` containing all 3 errors

#### Scenario: Non-retryable after retries started
- **WHEN** the first attempt fails with a retryable error, the retry is attempted, and the second attempt fails with a non-retryable error
- **THEN** the system SHALL return a `RetryError` with reason `ErrorNotRetryable` containing both errors

#### Scenario: First attempt non-retryable
- **WHEN** `MaxRetries=2` and the first attempt fails with a non-retryable error
- **THEN** the system SHALL return the original error unwrapped (not wrapped in `RetryError`)

#### Scenario: RetryError unwraps to last error
- **WHEN** a `RetryError` is returned
- **THEN** calling `errors.Is` or `errors.As` on the `RetryError` SHALL match the last attempt's error

### Requirement: Per-step retry in multi-step loops
In multi-step tool-use loops, each step's model invocation (`DoStream`/`DoGenerate`) SHALL be retried independently. Retry state SHALL NOT carry over between steps.

#### Scenario: Independent step retry
- **WHEN** step 1 succeeds after 1 retry and step 2 fails on the first attempt with a retryable error
- **THEN** step 2 SHALL have its full retry budget (up to `MaxRetries` retries)

### Requirement: Retry applies to both StreamText and GenerateText
The retry logic SHALL apply identically to both `StreamText` (wrapping `DoStream`) and `GenerateText` (wrapping `DoGenerate`).

#### Scenario: GenerateText retries
- **WHEN** `GenerateText` is called and `DoGenerate` returns a retryable error
- **THEN** the system SHALL retry with the same exponential backoff as `StreamText`
