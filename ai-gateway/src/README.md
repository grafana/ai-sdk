# AI Gateway

AI Gateway gives your applications one API for Anthropic and OpenAI models,
called through your Grafana stack with a Grafana credential. It speaks the AI
SDK language-model protocol, so Go services use the
[Grafana provider](https://github.com/grafana/ai-sdk/blob/main/docs/providers/grafana-gateway.md)
and TypeScript services use the
[Vercel AI SDK Gateway provider](https://ai-sdk.dev/providers/ai-sdk-providers/ai-gateway),
with the same streaming, tool calling and structured output as a direct
provider.

## What you can do

- **[Bring your own provider key](https://github.com/grafana/ai-sdk/blob/main/docs/providers/grafana-gateway.md#use-your-own-provider-credentials)**:
  in Grafana Cloud, call any Anthropic or OpenAI model with your own account,
  sending your key with each request.
- **[Authenticate with Grafana](https://github.com/grafana/ai-sdk/blob/main/docs/guides/gateway-authentication.md)**:
  applications use a service account token; Grafana services and App Platform
  apps use token exchange.
- **[Fall back and route](https://github.com/grafana/ai-sdk/blob/main/docs/providers/grafana-gateway.md#route-a-call-to-other-models-and-providers)**:
  try other models and provider accounts in order when a call fails.
- **[Use tools](https://github.com/grafana/ai-sdk/blob/main/docs/providers/grafana-gateway.md#function-tools)
  and [structured output](https://github.com/grafana/ai-sdk/blob/main/docs/providers/grafana-gateway.md#structured-output)**
  through the Gateway, as with a direct provider.
- **[Monitor calls](https://github.com/grafana/ai-sdk/blob/main/ai-gateway/docs/text-observability.md)**
  with logs, metrics, traces and Agent Observability.

## Get started

1. Open **AI Gateway** in Grafana. Its overview page shows your stack's
   Gateway URL and a quick start for Go and TypeScript.
2. [Create a service account](https://grafana.com/docs/grafana/latest/administration/service-accounts/)
   for your application and add a token to it.
3. Call the Gateway from your server with the token and your provider key.
   Keep both out of browser code.

## Who can call it

| Caller | Credential | Models |
|---|---|---|
| Your application | Service account token | Your own Anthropic or OpenAI account |
| A Grafana service | Cloud access policy token, through token exchange | The models Grafana configured for the service, once it is allowed |
| An App Platform app | The token exchange Grafana gives its backend | The configured models once the app is allowed; until then, your own key on behalf of a user |

See [Authenticate to the Gateway](https://github.com/grafana/ai-sdk/blob/main/docs/guides/gateway-authentication.md)
for each flow.

## Self-hosted Grafana

In self-hosted Grafana, the Gateway serves the models an administrator
configures and refuses requests that bring their own provider keys. It needs
Grafana 13.3 or later and the feature toggles listed in the
[requirements](https://github.com/grafana/ai-sdk/blob/main/ai-gateway/README.md#requirements).
The [Gateway guide](https://github.com/grafana/ai-sdk/blob/main/ai-gateway/README.md)
explains how to install it, configure models and choose who can use it.

## Documentation

- [Gateway guide](https://github.com/grafana/ai-sdk/blob/main/ai-gateway/README.md)
- [Authenticate to the Gateway](https://github.com/grafana/ai-sdk/blob/main/docs/guides/gateway-authentication.md)
- [Call the Gateway from Go](https://github.com/grafana/ai-sdk/blob/main/docs/providers/grafana-gateway.md)
- [Go API reference](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/grafana)
- [Gateway architecture](https://github.com/grafana/ai-sdk/blob/main/docs/concepts/gateway-architecture.md)
- [Report an issue](https://github.com/grafana/ai-sdk/issues/new)

## License

Licensed under the
[GNU Affero General Public License v3.0](https://github.com/grafana/ai-sdk/blob/main/ai-gateway/LICENSE).
