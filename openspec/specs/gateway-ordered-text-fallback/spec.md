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

### Requirement: Configured topology and operator identity boundaries
Authenticated configured discovery SHALL expose authorized candidate count/order, provider-instance/provider identifiers and configured model IDs through the gateway-configured-discovery projection at the same visibility boundary as resolution. Credentials, secret references and unrelated account configuration SHALL remain excluded. Discovery SHALL describe configured possibilities, not whether fallback actually occurred or which candidate completed a request.

Public model resolution errors SHALL retain their requested/canonical public identity projection without listing candidates or configured backend mappings. HTTP access logs and WP8 logical logs/metrics/metadata-only Agent Observability SHALL retain their canonical identity and payload exclusions. Operator capture restrictions SHALL NOT suppress authorized discovery facts or require payload capture to be enabled.

Successful ProviderWire JSON/SSE and client-visible normal output SHALL preserve supported native warnings, source IDs/display and registered native response id/modelId/timestamp as defined by their runtime/client contracts. A provider-reported modelId that names a backend SHALL NOT be replaced or withheld merely because it identifies a configured backend. Requested/canonical route identity SHALL remain separate and authoritative for resolution/operator metrics. Runtime output SHALL NOT add selected-candidate/attempt/failure evidence or configured topology fields; discovery remains separately owned. Configured credentials and another tenant's state SHALL remain protected.

#### Scenario: Primary and secondary produce equivalent success
- **WHEN** otherwise equivalent eligible calls are served by different physical candidates
- **THEN** both SHALL use the same registered protocol shape and preserve the actual selected provider's supported returned warning/source/response identity values
- **AND** normal inference output SHALL NOT introduce candidate-order/count, provider-instance or attempt/fallback-occurrence fields

#### Scenario: Fallback chain is exhausted
- **WHEN** every candidate fails before commitment
- **THEN** the existing safe fixed public error mapping SHALL continue to apply to the aggregate error without listing candidates or copying provider error text as a consequence of this discovery feature

#### Scenario: Discovery lists fallback route
- **WHEN** authorized authenticated discovery lists a route backed by multiple configured candidates
- **THEN** it SHALL emit one canonical public row with aliases, primary and configured fallbacks in declared order
- **AND** it SHALL exclude credentials, secret references and unrelated account state without invoking any candidate

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
Enabling function tools on direct routes in WP11 or WP12 SHALL NOT enable them on fallback-configured routes. Non-empty tools, tool-call/result history, and any tool choice other than an absent choice or pure automatic choice with no tools SHALL activate the route guard even when no tool is selected by the model. A pure automatic choice SHALL have type auto and no tool name. Otherwise supported text-only requests with absent or empty tools and an automatic choice SHALL pass the guard, and every attempted physical candidate SHALL receive the original choice unchanged. All other unsupported-effect checks SHALL remain in force. The guard SHALL use a fixed safe non-retryable unsupported-request error and SHALL NOT reveal topology or silently route to primary only. This exception SHALL NOT change fallback ordering, retry eligibility, or commitment semantics.

#### Scenario: Unary or streaming tool request after WP11 and WP12
- **WHEN** a schema-valid supported function-tool request targets a fallback-configured public model
- **THEN** no physical candidate SHALL run and the client SHALL receive a fixed safe unsupported-request error

#### Scenario: Text-only automatic choice reaches a candidate
- **WHEN** an otherwise supported text request with absent or empty tools and pure automatic choice targets a fallback-configured route through either unary or streaming invocation
- **THEN** the guard SHALL allow the call and the physical candidate SHALL receive the same automatic choice

#### Scenario: Failover preserves automatic choice
- **WHEN** a permitted text-only automatic-choice request encounters a retry-eligible primary failure before commitment
- **THEN** the next configured candidate SHALL receive the same original automatic choice and other call options
- **AND** the ordinary ordering, retry, and commitment rules SHALL still apply

#### Scenario: Other choices remain guarded
- **WHEN** a direct unary or streaming guard call supplies none, required, named, an unknown choice type, or an automatic choice carrying a tool name
- **THEN** it SHALL return the fixed unsupported-request error before any physical candidate executes

#### Scenario: Automatic choice does not bypass effect guards
- **WHEN** a unary or streaming call combines automatic choice with nonempty tools, tool history, or another unsupported control
- **THEN** it SHALL fail before any physical candidate executes rather than selecting a fallback or silently removing those options

### Requirement: Empty message options retain text fallback eligibility
Otherwise eligible text requests SHALL remain eligible for fallback when ordinary message-level provider-option namespaces contain only empty JSON objects. The guard SHALL assess semantic emptiness rather than namespace-map length, without removing or mutating the options it receives. A namespace with any member, including null, false, zero, empty strings, arrays, or nested objects, SHALL count as active and remain rejected. All ordinary namespaces SHALL remain present for the guard; selected-backend namespace/field filtering SHALL NOT sanitize active options into fallback eligibility. Invalid options and unconsumed reserved host namespaces SHALL fail before model invocation. File content, effectful history, tools, call/part options, headers and all other existing route restrictions SHALL remain enforced. Broader mapped-capability fallback eligibility SHALL remain separately owned work.

#### Scenario: Authenticated text failover preserves empty namespaces
- **WHEN** an authenticated unary or streaming text request carries message options `{"anthropic":{}}` for an Anthropic fallback route and the primary fails under existing retry rules before commitment
- **THEN** the request SHALL retain its prior text fallback eligibility
- **AND** each attempted candidate SHALL receive the same empty namespace object without removal or promotion to call options

#### Scenario: Active values are not recursively treated as empty
- **WHEN** Anthropic message options contain `{"anthropic":{"cacheControl":false}}`, `{"anthropic":{"cacheControl":null}}`, or `{"anthropic":{"cacheControl":{}}}`
- **THEN** the fallback route guard SHALL reject the request before physical invocation rather than recursively treating the namespace as empty

#### Scenario: Irrelevant active options are not silently removed
- **WHEN** an authenticated text request carries active ordinary message options only under a namespace irrelevant to its candidates
- **THEN** that namespace SHALL remain in the mapped request and the existing fallback guard SHALL reject it before physical invocation
- **AND** this rejection SHALL NOT be presented as native direct-route support or a permanent mapped-fallback policy

#### Scenario: Empty options do not enable file fallback
- **WHEN** a request combines an empty message-option namespace with files or disallowed tool history
- **THEN** the existing route restriction SHALL fail safely before any physical candidate is invoked

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
The service SHALL construct each configured provider/backend candidate once with the hardened HTTP client and build one immutable static catalog once at startup. Direct construction SHALL follow anthropic-provider-construction so ambient SDK environment defaults cannot alter service-owned endpoint, credential, auth mechanism, profile/federation behavior or headers. A route with no fallback SHALL use its direct primary; a fallback route SHALL compose constructed candidates in declared order through the Apache fallback primitive and one private attempt hook. Repeated canonical/alias resolution SHALL return the same final composed model and canonical catalog identity.

ProviderWire output SHALL preserve supported native warnings/source display and registered response identity from the selected provider. Stream response-metadata SHALL NOT substitute canonical route modelId. Unary raw response SHALL retain registered native response identity even though pinned TS/Go clients replace typed response with Gateway-hop information. This output correction SHALL NOT change construction, fallback eligibility, commitment or private attempt observation.

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

#### Scenario: Native response identity differs from the composed route
- **WHEN** an eligible selected provider returns response identity different from its canonical route
- **THEN** registered raw unary/stream identity SHALL retain provider values while the composed model/resolution/operator identity stays canonical

### Requirement: Authenticated direct-Anthropic text execution

An authenticated registered Gateway client SHALL complete supported unary/stream text calls through configured direct Anthropic routes or currently eligible ordered fallback routes. The service SHALL preserve validation/mapping order after authentication, forward supported mapped scalar/text options unchanged to each attempted candidate and preserve bounded safe unary/SSE behavior. Successful output SHALL preserve supported native warnings, source ID/display and registered native response identity without normalizing them to canonical route identity. Fallback SHALL occur only before unary success or the first provider stream part; behavior after selection SHALL retain the existing strict adapter lifecycle. No fallback eligibility expansion, new routing or attempt transport SHALL follow from preserving these response values.

#### Scenario: Direct unary alias call remains valid
- **WHEN** an authenticated alias call targets a direct route with a supported text request
- **THEN** the primary SHALL receive it exactly once and raw unary output SHALL retain supported native warnings/source/response identity
- **AND** client typed response replacement SHALL remain unchanged without candidate/topology fields

#### Scenario: Unary alias call reaches a fallback candidate
- **WHEN** an eligible alias call encounters an eligible primary precommit error and secondary success
- **THEN** candidates SHALL receive the same supported mapped request once each in configured order
- **AND** raw unary output SHALL retain the successful provider's supported native values without adding attempt count/order or topology

#### Scenario: Streaming premature EOF reaches next candidate
- **WHEN** an eligible stream's primary closes before any part and the secondary produces a valid stream
- **THEN** clients SHALL consume only the secondary's normalized start, native warning/identity values, text, finish and clean EOF

#### Scenario: Streaming provider error commits candidate
- **WHEN** a selected candidate's first or later part is PartError
- **THEN** clients SHALL receive the safe error in order and no later candidate SHALL run

#### Scenario: Normal direct stream remains valid
- **WHEN** a direct Anthropic route emits a valid stream
- **THEN** clients SHALL consume native warning/identity values through the unchanged normalized-start/content/finish/clean-EOF lifecycle
