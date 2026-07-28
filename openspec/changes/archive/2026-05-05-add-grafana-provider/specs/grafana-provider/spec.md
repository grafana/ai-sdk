## ADDED Requirements

### Requirement: Module location and naming

The Grafana provider SHALL be implemented as a separate Go module located at `providers/grafana/` with module path `github.com/grafana/ai-sdk/providers/grafana`. It MUST NOT be a subpackage of the root `aisdk` module.

#### Scenario: Module path

- **WHEN** a consumer runs `go get github.com/grafana/ai-sdk/providers/grafana`
- **THEN** the module is fetched independently of the root `github.com/grafana/ai-sdk` module

#### Scenario: Dependency isolation

- **WHEN** a consumer imports only the root `aisdk` package
- **THEN** `github.com/grafana/authlib` MUST NOT appear in their dependency graph

### Requirement: LanguageModel interface implementation

The Grafana provider SHALL implement `provider.LanguageModel` from `github.com/grafana/ai-sdk/provider`. The implementation MUST behave like a transparent transport: tools, multi-step orchestration, middleware, fallback, and `@ai-sdk/react` wire emission stay in the consumer process via `aisdk.StreamText` and existing HTTP/UI helpers.

#### Scenario: Conformance with LanguageModel

- **WHEN** the provider returns a model from `LanguageModel(modelID)`
- **THEN** the returned value implements `SpecificationVersion`, `Provider`, `ModelID`, `SupportedURLs`, `DoStream`, and `DoGenerate`

#### Scenario: Behavioral parity with direct providers

- **WHEN** a consumer uses the Grafana provider with `aisdk.StreamText` followed by `ToUIMessageStream` or `aisdk.WriteUIMessageStream`
- **THEN** the resulting UI message stream is produced locally and is compatible with `@ai-sdk/react`, just as it is for direct providers

### Requirement: Constructor naming reflects cloud-only auth

The provider SHALL expose a constructor whose name makes the cloud-only authentication mode explicit, `NewWithCloudAuth`. The constructor MUST accept a typed configuration value carrying cloud auth parameters and MAY accept functional options for testability and future non-breaking knobs.

#### Scenario: Cloud-only constructor available

- **WHEN** a consumer reads the public API surface
- **THEN** the cloud-flavored constructor name is the primary way to create a provider, leaving room for additional auth modes later

### Requirement: Cloud auth configuration

Cloud auth configuration SHALL include at minimum:

- `CAPToken` (string, required): Grafana Cloud Access Policy token used to mint access tokens.
- `TokenExchangeURL` (string, required): URL of the auth-api endpoint that signs access tokens.
- `Audience` (string, optional, default `"ai-sdk"`): audience to request when minting access tokens.
- `Namespace` (string, required): namespace claim for the minted access token.
- `BaseURL` (string, required): base URL of the Grafana hosted ai-sdk provider-wire endpoint.
- `HTTPClient` (`*http.Client`, optional): override for the underlying HTTP client.

#### Scenario: Default audience is ai-sdk

- **WHEN** the consumer omits `Audience` from the config
- **THEN** access tokens are minted with audience `"ai-sdk"`

#### Scenario: Required fields are validated at construction

- **WHEN** any of `CAPToken`, `TokenExchangeURL`, `Namespace`, or `BaseURL` is empty
- **THEN** the constructor returns an error rather than producing a misconfigured provider

### Requirement: Token exchange uses authlib

Per call, the provider SHALL acquire a short-lived access token by exchanging the configured CAP token via `github.com/grafana/authlib/authn`'s `TokenExchangeClient` unless a test `authn.TokenExchanger` has been injected. The provider MUST NOT mint or sign tokens itself and MUST NOT implement its own token cache.

#### Scenario: Access token attached to request

- **WHEN** the provider makes an HTTP provider-wire call to the hosted endpoint
- **THEN** the request carries `Authorization: Bearer <access-token>` where the token was obtained via `TokenExchangeClient.Exchange`

#### Scenario: Token caching is delegated to authlib

- **WHEN** multiple calls happen in close succession
- **THEN** the provider relies on `authlib`'s built-in token caching and does not implement its own

### Requirement: Optional user ID token forwarding

The provider SHALL expose a context helper to attach a user ID token to a `context.Context`. When the helper has been used on the call's context, the provider MUST forward the token in the `X-Grafana-Id` HTTP header. When absent, the provider MUST NOT set the header.

#### Scenario: User-attached context

- **WHEN** the consumer calls `grafana.WithUserIDToken(ctx, idToken)` and passes the resulting context to `aisdk.StreamText`
- **THEN** the outbound HTTP request carries `X-Grafana-Id: <idToken>`

#### Scenario: No user attached

- **WHEN** the call's context has no ID token attached
- **THEN** the outbound HTTP request omits `X-Grafana-Id` entirely

### Requirement: Gateway-style provider-wire HTTP contract

The provider SHALL use the existing `github.com/grafana/ai-sdk/provider/wire` JSON+HTTP/SSE contract. Both `DoStream` and `DoGenerate` SHALL call `POST <BaseURL>/language-model`, where the path comes from `wire.PathLanguageModel`. The request body SHALL be the JSON encoding of `provider.CallOptions`. The model ID and mode SHALL be carried by provider-wire headers, not by a protobuf or JSON envelope.

#### Scenario: Streaming request shape

- **WHEN** the consumer invokes `DoStream`
- **THEN** the provider sends `POST /language-model` with body `wire.EncodeCallOptions(opts)`, header `ai-language-model-streaming: true`, header `ai-language-model-id: <modelID>`, and header `ai-language-model-specification-version: 4`

#### Scenario: Generate request shape

- **WHEN** the consumer invokes `DoGenerate`
- **THEN** the provider sends `POST /language-model` with body `wire.EncodeCallOptions(opts)`, header `ai-language-model-streaming: false`, header `ai-language-model-id: <modelID>`, and header `ai-language-model-specification-version: 4`

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

For `DoStream`, the provider SHALL receive a `text/event-stream` response whose events are JSON `provider.StreamPart` payloads written according to `provider/wire`. The provider SHALL decode events through `wire.NewSSEReader` and forward each `provider.StreamPart` on the channel returned by `DoStream`. Normal HTTP body EOF SHALL close the channel cleanly. No internal `[DONE]` sentinel is used.

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

- **WHEN** the server emits any `provider.StreamPart` shape supported by the root `provider/wire` round-trip suite
- **THEN** the provider decodes it into the same `StreamPart` value the consumer would have received from a direct provider

### Requirement: Non-streaming response handling

For `DoGenerate`, a successful 2xx response SHALL have a JSON body decoded through `wire.DecodeGenerateResult` into `provider.GenerateResult`. Non-2xx responses SHALL be decoded as JSON `provider.APICallError` through `wire.DecodeErrorResponse` where possible.

#### Scenario: Successful non-streaming call

- **WHEN** the server returns a successful JSON `provider.GenerateResult`
- **THEN** the provider returns that `GenerateResult` from `DoGenerate` with no field loss

#### Scenario: Non-2xx generate response

- **WHEN** the server returns HTTP 429 with a JSON `provider.APICallError` body carrying `isRetryable: true`
- **THEN** `DoGenerate` returns a `*provider.APICallError` with `IsRetryable == true` and `StatusCode == 429`

### Requirement: Error reconstruction preserves retry semantics

The provider SHALL surface server and transport errors as `*provider.APICallError` where possible. Server-side provider failures SHALL cross the HTTP boundary as JSON `provider.APICallError` values. If a server or transport failure cannot be decoded as `provider.APICallError`, the client SHALL synthesize one with best-effort status, URL, response body, response headers, cause, and retryability.

#### Scenario: Server returns a retryable error

- **WHEN** the server returns or streams an API-call error payload with `isRetryable: true`
- **THEN** the provider surfaces a `*provider.APICallError` with `IsRetryable == true`, and `aisdk.StreamText` retries according to its retry configuration

#### Scenario: Non-retryable error

- **WHEN** the server returns or streams an API-call error payload with `isRetryable: false`
- **THEN** the provider surfaces a `*provider.APICallError` with `IsRetryable == false` and `aisdk.StreamText` does not retry

### Requirement: Streaming channel buffering

`DoStream` SHALL return a `*provider.StreamResult` whose `Stream` is a buffered receive-only channel once the streaming HTTP response has started successfully. The channel buffer size MUST be at least 64 events to match the existing Anthropic provider's behavior and provide bounded backpressure.

#### Scenario: Caller-controlled draining

- **WHEN** a consumer is slow to drain the channel
- **THEN** the provider applies backpressure rather than buffering unbounded events in memory

### Requirement: Registry integration

The provider package SHALL expose a value or constructor returning one that satisfies `registry.Provider` from `github.com/grafana/ai-sdk/registry`. Consumers MUST be able to register it under any provider id and resolve `<id>:<modelID>` model identifiers.

#### Scenario: Composite ID resolution

- **WHEN** the consumer registers the provider as `"grafana"` and asks for `"grafana:claude-sonnet-4-5-20250929"`
- **THEN** the registry returns a `provider.LanguageModel` whose `ModelID()` is `"claude-sonnet-4-5-20250929"`

#### Scenario: Compose with middleware

- **WHEN** the consumer composes the provider with `registry.WithLanguageModelMiddleware`
- **THEN** middleware wraps Grafana provider models the same way it wraps any other provider

### Requirement: Identity reporting

The provider's `Provider()` method SHALL return `"grafana"`. Its `ModelID()` SHALL return the exact `modelID` passed to `LanguageModel(modelID)`.

#### Scenario: Stable provider name

- **WHEN** a consumer calls `Provider()` on a Grafana model
- **THEN** the returned identifier is `"grafana"`, useable for logging and telemetry

### Requirement: Integration tests against a fake provider-wire hosted endpoint

The module SHALL include integration tests that exercise the full JSON+HTTP/SSE provider-wire contract against an in-process fake hosted endpoint. Tests MUST cover at minimum: successful streaming, successful non-streaming, retryable and non-retryable errors, context cancellation, request header validation, auth headers, and representative `CallOptions` and `StreamPart` shapes.

#### Scenario: Request contract validated

- **WHEN** the integration test fake endpoint receives a request
- **THEN** it asserts method, path, provider-wire headers, content type, accept header, authorization header, optional user ID header, and JSON call-options body

#### Scenario: Retry semantics validated end-to-end

- **WHEN** the fake server emits a retryable error followed by success on retry
- **THEN** `aisdk.StreamText` configured with retries produces the same final result as it would with a direct provider in the equivalent scenario

### Requirement: Conformance suite reuses Anthropic fixtures to prove transparent-transport equivalence

The module SHALL participate in the repository's conformance harness at `test/conformance/`. The Grafana provider's conformance run MUST replay the same Anthropic fixture cases and compare the resulting `UIMessageChunk` sequence against the same `expected.jsonl` files. The Grafana provider passes only when its output is byte-identical to the direct Anthropic provider's output for every shared fixture.

#### Scenario: Same fixtures, same expected output

- **WHEN** a fixture exists under `test/conformance/anthropic/{upstream,recorded}/<case>/` with an `expected.jsonl`
- **THEN** the Grafana conformance run executes the same case and asserts byte-identical output against the same `expected.jsonl`

#### Scenario: Fake hosted endpoint contract

- **WHEN** the Grafana conformance run starts
- **THEN** an in-process fake hosted endpoint implements `POST /language-model`, validates provider-wire/auth headers, decodes JSON call options, and streams provider-wire SSE `StreamPart` messages derived from the fixture

### Requirement: Provider relies on root provider-wire schema coverage

The Grafana provider SHALL rely on the root `provider/wire` package for exhaustive JSON schema round-trip coverage. The provider module SHALL add transport-level tests over representative values, but MUST NOT duplicate every root wire schema test unless the provider introduces additional transformations.

#### Scenario: No provider-specific schema DTOs

- **WHEN** the Grafana provider source is inspected
- **THEN** there are no parallel DTO structs for `CallOptions`, `StreamPart`, `GenerateResult`, or `APICallError`; the provider uses the root provider types and `provider/wire` helpers directly
