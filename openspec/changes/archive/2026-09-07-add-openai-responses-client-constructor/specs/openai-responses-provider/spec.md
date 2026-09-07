## MODIFIED Requirements

### Requirement: Provider construction and identity
The system SHALL provide a `providers/openai` Go module exposing both
`NewResponses(apiKey, modelID string, opts ...Option) provider.LanguageModel`
and
`NewResponsesWithClient(client openai.Client, modelID string, opts ...Option) provider.LanguageModel`.
Each constructor SHALL return a value implementing `provider.LanguageModel`
backed by the OpenAI Responses API via `github.com/openai/openai-go`. The model
SHALL report `SpecificationVersion() == "v4"`, `Provider() == "openai"`, and
`ModelID()` equal to the constructor `modelID`. The API-key constructor SHALL
create a standard OpenAI client configured with the supplied key. The
preconfigured-client constructor SHALL preserve the client's endpoint,
authentication, middleware, retries, and transport. Both constructors SHALL
accept functional options including `WithRequestOptions(...)` for model request
options. Construction SHALL NOT panic or perform network calls.

#### Scenario: Construct a Responses model
- **WHEN** `NewResponses("test-key", "gpt-4o")` is called
- **THEN** it returns a non-nil `provider.LanguageModel`
- **AND** `Provider()` is `"openai"`, `ModelID()` is `"gpt-4o"`, and `SpecificationVersion()` is `"v4"`

#### Scenario: Base URL override for testing
- **WHEN** `NewResponses("test-key", "gpt-4o", WithRequestOptions(option.WithBaseURL(server.URL)))` is constructed
- **THEN** subsequent `DoGenerate`/`DoStream` calls target the overridden base URL

#### Scenario: Use a preconfigured Bedrock client
- **WHEN** an OpenAI Go client is configured for the Bedrock Mantle Responses endpoint with AWS SigV4 credentials and passed to `NewResponsesWithClient`
- **THEN** subsequent model calls use the configured Mantle endpoint
- **AND** requests retain the client's SigV4 authentication scoped to the `bedrock-mantle` service
