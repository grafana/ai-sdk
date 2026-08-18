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
