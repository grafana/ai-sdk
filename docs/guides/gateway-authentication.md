# Authenticate to Grafana AI Gateway

Choose credentials for the **endpoint you call**, not for the SDK you use.
Application login authenticates a person to your app; Gateway authentication
allows your backend to use a model; provider API keys are configured on the
Gateway server. Keep Gateway CAPs and model-provider API keys out of browser code; app login has its own browser-facing session flow.

## Choose the endpoint and credential

| Endpoint | Go client | Server-side Vercel client | Header received at the endpoint |
| --- | --- | --- | --- |
| Public Cloud URL through cortex-gw | `NewWithCloudCredentials` | `createGateway({ apiKey })` | `Authorization: Bearer <stack-id>:<CAP-token>` |
| Explicitly configured `access-token` Gateway endpoint | `NewWithTokenExchange` or `NewWithAccessToken` | No built-in Grafana JWT flow | `X-Access-Token: <signed-JWT>` |

The public URL is authenticated by cortex-gw. It checks the Cloud Access Policy
(CAP) and stack access, strips credentials, and replaces `X-Scope-OrgID` before
forwarding to the backend's `cloud-gateway` listener. **Do not call that listener
directly:** it trusts the proxy's assertion, not the client. Its operational
listener is separate and unauthenticated; see the [server trust
contract](../../ai-gateway/docs/cloud-authentication.md).

An access-token JWT can be used only with an endpoint deliberately configured
to verify it. The deployed Cloud route does not accept the JWT from Go token
exchange. The two flows do not fall back to each other.

## Use a Cloud policy from Go

Provision a CAP for the target stack with `ai-gateway:read` to discover models
and `ai-gateway:write` to invoke them. Use an explicit stack realm when the
caller serves one stack. The decimal **stack ID** is not a Cloud organization
ID or a namespace such as `stacks-27038`. A multi-stack/system policy has a
much wider blast radius: review it separately rather than accepting the
provisioning helper's system-realm default.

On a trusted Go server, use the [Grafana provider](../providers/grafana-gateway.md)
with the public URL's `/api/v1/aisdk` API prefix:

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

`ListModels`, `DoGenerate`, and `DoStream` use the same Cloud Authorization
header. This constructor never exchanges tokens or sends `X-Access-Token`.
Do not attach `WithUserIDToken` to Cloud calls: this route conveys stack
identity, not an acting user. The client rejects conflicting identity headers.

## Use a Cloud policy from a Vercel server

The registered `@ai-sdk/gateway` client sends `apiKey` unchanged as a bearer
credential. On your application server (not in a browser):

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

For streaming, use `gateway('assistant').doStream` with the same prompt and
`maxOutputTokens`, then consume its stream. The Gateway supports a tested
subset of the Vercel provider protocol; authenticated requests using
unsupported options or high-level helpers can still fail. Check the
[server's client limitations](../../ai-gateway/docs/cloud-authentication.md#client-compatibility)
before treating authentication success as full API compatibility.

Vercel's `apiKey` performs **no Grafana token exchange**. Supplying a signed JWT
as `apiKey` does not make it a Go-style `X-Access-Token` request. Do not set
`AI_GATEWAY_API_KEY` in a browser or use Vercel's ambient OIDC default as a
substitute for this Grafana credential.

## Internal JWT authentication (Go)

For an explicitly JWT-verifying endpoint, an internal service can exchange an
allowed CAP token for a short-lived access token, scoped to namespace
`stacks-<stack-id>` and audience `ai-sdk` (the default):

```go
client, err := grafana.NewWithTokenExchange(grafana.TokenExchangeConfig{
    CAPToken:         capToken,
    TokenExchangeURL: tokenExchangeURL,
    Namespace:        namespace,
    BaseURL:          accessTokenGatewayURL,
})
```

Authlib caches exchanged tokens. The caller must keep the CAP secure and ensure
it has `access-token:sign` and the allowed audience. Alternatively,
`NewWithAccessToken` forwards a caller-managed JWT, which the caller must
refresh before expiry. Both JWT constructors send `X-Access-Token`; for
verified acting-user identity, they can additionally use
`grafana.WithUserIDToken(ctx, userIDToken)`.

**Go migration:** `NewWithCloudAuth` / `CloudAuthConfig` were renamed to
`NewWithTokenExchange` / `TokenExchangeConfig`; the old names were removed.
This is a name change, not a switch to direct CAP authentication. Migrate to
`NewWithCloudCredentials` only when changing the endpoint and policy for the
Cloud route. Vercel has no built-in equivalent of the Go exchange constructor.

## Keep credentials and failures bounded

Use HTTPS for the public endpoint, store CAPs in a server-side secret manager,
provision the minimum scopes/realms, rotate or expire them, and avoid logging
request headers or token-bearing errors. Do not give a system CAP to browsers or
to customer-controlled workers. Delegated k6 sessions need a separate
short-lived credential design; this client feature does not migrate them.

If a call fails:

1. Check the URL prefix and which endpoint handles it. A Cloud CAP sent as
   `X-Access-Token`, or an internal JWT sent in Cloud `Authorization`, cannot
   authenticate through the wrong edge.
2. Distinguish local configuration validation from a response from cortex-gw.
   Verify the credential is current, then its policy's **read/write scope** and
   **realm for the selected stack**. A valid credential alone is insufficient.
3. If authentication succeeded but a model call fails, check catalog model IDs,
   supported options and server capability. Do not bypass the proxy or retry
   against the trusted listener.

Local edge-shim tests verify client header/strip behavior, **not** real CAP
validation, revocation, policy provisioning, or deployed network isolation.
Deployment owners must validate those properties before declaring hosted support.

---

← [Fallback and registry](fallback-and-registry.md) · [Docs index](../README.md) · [Run AI Gateway in a container →](ai-gateway-container.md)
