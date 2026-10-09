## MODIFIED Requirements

### Requirement: Canonical public identity on every logical surface
Configured-account logical model telemetry SHALL identify provider as `grafana` and model as the canonical public catalog ID from startup composition. Requested aliases, provider instance names, provider types, backend model IDs, provider response IDs, response-derived model identity, and routing topology SHALL NOT appear in logical logs, Prometheus labels, Agent Observability model fields, or Agent Observability metadata.

Provider response metadata SHALL remain unmodified for the protocol adapter and later private physical-attempt observation. Logical observation SHALL ignore that metadata rather than rewriting the provider result or stream part.

#### Scenario: Backend response identity differs
- **WHEN** canonical model `grafana/assistant` receives a result or stream metadata naming provider `anthropic`, a backend model ID, and a provider response ID
- **THEN** every logical log, metric, and Agent Observability record SHALL use only `grafana` and `grafana/assistant` for model identity
- **AND** none of the backend identity or response ID values SHALL occur on those surfaces
- **AND** the downstream ProviderWire handler SHALL receive the original result or stream parts unchanged

#### Scenario: Alias is requested
- **WHEN** a caller invokes alias `assistant` for canonical model `grafana/assistant`
- **THEN** no logical model telemetry SHALL use `assistant` as its model identity
- **AND** logical model telemetry SHALL use `grafana/assistant`

### Requirement: Privacy-safe structured model logs
The Gateway SHALL emit fixed structured start and one terminal record for unary and streaming text using the reusable logger middleware. It SHALL keep all capture flags and per-stream-part logging disabled and SHALL apply a Gateway-owned attribute allowlist followed by default secret-key redaction.

Allowed logical values SHALL be limited to fixed event/schema names, a bounded generated call/correlation ID, call type, access-policy-appropriate logical identity, closed outcome and error classifications, normalized status/retryability, duration, usage counters, unified finish reason, warning type/count, bounded stream part type/count, time to first output, and the trusted observation context. Logs SHALL omit prompt/output/reasoning text, tools, files, raw parts, request/response bodies, headers, provider options/metadata, raw finish reasons, arbitrary warning feature/message/detail strings, arbitrary error text, provider response metadata, credentials, configured-account backend details, response-derived identity, and topology. Bounded BYOK requested provider/model identity SHALL be permitted under the request-scoped observation requirement.

#### Scenario: Unary provider failure contains secrets
- **WHEN** a unary provider error contains a credential, provider URL, backend model ID, response body, and arbitrary message
- **THEN** the logical terminal log SHALL retain only its approved closed error classification and timing
- **AND** no secret or provider-originated value SHALL be emitted

#### Scenario: Stream contains payload and provider metadata
- **WHEN** a stream contains text, response metadata, provider metadata, a provider error part, usage, and finish
- **THEN** logical logs SHALL include only approved lifecycle summaries and counters
- **AND** no per-part log or payload/provider detail SHALL be emitted

### Requirement: Bounded-cardinality logical model metrics
The Gateway SHALL register the reusable Prometheus model middleware exactly once against the existing service-owned registry and expose its collectors on the existing unauthenticated `/metrics` route. Model metrics SHALL use requested identity mode.

For configured-account calls, labels SHALL be limited to operation, canonical configured public model identity, closed status and error classes, normalized status code (`100`–`599`, `none`, or `other`), unified finish reason, token type, and closed stream-part type. Caller, namespace, correlation ID, alias, region, application, provider instance, backend model, response ID, arbitrary error, and arbitrary stream values SHALL NOT be metric labels. Duplicate registration SHALL fail startup before readiness.

#### Scenario: Unary and streaming text are scraped
- **WHEN** authenticated unary and streaming calls complete and `/metrics` is scraped
- **THEN** the scrape SHALL expose request count, in-flight calls, duration, token usage, and applicable stream timing/count metrics for the canonical public model
- **AND** no forbidden label value SHALL occur

#### Scenario: Collector registration collides
- **WHEN** model collectors cannot be registered exactly once in the service registry
- **THEN** startup SHALL fail before listener binding and readiness

BYOK requests SHALL use a fixed BYOK model class and, if exposed, only bounded supported-provider/access-policy labels. Caller-selected native model IDs SHALL NOT become Prometheus labels or allocate per-model observer/metric caches. These BYOK rules SHALL NOT consult the configured catalog.

#### Scenario: Many BYOK model selectors
- **WHEN** requests use many distinct native model names for one supported BYOK provider
- **THEN** the model metric series count SHALL remain bounded independently of those names

### Requirement: Metadata-only Agent Observability recording
When Agent Observability export is enabled, the Gateway SHALL use one process-wide client configured for metadata-only content capture, recording middleware only, requested logical identity, and an allowlisted context provider. Hooks, experimental SDK features, and request-controlled capture changes SHALL remain disabled.

Each authenticated unary or streaming text call SHALL produce one generation containing its access-policy-appropriate logical model identity, generation mode, approved trusted metadata, normalized usage marked as cache-inclusive without adding cache buckets again, unified finish reason, timing including streaming first output when available, and a closed error classification when applicable. It SHALL omit input/output content, system prompts, detailed errors, response/provider IDs, configured-account backend details, provider options/metadata, raw artifacts, credentials, and topology. BYOK requested provider/model identity SHALL remain an allowed bounded logical identity, never a metric label.

The Agent Observability client MAY add only its fixed SDK provenance/content-capture metadata markers and a mirrored closed error category after the Gateway filter. These fixed client-owned fields SHALL NOT carry request input, provider detail, exporter configuration, or arbitrary error text.

#### Scenario: Successful text generation is recorded
- **WHEN** an authenticated unary or streaming text call succeeds with Agent Observability enabled
- **THEN** exactly one metadata-only generation SHALL be finalized with access-policy-appropriate logical identity, approved context, usage, finish, and timing
- **AND** its exported record and span SHALL contain no text payload or configured-account backend details

#### Scenario: Provider error is recorded safely
- **WHEN** a provider call or stream part carries an error containing private provider details
- **THEN** the Agent Observability generation SHALL retain only the metadata-only closed error state supported by the exporter
- **AND** provider error text and details SHALL not be exported

#### Scenario: Export is disabled explicitly
- **WHEN** Agent Observability export is explicitly disabled in a development or test configuration
- **THEN** logging and Prometheus observation SHALL continue
- **AND** the model call SHALL not start an Agent Observability generation

### Requirement: Operator capture is independent from returned native values

Gateway metadata-only logger, Prometheus, enrichment and Agent Observability observations SHALL retain their existing payload exclusions and access-policy-appropriate logical model identity when native warning/source/response identity values become caller-visible. Arbitrary warning strings, source ID/URL/title/filename and response ID/modelId SHALL NOT become operator attributes, metric labels or metadata-only exported payloads. The observer chain SHALL pass original results/parts unchanged to protocol encoding; operator capture restrictions SHALL NOT censor returned values. Known credentials and another tenant's state SHALL remain protected independently of ordinary application scalar/display values.

Consumer applications SHALL be able to configure existing reusable middleware independently around providers/grafana without enabling server capture. Tests SHALL use middleware.WrapLanguageModel with Middleware.WrapGenerate to inspect actual unary warnings/content/Response.Body and WrapStream to observe forwarded PartStreamStart/PartSource/PartResponseMeta through a context-aware test tee. These hooks SHALL receive contracted values without mutating output; hook access SHALL NOT be equated with automatic built-in capture/export.

A separately configured consumer logger.Middleware with CaptureOptions.ResponseBody and a consumer-owned slog destination SHALL prove actual opt-in logging of the bounded Gateway unary body, including native nested identity/warnings/sources within its configured capture budget. Raw-body accessibility alone SHALL NOT be called capture proof. Tests/docs SHALL distinguish that logged Gateway body from native transport diagnostics and from typed native Response identity overwritten by both clients. This capability SHALL NOT add middleware/API surface, universal stream-warning/source export, an unsupported Agent Observability source recording representation or a diagnostic carrier.

#### Scenario: Native identity differs from canonical route
- **WHEN** an alias resolves a canonical route whose provider supplies a different native response ID/modelId
- **THEN** returned streaming identity and raw unary response identity SHALL retain native values
- **AND** logical operator metrics/logs/exports SHALL retain canonical identity and exclude native payload values

#### Scenario: Native warnings and display are returned without server capture
- **WHEN** a metadata-only configured service returns native warnings and URL/document sources with native IDs and file-path display
- **THEN** callers SHALL receive those values unchanged within protocol bounds
- **AND** server logger/metrics/metadata-only Agent Observability exports SHALL omit their arbitrary strings

#### Scenario: Consumer capture is separately configured
- **WHEN** consumer-owned middleware wraps providers/grafana with independent capture settings/destinations while server capture remains metadata-only
- **THEN** WrapGenerate/WrapStream tests SHALL observe contracted fields and a separate opt-in consumer logger test SHALL assert actual Gateway response-body capture at its own destination
- **AND** tests SHALL prove typed unary identity remains replaced while native identity is present in the Gateway response body
- **AND** neither observation location SHALL mutate responses to satisfy the other's capture settings
- **AND** hook access SHALL NOT imply automatic source/warning capture by every built-in middleware

## ADDED Requirements

### Requirement: Request-scoped BYOK logical observation
BYOK SHALL receive one request-local logical observer outside ordered attempts, with requested identity and trusted context. Credentials SHALL be excluded, model metrics SHALL use a fixed bucket and native response identity SHALL remain unchanged without model-ID caches.

#### Scenario: Logical BYOK observation policy
- **WHEN** a BYOK logical model is wrapped and invoked
- **THEN** BYOK SHALL compose one logical observer chain after host credential extraction and outside all credential attempts.
- **AND** It SHALL not reuse catalog/account-bound observers or retain request-scoped observers in global caches.
- **AND** Bounded structured logs/traces/Agent Observability records SHALL identify the requested provider/model without catalog lookup; configured calls SHALL retain canonical catalog identity.
- **AND** Operator capture SHALL remain metadata-only and exclude BYOK, authentication material and unrestricted native diagnostic payloads.
- **AND** Authentication provenance and account-access policy SHALL be observed per request, not copied from a startup auth mode.
- **AND** Wildcard namespaces SHALL remain service-level context unless verified acting-user binding supplies a concrete namespace.
- **AND** Neither namespace nor customer identity SHALL become metric labels.

#### Scenario: BYOK logical call uses two credentials
- **WHEN** the first credential candidate fails with a default-decider-eligible error and the second succeeds
- **THEN** the request SHALL produce one logical call observation with safe credential-attempt observations, not two logical generations
- **AND** no key-bearing options or request metadata SHALL reach generic middleware

#### Scenario: Both account policies run concurrently
- **WHEN** configured and BYOK requests share process-wide telemetry/export infrastructure
- **THEN** each SHALL retain its own verified provenance, account policy and appropriate logical identity without cross-request state

#### Scenario: Credential markers at automatic sinks
- **WHEN** real logger, enrichment, metrics and Agent Observability/export sinks capture a BYOK success, rejection, cancellation or late-result cleanup
- **THEN** no dummy credentials SHALL appear and applicable non-secret timing, usage, provider and outcome facts SHALL remain available

#### Scenario: Results are not operator capture policy
- **WHEN** a BYOK provider returns supported native content, warnings, identity or continuation metadata
- **THEN** the observer chain SHALL preserve them for protocol encoding independently of operator capture exclusions
