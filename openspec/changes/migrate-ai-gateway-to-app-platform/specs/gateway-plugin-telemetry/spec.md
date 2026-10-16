# Spec Delta

## Purpose

Deliver Gateway logs, fallback-attempt records, metrics and traces through the App Platform plugin host's logging, metrics and tracing contracts without changing existing telemetry privacy and cardinality policies.

## ADDED Requirements

### Requirement: Host log protocol
Every Gateway log record SHALL be emitted through the plugin host's structured log protocol with its level, message and attributes preserved; nested attribute groups SHALL be flattened into dotted keys. The Gateway SHALL NOT write raw text or foreign JSON to its standard streams, and each record SHALL stay below the host's per-line limit.

#### Scenario: Info record at production level
- **WHEN** the Gateway emits an info-level lifecycle or request record and the host relays at info level
- **THEN** the host SHALL receive the record at info level with the same message and attribute keys

#### Scenario: Grouped attributes
- **WHEN** a record contains nested attribute groups
- **THEN** the relayed record SHALL contain flattened dotted keys and no dropped attributes

### Requirement: Transport-only logging change
Moving logs to the host protocol SHALL NOT change which records are emitted, their allowlists, redaction, payload exclusions or access-policy-appropriate identity.

#### Scenario: Secret-bearing provider failure
- **WHEN** a provider error contains credentials and arbitrary text
- **THEN** the relayed logical log SHALL contain only the approved fields, as before the migration

### Requirement: Attempt records through the host log protocol
Fallback-attempt records SHALL be emitted through the host log protocol by one bounded asynchronous worker, retaining the closed record fields, bounded queue and drop accounting. Request execution SHALL NOT wait for attempt-record output, queue saturation SHALL drop records with the existing bounded counter, and closing the sink SHALL abandon a stalled worker at its deadline.

#### Scenario: Attempt output stalls
- **WHEN** the attempt worker's log write blocks while requests continue
- **THEN** enqueueing attempt records SHALL not block, records beyond the queue SHALL be dropped and counted, and close SHALL return by its deadline

### Requirement: Host-collected metrics
In the plugin process, Gateway collectors SHALL be registered exactly once in the process default registry that the plugin host collects, with existing metric names and label bounds unchanged. Route request metrics SHALL label routes by fixed route names without namespace values. The Gateway SHALL NOT serve its own metrics endpoint or register a second process or runtime collector.

#### Scenario: Host collects plugin metrics
- **WHEN** the host collects the plugin's metrics after unary and streaming calls
- **THEN** the collection SHALL include the Gateway's model, route request and export-failure metrics without forbidden label values

### Requirement: Access decision metrics
The Gateway SHALL count route requests by bounded access decision: configured, BYOK or refused. Labels SHALL NOT contain caller identity, service identity, namespace or token values. Authentication outcomes owned by the platform SHALL NOT be counted as Gateway authentication metrics.

#### Scenario: Mixed callers
- **WHEN** internal, user and refused callers send requests
- **THEN** the access-decision counter SHALL increase under its three fixed values only

### Requirement: Host-configured tracing
Request handling SHALL continue the trace context that the plugin host's instrumented protocol server provides, and request-scoped log records SHALL carry its trace and span identifiers. The Gateway SHALL NOT configure its own trace exporter or add span attributes containing credentials, payloads or provider topology.

#### Scenario: Traced request
- **WHEN** a traced route request reaches the Gateway with host tracing configured
- **THEN** the Gateway's request-scoped log records SHALL carry the incoming trace identifier

#### Scenario: Tracing not configured
- **WHEN** the host supplies no tracing configuration
- **THEN** the Gateway SHALL serve requests without creating a trace exporter
