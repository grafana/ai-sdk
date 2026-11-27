## MODIFIED Requirements

### Requirement: One startup-composed logical middleware chain
The Gateway SHALL build its observer chains at startup: context enrichment, Agent Observability, structured logging, then Prometheus metrics. Listed operator models SHALL get canonical identity at startup; unlisted operator and stack models when built for the request. After selection, the runtime SHALL apply the source's chain once per call, outside the logical identity and never to physical candidates.

#### Scenario: Alias uses one canonical chain
- **WHEN** an authenticated text request resolves an alias for a configured canonical model
- **THEN** the catalog SHALL return the same startup-composed model used by the canonical ID
- **AND** logging, metrics, and Agent Observability SHALL each observe one logical call

#### Scenario: ProviderWire behavior remains unchanged
- **WHEN** equivalent unary or streaming text calls execute with and without the logical observers in a deterministic test
- **THEN** both calls SHALL invoke the direct model once with semantically identical call options
- **AND** SHALL produce the same ProviderWire status, response events, event order, and clean-EOF behavior

#### Scenario: Observers are not constructed per call
- **WHEN** many configured, routed, stack-catalog and BYOK calls run
- **THEN** observer middleware and metric collectors SHALL be the ones constructed at startup, and no call SHALL register collectors

### Requirement: Canonical public identity on every logical surface
Listed operator logical observations SHALL retain canonical public catalog identity. Unlisted operator exports SHALL use canonical qualified identity, as SHALL structured logs within their model ID limits; model metrics SHALL use a fixed class. BYOK logical logs/exports SHALL retain requested provider/model identity, while metrics SHALL use a fixed BYOK bucket. Native response identity, aliases, configured provider instances and routing details SHALL not substitute for logical identity.

#### Scenario: Canonical public identity on every logical surface policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** Listed operator logical model telemetry SHALL identify provider as `grafana` and model as the canonical public catalog ID from startup composition. Requested aliases, provider instance names, provider types, backend model IDs, provider response IDs, response-derived model identity, and routing topology SHALL NOT appear in logical logs, Prometheus labels, Agent Observability model fields, or Agent Observability metadata.
- **AND** Provider response metadata SHALL remain unmodified for the protocol adapter and later private physical-attempt observation. Logical observation SHALL ignore that metadata rather than rewriting the provider result or stream part.

#### Scenario: Backend response identity differs
- **WHEN** canonical model `grafana/assistant` receives a result or stream metadata naming provider `anthropic`, a backend model ID, and a provider response ID
- **THEN** every logical log, metric, and Agent Observability record SHALL use only `grafana` and `grafana/assistant` for model identity
- **AND** none of the backend identity or response ID values SHALL occur on those surfaces
- **AND** the downstream ProviderWire handler SHALL receive the original result or stream parts unchanged

#### Scenario: Alias is requested
- **WHEN** a caller invokes alias `assistant` for canonical model `grafana/assistant`
- **THEN** no logical model telemetry SHALL use `assistant` as its model identity

#### Scenario: Qualified ID exceeds the structured log bound
- **WHEN** a valid unlisted canonical ID exceeds 128 bytes or contains characters outside printable non-space ASCII
- **THEN** structured logs SHALL omit the model ID fields rather than altering the ID or substituting a backend identity

### Requirement: Bounded-cardinality logical model metrics
One shared service registry SHALL expose reusable model metrics. Listed operator metrics SHALL use canonical public identity; unlisted operator and BYOK metrics SHALL use fixed model classes without requested-model caches. Caller/customer/response/topology values SHALL not become labels; registration collisions SHALL fail startup.

#### Scenario: Bounded-cardinality logical model metrics policy
- **WHEN** the activated Gateway enforces this contract
- **THEN** The Gateway SHALL register the reusable Prometheus model middleware exactly once against the registerer the Gateway was constructed with, which in the plugin process is the default registry that the plugin host collects. Model metrics SHALL use requested identity mode.
- **AND** For configured-account calls, labels SHALL be limited to operation, canonical listed public model identity or the fixed model label `unlisted`, closed status and error classes, normalized status code (`100`–`599`, `none`, or `other`), unified finish reason, token type, and closed stream-part type. Caller, namespace, correlation ID, alias, region, application, provider instance, backend model, response ID, arbitrary error, and arbitrary stream values SHALL NOT be metric labels. Duplicate registration SHALL fail startup before readiness.

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

#### Scenario: Many unlisted operator selectors
- **WHEN** configured-account calls select distinct unlisted operator models, with or without routing controls
- **THEN** metrics SHALL use the fixed model label `unlisted` rather than the qualified IDs
- **AND** execution and Agent Observability SHALL retain the canonical qualified identity; structured logs SHALL use it subject to their existing model ID limits
