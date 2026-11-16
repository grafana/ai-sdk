## ADDED Requirements

### Requirement: Warning-aware request transformation

The OpenAI-compatible provider SHALL offer an optional callback that receives the completed request body and current call's warnings, and returns a transformed body, complete warning slice and error. The callback SHALL run after request conversion and streaming-field insertion for both generation and streaming. The returned body SHALL be serialized identically for HTTP and request metadata. A transform error or nil returned body SHALL fail before HTTP transport.

#### Scenario: Body replacement and warning extension

- **WHEN** a callback replaces the request body and appends a compatibility warning to existing conversion warnings
- **THEN** generation SHALL return the complete warning slice and streaming SHALL emit it in stream-start
- **AND** HTTP body and request metadata SHALL contain the same transformed JSON

#### Scenario: Core consumers

- **WHEN** core GenerateText or StreamText uses a model with a warning-aware callback
- **THEN** its collected warnings and step warnings SHALL include the callback's warnings

#### Scenario: Rejected transformation

- **WHEN** the callback returns an error or a nil body
- **THEN** generation and streaming SHALL return an error before sending an HTTP request

### Requirement: Backward-compatible transform configuration

The existing body/error transform option SHALL preserve its signature, body rewriting and error semantics, and SHALL retain conversion warnings. Both transform options SHALL configure the same callback slot: the last supplied option SHALL win, and a nil callback SHALL disable transformation.

#### Scenario: Existing callers

- **WHEN** a caller configures only the existing transform option
- **THEN** its body rewrite SHALL apply and the call's conversion warnings SHALL remain intact

#### Scenario: Option replacement and disabling

- **WHEN** multiple transform options are supplied
- **THEN** only the final transform SHALL run, or no transform SHALL run if that final callback is nil

### Requirement: Per-call transform state

The provider SHALL create request bodies and conversion-warning slices per call and SHALL NOT store callback warnings on the model. Callbacks SHALL be documented as potentially concurrent, with synchronization of their own captured mutable state the caller's responsibility.

#### Scenario: Reused model

- **WHEN** sequential or concurrent calls use the same model and append distinct call-specific warnings
- **THEN** each result SHALL contain only its own call's conversion and transform warnings
