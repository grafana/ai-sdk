# gateway-caller-response-policy Specification

## Purpose
Define delivered caller-visible diagnostic boundaries without changing Gateway execution ownership, public error APIs or stream lifetimes.

## Requirements

### Requirement: Minimal provider diagnostics without lifetime or public API redesign
The foundation SHALL preserve reviewed actionable provider diagnostics for trusted configured direct routes through existing unary/setup/stream-error paths. Diagnostic authority SHALL derive from trusted adapter configuration, not caller namespaces/body claims or logical grafana identity. Unconfigured/unreviewed routes, ambiguous fallback aggregates, message-only/local/internal/transport sources SHALL retain safe diagnostics with explicit support gaps. Fixed host authentication/permission/internal failures SHALL remain distinct from provider-account failures. No new stream reader/lifecycle layer or public error API redesign SHALL be required.

Projection SHALL use bounded reviewed structured source fields, not generic Error()/APICallError.Message, whole Data/ResponseBody, URLs/headers/request bodies or causes. Source and complete error SHALL be at most 16384 bytes, message/projected detail at most 4096 bytes, type/string-code at most 256 bytes, with original-byte UTF-8/JSON/number validation and complete enclosing server bounds before output. Client limits remain independent. Malformed/oversized/unreviewed detail SHALL fall back safely, not be truncated or copied generically.

The existing registered message/type/code/param envelope and existing Go GatewayError.Code string/API SHALL remain. Public category code SHALL remain a Gateway string; reviewed native type/code/parameter details, including number/null codes where represented, SHALL use bounded existing param/cause/body access. No new typed properties or full native top-level-code parity SHALL be claimed. Credential/auth prose or parameters that may echo keys SHALL use fixed provider-account authorization prose with protected sources excluded, not Gateway-key guidance. Ordinary application strings SHALL not be heuristically censored.

#### Scenario: Direct structured provider failure
- **WHEN** a trusted direct adapter supplies reviewed structured diagnostic fields within bounds
- **THEN** both clients SHALL preserve the minimal contracted message/status/native details through existing fields/cause/body
- **AND** internal causes, native transport and credential-bearing sources SHALL remain excluded

#### Scenario: Unsupported richer error source
- **WHEN** trusted diagnostics need invasive representation/transport/API work or aggregate provenance unavailable in the existing path
- **THEN** fixed-safe behavior and an explicit Gateway follow-up candidate coordinated with #299 SHALL remain
- **AND** separately registering that work SHALL require mutation authority; local identification SHALL not be reported as completed registration

### Requirement: Status retry and lifecycle authority
Reviewed direct provider non-2xx status SHALL be preserved: 400/422 use invalid_request_error, 429 rate_limit_exceeded, remaining 4xx failed_dependency and 5xx internal_server_error. HTTP retry SHALL match the pinned 408/409/429/5xx rule; nonrepresentable provider retry overrides SHALL be documented, not tunneled through new flags/status fiction. Existing available direct stream errors SHALL retain ordered non-terminal provider behavior and reviewed statusCode/retryable without changing core termination. Original errors.As/Is/native fallback eligibility and commitment SHALL remain effective. No implicit client/fallback replay after output or exactly-once generation promise SHALL be introduced.

#### Scenario: Native conflict is retryable
- **WHEN** a reviewed direct provider returns HTTP 409
- **THEN** its status and pinned retryability SHALL survive rather than become nonretryable 424

#### Scenario: Reviewed error is followed by valid stream output
- **WHEN** a direct provider supplies reviewed structured PartError evidence followed by valid content and finish
- **THEN** the error and later parts SHALL remain ordered through existing reader and cleanup ownership
