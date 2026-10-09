# Authenticate to Grafana AI Gateway

Choose your credentials based on the Gateway URL you are connecting to:

- **Self-hosted Gateway:** use a static key supplied by your operator to access
  the models they have configured. You do not need a Grafana Cloud account.
- **Private Gateway with JWT authentication:** use an access token from your
  organization's token issuer to access configured models.
- **Grafana Cloud Gateway:** use a Cloud Access Policy (CAP) token and supply
  your own Anthropic or OpenAI key with each request.

Ask your operator for the API URL, ending in `/api/v1/aisdk`, and which
credentials it accepts. Keep all credentials on your application server;
never include them in browser code.

## Set up a self-hosted Gateway with a static key

Add `auth` and `server` to your provider/model configuration. The example below
makes an Anthropic model available as `assistant` and disables Cloud access.
For container startup commands, see [Run AI Gateway in a container](ai-gateway-container.md).

```yaml
providers:
  anthropic-primary:
    type: anthropic
    apiKeyEnv: ANTHROPIC_API_KEY
models:
  assistant:
    name: Assistant
    primary:
      provider: anthropic-primary
      model: claude-sonnet-4-6
auth:
  type: static-key
  staticKey:
    identities:
      deployment:
        keyEnv: AI_GATEWAY_KEY
server:
  cloud:
    enabled: false
```

Generate a Gateway key with at least 32 random bytes, for example:

```sh
openssl rand -hex 32
```

Provide the generated value as `AI_GATEWAY_KEY` to both the Gateway and your
application. Provide `ANTHROPIC_API_KEY` only to the Gateway. Load these values
from your secret manager at runtime; keep them out of YAML, images and source
control. Static keys do not expire automatically, even if you use a JWT as the
key value.

This configuration can run in production without a token issuer or Agent
Observability export. If you enable Cloud access or choose JWT authentication,
production also requires Agent Observability. See the
[Agent Observability configuration](../../ai-gateway/docs/text-observability.md#agent-observability-configuration)
for exporter settings.

### Connect your application

In Go, pass the Gateway key and URL to `NewWithAccessToken`:

```go
client, err := grafana.NewWithAccessToken(grafana.AccessTokenConfig{
    AccessToken: os.Getenv("AI_GATEWAY_KEY"),
    BaseURL: "https://gateway.example.com/api/v1/aisdk",
})
if err != nil {
    return err
}
rows, err := client.ListModels(ctx)
```

In a server-side TypeScript application, pass the same key to the Vercel client:

```ts
import { createGateway } from '@ai-sdk/gateway';

const gateway = createGateway({
  baseURL: 'https://gateway.example.com/api/v1/aisdk',
  apiKey: process.env.AI_GATEWAY_KEY,
});
const catalog = await gateway.getAvailableModels();
const model = gateway('assistant'); // use with generateText or streamText
```

Use either client to list available models, generate a response or stream it.
See the [Grafana Gateway provider guide](../providers/grafana-gateway.md) for
inference examples.

You can add named entries under `identities` to give applications separate
keys. Every key has access to the same configured models; the names help
attribute requests and do not provide separate permissions or tenant isolation.
Static keys cannot be used for Cloud BYOK requests.

### Put the Gateway behind your ingress

Use HTTPS for client connections, protect the connection from your ingress to
the Gateway, and keep the private API on private networking. Restrict the
unauthenticated operational port to your monitoring and infrastructure services.

The clients set authentication headers for you. For direct HTTP requests, send
one of `Authorization: Bearer <key>` or `X-Access-Token: <key>`, never both.
Keys in query parameters or request bodies are not accepted.

If your proxy authenticates end users, have it replace incoming authentication
headers with one Gateway credential and remove `X-Grafana-Id`, `X-Scope-OrgID`,
`X-Cloud-Org-ID` and `X-Access-Policy-ID`. These identity headers are not accepted
with static keys, even when empty. The Gateway attributes requests to the
proxy's configured identity. Prevent clients from bypassing the proxy if you
rely on it for per-user permissions.

### Rotate by restarting

To rotate or revoke a static key, update the secret supplied to the Gateway
and restart every replica. Coordinate the change with your applications:
restarted replicas accept the new key and reject the old one, while replicas
that have not restarted still accept the old key. Calls can fail during this
transition because each identity accepts only one key per replica.

Changing an environment variable or YAML file does not update a running
Gateway. Already authenticated requests can continue until completion or
shutdown. Provision and rotate keys through your secret manager; the Gateway
has no key-management API.

### Use JWT authentication or local development mode

For JWT authentication, configure your issuer's JWKS endpoint:

```yaml
auth:
  type: jwt
  jwt:
    jwksURL: https://identity.example.com/jwks
```

Then follow [Private configured-account calls](#private-configured-account-calls)
to connect. If you already configure authentication through command-line flags
or environment variables, you can continue to do so by leaving `auth` out of
the YAML. When switching to YAML, remove the legacy JWKS URL and unsafe-auth
settings to avoid a startup conflict. Use an explicit `auth.type`; an empty or
null `auth` block is invalid. Write auth settings directly, without YAML aliases
or merge keys.

For local development only, `auth: {type: unsafe}` requires development mode
and loopback addresses for every enabled listener. Use static-key or JWT
authentication in production. Cloud access is enabled by default; set
`server.cloud.enabled: false` when you do not need it.

## Connect to Grafana Cloud with your own provider keys

Create a CAP token with `ai-gateway:write` for your target stack, and obtain
the Cloud Gateway URL. You also need an Anthropic or OpenAI API key. Keep both
tokens on your application server.

Choose a model ID such as `anthropic/claude-sonnet-4-6` or
`openai/<native-model>`. OpenAI requests use the Responses API. Cloud BYOK does
not offer model discovery: `ListModels` and `getAvailableModels` return HTTP
400. Use the model name from your provider rather than a configured Gateway alias.

### Go

```go
client, err := grafana.NewWithCloudCredentials(grafana.CloudCredentialsConfig{
    StackID: stackID, CAPToken: capToken, BaseURL: cloudGatewayURL,
})
if err != nil {
    return err
}
model, err := client.LanguageModel("anthropic/claude-sonnet-4-6")
if err != nil {
    return err
}
credentials, err := json.Marshal(map[string]any{
    "byok": map[string]any{
        "anthropic": []map[string]string{{"apiKey": providerKey}},
    },
})
if err != nil {
    return err
}
maxTokens := 32
result, err := model.DoGenerate(ctx, provider.CallOptions{
    Prompt: []provider.Message{provider.UserText("Summarize this incident.")},
    MaxOutputTokens: &maxTokens,
    ProviderOptions: provider.ProviderOptions{
        "gateway": provider.RawProviderOption{Key: "gateway", Raw: credentials},
    },
})
if err != nil {
    return err
}
_ = result
```

For high-level Go calls, pass the same `RawProviderOption` through
`aisdk.WithProviderOptions`. See the [package reference](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/grafana)
for client options.

### Server-side Vercel client

```ts
import { createGateway } from '@ai-sdk/gateway';

const gateway = createGateway({
  baseURL: cloudGatewayURL,
  apiKey: `${stackID}:${capToken}`,
});
const result = await gateway('anthropic/claude-sonnet-4-6').doGenerate({
  prompt: [{ role: 'user', content: [{ type: 'text', text: 'Summarize this incident.' }] }],
  maxOutputTokens: 32,
  providerOptions: { gateway: { byok: { anthropic: [{ apiKey: providerKey }] } } },
});
```

Include the same `gateway.byok` credentials with streaming requests and every
subsequent tool or conversation request.

### Choose provider accounts

Cloud BYOK supports Anthropic and OpenAI API keys. OpenAI accounts can also
include `organization` and `project`. Use the field names shown in the examples
and leave `baseURL` unset to use the provider's native endpoint. The bundled
Gateway does not support custom BYOK endpoints, model mappings or alternate
credential types.

You can supply up to eight accounts per provider in the order you want them
tried. The Gateway may try the next account after an eligible failure, such as
a rate limit or temporary server error. Authentication failures typically stop
the request. It never falls back to an operator's configured provider account.

Once a stream starts emitting events, the Gateway does not switch accounts or
replay the request. A failed attempt may still have performed work at the
provider, so fallback or application retries can result in duplicate work or
charges. See [Fallback and registry](fallback-and-registry.md) before adding
application-level retries.

## Private configured-account calls

Ask your operator for the private Gateway URL and an access JWT for the
`ai-sdk` audience. Use `NewWithAccessToken` when you already have a token, or
`NewWithTokenExchange` when your application is authorized to obtain one.
Token exchange requires `access-token:sign` permission for the target namespace;
your operator supplies the regional exchange endpoint. Refresh short-lived
tokens before they expire.

The Go client sets the authentication header for you. With Vercel, pass the JWT
as `apiKey`:

```ts
const privateGateway = createGateway({ baseURL: privateGatewayURL, apiKey: accessJWT });
const catalog = await privateGateway.getAvailableModels();
```

Use configured model IDs or aliases for private calls; do not supply
`gateway.byok`. If your application needs to act on behalf of a user, ask your
operator about acting-user tokens. Go's `grafana.WithUserIDToken` sends that
additional token alongside the access JWT. A user ID token alone cannot
authenticate a request, and this option is not available with static keys.

## Keep credentials out of logs

Request metadata returned by Go and Vercel clients can contain your submitted
provider keys. Do not log request bodies or provider options directly, or send
them to browser clients.

Start with metadata-only logging. If you enable content capture, configure
[structured logging redaction](../middleware/structured-logging.md) for your
sensitive fields and review what your application and exporters collect.
Redacting credential fields does not remove secrets echoed in provider text or
error messages. Returned diagnostics may contain those echoes; only log or
display them when your capture policy allows it. See
[Inspect execution overviews and failures](../providers/grafana-gateway.md#inspect-execution-overviews-and-failures)
for details on the diagnostics available to clients.

Gateway operators enabling Cloud access should also follow the
[listener and network-isolation requirements](../../ai-gateway/docs/cloud-authentication.md).

---

← [Fallback and registry](fallback-and-registry.md) · [Docs index](../README.md) · [Run AI Gateway in a container →](ai-gateway-container.md)
