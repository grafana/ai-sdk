## ADDED Requirements

### Requirement: Explicit restriction ownership and account boundary
The Gateway SHALL classify request/response restrictions as protocol correctness, capability support, tenant/credential security, telemetry policy or obsolete shared-account concealment, and record retained, removed and deferred dispositions with concrete rationales. Customer provider-account ownership SHALL NOT imply caller authorization to override credentials, access another tenant's state, execute unsupported tools, bypass resource limits or enable effectful fallback. Current startup-configured accounts SHALL NOT be described as implemented BYOK provisioning. Historical acceptance of completed milestones SHALL remain historical.

#280 SHALL exclusively own supported provider-metadata codecs, schema changes, independent-client metadata filtering and subsequent-request continuation implementation. #303 SHALL own warning/source-identity/response-identity/error and restriction policy changes and integrate the #280 contract without duplicating its implementation. Metadata SHALL be ordinary supported response data, independent of IncludeRawChunks.

#### Scenario: Account ownership changes response policy
- **WHEN** a restriction exists solely to hide the selected provider account from its authorized caller
- **THEN** its inventory disposition SHALL remove that concealment rather than relabel it security
- **AND** separate protocol, authorization and telemetry protections SHALL remain effective

#### Scenario: Metadata work is integrated
- **WHEN** a #303 behavior package needs supported source, text, tool, reasoning or finish metadata
- **THEN** it SHALL depend on #280's reviewed contract and implementation rather than introduce another metadata projection or gate

### Requirement: Trusted per-candidate error provenance
The AGPL Gateway SHALL install a reviewed adapter-policy model wrapper around each physical candidate before fallback composition and logical identity wrapping. Policy SHALL originate from trusted provider configuration, not caller input, logical model identity, error-body claims or another candidate. A server-only internal providererrors package SHALL own adapter sentinels, wrapping and projection selection; Apache provider/core/client contracts SHALL remain unchanged. An unconfigured/generic resolver or zero/unreviewed policy SHALL have no authority to disclose native diagnostics and SHALL retain fixed safe projection.

Unary and stream-setup errors SHALL use a standard two-child errors.Join node pairing the exact private adapter sentinel with that invocation's original error. Streaming PartError values SHALL be copied and their concrete APICallError recreated with unchanged exported values and explicit native retry flag, with that same paired node as Cause. Original objects SHALL NOT be mutated; provenance SHALL NOT be stored in Data, providerMetadata or public fields. Existing errors.As/Is, context-window recognition, retry eligibility and result-plus-error cleanup SHALL remain effective. The wrapper SHALL preserve physical identity for private observation without adding logical middleware.

Projection SHALL use bounded structural traversal and bind the selected policy and API source to the same paired original-error subtree. Cancellation/deadline priority SHALL remain authoritative. Exhausted fallback SHALL project only the first candidate-failure branch in its existing newest-first aggregate order; if that branch lacks a reviewed source, fixed safe classification SHALL apply rather than scanning older candidates. Separate aggregate-wide searches for policy and API errors, aggregate prose serialization and candidate enumeration SHALL be forbidden. Malformed, cyclic or over-budget traversal SHALL fail safely.

One bounded context-aware wrapper forwarding owner SHALL read and drain each wrapped source channel; fallback/handler SHALL own only its output channel. Non-nil result streams SHALL retain this ownership even when accompanied by setup errors. Nil error pointers and non-error parts SHALL retain existing behavior. Wrapping SHALL NOT synthesize parts, change first-part commitment, replay after output, change core termination or introduce duplicate channel readers/cleanup owners.

#### Scenario: Heterogeneous fallback is exhausted
- **WHEN** differently configured candidates fail with different structured error schemas before commitment
- **THEN** only the newest authoritative candidate's paired policy and original error SHALL determine public diagnostics
- **AND** no other candidate's marker, API source, identity or aggregate prose SHALL be combined into that projection

#### Scenario: Authoritative failure has no reviewed source
- **WHEN** the newest failure is unconfigured, premature EOF, invalid result or otherwise lacks reviewed provenance
- **THEN** fixed safe diagnostics SHALL apply without searching an older failure for an actionable message

#### Scenario: Generic resolver returns an API-shaped error
- **WHEN** an unwrapped model returns an error body resembling a reviewed provider schema
- **THEN** its shape or logical Provider value SHALL NOT authorize native diagnostic disclosure

#### Scenario: Stream error is copied without changing ownership
- **WHEN** a wrapped candidate emits a PartError followed by valid parts while its error is also reused by another invocation
- **THEN** the wrapper SHALL annotate a copy bound to that invocation's policy without modifying either original or later part order
- **AND** fallback SHALL commit on the first part with one reader per channel and bounded cancellation/drain

#### Scenario: Native decision facts survive wrapping
- **WHEN** an original error is nonretryable, signals context-window overflow or wraps recognizable cancellation
- **THEN** errors.As/Is and existing fallback decisions SHALL retain those facts rather than use sanitized public diagnostics to decide execution

### Requirement: Reviewed bounded provider error projection
The Gateway SHALL separate reviewed provider API failures from Gateway-internal, host authentication/authorization, transport, panic, resolver and adaptation failures. Internal/host failures SHALL retain fixed safe documents. Provider diagnostics SHALL be extracted only from the selected trusted adapter's reviewed structured error sources, never generic error strings, APICallError.Message, URLs, headers, request bodies, causes or whole opaque bodies/data. Reviewed sources SHALL cover OpenAI/Azure HTTP and structured Responses errors, Anthropic HTTP/structured SSE and reviewed OpenAI-compatible envelopes; unsupported/message-only/local conversion sources SHALL use fixed safe fallback and have their gap recorded.

Structured source bytes SHALL be capped at 16384 before JSON parsing and validated for UTF-8/syntax. Public message SHALL be at most 4096 UTF-8 bytes, native type and string code at most 256 bytes each, projected param at most 4096 encoded bytes, and complete error at most 16384 bytes and within its enclosing response/event budget. Code SHALL preserve reviewed string, finite JavaScript-safe number, null and absence normalized to null. Oversized/malformed diagnostic fields SHALL not be truncated or generically exposed; fixed prose/code/null detail SHALL retain valid status/category/retry classification. Invalid HTTP status SHALL select the fixed internal failure.

The registered envelope SHALL use message, Gateway-category type, code and param only. Provider param SHALL be a bounded closed projection with providerStatusCode and optional providerType and providerParam (string or null); native param absence SHALL remain distinct from explicit null. Unknown native siblings/details SHALL be omitted, not recursively copied. Provider credential/auth failures SHALL use fixed actionable `provider account authorization failed` prose and omit arbitrary native message/param while retaining reviewed status/type/code. Protection SHALL rely on provenance/owned credential sources, not identifying arbitrary secret-looking application strings.

#### Scenario: Structured actionable provider error
- **WHEN** a reviewed provider error carries a valid message, native type, numeric code and scalar parameter within all bounds
- **THEN** clients SHALL receive those reviewed diagnostics at the existing registered fields with no native transport or internal cause
- **AND** pinned TS code/param access SHALL be demonstrated through its bounded cause/body rather than claimed as typed public properties

#### Scenario: Credential material and application content differ
- **WHEN** a provider auth error can echo a credential and ordinary caller metadata contains a harmless token-shaped string
- **THEN** the auth error SHALL use fixed actionable prose with credential-bearing fields excluded
- **AND** ordinary supported metadata SHALL remain unchanged under #280 rather than being heuristically censored

#### Scenario: Unsupported or over-limit source
- **WHEN** an error has only a generic message or an oversized, malformed or unreviewed structured source
- **THEN** only fixed safe diagnostics SHALL be emitted, preserving valid status-derived classification without exposing source bytes

### Requirement: Registered retry and lifecycle boundary
Reviewed provider non-2xx HTTP status SHALL be preserved: 400/422 map to invalid_request_error, 429 to rate_limit_exceeded, remaining 4xx to failed_dependency, and 5xx to internal_server_error. Provider credential failure SHALL NOT become Gateway authentication guidance, and native provider model lookup failure SHALL NOT become public-route model_not_found. Unknown public routes and transport-only failures SHALL retain existing fixed classifications.

HTTP retryability SHALL match the pinned public client: 408, 409, 429 and 5xx are retryable. A provider override not representable by that client SHALL be explicitly tested and documented rather than tunneled through a new flag or status fiction. In-stream errors SHALL retain the existing statusCode/retryable fields and ordered non-terminal provider semantics, including valid status 200, without changing core termination. No client SHALL automatically replay an invocation after response/event delivery. Adaptation failures SHALL retain safe precommit JSON or postcommit terminal SSE behavior, including currently retryable HTTP 500; the Gateway SHALL NOT promise exactly-once generation.

#### Scenario: Provider status differs from old bucket
- **WHEN** a reviewed provider failure has HTTP 409
- **THEN** HTTP 409 and failed_dependency SHALL survive with pinned retryability true rather than becoming nonretryable 424

#### Scenario: Provider retry override is not representable
- **WHEN** a native nonretryable HTTP 500 reaches the pinned HTTP client
- **THEN** evidence SHALL show its status-derived retryability and explicitly record the override gap without changing the baseline or adding a dialect

#### Scenario: Error precedes valid stream completion
- **WHEN** a reviewed provider error appears between supported stream parts and a valid finish follows
- **THEN** its projection SHALL remain in order without altering active blocks or making it terminal at the provider boundary

#### Scenario: Adapter fails after paid generation
- **WHEN** unsupported or invalid output causes precommit adaptation failure, or a committed stream violates bounds/lifecycle
- **THEN** the Gateway SHALL retain its safe error boundary and cleanup/finish authority
- **AND** tests SHALL expose caller retry call counts and SHALL NOT imply exactly-once execution

### Requirement: Supported debugging and caller telemetry separation
Clients SHALL own Gateway request metadata and bounded Gateway response headers/body. Native provider transport (URLs, headers, request/response bodies, SDK dumps and causes) SHALL NOT be restored, tunneled through metadata or promised as raw passthrough. Registered response identity and reviewed bounded provider errors SHALL remain supported diagnostic data. Pinned typed unary response overwrite SHALL be explicitly described; native unary identity SHALL remain inspectable only in the bounded raw response body. No new provider/topology/debug field SHALL be invented.

Authorization/tenant and known credential-source protections SHALL remain independent from response semantics. Caller content, provider metadata, warning prose, source data, actual response identity and provider error details SHALL NOT enter logs, metric labels or metadata-only Agent Observability, including internal-team calls. Discovery SHALL remain an authorized public route catalog, not an operator dump. Tests SHALL distinguish fixture-context isolation from unimplemented per-customer credential lifecycle.

#### Scenario: Caller receives useful response data
- **WHEN** supported output carries unique markers in metadata, warnings, sources and actual identity
- **THEN** authorized caller fields SHALL retain their contracted semantics while logs, labels and metadata-only exports SHALL contain none of those markers

#### Scenario: Two fixture contexts differ
- **WHEN** two authenticated test contexts have distinct authorized/private state and credential markers
- **THEN** neither response nor telemetry SHALL disclose the other context's private state or provider auth material
- **AND** evidence SHALL NOT claim live BYOK provisioning or storage isolation

### Requirement: Independently green contract delivery and provenance
Each behavior package SHALL update its normative contracts, DTO/schema, independent client mappings, docs and tests together and pass required checks against the registered baseline. #280-dependent packages SHALL not claim metadata implementation or weaken tests before that dependency lands. Apache client code SHALL not import AGPL server DTOs/implementation. Public producer releases SHALL precede consumer pin adoption with GOWORK=off readonly proof; same-revision Gateway image/workspace delivery SHALL remain distinct.

Full authentic two-client replay acceptance SHALL depend on delivery of #201's currently undelivered harness and use its then-registered commands, not guessed task names or a duplicate local harness. Current focused ProviderWire/command and direct conformance proof SHALL remain independently green and SHALL NOT be reported as that full matrix. Existing authentic provider inputs and unchanged direct expectations SHALL be replayed through both clients where applicable once the prerequisite is delivered. Synthetic error/credential/transport/lifecycle cases SHALL remain focused tests, not recorded/upstream provider evidence. Raw strict server schemas, exact-pinned client differential/command tests, subsequent native provider requests and applicable schema-parsed UI integration SHALL provide separate proof. Remaining coverage/support gaps SHALL be reported and actionable follow-ups linked; PARITY.md SHALL contain stable evidence boundaries, not a dated issue catalog.

#### Scenario: Published consumer is accepted
- **WHEN** a behavior package changes a public producer needed by the independent client or command
- **THEN** acceptance SHALL distinguish candidate-source/image checks from published module resolution and SHALL require the relevant released pins and readonly consumer check

#### Scenario: Full replay harness is not delivered
- **WHEN** #201 remains undelivered in the checked worktree
- **THEN** focused packages SHALL use current registered validation without claiming full matrix acceptance
- **AND** the missing harness SHALL remain an explicit external prerequisite rather than be recreated or bypassed

#### Scenario: Regression input is synthetic
- **WHEN** a transport fault or credential-bearing provider failure is synthesized
- **THEN** it SHALL exercise a focused deterministic test without modifying authentic input or direct expected goldens to conceal a difference
