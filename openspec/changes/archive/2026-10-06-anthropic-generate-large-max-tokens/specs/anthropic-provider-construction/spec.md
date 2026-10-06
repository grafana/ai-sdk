## ADDED Requirements

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
