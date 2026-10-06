## Why

Grafana plugin teams and core Grafana developers building Go services need access to LLMs without provisioning vendor keys, implementing provider fallback, or building custom billing/telemetry integrations. Grafana already operates a hosted assistant backend that handles this. The ai-sdk should expose that backend as a first-class `LanguageModel` provider so consumers can use it through `aisdk.StreamText`, middleware, fallback, and the registry like any direct provider.

PR 162 made the provider runtime types JSON-serializable and added the `provider/wire` HTTP+SSE helpers. This change should build on that direction instead of introducing protobuf/Connect. The Grafana provider becomes a thin upstream-gateway-style HTTP client: provider call options go out as JSON, streaming results come back as JSON `provider.StreamPart` events over SSE, and unary results come back as JSON `provider.GenerateResult`.

## What Changes

- Add a new `providers/grafana/` package as a separate Go module.
- Implement `provider.LanguageModel` that calls Grafana's hosted ai-sdk endpoint using the shared `provider/wire` JSON+HTTP/SSE contract.
- Use an upstream AI SDK gateway-style protocol shape:
  - single `POST /language-model` endpoint
  - `ai-language-model-id`, `ai-language-model-streaming`, and `ai-language-model-specification-version` headers
  - request body is JSON `provider.CallOptions`
  - unary success body is JSON `provider.GenerateResult`
  - streaming success body is `text/event-stream` of JSON `provider.StreamPart` events
  - HTTP non-2xx errors carry JSON `provider.APICallError`
- Preserve transparent provider semantics: no server-side orchestration, no React/UI SSE from the hosted service, and no tool execution on the provider client side beyond what `aisdk.StreamText` already performs locally.
- Add cloud-only authentication built on `github.com/grafana/authlib/authn`:
  - exchange a Cloud Access Policy token for short-lived access tokens with audience `ai-sdk` by default
  - attach `Authorization: Bearer <token>` to provider-wire requests
  - optionally forward a per-request `X-Grafana-Id` ID token from `context.Context`
- Register seamlessly with `registry.ProviderRegistry` so consumers can resolve models such as `grafana:claude-sonnet-4-5-20250929`.
- Preserve retry semantics by reconstructing or synthesizing `*provider.APICallError` from HTTP failures and stream `PartError` events.

## Capabilities

### New Capabilities

- `grafana-provider`: JSON+HTTP/SSE-backed `provider.LanguageModel` implementation that proxies model calls to Grafana's hosted ai-sdk endpoint, with cloud-mode authentication, lossless provider-wire transport, retryable error reconstruction, and registry integration.

### Modified Capabilities

None. The provider is additive and does not change the `provider.LanguageModel` interface, registry contract, or existing `provider/wire` capability.

## Impact

- **New code**: `providers/grafana/` as a new module sibling to `anthropic/`, with its own `go.mod` to keep `github.com/grafana/authlib` out of the root module dependency graph.
- **Wire contract**: reuses the existing root `provider/wire` JSON+HTTP/SSE helpers and current JSON-serializable provider schemas. No protobuf, no Connect, no generated wire package.
- **Hosted endpoint coordination**: server-side implementation in `grafana-assistant-app` must implement the same `provider/wire` contract: `POST /language-model`, model/stream/spec headers, JSON call options, JSON generate result, SSE stream parts, and JSON `APICallError` envelopes.
- **Build/test**: add the new module to root `Makefile` targets. Add in-module fake-server tests and conformance coverage through `test/conformance/`.
- **Public surface**: consumers import `github.com/grafana/ai-sdk/providers/grafana` and construct a cloud-auth provider for use with the registry or directly.
- **Auth provisioning**: audience `ai-sdk` and CAP policy provisioning must be coordinated with the assistant/deployment teams, out of scope for this repository.
