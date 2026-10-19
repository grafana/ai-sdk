# Authenticate to Grafana AI Gateway

The Gateway runs as the Grafana app plugin `grafana-aigateway-app`. Grafana, or
the App Platform router in Grafana Cloud, authenticates your client before the
Gateway sees a request. The Gateway then decides what you can use from the
verified identity and its access mode:

| Deployment | Caller | Credential | You get |
|---|---|---|---|
| Grafana Cloud | Internal service on the Gateway allowlist | Access policy token, exchanged by the Go client | Configured models and discovery |
| Grafana Cloud | Application or person | Grafana service account token | Your own provider keys (BYOK) |
| Self-hosted Grafana | Application or person | Grafana service account token | Configured models and discovery |

Every client uses the plugin route prefix as its base URL:

```text
https://<grafana>/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/<namespace>
```

The namespace is `stacks-<stack ID>` in Grafana Cloud and usually `default` in
self-hosted Grafana. Keep all credentials on your application server; never
include them in browser code.

## Internal services in Grafana Cloud

Internal services use the Go client's token exchange. The Gateway operator must
add your service's identity to the internal service allowlist. Exchange for the
`grafana-aigateway-app` audience and your stack namespace:

```go
client, err := grafana.NewWithTokenExchange(grafana.TokenExchangeConfig{
	CAPToken:         os.Getenv("GRAFANA_CAP_TOKEN"),
	TokenExchangeURL: tokenExchangeURL,
	Namespace:        "stacks-123",
	Audience:         "grafana-aigateway-app",
	BaseURL:          "https://grafana.example.com/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-123",
})
```

The client caches exchanged tokens and exchanges again before they expire; you
own rotating the access policy token itself. Configured discovery, aliases and
fallback work as described in the [provider guide](../providers/grafana-gateway.md).

### Call on behalf of a user

To attribute a call to a user, put a subject token in the call context. Use the
user's Grafana ID token, or the delegated access token your plugin received for
that user:

```go
ctx = grafana.WithOnBehalfOf(ctx, subjectToken)
result, err := aisdk.GenerateText(ctx, model, aisdk.WithModelMessages(provider.UserText("Summarize this incident.")))
```

The exchange uses that token as its subject, so the Gateway receives one token
naming your service and the user. You keep the configured models, and the
Gateway attributes the request to your service. On-behalf-of calls need a
concrete namespace; a client that exchanges for every namespace refuses them
before any network request, as do the access token and service account flows.

The pinned Vercel client always sends its key as an `Authorization` bearer, so
it cannot authenticate as an internal service yet. Use the Go client.

## Your own provider keys in Grafana Cloud

Any stack user's application can call the Gateway with a Grafana service account
token and supply its own provider keys with each request. No Gateway role is
needed. Configured discovery is unavailable in this mode and returns the fixed
BYOK unsupported error.

```go
client, err := grafana.NewWithServiceAccountToken(grafana.ServiceAccountConfig{
	Token:   os.Getenv("GRAFANA_SERVICE_ACCOUNT_TOKEN"),
	BaseURL: "https://grafana.example.com/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-123",
})
```

From a server-side Vercel application, pass the service account token as the
key:

```ts
import { createGateway } from '@ai-sdk/gateway';

const gateway = createGateway({
  baseURL: 'https://grafana.example.com/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-123',
  apiKey: process.env.GRAFANA_SERVICE_ACCOUNT_TOKEN,
});
```

Select the provider's native model with a `provider/model` ID and send the
accounts in `providerOptions.gateway.byok`, as shown in
[Use your own provider credentials](../providers/grafana-gateway.md#use-your-own-provider-credentials).

### Choose provider accounts

Supply credentials on each call, including follow-up conversation steps. If you
provide multiple accounts, put them in priority order. Account fallback may
advance after an eligible failure, but authentication failures stop the chain.
Omit an account's `baseURL` to use the native provider endpoint; other URLs are
rejected.

## Self-hosted Grafana

In self-hosted Grafana, users and service accounts use the models the operator
configured. Create one Grafana service account per application or person you
want to attribute usage to, since service accounts have no owner. Catalog access
follows Grafana's plugin app access, so your operator may restrict which roles
can use it.

```go
client, err := grafana.NewWithServiceAccountToken(grafana.ServiceAccountConfig{
	Token:   os.Getenv("GRAFANA_SERVICE_ACCOUNT_TOKEN"),
	BaseURL: "https://grafana.example.com/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/default",
})
rows, err := client.ListModels(ctx)
```

The Vercel client works the same way with the service account token as
`apiKey`, including `getAvailableModels()`. Sending BYOK provider options is
refused in this mode. Self-hosted Grafana currently needs extra platform
configuration to pass caller identity to the plugin; see the
[operator guide](../../ai-gateway/README.md#self-hosted-grafana-today).

## Errors from the platform

Grafana or the router rejects some requests before the Gateway runs, for example
a missing or invalid credential, a namespace your credential does not cover, or
a call that exceeds the platform's 60-second route deadline. Those responses use
Kubernetes `Status` JSON rather than Gateway error documents, so clients report
them as protocol errors with the HTTP status. Gateway refusals, such as a caller
without access, use the fixed Gateway error documents.

## Keep credentials out of logs

Request metadata returned by Go and Vercel clients can contain your submitted
provider keys. Do not log request bodies or provider options directly, or send
them to browser clients.

Start with metadata-only logging. If you enable content capture, configure
[structured logging redaction](../middleware/structured-logging.md) for your
sensitive fields and review what your application and exporters collect.
Redacting credential fields does not remove secrets echoed in provider text or
error messages. Gateway-produced summaries replace exact configured provider-key
and enabled exporter-credential matches, including collisions with BYOK values.
BYOK-only echoes remain available; ordinary content and opaque native metadata
are outside that summary policy. Returned diagnostics can still contain secrets,
so only log or display them when your capture policy allows it. See
[Inspect execution overviews and failures](../providers/grafana-gateway.md#inspect-execution-overviews-and-failures)
for details on the diagnostics available to clients.

---

← [Call Grafana AI Gateway from Go](../providers/grafana-gateway.md) · [Docs index](../README.md) · [Production checklist →](../best-practices/production.md)
