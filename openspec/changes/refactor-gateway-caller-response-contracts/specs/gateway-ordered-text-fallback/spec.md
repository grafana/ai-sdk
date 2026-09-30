## MODIFIED Requirements

### Requirement: Fallback remains beneath one logical middleware chain
Each canonical route SHALL have exactly one WP8 logical chain ordered as approved enrichment → Agent Observability → structured logging/Prometheus → canonical public identity → direct or fallback model. A fallback route SHALL place all physical candidates beneath that chain and SHALL NOT wrap individual candidates with any logical middleware. The AGPL-only candidate error-provenance wrapper defined by gateway-caller-response-policy SHALL be installed before fallback and logical composition, preserve the lower physical identity and SHALL NOT add another logical observation chain. Existing attempt hooks, candidate ordering, request guards, retry eligibility and first-part commitment SHALL remain unchanged.

#### Scenario: Primary fails and secondary succeeds
- **WHEN** one logical unary or streaming call invokes two physical candidates
- **THEN** WP8 logical middleware SHALL start and finish once for the canonical route while private attempt observation SHALL record each invoked candidate
- **AND** error provenance SHALL remain bound per candidate rather than inferred from the outer grafana model

#### Scenario: Alias invokes fallback
- **WHEN** a caller selects an alias for a fallback route
- **THEN** logical telemetry SHALL use the resolved canonical public ID and SHALL not use the alias or any candidate identity as logical model identity

### Requirement: Public topology confidentiality
Discovery and public model resolution SHALL expose requested/public canonical route identity only, not provider configuration or fallback topology. HTTP access logs and WP8 logical logs/metrics/metadata-only Agent Observability SHALL retain canonical low-cardinality identity and SHALL NOT contain actual response identity, arbitrary provider diagnostics or caller content/metadata. Configured credentials, endpoints, provider-instance/environment-reference names, candidate count/order and operator fallback records SHALL NOT be serialized in caller responses or discovery.

ProviderWire JSON/SSE and client-visible data SHALL preserve supported actual provider response identity, ordinary metadata under #280 and reviewed actionable diagnostics at registered fields under gateway-caller-response-policy. These are caller response semantics, not an operator topology dump: they SHALL NOT be suppressed solely because they can distinguish providers or physical winners. Backend model values supplied as registered actual response model identity SHALL survive; configured backend model IDs SHALL NOT be synthesized as extra public configuration fields. Native transport and joined aggregate prose SHALL remain excluded.

After context cancellation/deadline priority, exhausted-fallback projection SHALL select only the first candidate-failure branch in the existing newest-first aggregate order and use the trusted policy paired with that same original error. An authoritative unreviewed/non-API failure SHALL use fixed safe diagnostics without searching older candidates. No public field SHALL enumerate candidates, identify their configuration instances or state that fallback occurred.

#### Scenario: Primary and secondary produce different actual identities
- **WHEN** otherwise equivalent calls are served by physical candidates with different supplied registered response model identities
- **THEN** caller response identity SHALL preserve the selected provider values rather than be normalized to the canonical route
- **AND** logical telemetry/discovery SHALL remain route-owned without candidate configuration or topology

#### Scenario: Fallback chain is exhausted
- **WHEN** every candidate fails before commitment
- **THEN** only the newest authoritative failure's paired reviewed diagnostics, or its fixed safe fallback, SHALL determine the public error
- **AND** candidate lists, aggregate error prose and diagnostics from older candidates SHALL NOT be serialized or cross-paired

#### Scenario: Discovery lists fallback route
- **WHEN** authenticated discovery lists a route backed by multiple candidates
- **THEN** it SHALL emit only the route's canonical public row, aliases and guaranteed capabilities and SHALL not emit topology or backend configuration

### Requirement: Deterministic fallback and privacy evidence
Automated evidence SHALL retain unary/setup selection, premature EOF, leading/later/multiple error parts, non-retryable failures, cancellation races, nil/invalid results, blocked consumers, silent/ready abandoned providers, observer panic/saturation, strict configuration, one logical record and private winner correlation. It SHALL additionally prove direct and heterogeneous-fallback per-candidate provenance, newest-first exhaustion selection without policy/API cross-pairing, fixed unconfigured-resolver behavior, original errors.As/Is/context-window/native-retry semantics, shared-error immutability and wrapper channel ownership.

Caller-visible registered response identity/reviewed diagnostics SHALL be tested independently from retained credential/configuration/topology and telemetry protections. Tests SHALL place secret markers in known protected auth, transport or configuration sources, with ordinary supported response values as controls rather than identifying arbitrary application strings as secrets. Deterministic fake models/transports and focused race tests SHALL remain separate from authentic recorded/upstream conformance inputs.

#### Scenario: Fallback verification runs
- **WHEN** root and isolated Gateway verification suites run with the committed module boundary
- **THEN** focused tests SHALL prove selection, provenance, lifecycle bounds, logical/physical separation and source-specific confidentiality without changing the baseline or weakening authentic direct expectations

### Requirement: Immutable direct-Anthropic construction
The service SHALL construct each configured provider/backend candidate once with the hardened HTTP client and build one immutable static catalog once at startup. Direct provider construction SHALL use anthropic-provider-construction so ambient SDK defaults cannot alter the service-owned endpoint, credential, auth mechanism, profile/federation behavior or headers. Each physical model SHALL then receive gateway-caller-response-policy's reviewed server-only error wrapper before direct/fallback composition; the policy SHALL derive from trusted provider configuration, not logical identity or caller input.

A route without fallback SHALL use its wrapped direct primary; a route with fallback SHALL compose already constructed/wrapped candidates in declared order through the unchanged Apache fallback primitive and one private attempt hook. Repeated canonical/alias resolution SHALL return the same final composed model and canonical catalog identity. Logical identity SHALL remain canonical, but streaming response-metadata SHALL preserve supplied actual response identity, and unary output SHALL retain registered response identity without native transport. Pinned clients replace typed unary response identity; it SHALL remain available only through bounded raw response body. No new catalog/provider/core provenance field or public topology member SHALL be required.

#### Scenario: Alias and canonical ID share a composed model
- **WHEN** a canonical ID and one of its aliases are resolved repeatedly for a fallback route
- **THEN** all resolutions SHALL return the same composed instance and the configured canonical catalog ID
- **AND** that route identity SHALL not substitute for actual supplied response identity

#### Scenario: Candidate instances are constructed once
- **WHEN** two routes reference configured direct candidates and startup succeeds
- **THEN** every route candidate and its immutable trusted error policy SHALL be constructed once for that route before serving, without request-time provider factories

#### Scenario: Anthropic base URL is configured
- **WHEN** a provider instance declares an allowed base URL
- **THEN** requests from every candidate using that instance SHALL use it through the hardened transport
- **AND** discovery, public logs, logical metrics/Agent Observability and public errors SHALL NOT expose that configured endpoint

### Requirement: Authenticated direct-Anthropic text execution
An authenticated registered Gateway client SHALL complete supported strict unary/streaming text requests through configured direct Anthropic routes or ordered candidates. Authentication, ProviderWire validation/mapping order, supported scalar/text request forwarding, strict bounds and unary/SSE lifecycle SHALL remain unchanged. Canonical public identity SHALL govern routing and logical observation, not overwrite actual supplied registered response identity. Reviewed provider diagnostics SHALL use the same candidate's trusted provenance; unreviewed/internal failures SHALL retain fixed safe output.

Fallback SHALL occur only before unary success or the first provider stream part. Any first part, including PartError, SHALL commit the candidate; error projection SHALL NOT change retry eligibility, replay after commitment, core termination or authoritative finish. Existing effect/option/header route guards SHALL remain effective.

#### Scenario: Direct unary alias call remains valid
- **WHEN** an authenticated client invokes a direct-route alias with supported text
- **THEN** the configured primary SHALL receive the mapped request once and supported warnings/metadata/actual identity SHALL survive at registered fields
- **AND** typed unary identity SHALL remain subject to the pinned overwrite boundary while bounded raw body retains it

#### Scenario: Unary alias call reaches a fallback candidate
- **WHEN** the primary has an eligible precommit error and a secondary succeeds
- **THEN** candidates SHALL receive the same supported mapped request in configured order exactly once
- **AND** actual response identity SHALL survive without publishing candidate count/order or configured credentials/endpoints

#### Scenario: Streaming premature EOF reaches next candidate
- **WHEN** the primary closes before any part and the secondary produces a valid stream
- **THEN** the client SHALL consume only the secondary's supported start, actual response metadata when supplied, text, finish and clean EOF

#### Scenario: Streaming provider error commits candidate
- **WHEN** the first or later selected-candidate part is PartError
- **THEN** its reviewed bounded projection or fixed safe fallback SHALL be delivered in order and no later candidate SHALL run
- **AND** later valid parts/finish SHALL retain existing non-terminal provider behavior and cleanup authority

#### Scenario: Normal direct stream remains valid
- **WHEN** a direct provider produces a valid text stream
- **THEN** meaningful warnings and actual registered response identity SHALL survive without changing start/text/finish/clean-EOF lifecycle or logical telemetry identity
