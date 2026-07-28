## MODIFIED Requirements

### Requirement: Gateway-style provider-wire HTTP contract

The provider SHALL use the `github.com/grafana/ai-sdk/gateway/providerwire` JSON+HTTP/SSE contract. Both `DoStream` and `DoGenerate` SHALL call `POST <BaseURL>/language-model`, where the path comes from `providerwire.PathLanguageModel`. The request body SHALL be the JSON encoding of `provider.CallOptions`. The model ID and mode SHALL be carried by provider-wire headers, not by a protobuf or JSON envelope. This import-path migration is source-breaking for direct wire-helper consumers but MUST NOT change the request bytes or protocol semantics emitted by the Grafana provider.

#### Scenario: Streaming request shape

- **WHEN** the consumer invokes `DoStream`
- **THEN** the provider sends `POST /language-model` with body `providerwire.EncodeCallOptions(opts)`, header `ai-language-model-streaming: true`, header `ai-language-model-id: <modelID>`, and header `ai-language-model-specification-version: 4`

#### Scenario: Generate request shape

- **WHEN** the consumer invokes `DoGenerate`
- **THEN** the provider sends `POST /language-model` with body `providerwire.EncodeCallOptions(opts)`, header `ai-language-model-streaming: false`, header `ai-language-model-id: <modelID>`, and header `ai-language-model-specification-version: 4`

#### Scenario: Model id carried in headers

- **WHEN** a consumer calls `reg.LanguageModel("grafana:claude-sonnet-4-5-20250929")` and uses the result
- **THEN** the HTTP request's `ai-language-model-id` header is `"claude-sonnet-4-5-20250929"`

#### Scenario: CallOptions round-trip

- **WHEN** the consumer sets `Prompt`, `Tools`, `ToolChoice`, `MaxOutputTokens`, `Temperature`, `TopP`, `TopK`, `PresencePenalty`, `FrequencyPenalty`, `Reasoning`, `ResponseFormat`, `StopSequences`, `Headers`, or `ProviderOptions`
- **THEN** the JSON request body carries the current JSON-serializable provider schema such that the hosted endpoint can decode an equivalent `provider.CallOptions`

#### Scenario: No protobuf or Connect dependency

- **WHEN** the Grafana provider transport is inspected
- **THEN** it SHALL NOT import generated protobuf packages, `wirepb`, or Connect runtime packages for model-call transport

### Requirement: Streaming response handling

For `DoStream`, the provider SHALL receive a `text/event-stream` response whose events are JSON `provider.StreamPart` payloads written according to `gateway/providerwire`. The provider SHALL decode events through `providerwire.NewSSEReader` and forward each `provider.StreamPart` on the channel returned by `DoStream`. Normal HTTP body EOF SHALL close the channel cleanly. No internal `[DONE]` sentinel is used.

#### Scenario: Normal completion

- **WHEN** the server sends one or more SSE events carrying JSON `provider.StreamPart` values and then closes the response body successfully
- **THEN** the channel receives each decoded `provider.StreamPart` in order and is closed cleanly

#### Scenario: Transport-level stream failure

- **WHEN** the stream fails before normal EOF after a 2xx response has started
- **THEN** the provider emits a final `provider.StreamPart{Type: PartError}` with a synthesized retryable `*provider.APICallError` when retry is appropriate and then closes the channel

#### Scenario: Context cancellation

- **WHEN** the call's `context.Context` is cancelled
- **THEN** the provider cancels the HTTP request, releases response resources, and closes the returned channel without panicking

#### Scenario: StreamPart values are transportable

- **WHEN** the server emits any `provider.StreamPart` shape supported by the root `gateway/providerwire` round-trip suite
- **THEN** the provider decodes it into the same `StreamPart` value the consumer would have received from a direct provider

### Requirement: Non-streaming response handling

For `DoGenerate`, a successful 2xx response SHALL have a JSON body decoded through `providerwire.DecodeGenerateResult` into `provider.GenerateResult`. Non-2xx responses SHALL be decoded as JSON `provider.APICallError` through `providerwire.DecodeErrorResponse` where possible.

#### Scenario: Successful non-streaming call

- **WHEN** the server returns a successful JSON `provider.GenerateResult`
- **THEN** the provider returns that `GenerateResult` from `DoGenerate` with no field loss

#### Scenario: Non-2xx generate response

- **WHEN** the server returns HTTP 429 with a JSON `provider.APICallError` body carrying `isRetryable: true`
- **THEN** `DoGenerate` returns a `*provider.APICallError` with `IsRetryable == true` and `StatusCode == 429`

### Requirement: Provider relies on root provider-wire schema coverage

The Grafana provider SHALL rely on the root `gateway/providerwire` package for exhaustive JSON schema round-trip coverage. The provider module SHALL add transport-level tests over representative values, but MUST NOT duplicate every root wire schema test unless the provider introduces additional transformations.

#### Scenario: No provider-specific schema DTOs

- **WHEN** the Grafana provider source is inspected
- **THEN** there are no parallel DTO structs for `CallOptions`, `StreamPart`, `GenerateResult`, or `APICallError`; the provider uses the root `provider` types and `gateway/providerwire` helpers directly
