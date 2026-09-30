## ADDED Requirements

### Requirement: Foundation sequencing and explicit acceptance handoff
The #303 foundation SHALL be independently green and completable before #280, followed by #238, #239 and #240. It SHALL deliver restriction dispositions, non-metadata warning/source-identity/display/response-identity behavior, supported Anthropic caller request policy and minimal actionable provider diagnostics using current API/lifecycle. It SHALL NOT require successor metadata transport/continuation, future tool helpers or #201's undelivered matrix as foundation completion gates. Full original refactor acceptance SHALL remain explicitly mapped to subsequent owners rather than be claimed or silently dropped.

#280 SHALL exclusively implement ordinary supported metadata server/client/schema transport, unknown metadata/presence/budgets and actual-output continuation/source-metadata integration. Desired metadata semantics SHALL be independent of IncludeRawChunks, but unimplemented target transport SHALL remain a handoff, not a foundation normative sync claiming delivery. Customer ownership SHALL NOT imply BYOK provisioning, credential substitution, unsupported tools/MCP, effectful fallback or relaxed bounds/tenant/telemetry guards. Historical milestone acceptance SHALL remain historical.

#### Scenario: Foundation completes before metadata
- **WHEN** #303's implemented foundation behavior and current registered checks pass without #280 or #201's harness
- **THEN** foundation completion SHALL be reported separately from metadata/actual continuation/full-matrix acceptance
- **AND** successors SHALL retain their explicit handoffs and implementation ownership

### Requirement: Reviewed restriction dispositions
Every request/response restriction SHALL have a concrete keep/remove/defer classification by protocol correctness, capability support, credential/tenant authorization, telemetry or obsolete account concealment. Current allowlist membership SHALL NOT establish relevance or harmless omission. A supported consumed option filtered today SHALL be corrected or explicitly handed off with scope/reason/owner; genuinely ignored fields require pinned consumer evidence. Unknown/effectful provider extensions SHALL not bypass registered mapped roles/unions/tools/credentials.

#### Scenario: Consumed caller field is missing
- **WHEN** native conversion consumes caller on already-supported assistant function history but the command policy removes it
- **THEN** the foundation SHALL restore that request semantic and prove it independently from response metadata

#### Scenario: Fallback cannot route consumed options
- **WHEN** active options are consumed by a fallback candidate but no safe common routing is supported
- **THEN** the request SHALL fail explicitly before physical invocation rather than succeed with silently lost semantics

### Requirement: Minimal provider diagnostics without lifetime or public API redesign
The foundation SHALL preserve reviewed actionable provider diagnostics for trusted configured direct routes through existing unary/setup/stream-error paths. Diagnostic authority SHALL derive from trusted adapter configuration, not caller namespaces/body claims or logical grafana identity. Unconfigured/unreviewed routes, ambiguous fallback aggregates, message-only/local/internal/transport sources SHALL retain safe diagnostics with explicit support gaps. Fixed host authentication/permission/internal failures SHALL remain distinct from provider-account failures. No new stream reader/lifecycle layer or public error API redesign SHALL be required for this foundation.

Projection SHALL use bounded reviewed structured source fields, not generic Error()/APICallError.Message, whole Data/ResponseBody, URLs/headers/request bodies or causes. Source and complete error SHALL be at most 16384 bytes, message/projected detail at most 4096 bytes, type/string-code at most 256 bytes, with original-byte UTF-8/JSON/number validation and complete enclosing server bounds before output. Client limits remain independent. Malformed/oversized/unreviewed detail SHALL fall back safely, not be truncated or copied generically.

The existing registered message/type/code/param envelope and existing Go GatewayError.Code string/API SHALL remain. Public category code SHALL remain a Gateway string; reviewed native type/code/parameter details, including number/null codes where represented, SHALL use bounded existing param/cause/body access. No new typed properties or full native top-level-code parity SHALL be claimed. Credential/auth prose or parameters that may echo keys SHALL use fixed provider-account authorization prose with protected sources excluded, not Gateway-key guidance. Ordinary application strings SHALL not be heuristically censored.

#### Scenario: Direct structured provider failure
- **WHEN** a trusted direct adapter supplies reviewed structured diagnostic fields within bounds
- **THEN** both clients SHALL preserve the minimal contracted message/status/native details through existing fields/cause/body
- **AND** internal causes, native transport and credential-bearing sources SHALL remain excluded

#### Scenario: Unsupported richer error source
- **WHEN** trusted diagnostics need invasive representation/transport/API work or aggregate provenance unavailable in the existing path
- **THEN** fixed-safe behavior and a concrete separately registered Gateway handoff coordinated with #299 SHALL remain
- **AND** the foundation SHALL not invent a sentinel-tree protocol or add stream lifetimes/public API breaks to hide that gap

### Requirement: Status retry and lifecycle authority
Reviewed direct provider non-2xx status SHALL be preserved: 400/422 use invalid_request_error, 429 rate_limit_exceeded, remaining 4xx failed_dependency and 5xx internal_server_error. HTTP retry SHALL match the pinned 408/409/429/5xx rule; nonrepresentable provider retry overrides SHALL be documented, not tunneled through new flags/status fiction. Existing available direct stream errors SHALL retain ordered non-terminal provider behavior and reviewed statusCode/retryable without changing core termination. Original errors.As/Is/native fallback eligibility and commitment SHALL remain effective. No implicit client/fallback replay after output or exactly-once generation promise SHALL be introduced.

#### Scenario: Native conflict is retryable
- **WHEN** a reviewed direct provider returns HTTP 409
- **THEN** its status and pinned retryability SHALL survive rather than become nonretryable 424

#### Scenario: Paid-generation adaptation fails
- **WHEN** output adaptation or transport fails at precommit/postcommit boundaries
- **THEN** existing safe failure/cleanup/finish authority SHALL remain and caller retry call counts SHALL be exposed without exactly-once claims

### Requirement: Debug privacy telemetry and independently green delivery
Supported debugging SHALL remain client-owned Gateway request and bounded HTTP response headers/body, registered actual response identity with typed unary overwrite caveat, and delivered reviewed diagnostics. Native provider transport SHALL NOT be tunneled through metadata/debug fields. Credentials/other-tenant/configuration/topology protections SHALL rely on source/authorization boundaries rather than arbitrary secret-looking string identification.

Caller content/metadata/warnings/source/actual identity/actionable error fields SHALL NOT enter logs, metric labels or metadata-only Agent Observability, including internal teams. Apache client SHALL remain independent of AGPL implementation/DTOs; candidate-source/image proof and public producer releases/consumer readonly adoption SHALL remain distinct. Strict server schema, exact-pinned TS/Go/native-request, synthetic lifecycle/security and authentic conformance evidence SHALL remain separate. Each owner SHALL sync/archive only its implemented behavior; #201 SHALL own later full authentic replay without weakened goldens or duplicate harness.

#### Scenario: Useful caller data is not telemetry
- **WHEN** foundation fields carry unique application markers and protected transport carries credential markers
- **THEN** contracted caller values SHALL survive while protected sources and all arbitrary markers SHALL remain absent from telemetry
- **AND** scoped fixture tests SHALL not be described as implemented BYOK storage isolation
