## ADDED Requirements

### Requirement: APICallError type definition
The `provider` package SHALL export an `APICallError` struct that implements the `error` interface. The struct SHALL contain the following fields: `StatusCode` (int), `URL` (string), `RequestBodyValues` (any), `ResponseHeaders` (map[string][]string), `ResponseBody` (string), `IsRetryable` (bool), `Data` (json.RawMessage). The struct SHALL expose `Error()` and `Unwrap()` methods.

#### Scenario: APICallError implements error interface
- **WHEN** an `*APICallError` value is assigned to a variable of type `error`
- **THEN** the assignment SHALL compile successfully (compile-time interface check)

#### Scenario: Error() includes status code and message
- **WHEN** `Error()` is called on an `APICallError` with `StatusCode` 429 and message "rate limit exceeded"
- **THEN** the returned string SHALL contain both "429" and "rate limit exceeded"

#### Scenario: Unwrap returns the cause
- **WHEN** `Unwrap()` is called on an `APICallError` with a non-nil cause
- **THEN** the returned error SHALL be the original cause error
- **AND** `errors.Is(apiCallError, cause)` SHALL return true

### Requirement: Default retryability from status code
The `NewAPICallError` constructor SHALL auto-compute `IsRetryable` from the status code when the caller does not explicitly set it. Status codes 408 (Request Timeout), 409 (Conflict), 429 (Too Many Requests), and >= 500 (Server Error) SHALL default to retryable. All other status codes SHALL default to non-retryable.

#### Scenario: 429 defaults to retryable
- **WHEN** `NewAPICallError` is called with `StatusCode` 429 and no explicit `IsRetryable`
- **THEN** the resulting error SHALL have `IsRetryable` equal to `true`

#### Scenario: 500 defaults to retryable
- **WHEN** `NewAPICallError` is called with `StatusCode` 500 and no explicit `IsRetryable`
- **THEN** the resulting error SHALL have `IsRetryable` equal to `true`

#### Scenario: 503 defaults to retryable
- **WHEN** `NewAPICallError` is called with `StatusCode` 503 and no explicit `IsRetryable`
- **THEN** the resulting error SHALL have `IsRetryable` equal to `true`

#### Scenario: 408 defaults to retryable
- **WHEN** `NewAPICallError` is called with `StatusCode` 408 and no explicit `IsRetryable`
- **THEN** the resulting error SHALL have `IsRetryable` equal to `true`

#### Scenario: 409 defaults to retryable
- **WHEN** `NewAPICallError` is called with `StatusCode` 409 and no explicit `IsRetryable`
- **THEN** the resulting error SHALL have `IsRetryable` equal to `true`

#### Scenario: 400 defaults to non-retryable
- **WHEN** `NewAPICallError` is called with `StatusCode` 400 and no explicit `IsRetryable`
- **THEN** the resulting error SHALL have `IsRetryable` equal to `false`

#### Scenario: 401 defaults to non-retryable
- **WHEN** `NewAPICallError` is called with `StatusCode` 401 and no explicit `IsRetryable`
- **THEN** the resulting error SHALL have `IsRetryable` equal to `false`

#### Scenario: 403 defaults to non-retryable
- **WHEN** `NewAPICallError` is called with `StatusCode` 403 and no explicit `IsRetryable`
- **THEN** the resulting error SHALL have `IsRetryable` equal to `false`

#### Scenario: Explicit IsRetryable overrides default
- **WHEN** `NewAPICallError` is called with `StatusCode` 400 and `IsRetryable` explicitly set to `true`
- **THEN** the resulting error SHALL have `IsRetryable` equal to `true`

#### Scenario: Explicit non-retryable overrides default for 500
- **WHEN** `NewAPICallError` is called with `StatusCode` 500 and `IsRetryable` explicitly set to `false`
- **THEN** the resulting error SHALL have `IsRetryable` equal to `false`

### Requirement: Anthropic provider wraps API errors
The anthropic provider SHALL wrap errors from `anthropic-sdk-go` into `*provider.APICallError` for all API call failures. The original SDK error SHALL be preserved as the `Cause` (accessible via `Unwrap`). The `StatusCode` SHALL be extracted from the `anthropic.Error` type.

#### Scenario: DoGenerate wraps API error
- **WHEN** `DoGenerate` receives a 429 error from the Anthropic API
- **THEN** the returned error SHALL be a `*provider.APICallError`
- **AND** `StatusCode` SHALL be 429
- **AND** `IsRetryable` SHALL be `true`
- **AND** `errors.As(err, &anthropicErr)` on the cause SHALL succeed

#### Scenario: DoGenerate wraps 400 error
- **WHEN** `DoGenerate` receives a 400 error from the Anthropic API
- **THEN** the returned error SHALL be a `*provider.APICallError`
- **AND** `StatusCode` SHALL be 400
- **AND** `IsRetryable` SHALL be `false`

#### Scenario: Stream wraps API error
- **WHEN** the streaming connection encounters an API error with status 529 (overloaded)
- **THEN** the emitted `PartError` stream part SHALL contain a `*provider.APICallError`
- **AND** `IsRetryable` SHALL be `true`

#### Scenario: Non-API errors pass through unwrapped
- **WHEN** `DoGenerate` or the stream encounters a non-API error (e.g. network timeout, DNS failure)
- **THEN** the error SHALL NOT be wrapped in `APICallError`
- **AND** the original error SHALL be returned as-is

### Requirement: Fallback decider uses structured error inspection
The fallback decider SHALL use `errors.As` to extract `*provider.APICallError` and inspect `IsRetryable` to determine whether to try the next candidate model. Unknown errors (not `APICallError`) SHALL default to trying the next candidate.

#### Scenario: Retryable API error triggers fallback
- **WHEN** the current model returns a `*provider.APICallError` with `IsRetryable` true
- **THEN** the decider SHALL return `true` (try next candidate)

#### Scenario: Non-retryable API error stops fallback
- **WHEN** the current model returns a `*provider.APICallError` with `IsRetryable` false
- **THEN** the decider SHALL return `false` (do not try next candidate)

#### Scenario: Unknown error triggers fallback
- **WHEN** the current model returns an error that is not a `*provider.APICallError`
- **THEN** the decider SHALL return `true` (try next candidate)

#### Scenario: Context-length error (400) stops fallback
- **WHEN** the current model returns a `*provider.APICallError` with `StatusCode` 400 (context length exceeded)
- **THEN** `IsRetryable` SHALL be `false`
- **AND** the decider SHALL return `false`
