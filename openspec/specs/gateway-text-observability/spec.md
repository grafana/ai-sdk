# gateway-text-observability Specification

## Purpose

Define privacy-safe, bounded-cardinality, once-per-logical-call observability for Gateway text generation across logs, Prometheus metrics, and Agent Observability.

## Requirements

### Requirement: Request-scoped BYOK logical observation
BYOK SHALL receive one request-local logical observer outside ordered attempts, with requested identity and trusted context. Configured sensitive fields SHALL follow operator capture policy, model metrics SHALL use a fixed bucket and native response identity SHALL remain unchanged without model-ID caches.

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
- **THEN** configured sensitive fields SHALL follow logger policy and metadata-only exporters SHALL retain their exclusions while timing, usage and outcome facts remain available
- **AND** independent caller-visible native diagnostics SHALL preserve available producer echoes

#### Scenario: Results are not operator capture policy
- **WHEN** a BYOK provider returns supported native content, warnings, identity or continuation metadata
- **THEN** the observer chain SHALL preserve them for protocol encoding independently of operator capture exclusions

### Requirement: One startup-composed logical middleware chain

The Gateway SHALL wrap each configured canonical catalog entry's logical text model exactly once at startup with approved context enrichment, Agent Observability recording, structured logging, Prometheus model metrics, canonical public identity, and then the inner model in that request order. In WP8 the inner model SHALL be the direct provider; WP9 MAY replace it with fallback beneath the unchanged logical wrapper and SHALL NOT wrap physical candidates with WP8 observers.

#### Scenario: Alias uses one canonical chain
- **WHEN** an authenticated text request resolves an alias for a configured canonical model
- **THEN** the catalog SHALL return the same startup-composed model used by the canonical ID
- **AND** logging, metrics, and Agent Observability SHALL each observe one logical call

#### Scenario: ProviderWire behavior remains unchanged
- **WHEN** equivalent unary or streaming text calls execute with and without the logical observers in a deterministic test
- **THEN** both calls SHALL invoke the direct model once with semantically identical call options
- **AND** SHALL produce the same ProviderWire status, response events, event order, and clean-EOF behavior

### Requirement: Shared logical chain traversal and adapter isolation

Response observation SHALL occur in reverse order. Alias and canonical resolution SHALL return the same composed model instance, and one invocation SHALL traverse each logical observer exactly once. The chain SHALL be shared service behavior below public API adapters. It SHALL NOT be constructed in ProviderWire request handling and SHALL NOT change ProviderWire validation, public bytes, error mapping, commitment, stream ordering, timeout, or cancellation precedence.

#### Scenario: Shared logical chain traversal and adapter isolation
- **WHEN** alias and canonical calls traverse the startup chain
- **THEN** each SHALL use the same composed instance and reverse response-observation order without constructing observers in ProviderWire or changing public protocol behavior

### Requirement: Canonical public identity on every logical surface
Configured logical observations SHALL retain canonical public catalog identity. BYOK logical logs/exports SHALL retain requested provider/model identity, while metrics SHALL use a fixed BYOK bucket. Native response identity, aliases, configured provider instances and routing details SHALL not substitute for logical identity.

#### Scenario: Canonical public identity on every logical surface policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** Configured-account logical model telemetry SHALL identify provider as `grafana` and model as the canonical public catalog ID from startup composition. Requested aliases, provider instance names, provider types, backend model IDs, provider response IDs, response-derived model identity, and routing topology SHALL NOT appear in logical logs, Prometheus labels, Agent Observability model fields, or Agent Observability metadata.
- **AND** Provider response metadata SHALL remain unmodified for the protocol adapter and later private physical-attempt observation. Logical observation SHALL ignore that metadata rather than rewriting the provider result or stream part.

#### Scenario: Backend response identity differs
- **WHEN** canonical model `grafana/assistant` receives a result or stream metadata naming provider `anthropic`, a backend model ID, and a provider response ID
- **THEN** every logical log, metric, and Agent Observability record SHALL use only `grafana` and `grafana/assistant` for model identity
- **AND** none of the backend identity or response ID values SHALL occur on those surfaces
- **AND** the downstream ProviderWire handler SHALL receive the original result or stream parts unchanged

#### Scenario: Alias is requested
- **WHEN** a caller invokes alias `assistant` for canonical model `grafana/assistant`
- **THEN** no logical model telemetry SHALL use `assistant` as its model identity
- **AND** logical model telemetry SHALL use `grafana/assistant`

### Requirement: Unmodified native response metadata for adapters

Provider response metadata SHALL remain unmodified for the protocol adapter and later private physical-attempt observation. Logical observation SHALL ignore that metadata rather than rewriting the provider result or stream part.

#### Scenario: Unmodified native response metadata for adapters
- **WHEN** a provider result contains backend response metadata
- **THEN** logical observers SHALL ignore it and forward it unchanged for protocol and private physical-attempt observation

### Requirement: Trusted bounded request observation context

The Gateway SHALL create one private immutable observation view per accepted HTTP request. It SHALL contain only a bounded Gateway-generated opaque request correlation ID, normalized verified caller service and namespace from the authenticated caller context, and validated static region and application values when configured. Empty optional values SHALL be omitted.

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

### Requirement: Trusted context input exclusions and provider isolation

The Gateway SHALL NOT enumerate arbitrary context values or derive observation-view fields from unverified request headers, raw or parsed tokens, full auth claims, acting-user email, permissions, groups, prompts, tool data, provider options, provider metadata, or request/response bodies. The observation view SHALL NOT mutate provider call headers or provider options. Correlation SHALL remain server-owned observation metadata.

#### Scenario: Trusted context input exclusions and provider isolation
- **WHEN** an authenticated request includes arbitrary headers, claims and provider options
- **THEN** those values SHALL NOT extend the observation view or mutate provider headers/options, and correlation SHALL remain server-owned

### Requirement: No client correlation protocol extension

Neither Gateway client SHALL send or receive a new correlation field or header under this capability.

#### Scenario: No client correlation protocol extension
- **WHEN** either Gateway client performs a model call
- **THEN** no new correlation field or header SHALL be sent or received

### Requirement: Privacy-safe structured model logs
Logical logs SHALL retain only fixed/bounded lifecycle facts, normalized usage/classification and access-policy-appropriate identity. They SHALL exclude raw request/provider payloads and arbitrary diagnostic text; operator policy SHALL not censor returned provider data.

#### Scenario: Privacy-safe structured model logs policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** The Gateway SHALL emit fixed structured start and one terminal record for unary and streaming text using the reusable logger middleware. It SHALL keep all capture flags and per-stream-part logging disabled and SHALL apply a Gateway-owned attribute allowlist followed by default secret-key redaction.
- **AND** Allowed logical values SHALL be limited to fixed event/schema names, a bounded generated call/correlation ID, call type, access-policy-appropriate logical identity, closed outcome and error classifications, normalized status/retryability, duration, usage counters, unified finish reason, warning type/count, bounded stream part type/count, time to first output, and the trusted observation context. Logs SHALL omit prompt/output/reasoning text, tools, files, raw parts, request/response bodies, headers, provider options/metadata, raw finish reasons, arbitrary warning feature/message/detail strings, arbitrary error text, provider response metadata, credentials, configured-account backend details, response-derived identity, and topology. Bounded BYOK requested provider/model identity SHALL be permitted under the request-scoped observation requirement.

#### Scenario: Unary provider failure contains secrets
- **WHEN** a unary provider error contains a credential, provider URL, backend model ID, response body, and arbitrary message
- **THEN** the logical terminal log SHALL retain only its approved closed error classification and timing
- **AND** no secret or provider-originated value SHALL be emitted

#### Scenario: Stream contains payload and provider metadata
- **WHEN** a stream contains text, response metadata, provider metadata, a provider error part, usage, and finish
- **THEN** logical logs SHALL include only approved lifecycle summaries and counters
- **AND** no per-part log or payload/provider detail SHALL be emitted

### Requirement: Structured logical log value allowlist

Allowed logical values SHALL be limited to fixed event/schema names, a bounded generated call/correlation ID, call type, access-policy-appropriate logical identity, closed outcome and error classifications, normalized status/retryability, duration, usage counters, unified finish reason, warning type/count, bounded stream part type/count, time to first output, and the trusted observation context.

#### Scenario: Structured logical log value allowlist
- **WHEN** a streaming terminal record includes usage and time to first output
- **THEN** only the approved fixed, bounded and normalized logical values SHALL be emitted

### Requirement: Structured logical log payload exclusions

Logs SHALL omit prompt/output/reasoning text, tools, files, raw parts, request/response bodies, headers, provider options/metadata, raw finish reasons, arbitrary warning feature/message/detail strings, arbitrary error text, provider response metadata, credentials, backend identity, and topology.

#### Scenario: Structured logical log payload exclusions
- **WHEN** a result includes reasoning, files, warning details and native response metadata
- **THEN** logs SHALL omit those payloads and arbitrary provider values

### Requirement: Bounded-cardinality logical model metrics
One shared service registry SHALL expose reusable model metrics. Configured metrics SHALL use canonical public identity; BYOK metrics SHALL use a fixed model class without requested-model caches. Caller/customer/response/topology values SHALL not become labels; registration collisions SHALL fail startup.

#### Scenario: Bounded-cardinality logical model metrics policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** The Gateway SHALL register the reusable Prometheus model middleware exactly once against the existing service-owned registry and expose its collectors on the existing unauthenticated `/metrics` route. Model metrics SHALL use requested identity mode.
- **AND** For configured-account calls, labels SHALL be limited to operation, canonical configured public model identity, closed status and error classes, normalized status code (`100`–`599`, `none`, or `other`), unified finish reason, token type, and closed stream-part type. Caller, namespace, correlation ID, alias, region, application, provider instance, backend model, response ID, arbitrary error, and arbitrary stream values SHALL NOT be metric labels. Duplicate registration SHALL fail startup before readiness.

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

### Requirement: Metric privacy and duplicate-registration refusal

Caller, namespace, correlation ID, alias, region, application, provider instance, backend model, response ID, arbitrary error, and arbitrary stream values SHALL NOT be metric labels. Duplicate registration SHALL fail startup before readiness.

#### Scenario: Metric privacy and duplicate-registration refusal
- **WHEN** a caller supplies an alias and correlation ID and collectors are registered twice
- **THEN** neither value SHALL become a label and the registration collision SHALL fail startup before readiness

### Requirement: Metadata-only Agent Observability recording
One process-wide metadata-only exporter SHALL produce one logical generation per call with approved context, normalized cache-inclusive usage, timing, finish and closed classification. Configured calls SHALL retain canonical identity and BYOK calls requested identity; raw payloads, native identity and arbitrary diagnostics SHALL remain excluded.

#### Scenario: Metadata-only Agent Observability recording policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** When Agent Observability export is enabled, the Gateway SHALL use one process-wide client configured for metadata-only content capture, recording middleware only, requested logical identity, and an allowlisted context provider. Hooks, experimental SDK features, and request-controlled capture changes SHALL remain disabled.
- **AND** Each authenticated unary or streaming text call SHALL produce one generation containing its access-policy-appropriate logical model identity, generation mode, approved trusted metadata, normalized usage marked as cache-inclusive without adding cache buckets again, unified finish reason, timing including streaming first output when available, and a closed error classification when applicable. It SHALL omit input/output content, system prompts, detailed errors, response/provider IDs, configured-account backend details, provider options/metadata, raw artifacts, credentials, and topology. BYOK requested provider/model identity SHALL remain an allowed bounded logical identity, never a metric label.
- **AND** The Agent Observability client MAY add only its fixed SDK provenance/content-capture metadata markers and a mirrored closed error category after the Gateway filter. These fixed client-owned fields SHALL NOT carry request input, provider detail, exporter configuration, or arbitrary error text.

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

### Requirement: One normalized Agent Observability generation

Each authenticated unary or streaming text call SHALL produce one generation containing access-policy-appropriate logical model identity, generation mode, approved trusted metadata, normalized usage marked as cache-inclusive without adding cache buckets again, unified finish reason, timing including streaming first output when available, and a closed error classification when applicable.

#### Scenario: One normalized Agent Observability generation
- **WHEN** an authenticated stream reports cache-inclusive usage and first output
- **THEN** one generation SHALL retain normalized usage without double-counting cache buckets, approved metadata and timing

### Requirement: Agent Observability payload exclusions and fixed client markers

Each generation SHALL omit input/output content, system prompts, detailed errors, response/provider IDs, backend identity, provider options/metadata, raw artifacts, credentials, and topology. The Agent Observability client MAY add only its fixed SDK provenance/content-capture metadata markers and a mirrored closed error category after the Gateway filter. These fixed client-owned fields SHALL NOT carry request input, provider detail, exporter configuration, or arbitrary error text.

#### Scenario: Agent Observability payload exclusions and fixed client markers
- **WHEN** an exported generation has provider details and client provenance markers available
- **THEN** provider details SHALL be omitted and only the fixed client markers and closed error category SHALL be permitted after filtering

### Requirement: Strict bounded Agent Observability exporter configuration

Gateway-owned settings SHALL explicitly select and validate Agent Observability enablement, protocol, endpoint, transport security, authentication secret reference, finite batch and queue sizes, finite payload bytes, finite retry/backoff behavior, a positive per-attempt HTTP/gRPC export timeout, and finite flush/shutdown durations before client construction. The export-attempt timeout SHALL default to 10 seconds and SHALL be no greater than 5 minutes, independently of flush/shutdown deadlines.

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

### Requirement: Exporter ambient-setting and credential restrictions

Ambient SDK timeout, retry, queue, and experimental-feature environment settings SHALL be rejected under their supported current and legacy spellings before client construction or secret resolution, without exposing their values in diagnostics. Literal credentials SHALL not be representable in a YAML file or command argument. Production SHALL reject cleartext export and missing required credentials. The exporter SHALL be asynchronous and fail open for model traffic.

#### Scenario: Exporter ambient-setting and credential restrictions
- **WHEN** a supported legacy SDK retry environment variable is set
- **THEN** startup SHALL reject it before client construction or secret resolution without exposing its value

### Requirement: Bounded exporter failure and shutdown reporting

Queue saturation, serialization failure, transport failure, and rejected exports SHALL produce only bounded counters and fixed diagnostic classes; they SHALL NOT expose credentials, endpoints, payloads, generation content, or raw exporter errors. The process SHALL flush and shut down the client after request serving stops using an independent bounded context.

#### Scenario: Bounded exporter failure and shutdown reporting
- **WHEN** export delivery fails while records are queued at shutdown
- **THEN** only fixed diagnostics and bounded counters SHALL be emitted, and flush/shutdown SHALL use an independent bounded context after serving stops

### Requirement: Logical text lifecycle semantics

Unary observation SHALL start immediately before the shared model invocation and finalize once after its result or pre-result error. Streaming observation SHALL start immediately before stream setup and finalize once after normal channel close, provider error observation, premature close, downstream cancellation, or timeout as represented at the model boundary.

#### Scenario: Stream error precedes later finish
- **WHEN** a stream emits usage, a provider error part, later text, and finish before closing
- **THEN** every part SHALL pass through unchanged and in order
- **AND** logical observers SHALL finalize once with the established error classification and strongest usage
- **AND** middleware SHALL not terminate the stream or trigger fallback

#### Scenario: Usage is split across stream parts
- **WHEN** different stream parts report independently stronger usage counters
- **THEN** the terminal logical log, metrics, and Agent Observability generation SHALL preserve the strongest normalized counters

### Requirement: Logical finalization precedence and strongest usage

When upstream closure and downstream context cancellation are both observable at finalization, cancellation or timeout SHALL take precedence consistently across every logical observer. Every observer SHALL pass through the original result, error, request metadata, response headers, and stream parts without mutation. Usage SHALL be recorded from the unary result or independently aggregated across every usage-bearing stream part using the established strongest-value semantics.

#### Scenario: Logical finalization precedence and strongest usage
- **WHEN** upstream closes while downstream cancellation is observable and several parts report stronger usage
- **THEN** all observers SHALL select cancellation consistently, preserve strongest counters and pass original boundary values unchanged

### Requirement: First-output timing and non-terminal error observation

Streaming time to first output SHALL start at the model call and stop at the first payload-bearing shared stream part. A provider error part SHALL be observed as an error without being made terminal by middleware; later parts and finish SHALL remain ordered and visible to ProviderWire.

#### Scenario: First-output timing and non-terminal error observation
- **WHEN** a provider error precedes payload text and finish
- **THEN** the error SHALL be observed without ending forwarding, and first-output timing SHALL stop at the first payload-bearing shared part

### Requirement: Bounded stream observation cleanup

Every logical stream observer SHALL own only the tee channel it introduces. On downstream cancellation it SHALL stop blocking on output, close its output exactly once, finalize its logical observation exactly once, and drain only its immediate upstream until channel close or an absolute configured deadline. A continuously ready or non-cooperative upstream SHALL not retain a Gateway-owned observer or drain goroutine beyond that deadline.

#### Scenario: Silent stream is canceled
- **WHEN** a committed stream is canceled while an observer tee is waiting
- **THEN** cancellation SHALL propagate through every logical layer
- **AND** each layer SHALL finalize and release its Gateway-owned work within its drain deadline

#### Scenario: Continuously ready upstream ignores cancellation
- **WHEN** an upstream keeps producing parts after cancellation
- **THEN** each observer SHALL stop draining at its absolute deadline even if receives remain continuously ready
- **AND** the HTTP handler SHALL not wait for observer drain completion

### Requirement: Observer drain configuration and protocol ownership

The Gateway SHALL configure every observer with a finite drain duration no greater than the validated ProviderWire stream-drain duration. ProviderWire SHALL remain owner of protocol termination and its immediate input; observer cleanup SHALL not write protocol events or extend handler latency.

#### Scenario: Observer drain configuration and protocol ownership
- **WHEN** a downstream consumer cancels a committed stream
- **THEN** observer drains SHALL be finite and no longer than ProviderWire's drain duration, without writing protocol events or extending handler latency

### Requirement: Work-package boundaries remain explicit

This capability SHALL end at one logical text-call observation. Work package 9 SHALL place fallback below this chain and SHALL exclusively own candidate attempt hooks, private provider/backend identity, retry decisions, winner selection, and physical outcomes. WP8 SHALL NOT wrap candidates individually or define physical attempt records.

#### Scenario: Fallback is added later
- **WHEN** work package 9 composes ordered fallback beneath the WP8 logical chain
- **THEN** one client call SHALL still produce one WP8 logical record per surface
- **AND** only WP9 private records SHALL describe selected, failed, or canceled physical outcomes plus the actual fallback decision, with retry represented by `failed` and `willFallback=true`

#### Scenario: Later capability is unsupported
- **WHEN** a request activates a content family not owned by text runtime
- **THEN** WP8 SHALL not inspect raw input or provider data to synthesize telemetry for that family
- **AND** its owning capability package SHALL add explicit normalized observation later

### Requirement: Adjacent capability ownership and future telemetry boundaries

Work package 6 SHALL own image/capacity integration, work package 7 the Go client, work package 10 production activation and smoke, work package 27 per-request Agent Observability controls, and each later content capability its own new observation mapping. Unsupported future content SHALL not be inferred from raw provider values in this change.

#### Scenario: Adjacent capability ownership and future telemetry boundaries
- **WHEN** a future content family reaches its owning work package
- **THEN** its explicit mapping SHALL be owned there rather than inferred from raw provider values by WP8

### Requirement: Operator capture is independent from returned native values
Server observations SHALL retain metadata-only exclusions without modifying caller-visible native data. Consumers SHALL configure independent middleware/capture and prove actual opt-in Gateway-body logging separately from hook access, typed identity replacement and unsupported native transport carriers.

#### Scenario: Operator capture is independent from returned native values policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** Gateway metadata-only logger, Prometheus, enrichment and Agent Observability observations SHALL retain their existing payload exclusions and access-policy-appropriate logical model identity when native warning/source/response identity values become caller-visible. Arbitrary warning strings, source ID/URL/title/filename and response ID/modelId SHALL NOT become operator attributes, metric labels or metadata-only exported payloads. The observer chain SHALL pass original results/parts unchanged to protocol encoding; operator capture restrictions SHALL NOT censor returned values. Account authorization and request isolation SHALL prevent attaching another request's state. Logger field policy and exporter exclusions SHALL apply only to observations, never rewrite returned provider values.
- **AND** Consumer applications SHALL be able to configure existing reusable middleware independently around providers/grafana without enabling server capture. Tests SHALL use middleware.WrapLanguageModel with Middleware.WrapGenerate to inspect actual unary warnings/content/Response.Body and WrapStream to observe forwarded PartStreamStart/PartSource/PartResponseMeta through a context-aware test tee. These hooks SHALL receive contracted values without mutating output; hook access SHALL NOT be equated with automatic built-in capture/export.
- **AND** A separately configured consumer logger.Middleware with CaptureOptions.ResponseBody and a consumer-owned slog destination SHALL prove actual opt-in logging of the bounded Gateway unary body, including native nested identity/warnings/sources within its configured capture budget. Raw-body accessibility alone SHALL NOT be called capture proof. Tests/docs SHALL distinguish that logged Gateway body from native transport diagnostics and from typed native Response identity overwritten by both clients. This capability SHALL NOT add middleware/API surface, universal stream-warning/source export, an unsupported Agent Observability source recording representation or a diagnostic carrier.

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

### Requirement: Returned-value preservation and independent consumer observation

The observer chain SHALL pass original results/parts unchanged to protocol encoding; operator capture restrictions SHALL NOT censor returned values. Known credentials and another tenant's state SHALL remain protected independently of ordinary application scalar/display values. Consumer applications SHALL be able to configure existing reusable middleware independently around providers/grafana without enabling server capture.

#### Scenario: Returned-value preservation and independent consumer observation
- **WHEN** consumer middleware surrounds providers/grafana while server capture is metadata-only
- **THEN** returned values SHALL remain unchanged, consumer configuration SHALL remain independent and credentials/cross-tenant state SHALL stay protected

### Requirement: Consumer hook evidence for native values

Tests SHALL use middleware.WrapLanguageModel with Middleware.WrapGenerate to inspect actual unary warnings/content/Response.Body and WrapStream to observe forwarded PartStreamStart/PartSource/PartResponseMeta through a context-aware test tee. These hooks SHALL receive contracted values without mutating output; hook access SHALL NOT be equated with automatic built-in capture/export.

#### Scenario: Consumer hook evidence for native values
- **WHEN** WrapGenerate and a context-aware WrapStream tee inspect native warnings, sources and response metadata
- **THEN** the hooks SHALL receive contracted fields without mutation and SHALL NOT be treated as automatic built-in capture/export

### Requirement: Opt-in consumer response-body capture proof

A separately configured consumer logger.Middleware with CaptureOptions.ResponseBody and a consumer-owned slog destination SHALL prove actual opt-in logging of the bounded Gateway unary body, including native nested identity/warnings/sources within its configured capture budget. Raw-body accessibility alone SHALL NOT be called capture proof. Tests/docs SHALL distinguish that logged Gateway body from native transport diagnostics and from typed native Response identity overwritten by both clients.

#### Scenario: Opt-in consumer response-body capture proof
- **WHEN** a consumer logger enables CaptureOptions.ResponseBody with its own slog destination
- **THEN** tests SHALL assert actual bounded Gateway-body logging, distinguishing raw-body access, native diagnostics and overwritten typed identity

### Requirement: Native-value observation surface boundary

This capability SHALL NOT add middleware/API surface, universal stream-warning/source export, an unsupported Agent Observability source recording representation or a diagnostic carrier.

#### Scenario: Native-value observation surface boundary
- **WHEN** native source values are returned to a consumer wrapper
- **THEN** this capability SHALL NOT introduce middleware APIs, universal built-in source/warning export, an unsupported Agent Observability source representation or diagnostic carrier
