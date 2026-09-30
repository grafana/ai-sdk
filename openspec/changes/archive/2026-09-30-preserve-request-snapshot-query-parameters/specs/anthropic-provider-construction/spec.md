## ADDED Requirements

### Requirement: Direct Anthropic requests use the upstream Messages target
Direct Anthropic models SHALL use `/v1/messages` without an implicit `beta=true` query for both unary and streaming requests, matching the registered TypeScript provider. Beta feature headers and request-body conversion SHALL remain intact. The provider SHALL remove the underlying SDK's implicit beta query before applying explicit caller request options, so unrelated caller query parameters, ordered repeated values and deliberate raw SDK overrides remain supported. Vertex construction and routing SHALL remain unchanged. Request snapshots SHALL compare the complete target without filtering the beta query or rewriting TypeScript expectations to accommodate it.

#### Scenario: Default unary and streaming targets
- **WHEN** a direct Anthropic model sends a unary or streaming request without explicit query options
- **THEN** the target is `/v1/messages` without a query
- **AND** existing authentic conformance requests match the registered TypeScript expectations

#### Scenario: Beta features remain header-driven
- **WHEN** a direct model requests beta features through provider options
- **THEN** their `anthropic-beta` headers remain present on unary and streaming requests
- **AND** no implicit `beta=true` query is added

#### Scenario: Explicit caller query options
- **WHEN** a caller supplies unrelated query options with ordered repeated values
- **THEN** those parameters and repeated-value order remain present
- **AND** the implicit SDK beta query is absent

#### Scenario: Deliberate caller beta override
- **WHEN** a caller explicitly sets the beta query using the existing raw SDK request options
- **THEN** that explicit value is retained rather than removed by the provider's default
