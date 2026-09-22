## ADDED Requirements

### Requirement: One startup-composed logical middleware chain
The Gateway SHALL wrap each configured canonical catalog entry's logical text model exactly once at startup with approved context enrichment, Agent Observability recording, structured logging, Prometheus model metrics, canonical public identity, and then the inner model in that request order. In WP8 the inner model SHALL be the direct provider; WP9 MAY replace it with fallback beneath the unchanged logical wrapper and SHALL NOT wrap physical candidates with WP8 observers. Response observation SHALL occur in reverse order. Alias and canonical resolution SHALL return the same composed model instance, and one invocation SHALL traverse each logical observer exactly once.

The chain SHALL be shared service behavior below public API adapters. It SHALL NOT be constructed in ProviderWire request handling and SHALL NOT change ProviderWire validation, public bytes, error mapping, commitment, stream ordering, timeout, or cancellation precedence.

#### Scenario: Alias uses one canonical chain
- **WHEN** an authenticated text request resolves an alias for a configured canonical model
- **THEN** the catalog SHALL return the same startup-composed model used by the canonical ID
- **AND** logging, metrics, and Agent Observability SHALL each observe one logical call

#### Scenario: ProviderWire behavior remains unchanged
- **WHEN** equivalent unary or streaming text calls execute with and without the logical observers in a deterministic test
- **THEN** both calls SHALL invoke the direct model once with semantically identical call options
- **AND** SHALL produce the same ProviderWire status, response events, event order, and clean-EOF behavior

### Requirement: Canonical public identity on every logical surface
Logical model telemetry SHALL identify provider as `grafana` and model as the canonical public catalog ID from startup composition. Requested aliases, provider instance names, provider types, backend model IDs, provider response IDs, response-derived model identity, and routing topology SHALL NOT appear in logical logs, Prometheus labels, Agent Observability model fields, or Agent Observability metadata.

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

### Requirement: Trusted bounded request observation context
The Gateway SHALL create one private immutable observation view per accepted HTTP request. It SHALL contain only a bounded Gateway-generated opaque request correlation ID, normalized verified caller service and namespace from the authenticated caller context, and validated static region and application values when configured. Empty optional values SHALL be omitted.

The Gateway SHALL NOT enumerate arbitrary context values or derive these fields from unverified request headers, raw or parsed tokens, full auth claims, acting-user email, permissions, groups, prompts, tool data, provider options, provider metadata, or request/response bodies. The observation view SHALL NOT mutate provider call headers or provider options.

Correlation SHALL remain server-owned observation metadata. Neither Gateway client SHALL send or receive a new correlation field or header under this capability.

#### Scenario: Authenticated request is enriched
- **WHEN** authentication stores a normalized caller and static region/application are configured
- **THEN** logical logs and Agent Observability metadata SHALL contain the same request correlation ID, caller service, namespace, region, and application under fixed keys
- **AND** the direct provider's call headers and provider options SHALL contain none of those values unless an independent future policy explicitly approves them

#### Scenario: Untrusted headers attempt enrichment
- **WHEN** an authenticated request supplies arbitrary request, region, application, user, or tenant headers
- **THEN** those header values SHALL NOT replace or extend the trusted observation view
- **AND** none SHALL become logical metadata or provider-bound options through this capability

#### Scenario: Optional deployment context is absent
- **WHEN** region or application has no configured trusted value
- **THEN** the field SHALL be absent from logical telemetry rather than populated from request input or a placeholder

#### Scenario: Existing clients require no change
- **WHEN** the registered Vercel client or WP7 Go client performs a text call
- **THEN** no new request field, request header, response field, or response header SHALL be required or returned for observability correlation

### Requirement: Privacy-safe structured model logs
The Gateway SHALL emit fixed structured start and one terminal record for unary and streaming text using the reusable logger middleware. It SHALL keep all capture flags and per-stream-part logging disabled and SHALL apply a Gateway-owned attribute allowlist followed by default secret-key redaction.

Allowed logical values SHALL be limited to fixed event/schema names, a bounded generated call/correlation ID, call type, canonical public identity, closed outcome and error classifications, normalized status/retryability, duration, usage counters, unified finish reason, warning type/count, bounded stream part type/count, time to first output, and the trusted observation context. Logs SHALL omit prompt/output/reasoning text, tools, files, raw parts, request/response bodies, headers, provider options/metadata, raw finish reasons, arbitrary warning feature/message/detail strings, arbitrary error text, provider response metadata, credentials, backend identity, and topology.

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

Labels SHALL be limited to operation, canonical configured public model identity, closed status and error classes, normalized status code (`100`–`599`, `none`, or `other`), unified finish reason, token type, and closed stream-part type. Caller, namespace, correlation ID, alias, region, application, provider instance, backend model, response ID, arbitrary error, and arbitrary stream values SHALL NOT be metric labels. Duplicate registration SHALL fail startup before readiness.

#### Scenario: Unary and streaming text are scraped
- **WHEN** authenticated unary and streaming calls complete and `/metrics` is scraped
- **THEN** the scrape SHALL expose request count, in-flight calls, duration, token usage, and applicable stream timing/count metrics for the canonical public model
- **AND** no forbidden label value SHALL occur

#### Scenario: Collector registration collides
- **WHEN** model collectors cannot be registered exactly once in the service registry
- **THEN** startup SHALL fail before listener binding and readiness

### Requirement: Metadata-only Agent Observability recording
When Agent Observability export is enabled, the Gateway SHALL use one process-wide client configured for metadata-only content capture, recording middleware only, requested canonical identity, and an allowlisted context provider. Hooks and request-controlled capture changes SHALL remain disabled.

Each authenticated unary or streaming text call SHALL produce one generation containing canonical public model identity, generation mode, approved trusted metadata, normalized usage, unified finish reason, timing including streaming first output when available, and a closed error classification when applicable. It SHALL omit input/output content, system prompts, detailed errors, response/provider IDs, backend identity, provider options/metadata, raw artifacts, credentials, and topology.

The Agent Observability client MAY add only its fixed SDK provenance/content-capture metadata markers and a mirrored closed error category after the Gateway filter. These fixed client-owned fields SHALL NOT carry request input, provider detail, exporter configuration, or arbitrary error text.

#### Scenario: Successful text generation is recorded
- **WHEN** an authenticated unary or streaming text call succeeds with Agent Observability enabled
- **THEN** exactly one metadata-only generation SHALL be finalized with canonical public identity, approved context, usage, finish, and timing
- **AND** its exported record and span SHALL contain no text payload or backend identity

#### Scenario: Provider error is recorded safely
- **WHEN** a provider call or stream part carries an error containing private provider details
- **THEN** the Agent Observability generation SHALL retain only the metadata-only closed error state supported by the exporter
- **AND** provider error text and details SHALL not be exported

#### Scenario: Export is disabled explicitly
- **WHEN** Agent Observability export is explicitly disabled in a development or test configuration
- **THEN** logging and Prometheus observation SHALL continue
- **AND** the model call SHALL not start an Agent Observability generation

### Requirement: Strict bounded Agent Observability exporter configuration
Gateway-owned settings SHALL explicitly select and validate Agent Observability enablement, protocol, endpoint, transport security, authentication secret reference, finite batch and queue sizes, finite payload bytes, finite retry/backoff behavior, and finite flush/shutdown durations before client construction. Literal credentials SHALL not be representable in a YAML file or command argument. Production SHALL reject cleartext export and missing required credentials.

The exporter SHALL be asynchronous and fail open for model traffic. Queue saturation, serialization failure, transport failure, and rejected exports SHALL produce only bounded counters and fixed diagnostic classes; they SHALL NOT expose credentials, endpoints, payloads, generation content, or raw exporter errors. The process SHALL flush and shut down the client after request serving stops using an independent bounded context.

#### Scenario: Exporter configuration is unsafe
- **WHEN** production configuration enables an insecure endpoint, names an unset secret, or provides a non-positive resource bound
- **THEN** startup SHALL fail before constructing models, binding, or readiness
- **AND** no secret value SHALL appear in the error or log

#### Scenario: Export destination is unavailable
- **WHEN** export queueing or delivery fails after startup
- **THEN** model calls and ProviderWire responses SHALL continue unchanged
- **AND** only a fixed export-failure diagnostic and bounded counter SHALL be emitted

#### Scenario: Process shuts down with queued records
- **WHEN** shutdown begins with Agent Observability records queued
- **THEN** request work SHALL stop according to the existing process order
- **AND** the process SHALL boundedly wait for already-started recorders before flushing, while refusing new recorder acquisition after close begins
- **AND** export flush and client shutdown SHALL finish or be abandoned within the configured independent deadline

### Requirement: Logical text lifecycle semantics
Unary observation SHALL start immediately before the shared model invocation and finalize once after its result or pre-result error. Streaming observation SHALL start immediately before stream setup and finalize once after normal channel close, provider error observation, premature close, downstream cancellation, or timeout as represented at the model boundary. When upstream closure and downstream context cancellation are both observable at finalization, cancellation or timeout SHALL take precedence consistently across every logical observer. Every observer SHALL pass through the original result, error, request metadata, response headers, and stream parts without mutation.

Usage SHALL be recorded from the unary result or independently aggregated across every usage-bearing stream part using the established strongest-value semantics. Streaming time to first output SHALL start at the model call and stop at the first payload-bearing shared stream part. A provider error part SHALL be observed as an error without being made terminal by middleware; later parts and finish SHALL remain ordered and visible to ProviderWire.

#### Scenario: Stream error precedes later finish
- **WHEN** a stream emits usage, a provider error part, later text, and finish before closing
- **THEN** every part SHALL pass through unchanged and in order
- **AND** logical observers SHALL finalize once with the established error classification and strongest usage
- **AND** middleware SHALL not terminate the stream or trigger fallback

#### Scenario: Usage is split across stream parts
- **WHEN** different stream parts report independently stronger usage counters
- **THEN** the terminal logical log, metrics, and Agent Observability generation SHALL preserve the strongest normalized counters

### Requirement: Bounded stream observation cleanup
Every logical stream observer SHALL own only the tee channel it introduces. On downstream cancellation it SHALL stop blocking on output, close its output exactly once, finalize its logical observation exactly once, and drain only its immediate upstream until channel close or an absolute configured deadline. A continuously ready or non-cooperative upstream SHALL not retain a Gateway-owned observer or drain goroutine beyond that deadline.

The Gateway SHALL configure every observer with a finite drain duration no greater than the validated ProviderWire stream-drain duration. ProviderWire SHALL remain owner of protocol termination and its immediate input; observer cleanup SHALL not write protocol events or extend handler latency.

#### Scenario: Silent stream is canceled
- **WHEN** a committed stream is canceled while an observer tee is waiting
- **THEN** cancellation SHALL propagate through every logical layer
- **AND** each layer SHALL finalize and release its Gateway-owned work within its drain deadline

#### Scenario: Continuously ready upstream ignores cancellation
- **WHEN** an upstream keeps producing parts after cancellation
- **THEN** each observer SHALL stop draining at its absolute deadline even if receives remain continuously ready
- **AND** the HTTP handler SHALL not wait for observer drain completion

### Requirement: Work-package boundaries remain explicit
This capability SHALL end at one logical text-call observation. Work package 9 SHALL place fallback below this chain and SHALL exclusively own candidate attempt hooks, private provider/backend identity, retry decisions, winner selection, and physical outcomes. WP8 SHALL NOT wrap candidates individually or define physical attempt records.

Work package 6 SHALL own image/capacity integration, work package 7 the Go client, work package 10 production activation and smoke, work package 27 per-request Agent Observability controls, and each later content capability its own new observation mapping. Unsupported future content SHALL not be inferred from raw provider values in this change.

#### Scenario: Fallback is added later
- **WHEN** work package 9 composes ordered fallback beneath the WP8 logical chain
- **THEN** one client call SHALL still produce one WP8 logical record per surface
- **AND** only WP9 private records SHALL describe selected, failed, or canceled physical outcomes plus the actual fallback decision, with retry represented by `failed` and `willFallback=true`

#### Scenario: Later capability is unsupported
- **WHEN** a request activates a content family not owned by text runtime
- **THEN** WP8 SHALL not inspect raw input or provider data to synthesize telemetry for that family
- **AND** its owning capability package SHALL add explicit normalized observation later
