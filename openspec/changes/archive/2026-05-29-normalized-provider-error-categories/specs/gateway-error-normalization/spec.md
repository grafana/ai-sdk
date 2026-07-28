## ADDED Requirements

### Requirement: Normalized gateway error with type discriminator

The `provider` package SHALL export a `GatewayError` type that carries a
normalized category as a typed string discriminator field `Type GatewayErrorType`,
mirroring the `@ai-sdk/gateway` `GatewayError.type` contract. `GatewayError` SHALL
implement the `error` interface and expose `Error()` and `Unwrap()`. The package
SHALL define typed constants for the upstream category vocabulary:

- `GatewayErrorAuthentication` = `"authentication_error"`
- `GatewayErrorInvalidRequest` = `"invalid_request_error"`
- `GatewayErrorRateLimit` = `"rate_limit_exceeded"`
- `GatewayErrorModelNotFound` = `"model_not_found"`
- `GatewayErrorInternalServer` = `"internal_server_error"`

`GatewayError` SHALL carry `Message string`, `StatusCode int`, and an optional
`ModelID string` (populated for `model_not_found`).

#### Scenario: Implements error interface
- **WHEN** a `*GatewayError` value is assigned to a variable of type `error`
- **THEN** the assignment SHALL compile successfully

#### Scenario: Type is a typed string enum
- **WHEN** the `GatewayError.Type` field is inspected
- **THEN** it SHALL be of named type `GatewayErrorType`, and the listed constants SHALL hold exactly the listed string values

#### Scenario: Error() includes status code and message
- **WHEN** `Error()` is called on a `GatewayError` with `StatusCode` 429 and `Message` "rate limit exceeded"
- **THEN** the returned string SHALL contain both "429" and "rate limit exceeded"

### Requirement: GatewayError preserves the originating APICallError as cause

A `GatewayError` produced by normalization SHALL carry the originating
`*APICallError` as its in-process cause, and `Unwrap()` SHALL return it. This
mirrors upstream `asGatewayError`, where the gateway error replaces the
`APICallError` as the primary error while keeping it as `cause`. `errors.As`
against `*provider.APICallError` SHALL therefore still reach the underlying call
error with its `StatusCode`, `ResponseHeaders`, `ResponseBody`, `Data`, and
`IsRetryable`.

#### Scenario: errors.As reaches the GatewayError
- **WHEN** a normalized `*GatewayError` is returned and `errors.As(err, &target)` is called with `target` of type `*provider.GatewayError`
- **THEN** `errors.As` SHALL return `true` and populate `target`

#### Scenario: errors.As reaches the originating APICallError
- **WHEN** a normalized `*GatewayError` is returned and `errors.As(err, &target)` is called with `target` of type `*provider.APICallError`
- **THEN** `errors.As` SHALL return `true` and `target` SHALL be the originating `*APICallError` with all fields preserved

### Requirement: Normalizer maps structured provider error to a normalized type

The `provider` package SHALL expose a normalizer that converts an
`*APICallError` into a `*GatewayError`, reading the structured provider error
`type` from `APICallError.Data` and falling back to parsing
`APICallError.ResponseBody` when `Data` is absent, mirroring upstream
`extractApiCallResponse` + `createGatewayErrorFromResponse`. The mapping SHALL be:

- `authentication_error` -> `GatewayErrorAuthentication`
- `invalid_request_error` -> `GatewayErrorInvalidRequest`
- `rate_limit_exceeded` (or provider rate-limit/overloaded signals) -> `GatewayErrorRateLimit`
- `model_not_found` -> `GatewayErrorModelNotFound` (populating `ModelID` when reported)
- `internal_server_error` -> `GatewayErrorInternalServer`
- any other or missing type -> `GatewayErrorInternalServer` (the upstream `default` branch)

The originating `*APICallError` SHALL be set as the resulting `GatewayError`'s
cause.

#### Scenario: Maps authentication error
- **WHEN** the normalizer receives an `APICallError` whose `Data` carries `type: "authentication_error"`
- **THEN** the result SHALL be a `*GatewayError` with `Type == GatewayErrorAuthentication`

#### Scenario: Maps rate-limit error
- **WHEN** the normalizer receives an `APICallError` whose `Data` carries a rate-limit type
- **THEN** the result SHALL be a `*GatewayError` with `Type == GatewayErrorRateLimit`

#### Scenario: Maps model-not-found with model id
- **WHEN** the normalizer receives an `APICallError` whose `Data` carries `type: "model_not_found"` and a model identifier
- **THEN** the result SHALL be a `*GatewayError` with `Type == GatewayErrorModelNotFound` and `ModelID` equal to that identifier

#### Scenario: Falls back to ResponseBody when Data is empty
- **WHEN** the normalizer receives an `APICallError` with empty `Data` but a `ResponseBody` containing a parseable structured error type
- **THEN** the type SHALL be read from `ResponseBody` and mapped accordingly

#### Scenario: Unknown type defaults to internal server error
- **WHEN** the normalizer receives an `APICallError` whose structured error type is missing or unrecognized
- **THEN** the result SHALL be a `*GatewayError` with `Type == GatewayErrorInternalServer` and the originating `APICallError` preserved as cause
