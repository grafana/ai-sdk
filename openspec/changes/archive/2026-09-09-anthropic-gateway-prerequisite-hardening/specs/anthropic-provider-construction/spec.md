## ADDED Requirements

### Requirement: Direct Anthropic construction ignores SDK environment defaults
`providers/anthropic.New` SHALL construct its underlying Anthropic SDK client by passing `option.WithoutEnvironmentDefaults()` and then `option.WithAPIKey(apiKey)` directly to `anthropic.NewClient`. `WithoutEnvironmentDefaults` SHALL NOT be deferred through the provider's per-request `WithRequestOptions` path because the SDK decides whether to load environment defaults during client construction.

The direct provider SHALL therefore ignore ambient Anthropic SDK configuration, including `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_BASE_URL`, `ANTHROPIC_PROFILE`, fallback profiles, federation variables, identity-token files/tokens, and `ANTHROPIC_CUSTOM_HEADERS`. The explicit `apiKey` argument and explicit provider request options SHALL remain authoritative. Vertex construction SHALL retain its separate explicit Google-auth path.

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
