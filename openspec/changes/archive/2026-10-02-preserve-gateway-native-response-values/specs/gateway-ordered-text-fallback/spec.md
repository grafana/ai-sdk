## RENAMED Requirements

- FROM: `### Requirement: Public topology confidentiality`
- TO: `### Requirement: Configured topology and operator identity boundaries`

## MODIFIED Requirements

### Requirement: Configured topology and operator identity boundaries

Discovery and public model resolution errors SHALL retain their existing requested/canonical public identity projection without adding candidate count/order, provider-instance names, configured backend mappings, raw provider failures or fallback-occurrence facts. HTTP access logs and WP8 logical logs/metrics/metadata-only Agent Observability SHALL retain their existing canonical identity and payload exclusions.

Successful ProviderWire JSON/SSE and client-visible normal output SHALL preserve supported native warnings, source IDs/display and registered native response id/modelId/timestamp as defined by their runtime/client contracts. A provider-reported modelId that names a backend SHALL NOT be replaced or withheld merely because configured backend mappings remain outside this delivery. Requested/canonical route identity SHALL remain separate and authoritative for resolution/operator metrics. This change SHALL NOT add selected-candidate/attempt/failure evidence or configured topology/discovery fields; those remain separately owned. Configured credentials and another tenant's state SHALL remain protected.

#### Scenario: Primary and secondary produce equivalent success
- **WHEN** otherwise equivalent eligible calls are served by different physical candidates
- **THEN** both SHALL use the same registered protocol shape and preserve the actual selected provider's supported returned warning/source/response identity values
- **AND** no candidate-order/count, provider-instance or attempt/fallback-occurrence field SHALL be introduced by this scalar/display change

#### Scenario: Fallback chain is exhausted
- **WHEN** every candidate fails before commitment
- **THEN** existing safe fixed public aggregate-error mapping SHALL apply without listing candidates or copying provider error text

#### Scenario: Discovery lists fallback route
- **WHEN** authenticated discovery lists a route backed by multiple candidates
- **THEN** it SHALL retain only its existing public row, aliases and guaranteed capabilities without adding configured topology/backend mappings

### Requirement: Immutable direct-Anthropic construction

The service SHALL construct each configured provider/backend candidate once with the hardened HTTP client and build one immutable static catalog once at startup. Direct construction SHALL follow anthropic-provider-construction so ambient SDK environment defaults cannot alter service-owned endpoint, credential, auth mechanism, profile/federation behavior or headers. A route with no fallback SHALL use its direct primary; a fallback route SHALL compose constructed candidates in declared order through the Apache fallback primitive and one private attempt hook. Repeated canonical/alias resolution SHALL return the same final composed model and canonical catalog identity.

ProviderWire output SHALL preserve supported native warnings/source display and registered response identity from the selected provider. Stream response-metadata SHALL NOT substitute canonical route modelId. Unary raw response SHALL retain registered native response identity even though pinned TS/Go clients replace typed response with Gateway-hop information. This output correction SHALL NOT change construction, fallback eligibility, commitment or private attempt observation.

#### Scenario: Alias and canonical ID share a composed model
- **WHEN** canonical ID and alias resolve repeatedly for a fallback route
- **THEN** they SHALL return the same fallback model and alias resolution SHALL report the configured canonical public ID

#### Scenario: Candidate instances are constructed once
- **WHEN** routes reference configured direct candidates and startup succeeds
- **THEN** every route candidate SHALL be constructed exactly once for that route before serving, without request-time provider factory work

#### Scenario: Anthropic base URL is configured
- **WHEN** an instance declares an allowed base URL
- **THEN** its candidates SHALL use it through the hardened transport
- **AND** discovery, public logs, logical metrics/Agent Observability and public errors SHALL NOT expose it

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
