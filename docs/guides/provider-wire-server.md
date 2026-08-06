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
DTOs, direct model-catalog resolution, safe errors, and bounded responses.

Mount the handlers under distinct base URLs because both serve the same
`/language-model` relative path:

```go
strictHandler, err := providerwirev4.NewHandler(
	modelCatalog,
	providerwirev4.WithTotalTimeout(2*time.Minute),
	providerwirev4.WithIdleTimeout(time.Minute),
)
if err != nil {
	return err
}

mux.Handle("/legacy"+providerwire.PathLanguageModel, legacyHandler)
mux.Handle("/strict"+providerwirev4.PathLanguageModel, strictHandler)
```

The strict handler passes the request-derived context and exact requested model
ID to `catalog.ModelResolver`, then invokes the returned model directly. It does
not add gateway middleware, identity, metadata, request IDs, policy, or a stream
proxy. Authenticate and decorate the catalog resolver outside the handler.

An absent or empty top-level `providerOptions.gateway` object is removed before
model invocation. Non-empty gateway controls and raw-chunk requests are rejected
before catalog resolution because this handler has no routing or raw-exposure
policy seam.

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
followed by one space, the JSON bytes, and the terminating blank line. These new
Grafana response limits apply only with `WithStrictProviderWire()`; default
legacy clients retain their original readers and behavior.

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

Legacy host adapters map `catalog.ErrUnknownModel` to their existing
non-retryable not-found API error. The strict handler recognizes that catalog
sentinel directly and redacts other catalog failures as internal errors.

## Configure cancellation and limits

The strict total timeout bounds catalog resolution, model invocation, and stream
consumption after request validation. The idle timeout detects an established
stream that stops producing parts. Both handlers rely on models and tools to
honor context cancellation; neither launches a goroutine to force a blocking
provider call to return.

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
