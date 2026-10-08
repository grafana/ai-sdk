## MODIFIED Requirements

### Requirement: Configured topology and operator identity boundaries
Authenticated configured discovery SHALL expose authorized candidate count/order, provider-instance/provider identifiers and configured model IDs through the gateway-configured-discovery projection at the same visibility boundary as resolution. Credentials, secret references and unrelated account configuration SHALL remain excluded. Discovery SHALL describe configured possibilities, not whether fallback actually occurred or which candidate completed a request.

Public model resolution errors SHALL retain their requested/canonical public identity projection without listing candidates or configured backend mappings. HTTP access logs and WP8 logical logs/metrics/metadata-only Agent Observability SHALL retain their canonical identity and payload exclusions. Operator capture restrictions SHALL NOT suppress authorized discovery facts or require payload capture to be enabled.

Successful ProviderWire JSON/SSE and client-visible normal output SHALL preserve supported native warnings, source IDs/display and registered native response id/modelId/timestamp as defined by their runtime/client contracts. A provider-reported modelId that names a backend SHALL NOT be replaced or withheld merely because it identifies a configured backend. Requested/canonical route identity SHALL remain separate and authoritative for resolution/operator metrics. Runtime output MAY add the separately governed optional compact gateway.execution overview of actual observed destinations/outcomes/failures. It SHALL NOT emit unrun configured destinations or synthesize credentials; configured discovery remains separately owned. Configured credentials and another tenant's state SHALL remain protected.

#### Scenario: Primary and secondary produce equivalent success
- **WHEN** otherwise equivalent eligible calls are served by different physical candidates
- **THEN** both SHALL use the same registered protocol shape and preserve the actual selected provider's supported returned warning/source/response identity values
- **AND** ordinary native output SHALL remain opaque; optional gateway.execution SHALL identify only actually observed destinations and outcomes under its separate contract

#### Scenario: Fallback chain is exhausted
- **WHEN** every candidate fails before commitment
- **THEN** the existing safe fixed public error mapping SHALL continue to apply to the aggregate error without this discovery feature changing classification; runtime enrichment MAY add separately governed protected candidate-local summaries

#### Scenario: Discovery lists fallback route
- **WHEN** authorized authenticated discovery lists a route backed by multiple configured candidates
- **THEN** it SHALL emit one canonical public row with aliases, primary and configured fallbacks in declared order
- **AND** it SHALL exclude credentials, secret references and unrelated account state without invoking any candidate

### Requirement: Authenticated direct-Anthropic text execution

An authenticated registered Gateway client SHALL complete supported unary/stream text calls through configured direct Anthropic routes or currently eligible ordered fallback routes. The service SHALL preserve validation/mapping order after authentication, forward supported mapped scalar/text options unchanged to each attempted candidate and preserve bounded safe unary/SSE behavior. Successful output SHALL preserve supported native warnings, source ID/display and registered native response identity without normalizing them to canonical route identity. Fallback SHALL occur only before unary success or the first provider stream part; behavior after selection SHALL retain the existing strict adapter lifecycle. Preserving these response values SHALL NOT expand fallback eligibility or routing. Separately governed optional execution overviews SHALL NOT change invocation/lifecycle policy.

#### Scenario: Direct unary alias call remains valid
- **WHEN** an authenticated alias call targets a direct route with a supported text request
- **THEN** the primary SHALL receive it exactly once and raw unary output SHALL retain supported native warnings/source/response identity
- **AND** client typed response replacement SHALL remain unchanged with any optional gateway.execution governed independently

#### Scenario: Unary alias call reaches a fallback candidate
- **WHEN** an eligible alias call encounters an eligible primary precommit error and secondary success
- **THEN** candidates SHALL receive the same supported mapped request once each in configured order
- **AND** raw unary output SHALL retain the successful provider's supported native values with any optional gateway.execution describing only actually observed destinations

#### Scenario: Streaming premature EOF reaches next candidate
- **WHEN** an eligible stream's primary closes before any part and the secondary produces a valid stream
- **THEN** clients SHALL consume only the secondary's normalized start, native warning/identity values, text, finish and clean EOF

#### Scenario: Streaming provider error commits candidate
- **WHEN** a selected candidate's first or later part is PartError
- **THEN** clients SHALL receive the safe error in order and no later candidate SHALL run

#### Scenario: Normal direct stream remains valid
- **WHEN** a direct Anthropic route emits a valid stream
- **THEN** clients SHALL consume native warning/identity values through the unchanged normalized-start/content/finish/clean-EOF lifecycle
