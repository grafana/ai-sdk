## Context

The Grafana AI SDK ships providers as independent modules when they need provider-specific dependencies. A provider implements `provider.LanguageModel`, can be resolved through `registry.ProviderRegistry`, and is consumed by `aisdk.StreamText`. Tools, multi-step orchestration, middleware, fallback, and `@ai-sdk/react` UI SSE emission stay in the consumer process.

PR 162 changed the implementation foundation for a remote provider: provider runtime types are now JSON-serializable, and `provider/wire` defines a JSON+HTTP/SSE Go-to-Go provider wire. That wire intentionally follows the upstream Vercel AI SDK gateway at the protocol level: one `/language-model` endpoint, headers for model/spec/streaming mode, JSON request/response bodies, SSE for streams, and structured API-call errors.

This change adds a `providers/grafana/` module that makes Grafana's hosted assistant backend look like a direct `provider.LanguageModel` by using the existing `provider/wire` contract. It must not reintroduce protobuf, Connect, generated DTOs, or a parallel schema tree.

## Goals / Non-Goals

**Goals:**

- Implement `provider.LanguageModel` so consumers use the Grafana provider like Anthropic: create or register it, resolve a model ID, call `aisdk.StreamText`.
- Use the current JSON-serializable `provider.CallOptions`, `provider.StreamPart`, `provider.GenerateResult`, and `provider.APICallError` shapes directly on the wire.
- Use the `provider/wire` package for route/header constants, request/response JSON encoding, SSE stream-part encoding/decoding, and HTTP error decoding.
- Keep the protocol close to upstream AI SDK gateway conventions: `POST /language-model`, `ai-language-model-*` headers, JSON body, SSE stream events, no `[DONE]` sentinel.
- Preserve `provider.APICallError.IsRetryable` across HTTP and stream failures so retry/fallback semantics match direct providers.
- Use Grafana Cloud auth via `github.com/grafana/authlib/authn`: CAP token exchange, audience `ai-sdk` by default, and optional per-request `X-Grafana-Id` forwarding from `context.Context`.
- Keep `authlib` isolated in the separate `providers/grafana/` module.

**Non-Goals:**

- Server-side endpoint implementation, model catalog, RBAC, telemetry, quota, and billing. Those are tracked in `grafana-assistant-app`.
- Server-side orchestration. The hosted service receives one provider-level model call and returns provider-level results.
- Non-cloud auth modes in v1.
- Wire compatibility with `@ai-sdk/react`, OpenAI, Anthropic, or TypeScript `LanguageModelV4` payloads. The protocol shape follows upstream gateway, but payload schemas are this Go SDK's JSON-serializable provider types.
- Protobuf, Connect, gRPC, or generated wire DTOs.

## Decisions

### D1. Module layout: `providers/grafana/` with its own `go.mod`

The new package lives at `providers/grafana/` with module path `github.com/grafana/ai-sdk/providers/grafana`. It requires the root `github.com/grafana/ai-sdk` module for `provider`, `provider/wire`, and `registry`, plus `github.com/grafana/authlib` for cloud auth.

**Why**: matches the provider-module pattern and keeps authlib out of consumers that only import the root SDK or other providers.

### D2. Wire protocol: provider/wire JSON+HTTP/SSE, gateway-shaped

The provider sends all model calls to `BaseURL + providerwire.PathLanguageModel` with method `POST`.

Every request sets:

- `Content-Type: application/json`
- `Accept: application/json` for generate calls or `Accept: text/event-stream` for stream calls
- `ai-language-model-id: <modelID>`
- `ai-language-model-streaming: true|false`
- `ai-language-model-specification-version: 4`
- `Authorization: Bearer <access-token>`
- optional `X-Grafana-Id: <id-token>`

The request body is `providerwire.EncodeCallOptions(opts)`.

For `DoGenerate`, a 2xx response body is decoded with `providerwire.DecodeGenerateResult`. A non-2xx response is decoded with `providerwire.DecodeErrorResponse` when possible, otherwise synthesized as `*provider.APICallError` using the HTTP status, response body, response headers, and request URL.

For `DoStream`, a 2xx response body is read with `providerwire.NewSSEReader(resp.Body)`. Each decoded `provider.StreamPart` is forwarded to the returned channel in order. `io.EOF` is clean stream completion. A read or decode error after the stream starts is emitted as a final `provider.StreamPart{Type: provider.PartError, APICallError: <error>}`.

**Why**: this uses the current codebase's wire contract and aligns with upstream AI SDK gateway routing/framing without adding a second schema system.

### D3. Runtime types are the wire payload

The provider must not create Grafana-specific DTOs for call options, stream parts, generate results, or API-call errors. It uses `provider/wire` helpers over the existing provider runtime types.

**Why**: PR 162 made these types losslessly JSON-serializable specifically so remote provider transports do not need protobuf or conversion layers. A thin provider client reduces drift when provider types evolve.

### D4. Streaming channel behavior

`DoStream` validates and encodes call options, acquires auth, issues the HTTP request, and returns `*provider.StreamResult{Stream: ch}` with a buffered channel of size 64 once a 2xx streaming response is available. A goroutine owns `resp.Body`, decodes SSE events, sends stream parts, drains/ closes the body, and closes the channel.

If request construction, auth, request execution, or pre-stream HTTP status handling fails before a stream exists, `DoStream` returns an error. If stream decoding or transport fails after a 2xx stream response has begun, the goroutine emits `PartError` and closes the channel.

**Why**: this matches direct-provider semantics: setup failures are returned from `DoStream`; mid-stream failures are represented as stream error parts so `aisdk.StreamText` can surface or retry based on `APICallError.IsRetryable`.

### D5. Cloud auth: CAP token to access token via authlib

Constructor: `grafana.NewWithCloudAuth(cfg CloudAuthConfig, opts ...Option) (*Provider, error)` returns a registry-compatible provider. `CloudAuthConfig` carries `CAPToken`, `TokenExchangeURL`, `Namespace`, `BaseURL`, optional `Audience` defaulting to `"ai-sdk"`, and optional `HTTPClient`.

At construction time, the provider builds an `authn.TokenExchangeClient` with `authn.NewTokenExchangeClient`. Tests can inject an `authn.TokenExchanger` through an option so they do not call auth-api. On each model call, the provider calls `Exchange` with `Namespace` and `Audiences: []string{Audience}`, then attaches the returned token as `Authorization: Bearer <token>` on the outbound HTTP request.

`grafana.WithUserIDToken(ctx, idToken)` stores an ID token in context. If present and non-empty, the provider forwards it as `X-Grafana-Id` on the model-call request.

**Why**: this is the canonical Grafana Cloud token exchange path, keeps service auth cached in authlib, and scopes user identity to the individual request context.

### D6. Error model

HTTP non-2xx responses are decoded as JSON `provider.APICallError` via `provider/wire` whenever possible. If decoding fails or the failure is a transport-level error, the provider synthesizes a `*provider.APICallError` with retryability inferred by `provider.NewAPICallError` from status code when a status is available. Network failures and stream receive failures should be retryable unless they are caused by context cancellation.

Stream-level `PartError` events from the server already carry `*provider.APICallError` in `StreamPart.APICallError`; the provider forwards them unchanged. If a malformed stream `PartError` has nil `APICallError`, `aisdk.StreamText` has a defensive synthesizer, but the Grafana provider should still treat nil details as a provider bug in tests.

**Why**: `aisdk.StreamText`, retry, and fallback inspect `*provider.APICallError` and `IsRetryable`. Opaque error strings would change behavior compared to a direct provider.

### D7. Registry composition

`Provider` implements `registry.Provider` with `LanguageModel(modelID string) (provider.LanguageModel, error)`. Consumers can use:

`registry.NewProviderRegistry(map[string]registry.Provider{"grafana": grafanaProvider})`

and resolve `grafana:claude-sonnet-4-5-20250929`. The returned language model reports `Provider() == "grafana"` and `ModelID() == modelID`.

### D8. Conformance through an HTTP/SSE fake hosted endpoint

The conformance run should reuse the Anthropic fixture cases, but the fake Grafana hosted endpoint speaks provider-wire, not Anthropic's API. For each fixture step, the fake endpoint receives `POST /language-model`, validates the gateway-style headers and auth headers, decodes JSON `provider.CallOptions`, converts the fixture's Anthropic events through the existing Anthropic conversion path into `provider.StreamPart` values, and writes those values with `providerwire.WriteSSEStreamPart`.

The resulting `UIMessageChunk` output from `aisdk.StreamText` + `ToUIMessageStream` must match the same `expected.jsonl` used by the direct Anthropic conformance run.

**Why**: unit tests prove provider-wire JSON round trips; conformance proves the full transparent-transport property across the hosted-provider boundary.

## Risks / Trade-offs

- **Wire coupling with hosted assistant**: both repos must use the same `provider/wire` JSON schema and gateway-style headers. Mitigation: shared contract docs, fake-server tests, and paired changes in `grafana-assistant-app`.
- **Go JSON schema is not TypeScript payload compatibility**: the protocol is gateway-shaped, but payloads are Go provider JSON. Mitigation: document this explicitly and keep `@ai-sdk/react` SSE local to the consumer.
- **`authlib` is a heavy transitive dependency**: isolated to `providers/grafana/`.
- **Provider types may evolve**: mitigated by root `provider/wire` round-trip tests and provider fake-server tests that fail when headers or schemas drift.
- **Mid-stream transport errors after partial output**: mapped to `PartError`, matching existing provider-channel behavior.
- **No non-cloud auth in v1**: constructor naming leaves room for future auth constructors.

## Test Strategy

- Unit tests for constructor validation, defaults, auth metadata, user token forwarding, registry integration, and middleware composition.
- Fake HTTP hosted-endpoint tests for successful streaming, successful generate, context cancellation, request decoding, headers, auth, non-2xx error decoding, malformed error fallback, and mid-stream read/decode failure mapping.
- Transport tests that assert requests use `provider/wire` constants and current JSON-serializable provider schemas.
- Retry tests that confirm retryable and non-retryable `APICallError` values affect `aisdk.StreamText` as expected.
- Conformance tests that reuse Anthropic fixtures and expected `UIMessageChunk` output through an HTTP/SSE fake Grafana endpoint.
