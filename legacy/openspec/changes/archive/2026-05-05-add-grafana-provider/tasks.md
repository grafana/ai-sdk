## 1. Module setup

- [x] 1.1 Create `providers/grafana/` directory at the repo root.
- [x] 1.2 Initialize `providers/grafana/go.mod` with module path `github.com/grafana/ai-sdk/providers/grafana`, Go 1.26, and a local replace to `../../` for development.
- [x] 1.3 Add dependencies: root `github.com/grafana/ai-sdk`, `github.com/grafana/authlib`, and `github.com/stretchr/testify`.
- [x] 1.4 Add package documentation for the Grafana provider module.
- [x] 1.5 Wire the new module into root `Makefile` `build`, `test`, `test-short`, `vet`, `lint`, and `tidy` targets so `make check` covers it.

## 2. Public configuration and constructors

- [x] 2.1 Define `CloudAuthConfig` with required fields `CAPToken`, `TokenExchangeURL`, `Namespace`, `BaseURL`, optional `Audience` defaulting to `"ai-sdk"`, and optional `HTTPClient`.
- [x] 2.2 Define functional options for testability and future knobs, including injecting an `authn.TokenExchanger` and overriding the HTTP client when not supplied by config.
- [x] 2.3 Implement `NewWithCloudAuth(cfg CloudAuthConfig, opts ...Option) (*Provider, error)` that validates required fields, normalizes `BaseURL`, applies defaults, constructs auth, and returns a registry-compatible provider.
- [x] 2.4 Implement `Provider` such that it satisfies `registry.Provider` with `LanguageModel(modelID string) (provider.LanguageModel, error)`.
- [x] 2.5 Add `WithUserIDToken(ctx context.Context, idToken string) context.Context` plus an unexported context key and getter.

## 3. LanguageModel implementation

- [x] 3.1 Implement the `model` struct with `provider.LanguageModel` methods: `SpecificationVersion`, `Provider`, `ModelID`, `SupportedURLs`, `DoStream`, `DoGenerate`.
- [x] 3.2 `SpecificationVersion()` returns `"v4"`.
- [x] 3.3 `Provider()` returns the stable identifier `"grafana"`.
- [x] 3.4 `ModelID()` returns the model ID passed to `LanguageModel(modelID)`.
- [x] 3.5 `SupportedURLs()` returns `nil` for v1.

## 4. Auth pipeline

- [x] 4.1 Construct `authn.NewTokenExchangeClient` once at provider construction time unless a test `authn.TokenExchanger` option is supplied.
- [x] 4.2 Per model call, call `Exchange` with `Namespace: cfg.Namespace` and `Audiences: []string{cfg.Audience}`.
- [x] 4.3 Attach the resulting access token as `Authorization: Bearer <token>` on the outbound HTTP provider-wire request.
- [x] 4.4 If `WithUserIDToken` set an ID token on `ctx`, set `X-Grafana-Id: <token>` on the outbound HTTP request. Otherwise omit the header entirely.
- [x] 4.5 Surface token exchange failures as wrapped local errors before issuing the model-call HTTP request.

## 5. provider/wire HTTP request construction

- [x] 5.1 Build requests to `BaseURL + wire.PathLanguageModel` using method `POST`.
- [x] 5.2 Encode the request body with `wire.EncodeCallOptions(opts)` for both `DoStream` and `DoGenerate`.
- [x] 5.3 Set `Content-Type: wire.MIMEJSON` on every request.
- [x] 5.4 Set `wire.HeaderModelID` to the model ID.
- [x] 5.5 Set `wire.HeaderSpecVersion` to `wire.SpecVersionV4`.
- [x] 5.6 Set `wire.HeaderStreaming` to `"true"` for `DoStream` and `"false"` for `DoGenerate`.
- [x] 5.7 Set `Accept: wire.MIMESSE` for `DoStream` and `Accept: wire.MIMEJSON` for `DoGenerate`.
- [x] 5.8 Treat `wire.EncodeCallOptions` errors as local non-retryable failures and do not issue the HTTP request.

## 6. Error mapping

- [x] 6.1 Decode HTTP non-2xx responses with `wire.DecodeErrorResponse` whenever possible.
- [x] 6.2 If error-body decoding fails, synthesize `*provider.APICallError` from status code, URL, headers, response body, and decode cause.
- [x] 6.3 Map transport failures before response headers to synthesized retryable `*provider.APICallError` unless caused by context cancellation.
- [x] 6.4 For streaming failures after a 2xx response starts, emit a final `provider.StreamPart{Type: provider.PartError, APICallError: err}` on the channel and then close.
- [x] 6.5 Preserve server-provided `APICallError.IsRetryable`, `StatusCode`, `ResponseHeaders`, `ResponseBody`, `URL`, and `Data` exactly when decoding succeeds.

## 7. DoStream

- [x] 7.1 Encode call options, attach auth and provider-wire headers, and issue the HTTP request.
- [x] 7.2 On non-2xx response, close the body and return a decoded or synthesized `*provider.APICallError` from `DoStream`.
- [x] 7.3 On 2xx response, return `*provider.StreamResult{Stream: ch}` with a buffered channel of size 64.
- [x] 7.4 In a goroutine, read `resp.Body` with `wire.NewSSEReader` and forward every decoded `provider.StreamPart` in order.
- [x] 7.5 Treat `io.EOF` from the SSE reader as clean completion and close the channel without synthetic events.
- [x] 7.6 On SSE read or decode error, emit a final `PartError` with a synthesized `*provider.APICallError`, close the response body, and close the channel.
- [x] 7.7 Respect `ctx.Done()` through `http.NewRequestWithContext` and avoid panics or goroutine leaks on cancellation.

## 8. DoGenerate

- [x] 8.1 Encode call options, attach auth and provider-wire headers, and issue the HTTP request.
- [x] 8.2 On non-2xx response, return the decoded or synthesized `*provider.APICallError`.
- [x] 8.3 On 2xx response, decode the body with `wire.DecodeGenerateResult` and return it.
- [x] 8.4 Close response bodies in all success and error paths.

## 9. Fake hosted endpoint test infrastructure

- [x] 9.1 Add an in-module fake HTTP server that handles `wire.PathLanguageModel` and validates method, model ID header, streaming header, spec-version header, content type, accept header, authorization header, and optional `X-Grafana-Id` header.
- [x] 9.2 The fake server must decode request bodies with `wire.DecodeCallOptions` and expose captured requests to tests.
- [x] 9.3 The fake server must support unary success with `wire.EncodeGenerateResult`, unary errors with `wire.WriteErrorResponse`, streaming success with `wire.WriteSSEStreamPart`, malformed error bodies, and abrupt/malformed streams.
- [x] 9.4 Add a fake `authn.TokenExchanger` so tests do not call auth-api.

## 10. Integration and transport tests (in-module)

- [x] 10.1 Test successful streaming end-to-end: JSON call options reach the fake server, SSE stream parts decode through `DoStream`, and the channel closes cleanly on EOF.
- [x] 10.2 Test successful non-streaming end-to-end via `DoGenerate`.
- [x] 10.3 Test retryable HTTP error followed by success on retry with `aisdk.StreamText(WithMaxRetries(2))`.
- [x] 10.4 Test non-retryable HTTP error produces no retry.
- [x] 10.5 Test mid-stream transport/decode failure produces `PartError` with retryable `APICallError`.
- [x] 10.6 Test context cancellation cancels the HTTP request, closes resources, and does not panic.
- [x] 10.7 Test representative `provider.StreamPart` shapes pass through the fake HTTP/SSE endpoint without field loss.
- [x] 10.8 Test representative `provider.CallOptions` fields pass through the fake endpoint without field loss, relying on root `provider/wire` for exhaustive schema round-trip coverage.

## 11. Conformance suite integration (`test/conformance/`)

This group reuses the existing fixture-based conformance harness so the Grafana provider is held to the same byte-identical equivalence bar as the Anthropic provider, against the same `expected.jsonl` files.

- [x] 11.1 Read `test/conformance/runner.go` and `test/conformance/anthropic/conformance_test.go` end-to-end.
- [x] 11.2 Update `test/conformance/go.mod` with a replace and require entry for `github.com/grafana/ai-sdk/providers/grafana`.
- [x] 11.3 Add `test/conformance/grafana/conformance_test.go` gated with `//go:build conformance` that discovers the Anthropic fixture cases without duplicating fixture files.
- [x] 11.4 Add a fake Grafana hosted endpoint for conformance that implements `POST /language-model`, validates provider-wire/auth headers, decodes JSON call options, translates Anthropic fixture events into `provider.StreamPart` values, and writes SSE events with `wire.WriteSSEStreamPart`.
- [x] 11.5 Implement the Grafana `ProviderFactory` so it constructs a Grafana provider pointed at the fake endpoint `BaseURL`, with a stub token exchanger returning a fixed access token.
- [x] 11.6 Wire the Grafana run to compare against the same `expected.jsonl` the Anthropic run uses for the same case.
- [x] 11.7 Ensure `make test-conformance` runs both Anthropic and Grafana harnesses.
- [x] 11.8 Confirm provider-tool, MCP, code execution, thinking, citations, and providerOptions fixtures remain byte-identical to the direct Anthropic path.
- [x] 11.9 Add one Grafana-specific auth scenario asserting `X-Grafana-Id` reaches the fake endpoint.
- [x] 11.10 Update `test/conformance/README.md` with the Grafana shared-fixture provider-wire fake-endpoint pattern.

## 12. Auth tests

- [x] 12.1 Test that `Authorization: Bearer <access-token>` is set on every outbound model-call request.
- [x] 12.2 Test that `X-Grafana-Id` is set when `WithUserIDToken` was applied to the context and absent otherwise.
- [x] 12.3 Test that token exchange failures surface as clear local errors.
- [x] 12.4 Test that default audience `"ai-sdk"` and configured audience overrides reach the token exchange request.
- [x] 12.5 Test that `CAPToken` and `TokenExchangeURL` are used to construct authlib's token exchange client in the non-injected path.

## 13. Registry integration

- [x] 13.1 Add a registry test: register the provider as `"grafana"`, resolve `"grafana:claude-sonnet-4-5-20250929"`, and verify the returned `LanguageModel` reports `Provider() == "grafana"` and `ModelID() == "claude-sonnet-4-5-20250929"`.
- [x] 13.2 Add a middleware composition test using `registry.WithLanguageModelMiddleware`.

## 14. Documentation

- [x] 14.1 Add `providers/grafana/README.md` documenting cloud auth setup, quick start, `X-Grafana-Id` forwarding, audience name, error semantics, and the transparent provider-wire model.
- [x] 14.2 Add `providers/grafana/AGENTS.md` covering module-specific test commands, package structure, and provider-wire alignment notes.
- [x] 14.3 Update the root `README.md` providers section with a short pointer to the Grafana provider package and its README.
- [x] 14.4 Add doc comments to public `Provider`, `CloudAuthConfig`, `NewWithCloudAuth`, option helpers, and `WithUserIDToken` symbols.

## 15. Release prep

- [x] 15.1 Run `make fmt`, `make vet`, `make lint`, `make test`, and `make test-conformance` from the repo root and ensure all targets cover the new module and Grafana conformance run.
- [x] 15.2 Add a CHANGELOG entry or equivalent release note for the new module.
- [x] 15.3 Verify `go vet ./...` and `go test ./...` from inside `providers/grafana/` pass.
- [x] 15.4 Confirm `go.sum` for the new module is committed and tidy with `go mod tidy`.
