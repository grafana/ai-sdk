# Authenticate to Grafana AI Gateway

For the public Grafana Cloud URL, authenticate your application server with
your stack ID and a Cloud Access Policy (CAP) token. These credentials are
separate from how users sign in to your app. Grafana AI Gateway manages the
underlying model-provider API keys.

## Choose the URL and credential

| Gateway URL | Go client | Server-side Vercel client | What you need |
| --- | --- | --- | --- |
| Public Grafana Cloud URL | `NewWithCloudCredentials` | `createGateway({ apiKey })` | Stack ID and Cloud Access Policy (CAP) token |
| Separately provided JWT-enabled URL | `NewWithTokenExchange` or `NewWithAccessToken` | No built-in Grafana JWT configuration | Short-lived access token or credentials to obtain one |

Use the public Grafana Cloud URL provided for your stack. It requires a CAP
with access to that stack. If your deployment provides a separate JWT-enabled
Gateway URL, use that URL for JWT-based Go clients; a JWT is not a credential
for the public Cloud URL.

## Use a Cloud policy from Go

Provision a CAP for your stack with `ai-gateway:read` to discover models and
`ai-gateway:write` to invoke them. Limit the policy to the stacks your app
needs; avoid granting access to all stacks by default.

On your Go server, use the [Grafana provider](../providers/grafana-gateway.md)
with the public Gateway URL ending in `/api/v1/aisdk`:

```go
client, err := grafana.NewWithCloudCredentials(grafana.CloudCredentialsConfig{
    StackID:  stackID,
    CAPToken: capToken,
    BaseURL:  "https://gateway.example.com/api/v1/aisdk",
})
if err != nil {
    return err
}
models, err := client.ListModels(ctx)
if err != nil {
    return err
}
_ = models
model, err := client.LanguageModel("assistant")
if err != nil {
    return err
}
maxTokens := 32
result, err := model.DoGenerate(ctx, provider.CallOptions{
    Prompt:          []provider.Message{provider.UserText("Summarize this incident.")},
    MaxOutputTokens: &maxTokens,
})
if err != nil {
    return err
}
_ = result
```

## Use a Cloud policy from a Vercel server

On your application server (not in a browser), configure the Vercel Gateway
client with your stack ID and CAP token:

```ts
import { createGateway } from '@ai-sdk/gateway';

const gateway = createGateway({
  baseURL: 'https://gateway.example.com/api/v1/aisdk',
  apiKey: `${stackID}:${capToken}`,
});
const catalog = await gateway.getAvailableModels();
const result = await gateway('assistant').doGenerate({
  prompt: [{ role: 'user', content: [{ type: 'text', text: 'Summarize this incident.' }] }],
  maxOutputTokens: 32,
});
```

## JWT-enabled Gateway URLs (Go)

If your deployment provides a JWT-enabled Gateway URL and token-exchange
service, ask its operator for the Gateway URL, token-exchange URL and namespace.
The Go client can then obtain a short-lived access token:

```go
client, err := grafana.NewWithTokenExchange(grafana.TokenExchangeConfig{
    CAPToken:         capToken,
    TokenExchangeURL: tokenExchangeURL,
    Namespace:        namespace,
    BaseURL:          accessTokenGatewayURL,
})
```

For token exchange, your policy must allow `access-token:sign` for the target
namespace and audience. If you already have a short-lived JWT, use
`NewWithAccessToken` and refresh it before expiry. JWT-enabled setups can
attach an acting-user token with `grafana.WithUserIDToken(ctx, userIDToken)`.

## Keep credentials secure

Use HTTPS, store CAPs in a server-side secret manager, grant only the scopes
and stacks your app needs, and rotate tokens regularly. Avoid logging request
headers or errors that may contain credentials. Never include a CAP in browser
code or distribute it to untrusted workers.

---

← [Fallback and registry](fallback-and-registry.md) · [Docs index](../README.md) · [Run AI Gateway in a container →](ai-gateway-container.md)
