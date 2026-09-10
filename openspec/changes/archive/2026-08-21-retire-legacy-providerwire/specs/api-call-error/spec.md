## MODIFIED Requirements

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
