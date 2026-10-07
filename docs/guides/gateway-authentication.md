# Authenticate to Grafana AI Gateway

One Gateway process serves two disjoint account policies:

- **Private API:** a short-lived access JWT authorizes configured models, aliases,
  provider accounts and discovery. This endpoint stays on private networking.
- **Public Cloud edge:** a stack-scoped Cloud Access Policy (CAP) authenticates
  the caller; each inference request supplies its own provider credentials (BYOK).
  This path never uses configured accounts, catalogs, aliases or fallback routes.

Use the URL supplied for the intended policy, ending in `/api/v1/aisdk`.
A JWT is not a credential for the public Cloud edge. Private networking does not
replace JWT verification. The operational listener is separate from both APIs.

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

Only API-key credentials for native Anthropic and OpenAI are supported. Arrays
retain caller order. Every entry, including unused provider entries, is validated.
Custom endpoints, organization/project overrides, alternate credential families
and Gateway routing controls are rejected rather than silently ignored.

The server permits at most eight credentials per provider, 4,096 UTF-8 bytes per
key, 65,536 raw JSON bytes for the BYOK map, and 2,048 UTF-8 bytes for the complete
model selector. These are local resource controls, not Vercel hosted-service limits.

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
submitted BYOK credentials. Never log `request.body` directly. Default Go logger
capture structurally redacts the entire BYOK subtree, including unfamiliar fields:

```go
model = logmiddleware.Wrap(model, logmiddleware.Options{
    Logger: logger,
    Capture: logmiddleware.CaptureOptions{ProviderOptions: true, RequestBody: true},
})
```

Copy the tested [TypeScript capture helper](../../ai-gateway/examples/redact-byok.ts)
for equivalent bounded, fail-closed request-body capture. It sanitizes a copy;
it does not change the request sent to the provider. These protections are not
substring scrubbers for arbitrary application text, custom metadata or opaque
error messages. Keep custom capture destinations and access controls independent
from the central Gateway's metadata-only observations.

Use HTTPS and server-side secret storage; rotate both CAP and provider credentials.
Deployment activation additionally requires the operator's
[listener and network-isolation proof](../../ai-gateway/docs/cloud-authentication.md).

---

← [Fallback and registry](fallback-and-registry.md) · [Docs index](../README.md) · [Run AI Gateway in a container →](ai-gateway-container.md)
