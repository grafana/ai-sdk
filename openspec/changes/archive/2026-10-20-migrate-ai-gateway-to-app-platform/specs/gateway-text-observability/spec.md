# Spec Delta

## MODIFIED Requirements

### Requirement: Request-scoped BYOK logical observation
BYOK SHALL receive one request-local logical observer outside ordered attempts, with requested identity and trusted context. Configured sensitive fields SHALL follow operator capture policy, model metrics SHALL use a fixed bucket and native response identity SHALL remain unchanged without model-ID caches.

#### Scenario: Logical BYOK observation policy
- **WHEN** a BYOK logical model is wrapped and invoked
- **THEN** BYOK SHALL compose one logical observer chain after host credential extraction and outside all credential attempts.
- **AND** It SHALL not reuse catalog/account-bound observers or retain request-scoped observers in global caches.
- **AND** Bounded structured logs/traces/Agent Observability records SHALL identify the requested provider/model without catalog lookup; configured calls SHALL retain canonical catalog identity.
- **AND** Operator capture SHALL remain metadata-only and exclude BYOK, authentication material and unrestricted native diagnostic payloads.
- **AND** Authentication provenance and account-access policy SHALL be observed per request, not copied from a startup auth mode.
- **AND** Namespace attribution SHALL be the platform-authorized route namespace, including for callers whose delegated token covers all namespaces.
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
- **AND** independent caller-visible native summaries SHALL preserve BYOK-only echoes subject to the configured provider/exporter key policy; operator capture SHALL not filter returned content or opaque metadata

#### Scenario: Results are not operator capture policy
- **WHEN** a BYOK provider returns supported native content, warnings, identity or continuation metadata
- **THEN** the observer chain SHALL preserve them for protocol encoding independently of operator capture exclusions

### Requirement: Bounded-cardinality logical model metrics
One shared service registry SHALL expose reusable model metrics. Configured metrics SHALL use canonical public identity; BYOK metrics SHALL use a fixed model class without requested-model caches. Caller/customer/response/topology values SHALL not become labels; registration collisions SHALL fail startup.

#### Scenario: Bounded-cardinality logical model metrics policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** The Gateway SHALL register the reusable Prometheus model middleware exactly once against the registerer the Gateway was constructed with, which in the plugin process is the default registry that the plugin host collects. Model metrics SHALL use requested identity mode.
- **AND** For configured-account calls, labels SHALL be limited to operation, canonical configured public model identity, closed status and error classes, normalized status code (`100`–`599`, `none`, or `other`), unified finish reason, token type, and closed stream-part type. Caller, namespace, correlation ID, alias, region, application, provider instance, backend model, response ID, arbitrary error, and arbitrary stream values SHALL NOT be metric labels. Duplicate registration SHALL fail startup before readiness.

#### Scenario: Unary and streaming text are scraped
- **WHEN** authenticated unary and streaming calls complete and the plugin host collects metrics
- **THEN** the collection SHALL include request count, in-flight calls, duration, token usage, and applicable stream timing/count metrics for the canonical public model
- **AND** no forbidden label value SHALL occur

#### Scenario: Collector registration collides
- **WHEN** model collectors cannot be registered exactly once in the service registry
- **THEN** plugin startup SHALL fail before serving requests

BYOK requests SHALL use a fixed BYOK model class and, if exposed, only bounded supported-provider/access-policy labels. Caller-selected native model IDs SHALL NOT become Prometheus labels or allocate per-model observer/metric caches. These BYOK rules SHALL NOT consult the configured catalog.

#### Scenario: Many BYOK model selectors
- **WHEN** requests use many distinct native model names for one supported BYOK provider
- **THEN** the model metric series count SHALL remain bounded independently of those names

### Requirement: Strict bounded Agent Observability exporter configuration

Gateway-owned settings SHALL explicitly select and validate Agent Observability enablement, protocol, endpoint, transport security and bearer token before client construction. The token SHALL be read only after the other settings and the model configuration are validated. Batch and queue sizes, payload bytes, retries and a 10-second per-attempt export timeout SHALL be fixed Gateway constants, and the plugin's final-flush budget SHALL bound flush and shutdown.

#### Scenario: Exporter configuration is unsafe
- **WHEN** configuration enables an endpoint that contradicts its transport security or names an unset secret
- **THEN** plugin startup SHALL fail before constructing models or serving requests
- **AND** no secret value SHALL appear in the error or log

#### Scenario: Export destination is unavailable
- **WHEN** export queueing or delivery fails after startup
- **THEN** model calls and ProviderWire responses SHALL continue unchanged
- **AND** only a fixed export-failure diagnostic and bounded counter SHALL be emitted

#### Scenario: Process shuts down with queued records
- **WHEN** shutdown begins with Agent Observability records queued
- **THEN** the plugin host SHALL first drain route serving
- **AND** the process SHALL boundedly wait for already-started recorders before flushing, while refusing new recorder acquisition after close begins
- **AND** the recorder wait, export flush and client shutdown SHALL use the plugin's shared final-flush context, finishing or being abandoned when it expires

### Requirement: Bounded exporter failure and shutdown reporting

Queue saturation, serialization failure, transport failure, and rejected exports SHALL produce only bounded counters and fixed diagnostic classes; they SHALL NOT expose credentials, endpoints, payloads, generation content, or raw exporter errors. The plugin SHALL flush and shut down the client after the host stops request serving, using the shared final-flush context; configured flush and shutdown durations SHALL only shorten each step.

#### Scenario: Bounded exporter failure and shutdown reporting
- **WHEN** export delivery fails while records are queued at shutdown
- **THEN** only fixed diagnostics and bounded counters SHALL be emitted, and flush/shutdown SHALL use the shared final-flush context after the host stops serving

### Requirement: Trusted bounded request observation context

The Gateway SHALL create one private immutable observation view per accepted HTTP request. It SHALL contain only a bounded Gateway-generated opaque request correlation ID and the normalized verified caller service and namespace from the authenticated caller context. Empty optional values SHALL be omitted.

#### Scenario: Authenticated request is enriched
- **WHEN** authentication stores a normalized caller
- **THEN** logical logs and Agent Observability metadata SHALL contain the same request correlation ID, caller service and namespace under fixed keys
- **AND** the direct provider's call headers and provider options SHALL contain none of those values unless an independent future policy explicitly approves them

#### Scenario: Untrusted headers attempt enrichment
- **WHEN** an authenticated request supplies arbitrary request, region, application, user, or tenant headers
- **THEN** those header values SHALL NOT replace or extend the trusted observation view
- **AND** none SHALL become logical metadata or provider-bound options through this capability

#### Scenario: Optional deployment context is absent
- **WHEN** the verified caller has no attributable service or namespace
- **THEN** the field SHALL be absent from logical telemetry rather than populated from request input or a placeholder

#### Scenario: Existing clients require no change
- **WHEN** the registered Vercel client or WP7 Go client performs a text call
- **THEN** no new request field, request header, response field, or response header SHALL be required or returned for observability correlation

## ADDED Requirements

### Requirement: Optional exporter with explicit transport security
Agent Observability export SHALL be optional. When export is disabled, the Gateway SHALL resolve no exporter credential and start no exporter workers. When it is enabled, the Gateway SHALL require a credential, and the endpoint SHALL match the configured transport security, which defaults to TLS.

#### Scenario: Plugin without export
- **WHEN** configuration disables Agent Observability export
- **THEN** the plugin SHALL start and serve requests without an exporter

#### Scenario: Endpoint contradicts transport security
- **WHEN** export is enabled with TLS and a cleartext endpoint, or without TLS and an https endpoint
- **THEN** plugin startup SHALL fail before serving requests
