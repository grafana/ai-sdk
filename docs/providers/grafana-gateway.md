# Call Grafana AI Gateway from Go

Use the Grafana provider when your service calls an internally provisioned
Grafana AI Gateway. It uses the same generation, streaming, and registry
interfaces as other Go providers. Install the separate client module:

```bash
go get github.com/grafana/ai-sdk/providers/grafana
```

The client module is Apache-2.0 and does not depend on the Gateway service
module. Its current executable response family is text. Requests preserve
representable provider options, files, tools, and structured-output settings;
the deployed Gateway decides which capabilities it can execute and returns a
public invalid-request error for unsupported calls.

## Connect with an access token

The base URL is the API prefix, including `/api/v1/aisdk`, rather than the host
root or a complete `/language-model` URL. Obtain the short-lived access token
from your control plane and refresh it in your application when necessary.

```go
client, err := grafana.NewWithAccessToken(grafana.AccessTokenConfig{
	AccessToken: accessToken,
	BaseURL:     "https://gateway.example/api/v1/aisdk",
})
if err != nil {
	return err
}
model, err := client.LanguageModel("assistant")
if err != nil {
	return err
}
result, err := aisdk.GenerateText(ctx, model,
	aisdk.WithModelMessages(provider.UserText("Summarize the incident.")),
)
```

This mode forwards the token as `X-Access-Token` without exchanging or caching
it. Supplying an `Authorization` header alone does not authenticate the Gateway.

## Connect with a Cloud Access Policy token

An internally provisioned service can instead use `NewWithCloudAuth` with its
CAP token, token-exchange URL, namespace, and Gateway base URL. Omitted audience
defaults to `ai-sdk`. Grafana authlib exchanges and caches the short-lived
token; the client adds no second token cache.
Exchange failures use a fixed public message rather than retaining token-service
response prose; cancellation remains identifiable through `errors.Is`.

```go
client, err := grafana.NewWithCloudAuth(grafana.CloudAuthConfig{
	CAPToken:         capToken,
	TokenExchangeURL: tokenExchangeURL,
	Namespace:        namespace,
	BaseURL:          "https://gateway.example/api/v1/aisdk",
})
```

Both constructors accept an HTTP client used for exchange and Gateway calls.
They preserve its transport and timeout settings in a private client value.
Redirects are rejected so Grafana credentials cannot follow a redirect to a
different endpoint. No credentials or endpoint URLs are discovered from the
environment.

For a request acting on behalf of a user, pass a context returned by
`grafana.WithUserIDToken(ctx, userIDToken)`. The Gateway validates that identity
separately from the calling service. An absent or empty user token omits
`X-Grafana-Id`.

## Discover and select public models

`client.ListModels(ctx)` returns public model IDs, names, optional descriptions,
and their specification identifiers. Aliases remain ordinary independent rows
in server order. The client neither caches this catalog nor infers backend or
fallback topology. A malformed or oversized response returns no partial catalog.

Use the returned ID with `client.LanguageModel(id)`, or register the client as a
`registry.Provider`; see [Fallback and registry](../guides/fallback-and-registry.md).

## Bound work and handle errors

Use a cancelable context for each generation or stream. Cancel it when a
consumer stops reading. The client closes its response body and stream channel
on cancellation. It does not replay Gateway requests; SDK retry and fallback
orchestration remain the caller's choice. Authlib may retry its token-exchange
request according to its own policy.

`grafana.DefaultLimits()` provides byte bounds for discovery, unary responses,
errors, and streams, plus stream event-size and event-count bounds. Copy that
value, adjust the required limits, and pass its address in the constructor.
Explicit limits must all be positive. Total stream bytes include wire framing;
event bytes count a complete event with CRLF normalized to one line ending.
Call contexts and HTTP client timeouts set latency bounds.

Use `errors.As` with `*grafana.GatewayError` for the public category/code and
with `*provider.APICallError` for retryability. Context cancellation and deadlines
remain identifiable with `errors.Is`. A malformed response is a non-retryable
protocol failure; a transport failure is retryable but never retried internally.
Warnings and public error messages remain server-provided text. Unknown private
metadata is not promoted into model identity or Gateway error fields. A unary
response's bounded raw HTTP body remains available in `Response.Body`.

Configured headers are applied before call headers, then client-owned auth,
content negotiation, and model protocol headers. Call headers also remain in
the request body, matching the registered client; a text-only Gateway may reject
them. See [package reference](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/grafana)
for configuration and result types.

---

← [Choose a provider](overview.md) · [Docs index](../README.md) · [Fallback and registry →](../guides/fallback-and-registry.md)
