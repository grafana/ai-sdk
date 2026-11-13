# gateway-plugin-telemetry Specification

## Purpose
Deliver Gateway logs, fallback-attempt records, metrics and traces through the App Platform plugin host's logging, metrics and tracing contracts without changing existing telemetry privacy and cardinality policies.

## Requirements

### Requirement: Host log protocol
Every Gateway log record SHALL be emitted through the plugin host's structured log protocol with its level, message and attributes preserved; nested attribute groups SHALL be flattened into dotted keys. The Gateway SHALL NOT write raw text or foreign JSON to its standard streams. Records SHALL stay below the host's per-line limit because their producers emit bounded fields, not through truncation in the log adapter.

#### Scenario: Info record at production level
- **WHEN** the Gateway emits an info-level lifecycle or request record and the host relays at info level
- **THEN** the host SHALL receive the record at info level with the same message and attribute keys

#### Scenario: Grouped attributes
- **WHEN** a record contains nested attribute groups
- **THEN** the relayed record SHALL contain flattened dotted keys and no dropped attributes

### Requirement: Transport-only logging change
Moving logs to the host protocol SHALL NOT change which records are emitted, their allowlists, redaction, payload exclusions or access-policy-appropriate identity, except that startup and plugin failure records carry their secret-safe reason as `gateway-plugin-host` specifies.

#### Scenario: Secret-bearing provider failure
- **WHEN** a provider error contains credentials and arbitrary text
- **THEN** the relayed logical log SHALL contain only the approved fields, as before the migration

### Requirement: Attempt records through the host log protocol
Fallback-attempt records SHALL be emitted through the host log protocol by one bounded asynchronous worker, retaining the closed record fields, bounded queue and drop accounting. Request execution SHALL NOT wait for attempt-record output, queue saturation SHALL drop records with the existing bounded counter, and closing the sink SHALL abandon a stalled worker at its deadline.

#### Scenario: Attempt output stalls
- **WHEN** the attempt worker's log write blocks while requests continue
- **THEN** enqueueing attempt records SHALL not block, records beyond the queue SHALL be dropped and counted, and close SHALL return by its deadline

### Requirement: Host-collected metrics
In the plugin process, Gateway collectors SHALL be registered exactly once in the process default registry that the plugin host collects, with existing model metric names and label bounds unchanged. Route request counts, latency and in-flight requests SHALL come from the host's route metrics; the Gateway SHALL NOT duplicate them. The Gateway SHALL NOT serve its own metrics endpoint or register a second process or runtime collector.

#### Scenario: Host collects plugin metrics
- **WHEN** the host collects the plugin's metrics after unary and streaming calls
- **THEN** the collection SHALL include the Gateway's model, access-decision and export-failure metrics without forbidden label values, and no Gateway route request metrics

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

### Requirement: Denied model-call outcome
Logical logs and model metrics SHALL record a call that an extension denied with the distinct outcome `denied`, not as an error, so denials are excluded from error rates. A failure under `fail-closed` SHALL remain an error. Agent Observability SHALL record denied calls like other failed calls.

#### Scenario: Denial before the model
- **WHEN** an admitter denies a configured call
- **THEN** the terminal log record SHALL carry outcome `denied`, and the model request counter SHALL increase under status `denied` with no `error` series added

#### Scenario: In-band stream denial
- **WHEN** a response inspector aborts a stream
- **THEN** logs and metrics SHALL record that stream with outcome `denied`

### Requirement: Extension event metrics
The Gateway SHALL count extension denials and failures by extension name, phase and a fixed event, `denied` or `failed`, whatever the failure policy. Labels SHALL take only registered extension names and fixed phase and event values, and SHALL NOT carry namespace, caller, model or error text.

#### Scenario: Mixed extension outcomes
- **WHEN** one extension denies at admit, another fails under `skip` in response inspection, and a third observer panics
- **THEN** the counter SHALL increase once for each, under that extension's name and phase, with event `denied` for the first and `failed` for the others

### Requirement: Extension failure logs
Every extension failure SHALL emit one warning log record with the extension name, phase, failure policy and a fixed class (error, panic or policy refusal), plus the call's correlation ID when one exists. Records SHALL NOT carry the error's text, request or response content, or configuration values.

#### Scenario: Failing transform with secret-bearing error
- **WHEN** a transform fails with an error whose text contains a credential
- **THEN** the log record SHALL carry only the approved fields, and no part of the error's text
