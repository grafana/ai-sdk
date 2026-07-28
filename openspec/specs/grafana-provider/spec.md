## Purpose

Define the Grafana provider module and its provider-wire transport behavior.

## Requirements

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

The provider SHALL expose `NewWithCloudAuth(cfg CloudAuthConfig, opts ...Option) (*Provider, error)` for the cloud-auth mode where the calling process holds a Cloud Access Policy token and exchanges it locally for short-lived access tokens. The constructor MUST accept a typed configuration value carrying cloud auth parameters and MAY accept functional options for future non-breaking knobs. The constructor name MUST make the cloud-auth mode explicit so it remains distinguishable from alternative auth-mode constructors (for example `NewWithAccessToken`) on the same `Provider`.

#### Scenario: Cloud-auth constructor name

- **WHEN** a consumer reads the public API surface
- **THEN** the cloud-auth constructor is named `NewWithCloudAuth`, explicitly identifying its auth mode

### Requirement: Pre-minted access token constructor

The provider SHALL expose a constructor `NewWithAccessToken(cfg AccessTokenConfig, opts ...Option) (*Provider, error)` for consumers who already hold a short-lived access token JWT minted out-of-band (for example by a separate process calling auth-api `/v1/sign-access-token`). The constructor MUST share the existing `Provider` struct with `NewWithCloudAuth` and MUST produce a provider whose `LanguageModel` returns models behaviorally identical to those of `NewWithCloudAuth`, differing only in how the per-call access token is sourced.

#### Scenario: Access-token constructor available alongside cloud-auth

- **WHEN** a consumer reads the public API surface of `providers/grafana`
- **THEN** both `NewWithCloudAuth` and `NewWithAccessToken` are exported and documented as the two supported ways to construct a provider

#### Scenario: Returned provider satisfies the same contract

- **WHEN** a consumer constructs a provider via `NewWithAccessToken` and resolves a `LanguageModel`
- **THEN** the model exposes the same `SpecificationVersion`, `Provider`, `ModelID`, `SupportedURLs`, `DoStream`, and `DoGenerate` behavior as a model from `NewWithCloudAuth`

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

### Requirement: Access token configuration

`AccessTokenConfig` SHALL include exactly the fields required for the static-token mode:

- `AccessToken` (string, required): pre-minted short-lived access token JWT to forward as `X-Access-Token` on every model call.
- `BaseURL` (string, required): base URL of the Grafana hosted ai-sdk provider-wire endpoint.
- `HTTPClient` (`*http.Client`, optional): override for the underlying HTTP client used for provider-wire requests.

`AccessTokenConfig` MUST NOT include `Namespace` or `Audience` fields. Those values are claims inside the pre-minted JWT and are never sent on the wire to the hosted endpoint by the provider in this mode.

#### Scenario: Required fields are validated at construction

- **WHEN** either `AccessToken` or `BaseURL` is empty
- **THEN** `NewWithAccessToken` returns an error rather than producing a misconfigured provider

#### Scenario: BaseURL validation matches the cloud-auth constructor

- **WHEN** `BaseURL` is not a valid `http://` or `https://` URL (for example, contains a query string, fragment, or unsupported scheme)
- **THEN** `NewWithAccessToken` returns an error consistent with `NewWithCloudAuth`'s URL validation

#### Scenario: No namespace or audience knobs

- **WHEN** a consumer reads the `AccessTokenConfig` struct
- **THEN** the struct has no `Namespace`, `Audience`, or related fields

### Requirement: Token exchange uses authlib

The provider SHALL acquire the per-call access token via an `authn.TokenExchanger` from `github.com/grafana/authlib/authn`. For providers constructed via `NewWithCloudAuth`, the exchanger MUST be an `authn.TokenExchangeClient` built from the configured `CAPToken` and `TokenExchangeURL`. For providers constructed via `NewWithAccessToken`, the exchanger MUST be `authn.NewStaticTokenExchanger(cfg.AccessToken)`. The provider MUST NOT mint or sign tokens itself and MUST NOT implement its own token cache. The provider MUST NOT expose any public functional option for injecting custom `authn.TokenExchanger` implementations.

#### Scenario: Cloud-auth provider exchanges via authlib

- **WHEN** the provider makes an HTTP provider-wire call to the hosted endpoint
- **THEN** the access token attached as `X-Access-Token` was obtained via `authn.TokenExchangeClient.Exchange` using the configured CAP and token-exchange URL

#### Scenario: Access-token provider uses static exchanger

- **WHEN** a provider constructed via `NewWithAccessToken` makes a provider-wire HTTP call
- **THEN** the access token attached as `X-Access-Token` was obtained via `authn.StaticTokenExchanger` and equals `AccessTokenConfig.AccessToken`

#### Scenario: Token caching is delegated to authlib

- **WHEN** multiple cloud-auth calls happen in close succession
- **THEN** the provider relies on `authlib`'s built-in token caching and does not implement its own

#### Scenario: No public token-exchanger injection

- **WHEN** a consumer reads the public functional-options surface of `providers/grafana`
- **THEN** there is no exported `Option` that injects a custom `authn.TokenExchanger`

### Requirement: Access token mode does not perform CAP exchange

A provider constructed via `NewWithAccessToken` SHALL NOT call any token-exchange URL. It MUST source every per-call access token from the configured static string via `authn.NewStaticTokenExchanger`. The provider MUST NOT mint, sign, parse, or otherwise inspect the access token; it is treated as an opaque bearer.

#### Scenario: No exchange HTTP traffic

- **WHEN** a consumer makes any number of `DoStream` or `DoGenerate` calls against a provider constructed via `NewWithAccessToken`
- **THEN** the provider issues HTTP requests only to the configured `BaseURL` provider-wire endpoint and never to a token-exchange URL

#### Scenario: Access token forwarded as bearer

- **WHEN** the provider makes a provider-wire HTTP call
- **THEN** the request carries `X-Access-Token: <AccessTokenConfig.AccessToken>` exactly as supplied at construction

### Requirement: Optional user ID token forwarding

The provider SHALL expose a context helper to attach a user ID token to a `context.Context`. When the helper has been used on the call's context, the provider MUST forward the token in the `X-Grafana-Id` HTTP header. When absent, the provider MUST NOT set the header.

#### Scenario: User-attached context

- **WHEN** the consumer calls `grafana.WithUserIDToken(ctx, idToken)` and passes the resulting context to `aisdk.StreamText`
- **THEN** the outbound HTTP request carries `X-Grafana-Id: <idToken>`

#### Scenario: No user attached

- **WHEN** the call's context has no ID token attached
- **THEN** the outbound HTTP request omits `X-Grafana-Id` entirely

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

### Requirement: Error reconstruction preserves retry semantics

The provider SHALL surface server and transport errors as
`*provider.APICallError` where possible. Server-side provider failures SHALL
cross the HTTP boundary as JSON `provider.APICallError` values. If a server or
transport failure cannot be decoded as `provider.APICallError`, the client SHALL
synthesize one with best-effort status, URL, response body, response headers,
cause, and retryability.

The provider SHALL preserve the decoded error's structured payload in
`APICallError.Data` end-to-end (the field already round-trips on the wire). As
the gateway analog of the Vercel AI SDK gateway, the provider SHALL run the
`provider` package normalizer on the decoded error and, when a category is
identified, surface a `*grafana.GatewayError` carrying the originating
`*provider.APICallError` as its cause. When no category can be identified the
provider SHALL surface the plain `*provider.APICallError`. Either way,
`errors.As(&provider.APICallError{})` SHALL still yield the decoded status,
headers, body, `Data`, and `IsRetryable`.

#### Scenario: Server returns a retryable error

- **WHEN** the server returns or streams an API-call error payload with `isRetryable: true`
- **THEN** the provider surfaces an error from which `errors.As` yields a `*provider.APICallError` with `IsRetryable == true`, and `aisdk.StreamText` retries according to its retry configuration

#### Scenario: Non-retryable error

- **WHEN** the server returns or streams an API-call error payload with `isRetryable: false`
- **THEN** the provider surfaces an error from which `errors.As` yields a `*provider.APICallError` with `IsRetryable == false` and `aisdk.StreamText` does not retry

#### Scenario: Decoded error preserves Data

- **WHEN** the wire error payload carries a structured error in `data`
- **THEN** the surfaced `*provider.APICallError.Data` SHALL equal the decoded `data` payload

#### Scenario: Categorized error surfaces a GatewayError

- **WHEN** the decoded error's structured type maps to a normalized category (e.g. `rate_limit_exceeded`, `authentication_error`, `model_not_found`)
- **THEN** the provider surfaces a `*grafana.GatewayError` with the corresponding `Type`, and `errors.As(&provider.APICallError{})` still yields the decoded `*provider.APICallError`

#### Scenario: Uncategorized error stays a plain APICallError

- **WHEN** the decoded error carries no identifiable structured type
- **THEN** the provider surfaces a plain `*provider.APICallError` as before

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

The Grafana provider SHALL rely on the root `gateway/providerwire` package for exhaustive JSON schema round-trip coverage. The provider module SHALL add transport-level tests over representative values, but MUST NOT duplicate every root wire schema test unless the provider introduces additional transformations.

#### Scenario: No provider-specific schema DTOs

- **WHEN** the Grafana provider source is inspected
- **THEN** there are no parallel DTO structs for `CallOptions`, `StreamPart`, `GenerateResult`, or `APICallError`; the provider uses the root `provider` types and `gateway/providerwire` helpers directly
