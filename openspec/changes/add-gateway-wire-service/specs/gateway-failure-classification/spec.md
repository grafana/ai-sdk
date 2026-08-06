## ADDED Requirements

### Requirement: Stable gateway failure categories

The repository SHALL provide a transport-neutral `gateway/failure` package with a typed `Kind` and category sentinels for unauthenticated, invalid call, unknown model, forbidden, rate limited, timeout, canceled, failed dependency, and internal failure. The package MUST NOT import `net/http`, a protocol adapter, or provider-wire DTOs.

#### Scenario: Category is protocol neutral

- **WHEN** a runtime or call policy classifies an error
- **THEN** it SHALL identify one stable failure kind without selecting an HTTP status or protocol error envelope

#### Scenario: Dependency boundary

- **WHEN** imports in `gateway/failure` are inspected
- **THEN** they SHALL NOT include `net/http`, `gateway/providerwire`, `gateway/providerwire/v4`, or a frontend orchestration package

### Requirement: Category wrapping preserves the private cause

The failure package SHALL wrap an originating error with a category sentinel without defining a custom error implementation. A categorized error MUST remain matchable through `errors.Is`, and its original cause MUST remain reachable through `errors.Is` or `errors.As`. A nil cause SHALL still produce an error matching the selected category.

Retryability MUST NOT be stored as an inherited `errors.Join` marker. Categorizing or wrapping an error SHALL preserve private cause inspection without making a nested retry decision authoritative at every outer boundary.

#### Scenario: Category remains inspectable

- **WHEN** a provider failure is categorized as failed dependency
- **THEN** `errors.Is(err, failure.ErrFailedDependency)` SHALL return true

#### Scenario: Original API error remains private and inspectable

- **WHEN** a `*provider.APICallError` is categorized and wrapped
- **THEN** `errors.As(err, &apiErr)` SHALL reach the original value without requiring it to be serialized publicly

#### Scenario: Retryability is not inherited as a sentinel

- **WHEN** a retryable provider cause is wrapped by a deterministic non-retryable runtime failure
- **THEN** the outer classification SHALL be able to derive non-retryable without an uncleared retryable marker in the error chain

### Requirement: Classification is a derived non-error value

The failure package SHALL derive a `Classification` value at the boundary where an error's meaning is known. `Classification` SHALL NOT implement `error`. It SHALL contain one `Kind`, a `Retryable` boolean, the private originating `Cause`, and typed allowlisted `SafeParameters`. Safe parameters MAY include the caller-owned requested public model ID or another explicitly approved public policy identifier; they MUST NOT contain arbitrary maps copied from provider errors.

Retryability SHALL be computed fresh from explicit boundary context and trusted causes. An outer, more authoritative boundary SHALL be able to override retryability inferred from a nested cause. Classifications MUST NOT select HTTP status, JSON envelope shape, SSE framing, or protocol error codes.

#### Scenario: Outer boundary clears retryability

- **WHEN** a nested cause was retryable but the active boundary identifies a deterministic encoding or size failure
- **THEN** the derived classification SHALL have `Retryable == false`

#### Scenario: Safe parameters are allowlisted

- **WHEN** unknown-model classification includes the requested public model ID
- **THEN** the value SHALL be stored in a typed safe parameter and no backend identity or provider data SHALL be copied

#### Scenario: Classification remains separate from error

- **WHEN** callers inspect a `Classification`
- **THEN** private error traversal SHALL use `Classification.Cause` while protocol mapping SHALL use kind, retryability, and safe parameters

### Requirement: Deterministic classification precedence

Classification SHALL return one deterministic `Kind` when an error matches multiple category sentinels. Explicit call-policy categories SHALL take precedence over inferred provider status categories; cancellation and timeout SHALL take precedence over generic internal or dependency classifications. Retryability SHALL be derived after kind precedence is resolved.

#### Scenario: Timeout wraps a provider failure

- **WHEN** an error matches both timeout and failed-dependency categories
- **THEN** classification SHALL return timeout kind and derive retryability for timeout at that boundary

#### Scenario: Explicit forbidden policy wraps another error

- **WHEN** call policy joins a forbidden category with an internal cause
- **THEN** classification SHALL return forbidden kind without inheriting internal retryability

### Requirement: Boundary-aware runtime classification

The runtime SHALL classify errors according to where they originate. `catalog.ErrUnknownModel` SHALL become unknown model. Request cancellation and total deadline expiry SHALL become canceled and timeout respectively. A provider rate limit SHALL become rate limited. Backend authentication, backend model-not-found, provider availability, provider transport failures, and otherwise unattributed provider 4xx responses SHALL become failed dependency rather than caller authentication, invalid public input, or public unknown model. Invalid call SHALL be used only for normalized-call validation or policy with trusted caller attribution. Unclassified runtime defects SHALL become internal failures.

Trusted transient provider/transport failures and timeouts SHALL derive retryable. Permanent backend credential/model failures, deterministic nil results, codec failures, size-limit failures, and unattributed provider 4xx responses SHALL derive non-retryable.

#### Scenario: Catalog route is absent

- **WHEN** resolution returns an error matching `catalog.ErrUnknownModel`
- **THEN** classification SHALL have unknown-model kind and safe requested public model ID

#### Scenario: Backend credentials fail

- **WHEN** a resolved provider returns HTTP 401 or 403 because backend credentials are invalid
- **THEN** classification SHALL have failed-dependency kind rather than unauthenticated

#### Scenario: Unattributed provider bad request is conservative

- **WHEN** a provider returns HTTP 400 without trusted caller attribution
- **THEN** classification SHALL have non-retryable failed-dependency kind rather than invalid-call kind

#### Scenario: Provider is rate limited

- **WHEN** a provider call returns a trusted rate-limit failure
- **THEN** classification SHALL have rate-limited kind and derived retryability

### Requirement: Protocol validation errors remain adapter owned

Method, media type, content negotiation, transport body size, and protocol DTO validation failures SHALL remain owned by each protocol adapter and SHALL NOT be forced through one runtime invalid-call-to-HTTP-400 mapping. An adapter MAY categorize a semantic normalized-call error for private consistency, but it SHALL select its own public status and envelope.

#### Scenario: V4 transport failures retain exact statuses

- **WHEN** the V4 adapter rejects method, `Accept`, request size, or `Content-Type`
- **THEN** it SHALL emit safe HTTP 405, 406, 413, or 415 respectively without asking the common failure package to choose that status

#### Scenario: Future Chat envelope remains local

- **WHEN** a future Chat adapter maps a failure
- **THEN** it MAY produce its own `message`, `type`, `param`, and `code` fields without changing common classification

### Requirement: Public error projection is safe by construction

A protocol adapter SHALL derive its public type, message, retry behavior, and optional parameters only from a `Classification`, adapter-local validation facts, and request-owned public values. It MUST NOT serialize `Classification.Cause`, an originating error string, provider URL, request body values, provider response headers/body, provider data, or backend identity. Hosts MAY use the private cause for internal logging.

#### Scenario: Provider API error is projected

- **WHEN** a provider failure contains URL, request values, response headers/body, and provider data
- **THEN** none of those fields or values SHALL appear in the public error payload

#### Scenario: Internal message is projected

- **WHEN** an internal cause contains a deployment-specific message
- **THEN** the public response SHALL use a stable adapter message rather than `err.Error()`

### Requirement: LanguageModelV4 gateway error mapping

The strict V4 adapter SHALL map runtime classifications to envelopes recognized by registered `@ai-sdk/gateway@4.0.33`. The inner error object SHALL contain safe `message`, recognized `type`, `statusCode`, and explicit `isRetryable`; it MAY contain a safe `param`. Runtime mappings SHALL be:

- unauthenticated: HTTP 401, `authentication_error`;
- invalid call: HTTP 400, `invalid_request_error`;
- unknown model: HTTP 404, `model_not_found`;
- forbidden: HTTP 403, `forbidden`;
- rate limited: HTTP 429, `rate_limit_exceeded`;
- timeout: HTTP 504, `internal_server_error`;
- canceled: HTTP 499, `internal_server_error`;
- non-retryable failed dependency: HTTP 424, `failed_dependency`;
- retryable failed dependency: HTTP 502, `failed_dependency`;
- internal: HTTP 500, `internal_server_error`.

The envelope's `isRetryable` SHALL equal `Classification.Retryable`. The pinned TypeScript client ignores that field and derives retryability from HTTP status, while Grafana reconstructs the explicit value. HTTP 500 internal failures remain retryable to the pinned TypeScript client even when classification and Grafana say false; this baseline asymmetry MUST be tested.

#### Scenario: Permanent failed dependency stays non-retryable

- **WHEN** classification is non-retryable failed dependency
- **THEN** V4 SHALL use type `failed_dependency`, status 424, and `isRetryable: false`, and both pinned clients SHALL observe non-retryable

#### Scenario: Transient failed dependency is retryable

- **WHEN** classification is retryable failed dependency
- **THEN** V4 SHALL use type `failed_dependency`, status 502, and `isRetryable: true`, and both pinned clients SHALL observe retryable

#### Scenario: Internal retryability differs by pinned client

- **WHEN** V4 emits a non-retryable internal failure as HTTP 500 with `isRetryable: false`
- **THEN** Grafana SHALL reconstruct non-retryable while the pinned TypeScript client SHALL classify HTTP 500 as retryable

#### Scenario: Unknown model carries safe parameter

- **WHEN** V4 emits unknown-model failure for public ID `alias-a`
- **THEN** it SHALL use type `model_not_found`, status 404, and `param.modelId == "alias-a"` without backend identity
