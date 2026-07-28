## ADDED Requirements

### Requirement: APICallError is JSON-serializable losslessly

`APICallError` SHALL declare every wire-relevant field as exported with a JSON tag. The unexported `cause error` field MAY remain for in-process `Unwrap()` support but MUST NOT participate in JSON serialization. The set of exported fields SHALL be:

- `Message string` — `json:"message"`
- `StatusCode int` — `json:"statusCode"`
- `URL string` — `json:"url,omitempty"`
- `RequestBodyValues json.RawMessage` — `json:"requestBodyValues,omitempty"` (changed from `any` to `json.RawMessage` for round-trip fidelity)
- `ResponseHeaders map[string][]string` — `json:"responseHeaders,omitempty"`
- `ResponseBody string` — `json:"responseBody,omitempty"`
- `IsRetryable bool` — `json:"isRetryable"`
- `Data json.RawMessage` — `json:"data,omitempty"`

#### Scenario: Field shape
- **WHEN** the `APICallError` struct is inspected
- **THEN** the listed fields SHALL be exported with the listed JSON tags, and `Message` SHALL be a public field (not an unexported `message`)

#### Scenario: APICallError round-trip preserves IsRetryable
- **WHEN** an `APICallError` with `IsRetryable: true` is marshaled and unmarshaled
- **THEN** the decoded value SHALL have `IsRetryable == true`

#### Scenario: APICallError round-trip preserves StatusCode and Message
- **WHEN** an `APICallError{StatusCode: 429, Message: "rate limit exceeded"}` is marshaled and unmarshaled
- **THEN** the decoded value SHALL have `StatusCode == 429` and `Message == "rate limit exceeded"`

#### Scenario: cause not on the wire
- **WHEN** an `APICallError` carrying a non-nil `cause` is marshaled to JSON and unmarshaled back
- **THEN** the decoded value's `Unwrap()` SHALL return `nil` while every exported field SHALL be preserved

### Requirement: NewAPICallError accepts json.RawMessage RequestBodyValues

`APICallErrorOptions.RequestBodyValues` SHALL be typed as `json.RawMessage` (not `any`). Callers that previously passed a typed value SHALL marshal it via `json.Marshal` before construction, or the `NewAPICallError` constructor SHALL accept the typed value via a sibling helper that internally marshals.

#### Scenario: Typed RequestBodyValues passed to constructor
- **WHEN** a caller has typed request data and calls a constructor that accepts a typed value
- **THEN** the resulting `APICallError.RequestBodyValues` SHALL be the JSON encoding of the typed value as `json.RawMessage`

## MODIFIED Requirements

### Requirement: APICallError type definition

The `provider` package SHALL export an `APICallError` struct that implements the `error` interface. The struct SHALL contain the following exported fields with JSON tags:

- `Message string` (`json:"message"`)
- `StatusCode int` (`json:"statusCode"`)
- `URL string` (`json:"url,omitempty"`)
- `RequestBodyValues json.RawMessage` (`json:"requestBodyValues,omitempty"`)
- `ResponseHeaders map[string][]string` (`json:"responseHeaders,omitempty"`)
- `ResponseBody string` (`json:"responseBody,omitempty"`)
- `IsRetryable bool` (`json:"isRetryable"`)
- `Data json.RawMessage` (`json:"data,omitempty"`)

The struct MAY retain an unexported `cause error` field for in-process `Unwrap()` support. The struct SHALL expose `Error()` and `Unwrap()` methods.

#### Scenario: APICallError implements error interface
- **WHEN** an `*APICallError` value is assigned to a variable of type `error`
- **THEN** the assignment SHALL compile successfully

#### Scenario: Error() includes status code and message
- **WHEN** `Error()` is called on an `APICallError` with `StatusCode` 429 and `Message` "rate limit exceeded"
- **THEN** the returned string SHALL contain both "429" and "rate limit exceeded"

#### Scenario: Unwrap returns the cause in process
- **WHEN** `Unwrap()` is called on an `APICallError` constructed with a non-nil cause
- **THEN** the returned error SHALL be the original cause error (only valid in the originating process — not preserved across the wire)

### Requirement: Default retryability from status code

The `NewAPICallError` constructor SHALL auto-compute `IsRetryable` from the status code when the caller does not explicitly set it. Status codes 408, 409, 429, and >= 500 SHALL default to retryable. All other status codes SHALL default to non-retryable. With this change, an `APICallError` reconstructed from the wire (which always carries an explicit `IsRetryable` field) bypasses the default-from-status logic.

#### Scenario: 429 defaults to retryable
- **WHEN** `NewAPICallError` is called with `StatusCode` 429 and no explicit `IsRetryable`
- **THEN** the resulting error SHALL have `IsRetryable` equal to `true`

#### Scenario: 500 defaults to retryable
- **WHEN** `NewAPICallError` is called with `StatusCode` 500 and no explicit `IsRetryable`
- **THEN** the resulting error SHALL have `IsRetryable` equal to `true`

#### Scenario: 400 defaults to non-retryable
- **WHEN** `NewAPICallError` is called with `StatusCode` 400 and no explicit `IsRetryable`
- **THEN** the resulting error SHALL have `IsRetryable` equal to `false`

#### Scenario: Explicit IsRetryable overrides default
- **WHEN** `NewAPICallError` is called with `StatusCode` 400 and `IsRetryable` explicitly set to `true`
- **THEN** the resulting error SHALL have `IsRetryable` equal to `true`

### Requirement: Anthropic provider wraps API errors

The anthropic provider SHALL wrap errors from `anthropic-sdk-go` into `*provider.APICallError` for all API call failures. The original SDK error SHALL be preserved as the in-process cause (accessible via `Unwrap`). The `StatusCode` SHALL be extracted from the `anthropic.Error` type. The wrapped error MUST round-trip losslessly through JSON for every exported field; only the `cause` chain is process-local. Stream-error events MUST emit the wrapped error via `StreamPart.APICallError` (not the removed `Error error` field).

#### Scenario: DoGenerate wraps API error
- **WHEN** `DoGenerate` receives a 429 error from the Anthropic API
- **THEN** the returned error SHALL be a `*provider.APICallError` with `StatusCode` 429, `IsRetryable` true, and the original SDK error accessible via `errors.As` on the cause

#### Scenario: Stream wraps API error
- **WHEN** the streaming connection encounters an API error with status 529 (overloaded)
- **THEN** the emitted `PartError` stream part SHALL carry a non-nil `*provider.APICallError` (via `StreamPart.APICallError`, not the removed `Error error` field) with `IsRetryable` true

#### Scenario: Non-API errors pass through unwrapped
- **WHEN** `DoGenerate` or the stream encounters a non-API error (e.g. network timeout, DNS failure)
- **THEN** the error SHALL NOT be wrapped in `APICallError`
- **AND** the original error SHALL be returned as-is

### Requirement: Fallback decider uses structured error inspection

The fallback decider SHALL use `errors.As` to extract `*provider.APICallError` and inspect `IsRetryable` to determine whether to try the next candidate model. Unknown errors (not `APICallError`) SHALL default to trying the next candidate. An `APICallError` reconstructed from the wire (with `cause == nil`) MUST be fully usable by the decider because `IsRetryable` is preserved as a first-class JSON field.

#### Scenario: Retryable API error triggers fallback
- **WHEN** the current model returns a `*provider.APICallError` with `IsRetryable` true (whether constructed locally or reconstructed from the wire)
- **THEN** the decider SHALL return `true`

#### Scenario: Non-retryable API error stops fallback
- **WHEN** the current model returns a `*provider.APICallError` with `IsRetryable` false
- **THEN** the decider SHALL return `false`

#### Scenario: Wire-reconstructed APICallError works in decider
- **WHEN** an `APICallError` is reconstructed from JSON via `provider/wire` and returned by a remote provider
- **THEN** the fallback decider SHALL successfully extract it via `errors.As` and inspect `IsRetryable`
