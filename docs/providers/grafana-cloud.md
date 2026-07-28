# Grafana Cloud

Grafana operates a hosted AI SDK endpoint that routes model calls and applies
hosted Agent Observability, tracing, metrics, and usage controls. Access is
currently available only to internal Grafana services whose credentials,
endpoint, namespace, and models are provisioned by Grafana.

The Go SDK handles streaming, tools, retries, multi-step orchestration, and UI
message conversion in the calling service.

## External teams (TODO)

**Status: TODO — public access is not yet supported.**

Customer-created Cloud Access Policy (CAP) tokens and Grafana service account
tokens do not authenticate to the hosted AI SDK endpoint. The authentication
constructors described below depend on internal Grafana token exchange and
provisioning.

TODO: document the supported public credential flow, authorization model,
endpoint and model discovery, tenant and billing context, and token lifecycle
when the external integration becomes available.

Until then, external applications should use one of the direct services in
[Choose a provider](overview.md).

## Install

The remaining guide applies to internally provisioned Grafana services.

```bash
go get github.com/grafana/ai-sdk
go get github.com/grafana/ai-sdk/providers/grafana
```

## Internal prerequisites

The service owner or internal control plane supplies:

- the hosted AI SDK endpoint URL;
- the namespace and model IDs provisioned for the service;
- either an internally provisioned CAP token and token-exchange URL or a
  short-lived access token minted by an internal control plane.

Read these values from server configuration and keep credentials in a secret
manager.

## Make an internally authenticated request

For a service with an internally provisioned CAP token, construct the provider
with cloud authentication and call the model like any other language model:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/grafana"
)

func main() {
	grafanaProvider, err := grafana.NewWithCloudAuth(grafana.CloudAuthConfig{
		CAPToken:         os.Getenv("GRAFANA_CAP_TOKEN"),
		TokenExchangeURL: os.Getenv("GRAFANA_TOKEN_EXCHANGE_URL"),
		Namespace:        os.Getenv("GRAFANA_NAMESPACE"),
		BaseURL:          os.Getenv("GRAFANA_AI_SDK_URL"),
	})
	if err != nil {
		log.Fatal(err)
	}

	model, err := grafanaProvider.LanguageModel("claude-sonnet-5")
	if err != nil {
		log.Fatal(err)
	}

	result := aisdk.StreamText(context.Background(), model,
		aisdk.WithModelMessages(provider.UserText("Explain exemplars in Prometheus.")),
	)
	for part := range result.FullStream() {
		if delta, ok := part.(aisdk.StreamTextDelta); ok {
			fmt.Print(delta.Text)
		}
	}
	if err := result.Err(); err != nil {
		log.Fatal(err)
	}
}
```

`LanguageModel` accepts the model ID without local validation. Model
availability is determined by the hosted deployment. Construct the Grafana
provider once and reuse it across requests.

## Choose an internal authentication flow

Both authentication flows require credentials provisioned through an internal
Grafana service or control plane. A customer-created CAP token or Grafana
service account token cannot be substituted for these credentials.

Both flows send `X-Access-Token` to the hosted endpoint. They differ in who
creates and refreshes that token. Keep every token in server-side secret storage
and never send it to a browser client.

### Exchange an internally provisioned CAP token

`NewWithCloudAuth` is intended for a long-running Grafana service. Its CAP policy
must grant the internal `access-token:sign` scope and permit exchange for the
hosted endpoint's audience. Customer-created access policies cannot request this
internal configuration.

The provider uses `authlib` to exchange the CAP token for a short-lived access
token. The configuration used in the preceding example supplies:

- `CAPToken`: the service credential authorized for access-token signing;
- `TokenExchangeURL`: the Auth API token-signing endpoint;
- `Namespace`: the resource namespace placed in the access token;
- `BaseURL`: the hosted AI SDK endpoint;
- `Audience`: the service allowed to accept the access token.

The audience is a JWT claim identifying the intended receiving service. It
defaults to `"ai-sdk"`; override it only when the internal deployment was
provisioned with another audience. The `authlib` client caches short-lived
tokens by namespace and audience, so reusing the Grafana provider avoids
unnecessary exchanges.

### Forward an internally minted access token

Use `NewWithAccessToken` when an internal Grafana control plane already mints
the short-lived access-token JWT:

```go
grafanaProvider, err := grafana.NewWithAccessToken(grafana.AccessTokenConfig{
	AccessToken: os.Getenv("GRAFANA_ACCESS_TOKEN"),
	BaseURL:     os.Getenv("GRAFANA_AI_SDK_URL"),
})
if err != nil {
	return err
}
```

The token carries its namespace and audience claims. An on-behalf-of token can
also carry an `act` actor claim and delegated permissions. The provider forwards
it unchanged and does not refresh it. Create a new provider with a fresh token
before the current token expires.

The auth API used to mint these tokens is an internal service-to-service API.
External applications must not call it directly.

## Configure the hosted model

Provider options for the underlying model retain their original namespace while
traveling through Grafana. For example, a hosted Claude model still reads
`anthropic.AnthropicOptions` from the `"anthropic"` namespace.

Install the Anthropic module only when the application needs its typed option
definitions:

```bash
go get github.com/grafana/ai-sdk/providers/anthropic
```

The request still goes through the Grafana provider and hosted endpoint:

```go
result := aisdk.StreamText(ctx, model,
	aisdk.WithModelMessages(provider.UserText("Solve this carefully.")),
	aisdk.WithProviderOptions(anthropic.AnthropicOptions{
		Thinking: &anthropic.ThinkingConfig{
			Type: anthropic.ThinkingAdaptive,
		},
	}),
)
```

Provider options on messages, content parts, and tools also cross the
provider-wire boundary. The hosted model and deployed backend must support the
selected options. See the [Anthropic provider guide](anthropic.md) for available
model options.

Constructor options configure a direct provider client and do not cross the
wire. For example, `anthropic.WithRequestOptions` does not apply to a Grafana
model.

## Control hosted middleware per request

`grafana.GrafanaOptions` controls Agent Observability, tracing, metrics, and
usage middleware running at the hosted endpoint. These controls are separate
from underlying model options and can be sent together:

```go
grafanaOptions := grafana.GrafanaOptions{
	AgentObservability: &grafana.AgentObservabilityControl{
		CaptureMode: grafana.CaptureModeMetadataOnly,
	},
}
if err := grafanaOptions.Validate(); err != nil {
	return err
}

result := aisdk.StreamText(ctx, model,
	aisdk.WithModelMessages(provider.UserText("Summarize this incident.")),
	aisdk.WithProviderOptions(
		grafanaOptions,
		anthropic.AnthropicOptions{
			Thinking: &anthropic.ThinkingConfig{
				Type: anthropic.ThinkingAdaptive,
			},
		},
	),
)
```

Call `GrafanaOptions.Validate` explicitly for values assembled from
configuration or user-controlled input. Neither `WithProviderOptions` nor the
Grafana provider validates these options automatically. Pass every provider
option namespace in one `WithProviderOptions` call; a later call replaces the
previous map.

A nil control defers to the hosted tenant configuration. It does not state
whether that middleware is enabled. The hosted deployment determines Agent
Observability availability, defaults, retention, and associated costs.

Metadata-only capture keeps generation metadata while omitting captured
content. To request that Agent Observability produce no generation record for a
request, set `Disabled`:

```go
disabled := true
grafanaOptions := grafana.GrafanaOptions{
	AgentObservability: &grafana.AgentObservabilityControl{
		Disabled: &disabled,
	},
}
```

`Disabled` takes precedence when it is combined with a capture mode. See
[Grafana Agent Observability](https://grafana.com/docs/grafana-cloud/monitor-applications/ai-observability/)
for the hosted product and [Agent Observability middleware](../middleware/agent-observability.md)
for client-side recording and policy hooks.

Hosted controls do not configure local middleware wrapped around the Grafana
model. Local logging, Prometheus metrics, enrichment, and Agent Observability
continue to run in the application independently.

## Forward end-user identity from an internal service

For an internally provisioned two-token on-behalf-of flow, attach a validated
user ID token to the call context:

```go
ctx := grafana.WithUserIDToken(r.Context(), idToken)
result := aisdk.StreamText(ctx, model,
	aisdk.WithModelMessages(provider.UserText("hello")),
)
```

The provider sends the ID token as `X-Grafana-Id` only for calls made with that
context. This works with either authentication constructor. Preserve the request
context so cancellation and identity travel together.

For a single-token flow, an internal Grafana control plane can mint the access
token by exchanging the user's ID token as its `subjectToken`. The resulting
token carries user identity in its `act` claim. Call the model with the normal
request context and do not also use `WithUserIDToken`.

Do not forward arbitrary client-provided tokens without validation. See
[Security](../best-practices/security.md) for request-boundary guidance.

## Handle errors and retries

Constructor validation errors are returned immediately. Recognized hosted error
categories returned in non-2xx responses before streaming starts, including
authentication, invalid requests, rate limits, and missing models, are surfaced
as `*grafana.GatewayError`. The originating `*provider.APICallError` remains
available through `errors.As`:

```go
var gatewayErr *grafana.GatewayError
if errors.As(err, &gatewayErr) {
	log.Printf("gateway category=%s model=%s", gatewayErr.Type, gatewayErr.ModelID)
}

var apiErr *provider.APICallError
if errors.As(err, &apiErr) {
	log.Printf("status=%d retryable=%t", apiErr.StatusCode, apiErr.IsRetryable)
}
```

Internal or unrecognized hosted errors can surface directly as
`*provider.APICallError`. Failures received after an SSE stream starts remain
`*provider.APICallError` stream parts and are available through `OnError`,
`FullStream`, and `StreamTextResult.Err`.

The core SDK retries eligible provider failures returned before a stream starts.
It does not restart a stream after parts have been emitted. See
[Retry and timeout](../guides/retry-and-timeout.md) and
[Error handling](../best-practices/error-handling.md).

## Understand request flow

```text
Internal Grafana service
  ├─ StreamText / GenerateText orchestration
  ├─ underlying model options, such as anthropic
  └─ hosted controls under the grafana option namespace
              │
              ▼
Grafana provider ── authenticated provider-wire request ──▶ hosted endpoint
              │                                           └─ model vendor
              ▼
provider stream parts ──▶ local tools / UI message stream / application code
```

The transport sends `provider.CallOptions` to the hosted `/language-model`
endpoint and returns the same provider-level stream or generate-result shapes
used by direct providers. The returned model works with registries, fallback,
structured output, tools, middleware, and browser UI streams. See
[Serving provider-wire models](../guides/provider-wire-server.md) and
[Fallback and registry](../guides/fallback-and-registry.md).

## Reference

- [`providers/grafana`](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/grafana)
- [`GrafanaOptions`](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/grafana#GrafanaOptions)
- [`GatewayError`](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/grafana#GatewayError)

---

← [OpenAI](openai.md) · [Docs index](../README.md) · [OpenAI-compatible →](openai-compatible.md)
