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

The public `gateway/providerwire` package is the active tolerant legacy
transport. It carries provider-level call options, results, errors, and stream
parts, and remains the default for Grafana. It intentionally does not emit UI
message chunks or the browser `[DONE]` sentinel.

The sibling `gateway/providerwire/v4` directory contains the future strict
contract: OpenAPI 3.1, curated JSON Schema 2020-12 resources, stock-client
captures, response projections, and executable validation. It has no decoder,
handler, client, model adapter, host policy, or public DTO API, so it cannot be
mounted or selected at runtime.

The contract uses the exact package versions in
`test/conformance/upstream.yaml` as executable authority. Its registered source
commit has older Gateway, provider-utils, and ai workspace manifests; relied-on
source paths therefore carry explicit release-equivalence evidence, while ai
orchestration is captured from the exact installed package. Captures establish
stock-client request emission or response consumption only. They do not claim
private Vercel server behavior or live provider-recording provenance.

Compatibility is semantic JSON compatibility, not byte identity. Standard
objects and selected union arms are closed, while only documented opaque JSON
and keyed provider boundaries remain open. Moving the baseline requires one
coordinated update of the manifest, package pins, source evidence, schemas,
captures, projections, negative corpus, parity map, and lockfiles.

This contract phase defines status, media type, JSON SSE event framing, and EOF
termination. Server commitment, flushing, cancellation, timeouts, write
failures, and post-commit errors belong to the later streaming-service phase.
Until a later capability implements and adopts a strict runtime, use the legacy
package shown in this guide.

## Reference

- [`gateway/providerwire` package](https://pkg.go.dev/github.com/grafana/ai-sdk/gateway/providerwire)
- [Grafana Cloud provider](../providers/grafana-cloud.md)

---

← [Gateway model catalog](gateway-model-catalog.md) · [Docs index](../README.md) · [Choose a model service](../README.md#choose-a-model-service)
