# Serving provider-wire models

Use `gateway/providerwire` to centralize model access behind an HTTP service for
other AI SDK backends. Client services can call the remote model with the same
messages, tools, options, errors, and streaming behavior they use with a local
model.

Provider wire serves backend-to-backend model calls. Browser chat endpoints use
the UI stream helpers in [Streaming over HTTP](streaming-http.md).

## Mount a model endpoint

A resolver applies host policy and returns the model requested by the client:

```go
resolver := providerwire.ModelResolverFunc(
	func(r *http.Request, modelID string) (provider.LanguageModel, error) {
		return resolveModel(r.Context(), modelID)
	},
)

handler, err := providerwire.NewHandler(
	resolver,
	providerwire.WithTotalTimeout(2*time.Minute),
	providerwire.WithIdleTimeout(time.Minute),
	providerwire.WithMaxRequestBodyBytes(8<<20),
)
if err != nil {
	return err
}

mux.Handle(providerwire.PathLanguageModel, authenticate(handler))
```

The handler validates and decodes provider calls, invokes the model, and writes
unary JSON or streaming SSE responses. It derives model contexts from the HTTP
request so disconnects and deadlines can cancel work.

## Migrate to the strict V4 service

The legacy handler remains available for clients that depend on tolerant Go-only
payloads or its existing error disclosure. The strict handler implements the
same external LanguageModelV4 route and headers through independent canonical
DTOs, a shared execution runtime, safe errors, and bounded responses.

Mount the handlers under distinct base URLs because both serve the same
`/language-model` relative path:

```go
catalogResolver, err := runtime.AdaptCatalogResolver(modelCatalog)
if err != nil {
	return err
}

gatewayRuntime, err := runtime.New(
	catalogResolver,
	runtime.WithCallPolicies(callPolicy),
	runtime.WithMiddleware(modelMiddleware...),
)
if err != nil {
	return err
}

strictHandler, err := providerwirev4.NewHandler(
	gatewayRuntime,
	providerwirev4.WithMetadataExtractor(authenticatedMetadata),
)
if err != nil {
	return err
}

mux.Handle("/legacy"+providerwire.PathLanguageModel, legacyHandler)
mux.Handle("/strict"+providerwirev4.PathLanguageModel, strictHandler)
```

The metadata extractor runs after host authentication. It may supply a gateway
request ID and authenticated tenant or project attributes. The handler generates
a request ID when one is absent. Request bodies and caller headers never become
trusted metadata automatically.

The default catalog adapter accepts calls without gateway routing controls. It
rejects non-empty `providerOptions.gateway` controls it cannot honor rather than
forwarding or ignoring them. A host that supports provider ordering, fallback,
BYOK, or other controls supplies a call-aware runtime resolver and policy.

Migrate a Grafana client explicitly by changing both its base URL and codec:

```go
client, err := grafana.NewWithAccessToken(
	grafana.AccessTokenConfig{
		AccessToken: token,
		BaseURL:     "https://gateway.example.com/strict",
	},
	grafana.WithStrictProviderWire(),
)
```

The default Grafana mode remains legacy. There is no automatic codec negotiation
and an established streaming POST is never replayed during cutover. Roll back by
restoring the legacy base URL and removing the strict option.

Strict request reads are bounded before unbounded allocation. Unary results and
SSE events are encoded and size-checked before their bytes are committed, but
encoding may allocate a value that is subsequently rejected. The server and
strict Grafana client count complete canonical events identically: `data:`
followed by one space, the JSON bytes, and the terminating blank line.

The intended follow-up end state is to make strict V4 canonical after adoption
evidence, then switch Grafana's default and remove its mode selection. Legacy
provider wire and provider-owned transport JSON can be deprecated and removed
only through a coordinated breaking change.

## Keep host policy outside the handler

The host remains responsible for:

- authentication and authorization;
- tenant and user identity;
- choosing which model IDs are visible;
- request-body limits appropriate to the deployment;
- rate limits, billing, logging, and audit policy;
- route prefixes and public error policy.

Perform authentication before model resolution. Do not let a client-controlled
model ID bypass the same allowlist used by model discovery.

## Resolve public model IDs

Adapt a [gateway model catalog](gateway-model-catalog.md) when clients should use
stable public names. Preserve `ResolvedModel.ID` for policy and logs, even though
the handler ultimately needs only the resolved `LanguageModel`.

Map `catalog.ErrUnknownModel` to a non-retryable not-found API error. Let other
provider and infrastructure errors retain their classification so the handler
can normalize them consistently.

## Configure cancellation and limits

The total timeout bounds the call after validation and resolution. The idle
timeout detects a streaming model that stops producing parts. Models and tools
must honor context cancellation for those bounds to work.

Provider wire exports timeout sentinel errors for host observability. Classify
client cancellation separately from provider failure and server timeout.

## Understand the protocol boundary

Provider wire transports provider-level call options, results, errors, and
stream parts. It intentionally does not emit UI message chunks or the browser
`[DONE]` sentinel. The wire format tracks the registered upstream
LanguageModelV4 baseline so upstream gateway clients and the Go Grafana provider
can use the same endpoint.

## Reference

- [`gateway/providerwire` package](https://pkg.go.dev/github.com/grafana/ai-sdk/gateway/providerwire)
- [Grafana Cloud provider](../providers/grafana-cloud.md)

---

← [Gateway model catalog](gateway-model-catalog.md) · [Docs index](../README.md) · [Choose a model service](../README.md#choose-a-model-service)
