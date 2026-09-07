## MODIFIED Requirements

### Requirement: Provider construction and identity
The system SHALL provide a `providers/openai` Go module exposing both
`NewResponses(apiKey, modelID string, opts ...Option) provider.LanguageModel`
and
`NewResponsesWithClient(client openai.Client, modelID string, opts ...Option) provider.LanguageModel`.
Each constructor SHALL return a value implementing `provider.LanguageModel`
backed by the OpenAI Responses API via `github.com/openai/openai-go`. The model
SHALL report `SpecificationVersion() == "v4"` and `ModelID()` equal to the
constructor `modelID`. The API-key constructor SHALL create a standard OpenAI
client configured with the supplied key and report `Provider() == "openai"` by
default. The preconfigured-client constructor SHALL preserve provider-owned
client configuration. Both constructors SHALL accept functional options,
including `WithRequestOptions(...)` for model request options and
`WithProviderName(...)` for provider integrations to override the identity
reported by `Provider()` and used for provider metadata. OpenAI-specific call
options SHALL continue to use the `"openai"` key regardless of provider identity.
Construction SHALL NOT panic or perform network calls.

#### Scenario: Construct a Responses model
- **WHEN** `NewResponses("test-key", "gpt-4o")` is called
- **THEN** it returns a non-nil `provider.LanguageModel`
- **AND** `Provider()` is `"openai"`, `ModelID()` is `"gpt-4o"`, and `SpecificationVersion()` is `"v4"`

#### Scenario: Base URL override for testing
- **WHEN** `NewResponses("test-key", "gpt-4o", WithRequestOptions(option.WithBaseURL(server.URL)))` is constructed
- **THEN** subsequent `DoGenerate`/`DoStream` calls target the overridden base URL

#### Scenario: Provider integration supplies a configured client
- **WHEN** a provider integration passes a configured official SDK client to `NewResponsesWithClient`
- **THEN** subsequent model calls retain the client's endpoint, authentication, headers, retries, and transport

#### Scenario: Provider integration overrides identity
- **WHEN** a provider integration constructs a model with `WithProviderName("example.responses")`
- **THEN** `Provider()` returns `"example.responses"`
- **AND** generated provider metadata uses the `"example.responses"` namespace
- **AND** OpenAI-specific call options continue to resolve from the `"openai"` namespace
