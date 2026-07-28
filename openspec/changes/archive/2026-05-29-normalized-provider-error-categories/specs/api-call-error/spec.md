## MODIFIED Requirements

### Requirement: Anthropic provider wraps API errors

The anthropic provider SHALL wrap errors from `anthropic-sdk-go` into
`*provider.APICallError` for all API call failures. The original SDK error SHALL
be preserved as the in-process cause (accessible via `Unwrap`). The `StatusCode`
SHALL be extracted from the `anthropic.Error` type. The wrapped error MUST
round-trip losslessly through JSON for every exported field; only the `cause`
chain is process-local. Stream-error events MUST emit the wrapped error via
`StreamPart.APICallError` (not the removed `Error error` field).

Additionally, the anthropic provider SHALL preserve the provider's structured
error payload by populating `APICallError.Data` with the parsed error envelope
(at minimum the provider error `type` and `message`), mirroring upstream's
`createJsonErrorResponseHandler` which stores the parsed error in
`APICallError.data`. The provider SHALL NOT classify the error into a category at
this layer; classification is performed by the gateway normalizer. When the
error body cannot be parsed, `Data` MAY be empty and the raw body SHALL remain in
`ResponseBody`.

#### Scenario: DoGenerate wraps API error
- **WHEN** `DoGenerate` receives a 429 error from the Anthropic API
- **THEN** the returned error SHALL be a `*provider.APICallError` with `StatusCode` 429, `IsRetryable` true, and the original SDK error accessible via `errors.As` on the cause

#### Scenario: DoGenerate populates Data with structured type
- **WHEN** `DoGenerate` receives an Anthropic error whose body carries `error.type` (e.g. `rate_limit_error`)
- **THEN** the returned `*provider.APICallError.Data` SHALL contain the parsed structured error including that `type`

#### Scenario: DoGenerate wraps 400 error
- **WHEN** `DoGenerate` receives a 400 error from the Anthropic API
- **THEN** the returned error SHALL be a `*provider.APICallError` with `StatusCode` 400 and `IsRetryable` false

#### Scenario: Stream wraps API error
- **WHEN** the streaming connection encounters an API error with status 529 (overloaded)
- **THEN** the emitted `PartError` stream part SHALL carry a non-nil `*provider.APICallError` (via `StreamPart.APICallError`, not the removed `Error error` field) with `IsRetryable` true, and `Data` populated with the structured error when parseable

#### Scenario: Non-API errors pass through unwrapped
- **WHEN** `DoGenerate` or the stream encounters a non-API error (e.g. network timeout, DNS failure)
- **THEN** the error SHALL NOT be wrapped in `APICallError`
- **AND** the original error SHALL be returned as-is

### Requirement: Fallback decider uses structured error inspection

The fallback decider SHALL use `errors.As` to extract `*provider.APICallError`
and inspect `IsRetryable` to determine whether to try the next candidate model.
Unknown errors (not `APICallError`) SHALL default to trying the next candidate.
An `APICallError` reconstructed from the wire (with `cause == nil`) MUST be
fully usable by the decider because `IsRetryable` is preserved as a first-class
JSON field.

Additionally, the decider SHALL NOT fail over when the error represents a
context-window/context-length failure, because the next candidate would fail
identically. Detection SHALL be a localized predicate inside the decider:
`*provider.APICallError` with `StatusCode == 400` and a context-length signal
read from `Data`/`Message`. This heuristic SHALL be confined to the decider and
SHALL NOT introduce a public context-window error category (upstream has none).

#### Scenario: Retryable API error triggers fallback
- **WHEN** the current model returns a `*provider.APICallError` with `IsRetryable` true (whether constructed locally or reconstructed from the wire)
- **THEN** the decider SHALL return `true`

#### Scenario: Non-retryable API error stops fallback
- **WHEN** the current model returns a `*provider.APICallError` with `IsRetryable` false
- **THEN** the decider SHALL return `false`

#### Scenario: Unknown error triggers fallback
- **WHEN** the current model returns an error that is not a `*provider.APICallError`
- **THEN** the decider SHALL return `true`

#### Scenario: Wire-reconstructed APICallError works in decider
- **WHEN** an `APICallError` is reconstructed from JSON via `provider/wire` and returned by a remote provider
- **THEN** the fallback decider SHALL successfully extract it via `errors.As` and inspect `IsRetryable`

#### Scenario: Context-window error stops fallback
- **WHEN** the current model returns a `*provider.APICallError` with `StatusCode` 400 whose `Data`/`Message` indicates a context-length/context-window failure
- **THEN** the decider SHALL return `false`
