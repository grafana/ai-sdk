## Purpose

Define construction constraints for direct Anthropic providers and ensure explicit configuration remains authoritative over ambient SDK environment defaults.

## Requirements

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

### Requirement: Non-streaming requests are not refused before sending

`DoGenerate` SHALL send a Messages request regardless of `max_tokens`, including the model's default maximum, as the registered `@ai-sdk/anthropic` does. When `anthropic-sdk-go` would refuse a non-streaming request because it expects `max_tokens` to need more than ten minutes or to exceed the model's non-streaming token limit, and no request timeout is configured, the provider SHALL set a request timeout: the remaining time before the context deadline when there is one, at least one second because the SDK sends whole seconds, otherwise the SDK's estimate of one hour per 128000 tokens, at least ten minutes. A request timeout configured through `WithRequestOptions` SHALL be left unchanged, and requests the SDK does not refuse SHALL keep the SDK's default timeout.

#### Scenario: Model default max tokens

- **WHEN** `DoGenerate` runs with no `MaxOutputTokens` for a model whose default maximum exceeds the SDK's non-streaming threshold
- **THEN** the request reaches the API with a request timeout derived from the SDK's estimate

#### Scenario: Context deadline

- **WHEN** the same call runs with a context that has a deadline
- **THEN** the request timeout is the remaining time before that deadline

#### Scenario: Caller timeout

- **WHEN** a request timeout is configured through `WithRequestOptions`
- **THEN** the request uses that timeout
