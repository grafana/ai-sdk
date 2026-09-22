# api-call-error Specification

## Purpose

Define structured API-call errors that support retry decisions, in-process cause inspection, and lossless JSON serialization and reconstruction.

## Requirements

### Requirement: APICallError is JSON-serializable losslessly

`APICallError` SHALL declare every JSON-serialized field as exported with a JSON tag. The unexported `cause error` field MAY remain for in-process `Unwrap()` support but MUST NOT participate in JSON serialization. The set of exported fields SHALL be:

- `Message string` -- `json:"message"`
- `StatusCode int` -- `json:"statusCode"`
- `URL string` -- `json:"url,omitempty"`
- `RequestBodyValues json.RawMessage` -- `json:"requestBodyValues,omitempty"` (changed from `any` to `json.RawMessage` for round-trip fidelity)
- `ResponseHeaders map[string][]string` -- `json:"responseHeaders,omitempty"`
- `ResponseBody string` -- `json:"responseBody,omitempty"`
- `IsRetryable bool` -- `json:"isRetryable"`; indicates eligibility for a fresh call, not whether an established stream can be replayed safely
- `Data json.RawMessage` -- `json:"data,omitempty"`

#### Scenario: Field shape
- **WHEN** the `APICallError` struct is inspected
- **THEN** the listed fields SHALL be exported with the listed JSON tags, and `Message` SHALL be a public field (not an unexported `message`)

#### Scenario: APICallError round-trip preserves IsRetryable
- **WHEN** an `APICallError` with `IsRetryable: true` is marshaled and unmarshaled
- **THEN** the decoded value SHALL have `IsRetryable == true`

#### Scenario: APICallError round-trip preserves StatusCode and Message
- **WHEN** an `APICallError{StatusCode: 429, Message: "rate limit exceeded"}` is marshaled and unmarshaled
- **THEN** the decoded value SHALL have `StatusCode == 429` and `Message == "rate limit exceeded"`

#### Scenario: Cause is not serialized
- **WHEN** an `APICallError` carrying a non-nil `cause` is marshaled to JSON and unmarshaled back
- **THEN** the decoded value's `Unwrap()` SHALL return `nil` while every exported field SHALL be preserved

### Requirement: APICallError type definition

The `provider` package SHALL export an `APICallError` struct that implements the `error` interface. The struct SHALL contain the exported fields listed in the JSON-serializable requirement with JSON tags. The struct MAY retain an unexported `cause error` field for in-process `Unwrap()` support. The struct SHALL expose `Error()` and `Unwrap()` methods.

#### Scenario: APICallError implements error interface
- **WHEN** an `*APICallError` value is assigned to a variable of type `error`
- **THEN** the assignment SHALL compile successfully

#### Scenario: Error() includes status code and message
- **WHEN** `Error()` is called on an `APICallError` with `StatusCode` 429 and `Message` "rate limit exceeded"
- **THEN** the returned string SHALL contain both "429" and "rate limit exceeded"

#### Scenario: Unwrap returns the cause in process
- **WHEN** `Unwrap()` is called on an `APICallError` constructed with a non-nil cause
- **THEN** the returned error SHALL be the original cause error (only valid in the originating process and not preserved by JSON serialization)

### Requirement: NewAPICallError accepts json.RawMessage RequestBodyValues

`APICallErrorOptions.RequestBodyValues` SHALL be typed as `json.RawMessage` (not `any`). Callers that previously passed a typed value SHALL marshal it via `json.Marshal` before construction, or the `NewAPICallError` constructor SHALL accept the typed value via a sibling helper that internally marshals.

#### Scenario: Typed RequestBodyValues passed to constructor
- **WHEN** a caller has typed request data and calls a constructor that accepts a typed value
- **THEN** the resulting `APICallError.RequestBodyValues` SHALL be the JSON encoding of the typed value as `json.RawMessage`

### Requirement: Default retryability from status code

The `NewAPICallError` constructor SHALL auto-compute `IsRetryable` from the status code when the caller does not explicitly set it. Status codes 408, 409, 429, and >= 500 SHALL default to retryable. All other status codes SHALL default to non-retryable. An `APICallError` reconstructed from JSON carries its serialized `IsRetryable` field and does not invoke constructor defaulting.

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

### Requirement: Retryability is distinct from stream replay safety

`APICallError.IsRetryable` SHALL indicate that the provider failure is eligible
for a fresh API call. It SHALL NOT imply that replaying an established stream is
safe. Any consumer that retries after receiving stream parts MUST separately
establish that no output or effects escaped the failed attempt.

#### Scenario: Retryable error follows partial output
- **WHEN** an established provider stream emits output and later emits an `APICallError` with `IsRetryable` true
- **THEN** the error SHALL remain eligible for a caller-controlled fresh call
- **AND** `IsRetryable` alone SHALL NOT authorize transparent replay of the established stream

### Requirement: Anthropic provider wraps API errors

The anthropic provider SHALL wrap errors from `anthropic-sdk-go` into
`*provider.APICallError` for all API call failures. The original SDK error SHALL
be preserved as the in-process cause (accessible via `Unwrap`). The `StatusCode`
SHALL be extracted from the `anthropic.Error` type. The wrapped error MUST
round-trip losslessly through JSON for every exported field; only the `cause`
chain is process-local. Stream-error events encountered after provider output begins MUST emit the
wrapped error via `StreamPart.APICallError` (not the removed `Error error`
field). An error received as the first SSE event MUST be returned synchronously
from `DoStream` as `*provider.APICallError`.

Additionally, the anthropic provider SHALL preserve the provider's structured
error payload by populating `APICallError.Data` with the parsed error envelope
(at minimum the provider error `type` and `message`), mirroring upstream's
`createJsonErrorResponseHandler` which stores the parsed error in
`APICallError.data`. Gateway error-category normalization SHALL remain outside
this provider. The provider MAY classify retry eligibility from the structured
Anthropic error type when an HTTP 200 streaming response does not carry a useful
failure status. Ordinary non-200 API responses SHALL continue to use status-code
retryability inference. When the error body cannot be parsed, `Data` MAY be empty
and the raw body SHALL remain in `ResponseBody`.

Before returning a successful stream, the provider SHALL inspect the first
semantic Anthropic SSE event. If that event is an error, `DoStream` SHALL return a
synchronous `*provider.APICallError` instead of exposing an established stream.
An initial `overloaded_error` SHALL be normalized to status 529 and an initial
error of any other type SHALL be normalized to status 500. HTTP 200 SSE
`api_error` and `overloaded_error` failures SHALL be retryable;
`rate_limit_error` and non-transient failures SHALL NOT become retryable solely
from their streamed error type. Errors encountered after a non-error event SHALL
remain `PartError` stream parts.

#### Scenario: DoGenerate wraps API error
- **WHEN** `DoGenerate` receives a 429 error from the Anthropic API
- **THEN** the returned error SHALL be a `*provider.APICallError` with `StatusCode` 429, `IsRetryable` true, and the original SDK error accessible via `errors.As` on the cause

#### Scenario: DoGenerate populates Data with structured type
- **WHEN** `DoGenerate` receives an Anthropic error whose body carries `error.type` (e.g. `rate_limit_error`)
- **THEN** the returned `*provider.APICallError.Data` SHALL contain the parsed structured error including that `type`

#### Scenario: DoGenerate wraps 400 error
- **WHEN** `DoGenerate` receives a 400 error from the Anthropic API
- **THEN** the returned error SHALL be a `*provider.APICallError` with `StatusCode` 400 and `IsRetryable` false

#### Scenario: Initial HTTP 200 overloaded stream error is promoted
- **WHEN** Anthropic returns HTTP 200 and the first semantic SSE event is an `overloaded_error`
- **THEN** `DoStream` SHALL return a synchronous `*provider.APICallError` instead of a stream
- **AND** the error SHALL have `StatusCode` 529 and `IsRetryable` true
- **AND** its message, inner provider error body, request URL, and structured `Data` SHALL be preserved

#### Scenario: Initial HTTP 200 API error is promoted and retryable
- **WHEN** Anthropic returns HTTP 200 and the first semantic SSE event is an `api_error`
- **THEN** `DoStream` SHALL return a synchronous `*provider.APICallError` with `StatusCode` 500 and `IsRetryable` true

#### Scenario: Other initial HTTP 200 stream errors are not retryable by type
- **WHEN** Anthropic returns HTTP 200 and the first semantic SSE event is a `rate_limit_error` or non-transient error
- **THEN** `DoStream` SHALL return a synchronous `*provider.APICallError` with `StatusCode` 500 and `IsRetryable` false

#### Scenario: Post-output transient stream error remains a stream part
- **WHEN** an Anthropic stream emits model output and later emits an `api_error` or `overloaded_error`
- **THEN** the established stream SHALL emit a `PartError` carrying a non-nil `*provider.APICallError`
- **AND** the error SHALL retain HTTP status 200 and have `IsRetryable` true

#### Scenario: Post-output unclassified stream error remains non-retryable
- **WHEN** an Anthropic stream emits model output and later emits a `rate_limit_error` or non-transient error
- **THEN** the established stream SHALL emit a `PartError` carrying a non-nil `*provider.APICallError`
- **AND** the error SHALL retain HTTP status 200 and have `IsRetryable` false

#### Scenario: Non-200 Anthropic API error retains status inference
- **WHEN** Anthropic returns a non-200 API error with a structured provider error type
- **THEN** the wrapped `*provider.APICallError` SHALL retain that HTTP status
- **AND** retryability SHALL be inferred from the status unless explicitly configured by another existing rule

#### Scenario: Non-API errors before stream establishment pass through unwrapped
- **WHEN** `DoGenerate` or `DoStream` encounters a non-API error before returning an established stream (e.g. network timeout or DNS failure)
- **THEN** the error SHALL NOT be wrapped in `APICallError`
- **AND** the original error SHALL be returned as-is

#### Scenario: Non-API errors after stream establishment satisfy the PartError contract
- **WHEN** an established Anthropic stream encounters a non-API error
- **THEN** the stream SHALL emit a `PartError` carrying a non-nil `*provider.APICallError`
- **AND** the original error SHALL remain available as the in-process cause
- **AND** the error SHALL default to non-retryable when no status or provider classification establishes retry eligibility

### Requirement: Fallback decider uses structured error inspection

The fallback decider SHALL use `errors.As` to extract `*provider.APICallError`
and inspect `IsRetryable` to determine whether to try the next candidate model.
Unknown errors (not `APICallError`) SHALL default to trying the next candidate.
An `APICallError` reconstructed through provider-domain JSON serialization (with
`cause == nil`) MUST be fully usable by the decider because `IsRetryable` is
preserved as a first-class JSON field. This provider-domain JSON invariant SHALL
NOT define an HTTP transport contract.

Additionally, the decider SHALL NOT fail over when the error represents a
context-window/context-length failure, because the next candidate would fail
identically. Detection SHALL be a localized predicate inside the decider:
`*provider.APICallError` with `StatusCode == 400` and a context-length signal
read from `Data`/`Message`. This heuristic SHALL be confined to the decider and
SHALL NOT introduce a public context-window error category (upstream has none).

#### Scenario: Retryable API error triggers fallback
- **WHEN** the current model returns a `*provider.APICallError` with `IsRetryable` true, whether constructed directly or reconstructed through provider-domain JSON
- **THEN** the decider SHALL return `true`

#### Scenario: Non-retryable API error stops fallback
- **WHEN** the current model returns a `*provider.APICallError` with `IsRetryable` false
- **THEN** the decider SHALL return `false`

#### Scenario: Unknown error triggers fallback
- **WHEN** the current model returns an error that is not a `*provider.APICallError`
- **THEN** the decider SHALL return `true`

#### Scenario: JSON-reconstructed APICallError works in decider
- **WHEN** an `APICallError` is marshaled and unmarshaled with `encoding/json`
- **THEN** the fallback decider SHALL successfully extract it via `errors.As` and inspect `IsRetryable`
- **AND** this SHALL establish only a provider-domain representation invariant, not an HTTP transport contract

#### Scenario: Context-window error stops fallback
- **WHEN** the current model returns a `*provider.APICallError` with `StatusCode` 400 whose `Data`/`Message` indicates a context-length/context-window failure
- **THEN** the decider SHALL return `false`
