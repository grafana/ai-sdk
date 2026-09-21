# gateway-ordered-text-fallback Specification

## Purpose

Define ordered text fallback beneath one logical Gateway observation chain, with first-part commitment, bounded lifecycle ownership, private physical-attempt attribution, public topology confidentiality, and rejection of effectful requests.

## Requirements

### Requirement: Ordered direct text routes
The Gateway SHALL construct each public text route from one required primary and zero or more ordered direct provider/backend candidates. Every invocation SHALL begin at the primary and consider later candidates only in configured order; no prior winner, request value, health signal, or concurrent call SHALL reorder or skip candidates.

#### Scenario: Later call follows original order
- **WHEN** one call is served by a fallback candidate and a later call uses the same canonical route
- **THEN** the later call SHALL begin again with the configured primary

#### Scenario: Client attempts to select a backend
- **WHEN** a request carries an unrecognized or reserved provider/fallback selection control
- **THEN** existing strict validation or host policy SHALL reject or ignore it according to its owning contract and SHALL NOT alter candidate order

### Requirement: Fallback remains beneath one logical middleware chain
Each canonical route SHALL have exactly one WP8 logical chain ordered as approved enrichment → Agent Observability → structured logging/Prometheus → canonical public identity → direct or fallback model. A fallback route SHALL place all physical candidates beneath that chain and SHALL NOT wrap individual candidates with any logical middleware.

#### Scenario: Primary fails and secondary succeeds
- **WHEN** one logical unary or streaming call invokes two physical candidates
- **THEN** WP8 logical middleware SHALL start and finish once for the canonical route while private attempt observation SHALL record each invoked candidate

#### Scenario: Alias invokes fallback
- **WHEN** a caller selects an alias for a fallback route
- **THEN** logical telemetry SHALL use the resolved canonical public ID and SHALL not use the alias or any candidate identity as logical model identity

### Requirement: Private physical decision attribution
WP9 SHALL define and own the private physical record type, bounded sink/queue, non-blocking projection, sink lifecycle, and bounded-cardinality drop accounting. The Gateway SHALL obtain logical correlation only through the service-local correlation accessor over WP8's immutable observation snapshot and SHALL project each reusable fallback decision into one WP9 physical record. The allowlist SHALL contain only logical correlation ID when present, one-based candidate index, configured provider-instance name, backend model ID, start/decision timing, an outcome from the closed selected/failed/canceled set, `WillFallback`, and selected-winner fact. `WillFallback=true` SHALL record a failed attempt's live decision-time intent to advance, not a separate outcome or proof of a subsequent invocation; later cancellation MAY prevent that invocation, and subsequent attempt records SHALL establish which candidates ran. Projection SHALL not record raw error text/data, credentials, request/response payloads, headers, endpoint URLs, provider metadata, or unapproved caller fields. A missing WP9 physical sink or missing correlation SHALL fail open for model execution; a full sink SHALL drop rather than block and SHALL expose only bounded-cardinality drop accounting.

#### Scenario: Secondary wins after primary failure
- **WHEN** a primary fails before commitment and a secondary is selected
- **THEN** private records SHALL correlate one failed/fallback primary decision and one selected/winner secondary decision with the single logical call

#### Scenario: Raw provider error contains secrets
- **WHEN** a candidate's setup error contains a credential, endpoint, private body, header, or arbitrary provider prose
- **THEN** the private physical record SHALL contain only its closed outcome/retry facts and configured attribution fields, not the raw error or secret-bearing values

#### Scenario: Private sink is saturated
- **WHEN** the bounded attempt-record sink cannot accept an event immediately
- **THEN** the Gateway SHALL drop that private telemetry event without changing candidate selection, extending request latency, or creating one goroutine per event

### Requirement: Public topology confidentiality
Discovery, public model resolution errors, ProviderWire JSON/SSE, HTTP access logs, WP8 logical logs/metrics/Agent Observability, and client-visible metadata SHALL expose only the requested identity where required and canonical public identity where authoritative. They SHALL NOT expose candidate count/order, provider-instance names, backend model IDs, raw provider failures, or whether fallback occurred.

#### Scenario: Primary and secondary produce equivalent success
- **WHEN** otherwise equivalent calls are served by different physical candidates
- **THEN** their public identity and protocol shape SHALL remain indistinguishable except for provider-independent generated content, usage, timing, and finish behavior

#### Scenario: Fallback chain is exhausted
- **WHEN** every candidate fails before commitment
- **THEN** the existing safe fixed public error mapping SHALL apply to the aggregate error without listing candidates or copying provider error text

#### Scenario: Discovery lists fallback route
- **WHEN** authenticated discovery lists a route backed by multiple candidates
- **THEN** it SHALL emit only the route's canonical public row, aliases, and guaranteed capabilities and SHALL not emit topology or backend identity

### Requirement: Text-only effect boundary
WP9 SHALL enable ordered fallback only for the strict text surface that rejects effectful tools before model invocation. The reusable commitment rule SHALL reserve any escaped effect as irrevocable selection, but this change SHALL NOT define replay or idempotency for tools or other effectful capability packages.

#### Scenario: Effectful tool request reaches WP9 service
- **WHEN** a request activates a tool or other unsupported effectful capability
- **THEN** the strict mapper SHALL reject it before catalog resolution while tools remain globally unsupported; after WP11/12 enables tools on direct routes, the fallback-route execution guard SHALL reject it before any physical candidate invocation

#### Scenario: Later capability introduces an effect
- **WHEN** a later package proposes fallback for effectful calls
- **THEN** that package SHALL define and test its replay/idempotency boundary before enabling candidate changes

### Requirement: Deterministic fallback and privacy evidence
Automated evidence SHALL cover unary setup selection, streaming setup selection, premature EOF, leading and later provider error parts, multiple ordered error parts, non-retryable failures, cancellation races, nil/invalid results, blocked downstream consumers, silent and continuously ready abandoned providers, observer panic/saturation, recursion-shaped YAML rejection, duplicate/missing references, one logical record, physical winner correlation, and public privacy. Tests SHALL use deterministic fake models/transports and focused race tests; they SHALL not present synthetic provider inputs as recorded conformance evidence.

#### Scenario: Fallback verification runs
- **WHEN** the root and isolated Gateway verification suites run with the committed module boundary
- **THEN** focused tests SHALL prove selection, ordering, lifecycle bounds, logical/physical separation, and privacy without changing the registered upstream baseline or public fixtures unnecessarily

### Requirement: Direct tools do not enable effectful fallback
Enabling function tools on direct routes in WP11 or WP12 SHALL NOT enable them on fallback-configured routes. Non-empty tools, tool choice, and tool-call/result history SHALL activate the route guard even when no tool is selected by the model. The guard SHALL use a fixed safe non-retryable unsupported-request error and SHALL NOT reveal topology or silently route to primary only.

#### Scenario: Unary or streaming tool request after WP11 and WP12
- **WHEN** a schema-valid supported function-tool request targets a fallback-configured public model
- **THEN** no physical candidate SHALL run and the client SHALL receive a fixed safe unsupported-request error

### Requirement: Strict model configuration and public IDs
A bounded strict YAML document SHALL own named providers and public model routes. YAML decoding SHALL reject unknown fields, duplicate mapping keys, and trailing documents. Configuration SHALL reject empty names, unknown provider types, missing provider or backend model references, duplicate or colliding canonical IDs and aliases, missing required presentation names, an empty model set, an empty effective route, and duplicate candidate tuples within a route.

Each provider SHALL have `type: anthropic`, a non-empty `apiKeyEnv` environment-variable reference, and an optional absolute base URL. The referenced environment variable SHALL exist and contain a non-empty API key at startup. Literal provider credentials SHALL not be representable in the YAML schema. A model route SHALL contain one required direct `primary` reference and zero or more ordered direct `fallback` references; each reference SHALL contain exactly a named provider instance and opaque backend model ID. References to public routes and nested fallback definitions SHALL not be representable, making self-reference and recursive route graphs invalid under strict decoding.

Every canonical public ID and alias SHALL be 1-128 ASCII bytes and SHALL match `^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`. This grammar SHALL be the only accepted model-ID surface for discovery and the `ai-language-model-id` header.

#### Scenario: Ordered Anthropic configuration is valid
- **WHEN** YAML defines multiple named Anthropic instances and a public model with one primary plus distinct ordered fallback provider/backend references
- **THEN** startup SHALL accept the route without contacting Anthropic or JWKS and SHALL preserve the declared candidate order

#### Scenario: Minimal direct Anthropic configuration remains valid
- **WHEN** YAML defines one named Anthropic provider by API-key environment reference and one header-safe public route with only a primary provider/backend reference
- **THEN** startup SHALL construct the direct route without requiring a fallback list or contacting Anthropic or JWKS

#### Scenario: Route references are invalid
- **WHEN** any route candidate has an empty field, names an unknown provider, repeats an earlier provider/backend tuple, attempts to name a public route, or contains a nested fallback field
- **THEN** startup SHALL fail before secret resolution, provider construction, or listener binding

#### Scenario: Unsafe public ID is configured
- **WHEN** a canonical ID or alias contains whitespace, control bytes, commas, non-ASCII bytes, or exceeds 128 bytes
- **THEN** startup SHALL fail before catalog construction

#### Scenario: Referenced credential is unavailable
- **WHEN** `apiKeyEnv` names an unset or empty environment variable
- **THEN** startup SHALL fail before constructing the catalog or serving requests
- **AND** logs and errors SHALL NOT contain any environment-variable value

### Requirement: Immutable direct-Anthropic construction
The service SHALL construct each configured provider/backend candidate once with the hardened HTTP client and build one immutable static catalog once at startup. Direct provider construction SHALL use the `anthropic-provider-construction` contract so ambient Anthropic SDK environment defaults cannot alter the service-owned endpoint, credential, auth mechanism, profile/federation behavior, or headers. A route with no fallback entries SHALL use its direct primary model; a route with fallback entries SHALL compose those already constructed candidates in declared order through the Apache fallback primitive and one private attempt hook. Repeated resolution of a canonical ID or alias SHALL return the same final composed model instance and canonical catalog identity. ProviderWire streaming metadata SHALL use canonical public identity when that stream part is emitted; minimal unary output SHALL not add response metadata that registered clients replace.

#### Scenario: Alias and canonical ID share a composed model
- **WHEN** a canonical ID and one of its aliases are resolved repeatedly for a fallback route
- **THEN** all resolutions SHALL return the same fallback model instance
- **AND** alias resolution SHALL report the configured canonical public ID

#### Scenario: Candidate instances are constructed once
- **WHEN** two routes reference configured direct candidates and startup succeeds
- **THEN** every route candidate SHALL be constructed exactly once for that route before serving and no request-time provider factory work SHALL occur

#### Scenario: Anthropic base URL is configured
- **WHEN** a provider instance declares an allowed base URL
- **THEN** requests from every candidate using that instance SHALL use that URL through the hardened transport
- **AND** discovery, public logs, logical metrics, logical Agent Observability, and public errors SHALL NOT expose it

### Requirement: Authenticated direct-Anthropic text execution
An authenticated registered Gateway client SHALL complete the strict runtime's supported unary and streaming text requests through either a configured direct Anthropic route or its ordered Anthropic fallback candidates. The service SHALL preserve the existing ProviderWire validation/mapping order after authentication, forward supported mapped scalar and text options unchanged to every attempted candidate, normalize successful output to canonical public identity, and preserve bounded safe unary and SSE behavior. Fallback SHALL occur only before unary success or the first provider stream part; all public behavior after selection SHALL remain the existing strict adapter behavior.

#### Scenario: Direct unary alias call remains valid
- **WHEN** an authenticated client invokes an alias for a direct-only route with a supported text request
- **THEN** the configured primary SHALL receive the mapped request exactly once and the minimal unary result SHALL expose no backend identity or redundant response metadata

#### Scenario: Unary alias call reaches a fallback candidate
- **WHEN** an authenticated client invokes an alias, the primary returns an eligible pre-commit error, and the secondary succeeds
- **THEN** each candidate SHALL receive the same mapped supported request in configured order exactly once
- **AND** the minimal unary result SHALL expose no backend identity, candidate count, or redundant response metadata

#### Scenario: Streaming premature EOF reaches next candidate
- **WHEN** an authenticated streaming call's primary channel closes before any provider part and the secondary produces a valid text stream
- **THEN** the registered client SHALL consume only the secondary's normalized start, text, finish, and clean EOF

#### Scenario: Streaming provider error commits candidate
- **WHEN** a selected candidate's first or later part is a provider `PartError`
- **THEN** the registered client SHALL receive the adapter's safe error part in order and no later candidate SHALL be invoked

#### Scenario: Normal direct stream remains valid
- **WHEN** a direct-only route's Anthropic provider produces a valid text stream
- **THEN** the registered client SHALL consume the existing normalized start, text, finish, and clean EOF lifecycle unchanged
