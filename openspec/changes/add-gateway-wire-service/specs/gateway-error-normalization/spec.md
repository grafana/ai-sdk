## MODIFIED Requirements

### Requirement: Normalized gateway error with type discriminator

The Grafana gateway provider package (`providers/grafana`) SHALL export a `GatewayError` type that carries a normalized category as a typed string discriminator field `Type GatewayErrorType`, mirroring the registered `@ai-sdk/gateway@4.0.33` `GatewayError.type` contract. Like upstream, this lives in the gateway provider package, not in the core `provider` package. `GatewayError` SHALL implement the `error` interface and expose `Error()` and `Unwrap()`. The package SHALL define typed constants for the registered category vocabulary used by the strict service:

- `GatewayErrorAuthentication` = `"authentication_error"`
- `GatewayErrorInvalidRequest` = `"invalid_request_error"`
- `GatewayErrorRateLimit` = `"rate_limit_exceeded"`
- `GatewayErrorModelNotFound` = `"model_not_found"`
- `GatewayErrorForbidden` = `"forbidden"`
- `GatewayErrorFailedDependency` = `"failed_dependency"`
- `GatewayErrorInternalServer` = `"internal_server_error"`

`GatewayError` SHALL carry `Message string`, `StatusCode int`, and an optional `ModelID string` populated for `model_not_found`.

#### Scenario: Implements error interface

- **WHEN** a `*GatewayError` value is assigned to a variable of type `error`
- **THEN** the assignment SHALL compile successfully

#### Scenario: Type is a typed string enum

- **WHEN** the `GatewayError.Type` field is inspected
- **THEN** it SHALL be of named type `GatewayErrorType`, and the listed constants SHALL hold exactly the listed string values

#### Scenario: Error includes status code and message

- **WHEN** `Error()` is called on a `GatewayError` with `StatusCode` 429 and `Message` "rate limit exceeded"
- **THEN** the returned string SHALL contain both "429" and "rate limit exceeded"

### Requirement: Normalizer maps structured provider error to a normalized type

The Grafana gateway provider package SHALL expose a normalizer that converts a `*provider.APICallError` into a `*GatewayError`, reading the structured provider error `type` from `APICallError.Data` and falling back to parsing `APICallError.ResponseBody` when `Data` is absent, mirroring the registered upstream `extractApiCallResponse` and `createGatewayErrorFromResponse` behavior. The mapping SHALL be:

- `authentication_error` or `permission_error` -> `GatewayErrorAuthentication`
- `invalid_request_error` or `billing_error` -> `GatewayErrorInvalidRequest`
- `rate_limit_exceeded`, provider rate-limit, or overloaded signals -> `GatewayErrorRateLimit`
- `model_not_found` or `not_found_error` -> `GatewayErrorModelNotFound`, populating `ModelID` when reported
- `forbidden` -> `GatewayErrorForbidden`
- `failed_dependency` -> `GatewayErrorFailedDependency`
- `internal_server_error`, `api_error`, or `timeout_error` -> `GatewayErrorInternalServer`
- any other or missing type -> `GatewayErrorInternalServer`

The originating `*provider.APICallError` SHALL be set as the resulting `GatewayError`'s cause with its status and retryability preserved. A strict-service stream error SHALL place only its safe category envelope in `APICallError.Data`, allowing the same normalizer to recover the category without exposing a private provider cause.

#### Scenario: Maps authentication error

- **WHEN** the normalizer receives an `APICallError` whose `Data` carries `type: "authentication_error"`
- **THEN** the result SHALL be a `*GatewayError` with `Type == GatewayErrorAuthentication`

#### Scenario: Maps rate-limit error

- **WHEN** the normalizer receives an `APICallError` whose `Data` carries a rate-limit type
- **THEN** the result SHALL be a `*GatewayError` with `Type == GatewayErrorRateLimit`

#### Scenario: Maps model-not-found with model id

- **WHEN** the normalizer receives an `APICallError` whose `Data` carries `type: "model_not_found"` and a model identifier
- **THEN** the result SHALL be a `*GatewayError` with `Type == GatewayErrorModelNotFound` and `ModelID` equal to that identifier

#### Scenario: Maps forbidden

- **WHEN** the normalizer receives an `APICallError` whose safe structured data carries `type: "forbidden"`
- **THEN** the result SHALL be a `*GatewayError` with `Type == GatewayErrorForbidden`

#### Scenario: Maps failed dependency with retryability

- **WHEN** the normalizer receives an `APICallError` whose safe structured data carries `type: "failed_dependency"`
- **THEN** the result SHALL be a `*GatewayError` with `Type == GatewayErrorFailedDependency` and the underlying `APICallError.IsRetryable` value SHALL remain reachable through `errors.As`

#### Scenario: Falls back to ResponseBody when Data is empty

- **WHEN** the normalizer receives an `APICallError` with empty `Data` but a `ResponseBody` containing a parseable structured error type
- **THEN** the type SHALL be read from `ResponseBody` and mapped accordingly

#### Scenario: Unknown type defaults to internal server error

- **WHEN** the normalizer receives an `APICallError` whose structured error type is missing or unrecognized
- **THEN** the result SHALL be a `*GatewayError` with `Type == GatewayErrorInternalServer` and the originating `APICallError` preserved as cause
