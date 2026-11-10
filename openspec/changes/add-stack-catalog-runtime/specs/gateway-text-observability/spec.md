## ADDED Requirements

### Requirement: Stack-catalog logical observation
Stack-catalog calls SHALL be observed by their own chain, built at startup from the shared middleware and picked by source. Logs and Agent Observability SHALL use provider `grafana` and the catalog's canonical model ID. Model metrics SHALL use the fixed model label `catalog`, through the one metrics registration. Keys, aliases, provider names and backend model IDs SHALL NOT appear on any logical surface. Agent Observability SHALL record these calls metadata-only, through the process-wide client.

#### Scenario: Stack-catalog call is observed once
- **WHEN** a stack-catalog call runs, unary or streaming
- **THEN** each logical observer SHALL observe it once
- **AND** its logs SHALL carry provider `grafana`, the canonical model ID and the catalog name
- **AND** its metric series SHALL carry provider `grafana` and model `catalog`

#### Scenario: Many catalogs and model IDs
- **WHEN** calls run against 20 stack catalogs with 20 distinct model IDs each
- **THEN** the model metric series count SHALL equal that of one catalog with one model, and no catalog name or model ID SHALL be a label value

#### Scenario: Provider error echoes a catalog key
- **WHEN** a provider fails with a message that contains the catalog's decrypted key
- **THEN** the key SHALL NOT appear in logs, metrics or Agent Observability

#### Scenario: Agent Observability records a stack-catalog call
- **WHEN** a stack-catalog call succeeds with Agent Observability enabled
- **THEN** the operator's process-wide client SHALL record one metadata-only generation with the canonical model ID and `gateway.catalog` metadata, and no per-catalog exporter SHALL exist

## MODIFIED Requirements

### Requirement: One startup-composed logical middleware chain
The Gateway SHALL construct its logical observer chains once at startup: context enrichment, Agent Observability recording, structured logging and Prometheus model metrics, in that request order. Operator-catalog entries SHALL get canonical identity once at startup, and stack-catalog models when built for their request. After selection, the runtime SHALL apply the chain of the selection's source once per call, outside that identity. Physical candidates SHALL NOT get logical observers.

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

### Requirement: Trusted bounded request observation context
The Gateway SHALL create one private immutable observation view per accepted HTTP request. It SHALL contain only a bounded Gateway-generated opaque request correlation ID, the normalized verified caller service and namespace from the authenticated caller context, and, for a stack-catalog call, the selected catalog's name. Empty or out-of-bound optional values SHALL be omitted.

#### Scenario: Authenticated request is enriched
- **WHEN** authentication stores a normalized caller
- **THEN** logical logs and Agent Observability metadata SHALL contain the same request correlation ID, caller service and namespace under fixed keys
- **AND** the direct provider's call headers and provider options SHALL contain none of those values unless an independent future policy explicitly approves them

#### Scenario: Stack-catalog call is enriched
- **WHEN** a call selects from a stack catalog
- **THEN** logical logs SHALL carry the catalog name as `catalog` and Agent Observability metadata as `gateway.catalog`
- **AND** a name that fails the shared observation value bound of 128 printable ASCII bytes SHALL be omitted

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
