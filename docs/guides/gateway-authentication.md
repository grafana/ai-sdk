# Authenticate to Grafana AI Gateway

Grafana AI Gateway is a Grafana app. Your application calls it through your
Grafana instance, with a Grafana credential. Which credential you use, and which
models you get, depends on where Grafana runs:

| You are | Credential | You get |
|---|---|---|
| An application on self-hosted Grafana | Grafana service account token | The models your administrator configured |
| An application on Grafana Cloud | Grafana service account token | Any model from your own provider account (bring your own key) |
| A Grafana service in Grafana Cloud | Cloud access policy token | The models Grafana configured for your service |

Keep credentials on your application server. Never include them in browser
code.

## Find your Gateway URL

Open **Grafana AI Gateway** in your Grafana instance. The overview page shows
the Gateway URL for your stack and a quick start you can copy. The URL has this
form:

```text
https://<your-grafana>/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/<namespace>
```

The namespace is `stacks-<stack ID>` in Grafana Cloud and usually `default` in
self-hosted Grafana. Use the whole URL as the client base URL.

## Use the configured models in self-hosted Grafana

1. [Create a service account](https://grafana.com/docs/grafana/latest/administration/service-accounts/)
   for your application, and add a token to it.
2. Store the token in your server's secret store.
3. Create a client with the token and your Gateway URL:

```go
client, err := grafana.NewWithServiceAccountToken(grafana.ServiceAccountConfig{
	Token:   os.Getenv("GRAFANA_SERVICE_ACCOUNT_TOKEN"),
	BaseURL: "https://grafana.example.com/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/default",
})
if err != nil {
	return err
}
models, err := client.ListModels(ctx)
```

From a server-side TypeScript application, pass the token as the Vercel
client's API key:

```ts
import { createGateway } from '@ai-sdk/gateway';

const gateway = createGateway({
  baseURL: 'https://grafana.example.com/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/default',
  apiKey: process.env.GRAFANA_SERVICE_ACCOUNT_TOKEN,
});
const { models } = await gateway.getAvailableModels();
```

Each service account is a separate caller, so create one per application or
person you want to tell apart. Your administrator decides which Grafana roles
can use the app. If the Gateway is not available yet, ask them to set it up as
described in the [Gateway guide](../../ai-gateway/README.md#install).

In self-hosted Grafana, the Gateway uses only the provider accounts your
administrator configured, and refuses requests that bring their own provider
keys.

## Bring your own provider key in Grafana Cloud

Any user of your stack can call the Gateway with a service account token, and
no extra role is needed. You supply your own provider key with every request,
so the Gateway uses your provider account and the models it offers.

Create the client the same way as in self-hosted Grafana, with your stack's
Gateway URL:

```go
client, err := grafana.NewWithServiceAccountToken(grafana.ServiceAccountConfig{
	Token:   os.Getenv("GRAFANA_SERVICE_ACCOUNT_TOKEN"),
	BaseURL: "https://grafana.example.com/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-123",
})
```

Then select a model by provider and name, such as `openai/gpt-5`, and send your
key with the request. See
[Use your own provider credentials](../providers/grafana-gateway.md#use-your-own-provider-credentials)
for Go and TypeScript examples.

Model listing is not available in this mode, because your provider account
decides which models exist.

## Call from a Grafana service in Grafana Cloud

Grafana services use the configured models with a Cloud access policy token.
Ask the Gateway owners to allow your service, then create a client that
exchanges your token for one the Gateway accepts:

```go
client, err := grafana.NewWithTokenExchange(grafana.TokenExchangeConfig{
	CAPToken:         os.Getenv("GRAFANA_CAP_TOKEN"),
	TokenExchangeURL: tokenExchangeURL,
	Namespace:        "stacks-123",
	Audience:         "grafana-aigateway-app",
	BaseURL:          "https://grafana.example.com/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-123",
})
```

Set `Namespace` to the stack you call. The client renews exchanged tokens before
they expire; you rotate the access policy token itself. Model aliases, discovery
and fallback work as described in the [provider guide](../providers/grafana-gateway.md).

This flow needs the Go client. The Vercel client cannot exchange tokens.

### Call on behalf of a user

When a request is made for a user, pass their token in the call context. Use
the user's Grafana ID token, or the access token your plugin received for them:

```go
ctx = grafana.WithOnBehalfOf(ctx, userToken)
result, err := aisdk.GenerateText(ctx, model, aisdk.WithModelMessages(provider.UserText("Summarize this incident.")))
```

The call still uses your service's models, and is made on the user's behalf.

## Troubleshoot authentication errors

| Response | Likely cause |
|---|---|
| `401 Unauthorized` | The token is missing, expired or revoked. |
| `403 Forbidden` from Grafana | The token does not belong to the stack in the URL, or your role cannot use the app. |
| `403` Gateway permission error | Your credential has no Gateway access, for example a Grafana service not yet allowed. |
| `400` reserved provider option namespace | You sent your own provider key to a self-hosted Gateway. |
| Request ends after 60 seconds | Grafana limits how long a request can run. Stream long responses. |

Grafana answers some of these before the request reaches the Gateway, in its own
error format rather than a Gateway error. The Go client reports them as
protocol errors with the HTTP status.

## Keep credentials out of logs

Requests that bring their own provider key carry that key, and the request
details your client returns can contain it. Do not log request bodies or
provider options, and do not send them to browsers.

Start with metadata-only logging. If you capture content, configure
[structured logging redaction](../middleware/structured-logging.md) and review
what your application and exporters collect. Redaction does not remove a key
that a provider echoes in its response or error text, so treat diagnostics as
sensitive. See
[Inspect execution overviews and failures](../providers/grafana-gateway.md#inspect-execution-overviews-and-failures)
for what clients can inspect.

---

← [Call Grafana AI Gateway from Go](../providers/grafana-gateway.md) · [Docs index](../README.md) · [Production checklist →](../best-practices/production.md)
