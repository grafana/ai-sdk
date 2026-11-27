## Purpose

Define construction and request-target requirements for direct and Vertex Anthropic providers, and keep explicit direct configuration authoritative over ambient SDK environment defaults.

## Requirements

### Requirement: Direct Anthropic construction ignores SDK environment defaults

`providers/anthropic.New` SHALL pass `option.WithoutEnvironmentDefaults()` then `option.WithAPIKey(apiKey)` directly to `anthropic.NewClient`, not defer environment suppression to per-request `WithRequestOptions`. Ambient SDK configuration SHALL be ignored; the explicit key and provider request options SHALL remain authoritative. Vertex SHALL retain its separate explicit Google-auth construction.

#### Scenario: Ambient SDK configuration is not loaded
- **WHEN** a direct model is constructed with ambient `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_BASE_URL`, `ANTHROPIC_PROFILE`, fallback profiles, federation variables, identity-token files/tokens, or `ANTHROPIC_CUSTOM_HEADERS`
- **THEN** the SDK SHALL ignore those sources during construction and requests
- **AND** the explicit `apiKey` argument and provider request options SHALL remain authoritative

#### Scenario: Environment base URL is poisoned
- **WHEN** `ANTHROPIC_BASE_URL` points to a poison server and the direct model has an explicit request-option base URL
- **THEN** unary and streaming requests SHALL use only the explicit base URL
- **AND** the poison server SHALL receive no request

#### Scenario: Environment credentials and profiles are poisoned
- **WHEN** ambient API key, auth token, explicit/fallback profile, federation, organization, and identity-token environment sources contain conflicting or invalid values
- **THEN** direct model construction and requests SHALL use only the `apiKey` argument
- **AND** no profile or federation configuration SHALL be loaded or surfaced

#### Scenario: Environment custom headers are poisoned
- **WHEN** `ANTHROPIC_CUSTOM_HEADERS` defines a marker or credential-bearing header
- **THEN** the direct provider SHALL not send that header
- **AND** explicit reviewed request options SHALL continue to work

### Requirement: Direct Anthropic requests use the upstream Messages target

Direct unary and streaming models SHALL use `/v1/messages` without implicit `beta=true`, preserving beta headers and body conversion. The SDK beta query SHALL be removed before explicit caller options, retaining other query parameters, repeated-value order and raw SDK overrides. Snapshots SHALL compare the full target against registered TypeScript expectations without filtering beta or rewriting expectations.

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

### Requirement: Vertex Anthropic requests use the upstream Vertex targets

Vertex Anthropic SHALL send unary requests to `:rawPredict` and streaming requests to `:streamRawPredict` with no implicit query, matching the registered TypeScript Vertex adapter. The provider SHALL remove the SDK's implicit `beta=true` before applying explicit caller request options. Existing Vertex routing, authentication, beta feature headers, and body conversion SHALL remain intact.

#### Scenario: Default Vertex unary and streaming targets
- **WHEN** a Vertex Anthropic model sends a unary or streaming request without explicit query options
- **THEN** the target uses the Vertex project, location, and resolved model route, ends in the corresponding operation, and has no query
- **AND** the request uses Google bearer authentication, preserves `anthropic-beta` features as headers, and sends `anthropic_version` in the body instead of a `model` field

#### Scenario: Explicit Vertex caller query options
- **WHEN** a caller supplies raw SDK request options with unrelated query parameters or an explicit `beta` query
- **THEN** unary and streaming requests retain those values, including repeated-value order, without an extra SDK-supplied `beta=true`

Credential-free transport tests compare the complete request target without filtering. They do not establish live Vertex acceptance.
