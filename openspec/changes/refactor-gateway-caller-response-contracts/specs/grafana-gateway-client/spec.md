## MODIFIED Requirements

### Requirement: Bounded normalized unary consumption
Successful unary responses SHALL require one complete bounded UTF-8 JSON document and strict supported content/finish/known-usage mapping. Request.Body and Response.Headers/Body SHALL remain client-owned Gateway transport replacing server-native typed fields. Supplied registered actual unary identity SHALL remain available only through bounded Response.Body, matching the pinned TS overwrite rather than new typed fields. Warnings SHALL preserve valid registered values/order and absent/null default to a non-nil empty slice, with required empties and optional-empty Go normalization.

Current metadata consumption/transport and raw usage SHALL remain unchanged in this foundation. #280 exclusively owns metadata client/server/schema extensions and continuation; no new placement/helper/gate belongs here. Unsupported content/execution or malformed fields SHALL retain bounded protocol failure without partial results.

#### Scenario: Foundation unary response
- **WHEN** the server returns valid supported content/usage/finish/warnings and actual response identity
- **THEN** warnings and current supported content SHALL survive with local transport and raw-body-only native identity
- **AND** future metadata transport SHALL remain a named successor requirement rather than presumed delivered

### Requirement: Closed Gateway error classification
Non-2xx responses SHALL remain bounded and use registered closed Gateway categories. The existing GatewayError.Code string, Category/Message/StatusCode/IsRetryable and underlying bounded APICallError API SHALL remain unchanged. Reviewed minimal direct-provider envelopes SHALL preserve native status/category/retry and bounded native type/code/parameter through existing param and underlying public envelope/cause/body; no new Param property or JSON-valued Code API SHALL be introduced. Public category code remains a Gateway string; richer native top-level/typed-code equivalence is an explicit separately registered boundary.

Strict fixed host/internal errors and minimal reviewed provider projections SHALL be distinguished. Provider auth SHALL not become Gateway-key guidance. Private decoding MAY accept reviewed status/param combinations beyond historical fixed documents while retaining strict category/shape/bounds; unknown/malformed/oversized/unreviewed responses SHALL produce bounded local protocol errors. Context identity/errors.As/Is, one HTTP call and existing stream ownership SHALL remain effective.

#### Scenario: Minimal provider error is consumed
- **WHEN** a reviewed direct-provider failure carries valid native status and bounded native detail in param
- **THEN** both clients SHALL expose the contracted category/message/status/retry and existing cause/body detail without public API redesign

#### Scenario: Error body is malformed
- **WHEN** media type, schema or byte/detail limits fail
- **THEN** the client SHALL return bounded local error text without copying raw response bytes into the primary message

#### Scenario: Provider auth differs from Gateway auth
- **WHEN** a provider account fails authorization
- **THEN** the existing failed-dependency category and safe actionable provider-account prose SHALL be used, not Gateway credential guidance

### Requirement: No implicit client retry or backend selection
The Grafana client SHALL issue at most one Gateway request per invocation and SHALL not select backends/candidates or replay after response/event delivery. Existing SDK retryability remains caller-owned. Registered actual response identity/source display and delivered minimal provider diagnostics SHALL remain caller data, not suppressed solely because they distinguish providers. Credentials/another tenant/configuration/fallback topology SHALL remain excluded; logical model identity remains the requested route.

#### Scenario: Retryable provider setup failure
- **WHEN** a reviewed setup failure is retryable under the pinned status rule
- **THEN** one invocation SHALL return that error after one HTTP request and leave retry to its caller

### Requirement: Exact-pinned differential and black-box evidence
Exact registered TS and independent Go tests SHALL prove foundation warnings/source ID/display/response identity with typed unary limitation, minimal errors/retry/cause-body, caller-supplied function history/native request, existing cancellation/bounds/discovery/DONE/raw/timestamp/EOF semantics. Apache production code SHALL not import AGPL DTOs/validators. Hostile strict-schema/lifecycle tests remain separate from permissive TS consumption. Metadata response/actual roundtrip and full authentic two-client matrix belong to #280/#201, not foundation gates.

#### Scenario: Caller history is supplied independently
- **WHEN** both clients send supported assistant function history containing caller through the authenticated command
- **THEN** captured native Anthropic caller semantics SHALL match the registered consumer without a prior response/helper
- **AND** this SHALL be reported as request evidence, not metadata response-roundtrip proof

### Requirement: Source response consumption
The independent Go client SHALL preserve provider source IDs/display/order in supported URL/document unary/stream parts while retaining required empty document title and optional-empty normalization. Current object-valued source metadata decoding/bounds SHALL remain unchanged until #280's successor integration. Missing fields/unknown variants/invalid values retain bounded protocol errors; no server implementation import is allowed.

#### Scenario: Native source identity survives
- **WHEN** registered URL/document sources arrive with repeated/native IDs and file_path display values
- **THEN** those values SHALL survive without source-N or Document substitution and without claiming future source-metadata transport
