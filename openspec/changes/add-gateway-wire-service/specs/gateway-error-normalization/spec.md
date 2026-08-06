## MODIFIED Requirements

### Requirement: Normalized gateway error with type discriminator

The Grafana provider SHALL export `GatewayError` with `Type GatewayErrorType`, `Message`, `StatusCode`, and optional `ModelID`. It SHALL implement `error`, preserve the originating `*provider.APICallError` as its private cause, and retain the registered category constants:

- `authentication_error`
- `invalid_request_error`
- `rate_limit_exceeded`
- `model_not_found`
- `forbidden`
- `failed_dependency`
- `internal_server_error`

The strict V4 handler is not required to produce policy-only categories, but strict decoding SHALL remain compatible with the registered Grafana vocabulary.

#### Scenario: Gateway error implements error

- **WHEN** `*GatewayError` is assigned to `error`
- **THEN** compilation SHALL succeed and `Unwrap` SHALL expose its API-call cause

#### Scenario: Gateway type remains named

- **WHEN** the `Type` field is inspected
- **THEN** it SHALL use named type `GatewayErrorType` with the registered string constants

### Requirement: Normalizer maps structured provider error to a normalized type

`NormalizeAPICallError` SHALL read a structured type from `APICallError.Data` and fall back to `ResponseBody`. It SHALL map authentication and permission to authentication; invalid request and billing to invalid request; rate-limit and overloaded signals to rate limit; model-not-found and not-found to model not found; `forbidden` to forbidden; `failed_dependency` to failed dependency; and internal, API, timeout, missing, or unknown types to internal server error.

The originating API-call error SHALL remain the cause with status and retryability intact. Strict stream errors SHALL carry only the safe category envelope in `Data`, never the original provider data.

#### Scenario: Failed dependency preserves retryability

- **WHEN** safe structured data contains `failed_dependency`
- **THEN** normalization SHALL return `GatewayErrorFailedDependency` and the caused API error SHALL retain explicit retryability

#### Scenario: Model not found preserves public ID

- **WHEN** safe structured data contains `model_not_found` and `param.modelId`
- **THEN** normalization SHALL populate `GatewayError.ModelID` with that public ID

#### Scenario: Registered forbidden type decodes

- **WHEN** strict decoding receives a registered `forbidden` envelope
- **THEN** normalization SHALL return `GatewayErrorForbidden` even though the simplified handler has no policy producer

#### Scenario: Response body remains fallback

- **WHEN** `Data` is empty and `ResponseBody` contains a structured registered type
- **THEN** normalization SHALL use that body type

#### Scenario: Unknown type becomes internal

- **WHEN** no registered type can be recovered
- **THEN** normalization SHALL return `GatewayErrorInternalServer` with the API-call cause preserved
