# Authenticate to Grafana AI Gateway

One Gateway process serves two disjoint account policies:

- **Private API:** a configured static key or short-lived access JWT authorizes configured models, aliases,
  provider accounts and discovery. This endpoint stays on private networking.
- **Public Cloud edge:** a stack-scoped Cloud Access Policy (CAP) authenticates
  the caller; each inference request supplies its own provider credentials (BYOK).
  This path never uses configured accounts, catalogs, aliases or fallback routes.

Use the URL supplied for the intended policy, ending in `/api/v1/aisdk`.
A JWT is not a credential for the public Cloud edge. Private networking does not
replace credential verification. The operational listener is separate from both APIs.

## Self-hosted production with a static key

Choose `static-key` in the existing provider/model YAML and disable unused Cloud
ingress. This complete configuration needs no Cloud account, CAP, JWKS service
or token issuer:

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

Generate at least 32 random bytes, for example `openssl rand -hex 32`, and inject
`AI_GATEWAY_KEY` and `ANTHROPIC_API_KEY` from your secret manager at runtime.
Keep values out of YAML, images, source control and browser code. Keys are opaque
bearer strings; punctuation and trailing `=` padding are supported. A JWT-looking
static value has no signature, claims or expiry semantics. Format validation does
not measure entropy.

Start in production mode; `auth.unsafe` is unnecessary. Agent Observability may
remain disabled only for production static-key with Cloud ingress disabled.
Enabled exporters retain their TLS, endpoint and secret requirements. Request
metrics and metadata logs remain available locally.

Go uses the existing client without token exchange:

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

The server-side Vercel client sends the same secret as a bearer credential:

```ts
import { createGateway } from '@ai-sdk/gateway';

const gateway = createGateway({
  baseURL: 'https://gateway.example.com/api/v1/aisdk',
  apiKey: process.env.AI_GATEWAY_KEY,
});
const catalog = await gateway.getAvailableModels();
const model = gateway('assistant'); // use with generateText or streamText
```

Both clients support configured discovery, unary generation and streaming.
Additional named identity entries can reference other environment variables;
every entry has the same configured-model permissions. Names provide attribution,
not ACLs or tenant isolation. Local callers have no Cloud stack or acting user,
and cannot use request-BYOK controls. Cloud ingress remains a separate trusted
proxy policy when explicitly enabled.

Send exactly one `Authorization: Bearer <key>` or `X-Access-Token: <key>`.
Duplicate, combined and malformed credentials fail closed, as do Cloud and
acting-user assertions, including empty ones. Query/body credentials are ignored.
A proxy that authenticates its own users must strip both credential alternatives,
insert one gateway credential, and remove `X-Grafana-Id`, `X-Scope-OrgID`,
`X-Cloud-Org-ID` and `X-Access-Policy-ID`. The Gateway sees the configured proxy
identity, not the end user. Prevent direct bypass if the proxy enforces policy.
Terminate client TLS at your ingress, protect the upstream transport, and isolate
the unauthenticated operational port from clients.

### Rotate by restarting

Keys and YAML are read only at startup. Replace the environment-backed secret,
restart each replica and coordinate client updates. One active key exists per
identity per process; mixed replicas may temporarily accept different keys and
rotation may disrupt calls. Revocation affects new requests on restarted replicas;
admitted requests are not reauthenticated and keep existing shutdown behavior.
The Gateway validates each key before copying it into a bounded mutable hashing
buffer, then clears that temporary buffer after hashing at boot and per request.
Environment variables and HTTP-owned credential strings remain unchanged; the
compiler, runtime and hash implementation may retain other copies. This cleanup
is limited to the application-owned buffer, with no memory-erasure guarantee.
There is no hot reload, overlap window or issuance API.

### Select JWT or development authentication

Omitting `auth` preserves legacy JWT/unsafe flags and environment variables.
Explicit YAML JWT uses `auth: {type: jwt, jwt: {jwksURL: https://identity.example.com/jwks}}`.
Explicit `auth: {type: unsafe}` remains development-only with loopback on every
active listener. Explicit YAML conflicts with a nonempty legacy JWKS URL or enabled
legacy unsafe flag. Null and empty auth blocks fail; they never select unsafe.
Non-string auth values or mapping keys, YAML merge keys and aliases are rejected to keep authentication presence unambiguous.
Cloud ingress defaults enabled when omitted. Only selected JWT authentication
uses JWKS tuning and adds its request timeout to the write-timeout budget.

## Cloud calls with request-scoped provider credentials

Provision a CAP with `ai-gateway:write` for the stacks your application needs.
Keep both the CAP and provider keys on your application server, never in browser
code. The edge validates the CAP, scope, stack/realm and applicable IP restrictions,
then replaces identity assertions and strips caller authentication credentials.
The application listener must only be reachable by that edge.

Select an explicit `anthropic/<native-model>` or `openai/<native-model>` ID;
OpenAI uses the Responses API. There is no catalog lookup or default model.
Customer discovery is unsupported: `ListModels` and `getAvailableModels` return
HTTP 400 with `catalog discovery is unsupported for BYOK`, even with read scope.
Do not discover configured models before making BYOK calls.

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
`aisdk.WithProviderOptions`. The client does not retry accounts or require
catalog membership. See the [package reference](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/grafana)
for constructors and bounded response handling.

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

The same BYOK object accompanies streaming and subsequent tool/continuation
requests. Ordinary application content and matching native provider options are
preserved within the Gateway's supported ProviderWire capabilities.

## Credential selection and limits

Only API-key accounts for native Anthropic and OpenAI are supported. Arrays retain
caller order. The existing eight-account behavior is retained temporarily as
execution policy. Required keys, provider selection and destinations are checked,
including supplied unused entries. OpenAI accounts may include `organization` and
`project`. Emit canonical field names `apiKey`, `baseURL`, `organization` and
`project`; account structs use ordinary Go field matching and additive fields are
ignored without applying them. Namespace/provider-map keys and discriminator
values are not normalized; use lowercase `gateway.byok`. Meaningful modelMappings
are unsupported, while absent/null/empty lists are inactive.

Omit `baseURL` to use the native endpoint. The bundled service does not configure
custom endpoint approvals, so other URLs are rejected. Alternate credential
families and Gateway routing controls are unsupported. Request-body and HTTP
header limits apply; there are no separate engine key, map or selector byte limits.
The Go client independently bounds its model-selector header to 2,048 bytes.

Ordered credentials use the existing Go fallback module's default policy, with
native retries disabled. Classified retryable failures (such as eligible 429/5xx)
and unknown pre-commit failures may advance; non-retryable errors, including typical
401/403 responses, stop. Any first stream part commits the selected candidate,
including metadata, warnings or an error. There is no replay after commitment and
no configured-account fallback. One execution deadline covers selection and all
attempts. A pre-commit failure can follow provider-side work: duplicate execution
or charging cannot be ruled out. Caller-level retries can multiply attempts.

## Private configured-account calls

Use `NewWithAccessToken` or `NewWithTokenExchange` against the private API. Token
exchange requires `access-token:sign` authority for the target namespace and
`ai-sdk` audience; ask the operator for the regional token-exchange endpoint.
Refresh short-lived JWTs before expiry.

The server verifies signature, token type, expiry, audience and namespace.
Concrete `stacks-<positive-int64>` namespaces and `*` are accepted; a service-identity
claim is optional and there is no service allowlist. Wildcard callers remain
service-level unless a verified acting-user token supplies an authorized concrete
namespace. Untrusted headers cannot choose a tenant.

Go sends `X-Access-Token`. Vercel can use the JWT directly:

```ts
const privateGateway = createGateway({ baseURL: privateGatewayURL, apiKey: accessJWT });
const catalog = await privateGateway.getAvailableModels();
```

Send exactly one access credential: `X-Access-Token` or bearer `Authorization`,
never both. Go constructors and call options reject reserved authentication-header
overrides. `grafana.WithUserIDToken(ctx, userIDToken)` adds separately verified
acting-user context; an ID token alone does not authenticate. Private calls reject
BYOK controls and use configured model IDs or aliases.

## Capture safely

Returned Go and Vercel request metadata is caller-owned and still contains the
submitted BYOK credentials. Never log `request.body` directly. Go logger capture
uses its configured Redactor for matching sensitive fields such as apiKey and
authentication headers, not a whole-BYOK-object rewrite:

```go
model = logmiddleware.Wrap(model, logmiddleware.Options{
    Logger: logger,
    Capture: logmiddleware.CaptureOptions{ProviderOptions: true, RequestBody: true},
})
```

Use DefaultRedactorWithExtraKeys or RedactorFunc for additional field policy.
Ordinary text and unfamiliar noncredential fields are not censored, even when
provider messages echo a key. TypeScript/application logs and other exporters
need independent capture controls; never mutate or log original metadata directly. These protections are not
substring scrubbers for arbitrary application text, custom metadata or opaque
error messages. Keep custom capture destinations and access controls independent
from the central Gateway's metadata-only observations.

Committed SSE errors may include available native message/type/code/status in
error.data.nativeError without changing Gateway classification or retryability.
That data can contain provider-originated credential echoes. BYOK currently lacks
Gateway attempt overviews and native summaries on unary/setup failures; treat
missing data as unavailable observation, not a guarantee that no work occurred.
Keep returned diagnostics out of logs unless your capture policy permits them.

Use HTTPS and server-side secret storage; rotate both CAP and provider credentials.
Deployment activation additionally requires the operator's
[listener and network-isolation proof](../../ai-gateway/docs/cloud-authentication.md).

---

← [Fallback and registry](fallback-and-registry.md) · [Docs index](../README.md) · [Run AI Gateway in a container →](ai-gateway-container.md)
