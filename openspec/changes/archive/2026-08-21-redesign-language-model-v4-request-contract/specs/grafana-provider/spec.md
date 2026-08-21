## MODIFIED Requirements

### Requirement: Gateway-style provider-wire HTTP contract

The provider SHALL use the tolerant `github.com/grafana/ai-sdk/gateway/providerwire` JSON+HTTP/SSE contract. Both `DoStream` and `DoGenerate` SHALL call `POST <BaseURL>/language-model`, where the path comes from `providerwire.PathLanguageModel`. The request body SHALL be produced only by `providerwire.EncodeCallOptions`; the Grafana provider SHALL NOT directly marshal `provider.CallOptions` or define parallel request DTOs. The model ID and mode SHALL be carried by provider-wire headers, not by a protobuf or JSON envelope. This provider-contract source break MUST NOT change the request bytes or protocol semantics emitted by the Grafana provider for values representable before the redesign.

The Grafana provider SHALL pass redesigned provider request values to the root codec without converting exact integers through `float64` or collapsing pointer or collection presence first. Newly representable distinctions carry only the compatibility guaranteed by the tolerant root adapter.

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
- **WHEN** the consumer sets `Prompt`, `Tools`, `ToolChoice`, `MaxOutputTokens`, `Temperature`, `TopP`, `TopK`, `PresencePenalty`, `FrequencyPenalty`, `Reasoning`, `ResponseFormat`, `StopSequences`, `Headers`, `ProviderOptions`, or an evidenced redesigned presence state
- **THEN** the request body SHALL carry the tolerant provider-wire representation such that the hosted endpoint decodes an equivalent `provider.CallOptions`

#### Scenario: Redesigned call options reach the codec
- **WHEN** the caller supplies exact large integers, fractional numeric settings, explicit false, explicit empty optional strings, an empty selected file-data arm, or non-nil empty collections
- **THEN** the Grafana model SHALL pass those values unchanged to `providerwire.EncodeCallOptions`

#### Scenario: Historical request bytes remain stable
- **WHEN** a call uses only values representable before the redesign
- **THEN** the emitted body and provider-wire headers SHALL match the corpus produced by parent commit `32e5ab7f1ab9e524477cc0ece04c690a89854a24`

#### Scenario: No protobuf or Connect dependency
- **WHEN** the Grafana provider transport is inspected
- **THEN** it SHALL NOT import generated protobuf packages, `wirepb`, or Connect runtime packages for model-call transport

#### Scenario: Provider generic JSON is not protocol authority
- **WHEN** the Grafana provider constructs a model request
- **THEN** it SHALL use `providerwire.EncodeCallOptions` and SHALL NOT directly marshal provider request structs
