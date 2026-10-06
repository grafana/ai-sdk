## MODIFIED Requirements

### Requirement: Error reconstruction preserves retry semantics

The provider SHALL surface server and transport errors as
`*provider.APICallError` where possible. Server-side provider failures SHALL
cross the HTTP boundary as JSON `provider.APICallError` values. If a server or
transport failure cannot be decoded as `provider.APICallError`, the client SHALL
synthesize one with best-effort status, URL, response body, response headers,
cause, and retryability.

The provider SHALL preserve the decoded error's structured payload in
`APICallError.Data` end-to-end (the field already round-trips on the wire). As
the gateway analog of the Vercel AI SDK gateway, the provider SHALL run the
`provider` package normalizer on the decoded error and, when a category is
identified, surface a `*provider.GatewayError` carrying the originating
`*provider.APICallError` as its cause. When no category can be identified the
provider SHALL surface the plain `*provider.APICallError`. Either way,
`errors.As(&provider.APICallError{})` SHALL still yield the decoded status,
headers, body, `Data`, and `IsRetryable`.

#### Scenario: Server returns a retryable error

- **WHEN** the server returns or streams an API-call error payload with `isRetryable: true`
- **THEN** the provider surfaces an error from which `errors.As` yields a `*provider.APICallError` with `IsRetryable == true`, and `aisdk.StreamText` retries according to its retry configuration

#### Scenario: Non-retryable error

- **WHEN** the server returns or streams an API-call error payload with `isRetryable: false`
- **THEN** the provider surfaces an error from which `errors.As` yields a `*provider.APICallError` with `IsRetryable == false` and `aisdk.StreamText` does not retry

#### Scenario: Decoded error preserves Data

- **WHEN** the wire error payload carries a structured error in `data`
- **THEN** the surfaced `*provider.APICallError.Data` SHALL equal the decoded `data` payload

#### Scenario: Categorized error surfaces a GatewayError

- **WHEN** the decoded error's structured type maps to a normalized category (e.g. `rate_limit_exceeded`, `authentication_error`, `model_not_found`)
- **THEN** the provider surfaces a `*provider.GatewayError` with the corresponding `Type`, and `errors.As(&provider.APICallError{})` still yields the decoded `*provider.APICallError`

#### Scenario: Uncategorized error stays a plain APICallError

- **WHEN** the decoded error carries no identifiable structured type
- **THEN** the provider surfaces a plain `*provider.APICallError` as before
