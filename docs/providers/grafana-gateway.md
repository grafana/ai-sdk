# Call Grafana AI Gateway from Go

Use the Grafana provider to call your Gateway from an application server. Select
a model, then use the same generation, streaming and tool APIs as other Go
providers. Available models and features depend on your deployment.

```bash
go get github.com/grafana/ai-sdk github.com/grafana/ai-sdk/providers/grafana
```

Keep Gateway and model-provider credentials on your server, not in browser code.

## Inspect execution overviews and failures

Unary results and stream finish metadata may contain `gateway.execution`.
It identifies the requested/canonical public model and ordered observed attempts,
including configured provider instance, provider and native model, outcome,
and optional native failure summaries. Earlier eligible failures remain
visible when a later candidate is selected. Selection commits a stream at its
first accepted part; it does not mean completion or replay safety.

Go applications can inspect result/finish `ProviderMetadata`, including through
consumer middleware. TypeScript applications can inspect result or awaited
stream `providerMetadata`. Each invocation has its own overview; application
retries and tool continuations start new invocations.

HTTP setup/unary errors carry optional top-level `providerMetadata` in the
bounded error body. Go exposes it through `*provider.APICallError.Data`;
the TypeScript Gateway error exposes the API cause's `data`.
Committed stream errors carry `error.data.providerMetadata` and an optional
current `error.data.nativeError` summary. Go exposes that payload through
the part's `APICallError.Data`; TypeScript low-level parts expose `error.data`
and high-level error parts wrap the Gateway error. Later content/finish parts
remain ordered and readable. Current errors are not accumulated into finish
metadata or used to restart fallback. Current native summaries do not require
configured catalog attribution; a plain request selection can return them
without a configured execution overview. Plain selections currently lack
request-account attempt overviews and unary/setup native summaries; this is an
[observation capability gap](https://github.com/grafana/ai-sdk/issues/317), not a
confidentiality rule or a permanent restriction.

These extensions are best-effort, independent of Gateway operator observation,
and do not change public error classification or retryability. Complete
response/frame bounds still apply as transitional transport policy; an overview
can be omitted rather than
invalidate fitting original output. Native `gateway` metadata moves under
`gateway.nativeMetadata` only when enrichment fits. Otherwise it stays opaque,
so namespace presence alone does not establish provenance, and an absent
overview does not imply no provider attempts.

Summaries select native message, type, string-or-number code and status, not
raw bodies, headers or arbitrary cause trees. Returned provider values are not
filtered and may echo caller-supplied or service-owned credentials. Account
authorization and request isolation still apply. Logger credential-field policy
is separate and does not rewrite caller output; independently authorize logging
or display of native diagnostics. These fields
are not automatically UI message metadata or display content; map only
deliberately selected values for your frontend.

## Authenticate the client

Ask your Gateway operator for the URL and credentials your application should
use. Include the API prefix in the URL, for example
`https://gateway.example.com/api/v1/aisdk`. Cloud access uses request-supplied
provider keys, not configured accounts. Create its client with your stack ID and
Cloud Access Policy (CAP) token:

```go
client, err := grafana.NewWithCloudCredentials(grafana.CloudCredentialsConfig{
	StackID:  stackID,
	CAPToken: capToken,
	BaseURL:  gatewayURL,
})
if err != nil {
	return err
}
```

For the private configured-account URL, use `NewWithTokenExchange` or
`NewWithAccessToken`. See [Authenticate to Grafana AI Gateway](../guides/gateway-authentication.md)
for credential setup and Go/Vercel examples. Use HTTPS and configure
credentials through the client rather than adding authentication headers to
individual calls.

## Generate or stream a response

For private configured-account calls, choose a public model ID from your operator
or the model list below. This example assumes a private JWT client. Cloud calls
use a native `provider/model` ID and provider keys; see
[Use your own provider credentials](#use-your-own-provider-credentials).

```go
model, err := client.LanguageModel(modelID)
if err != nil {
	return err
}
result, err := aisdk.GenerateText(ctx, model,
	aisdk.WithModelMessages(provider.UserText("Summarize this incident.")),
)
if err != nil {
	return err
}
fmt.Println(result.Text)
```

Use `aisdk.StreamText` when your application should receive text as it arrives.
See [Generate text from Go](../getting-started/backend-only.md#stream-text-as-it-arrives)
for the stream-consumption pattern, or [Full-stack chat](../getting-started/full-stack-chat.md)
to connect a streaming response to your frontend. Consume or cancel every stream.

## Discover and select public models

Discovery is private configured-account functionality. Cloud/BYOK discovery
returns an explicit HTTP 400 invalid-request error, not an empty list.

Use `client.ListModels(ctx)` to build a model picker:

```go
rows, err := client.ListModels(ctx)
if err != nil {
	return err
}
for _, row := range rows {
	fmt.Println(row.ID, row.Name)
	if row.Gateway != nil {
		fmt.Println(row.Gateway.Aliases)
	}
}
```

Pass a row's ID or a known alias to `client.LanguageModel`. Aliases are other
names for the same configured model, not separate model-list entries. You can
also use an ID supplied by your operator without listing models first.

Some deployments include provider choices in `row.Gateway`: the primary model
and ordered fallbacks. Check for nil before using them. These describe the
configuration, not which provider served a previous call. Do not substitute a
candidate's provider-model ID for the public ID when calling a configured model.
Listing models does not check whether the providers are currently available.

### Access from TypeScript

Use `getAvailableModels()` on the Vercel Gateway client to list public model IDs.
If your application also needs aliases and configured provider choices, copy
[`configured-discovery.ts`](../../ai-gateway/examples/configured-discovery.ts)
into your server application:

```ts
import { fetchConfiguredModels } from "./configured-discovery";

const { models } = await fetchConfiguredModels({
  baseURL,
  headers: { "X-Access-Token": accessToken },
  signal,
});
const aliases = models[0]?.gateway?.aliases;
```

A private access JWT may alternatively use bearer `Authorization`; never send
both access headers. Use the same private Gateway URL and credentials as your
client; see the [authentication guide](../guides/gateway-authentication.md).
This helper does not support Cloud/BYOK discovery.

### Handle large catalogs

Discovery returns a complete model list or an error, never a partial list. If
large catalogs exceed your Go client's limit, adjust `DiscoveryBytes` in a copy
of `grafana.DefaultLimits()` and supply it when creating the client. See the
[client reference](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/grafana#Limits).
The TypeScript helper's `maxBytes` setting can lower its default limit, not raise
it; ask your operator for a smaller visible catalog if that limit is exceeded.

To combine the Gateway with other providers in your application, see
[Fallback and registry](../guides/fallback-and-registry.md).

## Configure provider-specific settings

Use `aisdk.WithProviderOptions` for settings such as Claude thinking or OpenAI
reasoning effort. Follow the [Anthropic](anthropic.md#enable-reasoning-deliberately)
and [OpenAI](openai.md#configure-a-call) examples with your Gateway model.

Choose a model that supports the file types, tools and reasoning settings your
application needs. When using fallback, provide appropriate settings for each
provider that may receive the request.

## Function tools

Define tools in your application to let the model request actions such as
looking up data or calling an API. Your application executes these functions
and returns their results to the model.

See [Tools](../guides/tools.md) to define a tool and [Agent loops](../guides/agent-loops.md)
to let the SDK manage multiple steps. The Gateway does not retain conversation
state; include the history needed for each subsequent call.

## Provider-defined tools

Some tools run on the model provider; others ask your application to perform an
action. Check `providerExecuted` on returned calls: do not execute a call in your
application when the provider has already executed it. A preliminary tool
result is a preview; wait for the final result before treating it as complete.

Provider-hosted MCP tools use the selected provider's tool or call settings;
see the [Anthropic](anthropic.md) and [OpenAI](openai.md#use-built-in-tools) guides.
For Anthropic MCP, use distinct server names and HTTPS URLs without embedded
credentials or fragments. Include the server configuration again when continuing
the conversation, along with prior calls, results and provider metadata.

Tool approvals, custom tool-result formats and generated media responses are
not supported.

## Use your own provider credentials

Cloud calls use bring your own key (BYOK): send your provider credentials with
each request. Confirm the Cloud/BYOK URL and availability with your operator;
Gateway authentication is still required. These calls never use configured
accounts, aliases or discovery; native providers decide model availability.

For BYOK, select the provider's native model using a `provider/model` ID rather
than a configured model or alias. For example, pass an OpenAI key with an
`openai/gpt-5` selection:

```go
credentials, err := json.Marshal(map[string]any{
	"byok": map[string]any{
		"openai": []map[string]string{{"apiKey": apiKey}},
	},
})
if err != nil {
	return err
}
model, err := client.LanguageModel("openai/gpt-5")
if err != nil {
	return err
}
result, err := aisdk.GenerateText(ctx, model,
	aisdk.WithModelMessages(provider.UserText("Summarize this incident.")),
	aisdk.WithProviderOptions(provider.RawProviderOption{
		Key: "gateway",
		Raw: credentials,
	}),
)
if err != nil {
	return err
}
fmt.Println(result.Text)
```

Read `apiKey` from your server's secret store. The `gateway.byok` account list
contains provider credentials; ordinary inference settings still belong in the
selected provider's options. From a Vercel server, pass the same account data:

```ts
import { generateText } from "ai";

const result = await generateText({
  model: gateway("openai/gpt-5"),
  prompt: "Summarize this incident.",
  providerOptions: {
    gateway: { byok: { openai: [{ apiKey }] } },
    openai: { store: false },
  },
});
```

Supply credentials on each call, including follow-up conversation steps. If you
provide multiple accounts, put them in priority order. Account fallback may
advance after an eligible failure, but authentication failures stop the chain.

Omit an account's `baseURL` to use the native provider endpoint. The bundled
service does not configure custom endpoint approvals, so other URLs are rejected.
This is separate from the client URL, which points to your Gateway.
OpenAI accounts can also specify `organization` and `project`; leave them out
when you do not need them. Anthropic accounts use the key and optional base URL.

## Keep credentials out of logs

Call options and returned request metadata can contain submitted provider keys.
Do not log them directly or send them to browser clients or telemetry. Remove
credentials from a copy before adding your own diagnostics.

Go's [structured logging middleware](../middleware/structured-logging.md) applies
its existing Redactor policy to fields such as apiKey and authentication headers
in structured capture copies. Use DefaultRedactorWithExtraKeys or RedactorFunc
for other sensitive fields; the complete BYOK object is not replaced. Ordinary
fields, timeout settings and provider text remain unchanged, including echoes.
TypeScript/application logging and other exporters need independent capture policy.
Start with metadata-only logs and avoid unnecessary payload/error-message capture.

## Configure fallback

For private configured-account calls, your Gateway operator can configure a
primary model and ordered backups behind one public model ID. Your application
keeps using that ID; it does not need to select backups itself.

Choose backups that support your required tools, file types, reasoning settings
and conversation history. Each call starts with the primary again, including
follow-up calls in a tool loop.

Once a provider returns a result or sends a stream event, the Gateway does not
switch providers for that call. A stream can therefore fail before visible text
arrives without trying a backup. A failed attempt may also have incurred charges
or performed a provider-hosted action, so fallback and application retries can
repeat work. Use idempotency where possible and avoid automatic replay for
operations that cannot tolerate duplication.

For troubleshooting, ask your operator to inspect private attempt logs. Public
model names and configured candidate lists do not identify which backend served
a particular request. See [Gateway observability](../../ai-gateway/docs/text-observability.md#inspect-fallback-attempts)
for operator diagnostics.

## Continue conversations with provider metadata

Models may return data they need on later calls, such as Claude thinking
signatures or OpenAI encrypted reasoning. Preserve that metadata with its
content when sending the next request; rebuilding history from text alone can
lose information the model needs.

Use the SDK's [agent loops](../guides/agent-loops.md) to manage history and tool
results. If you build messages yourself, retain previous calls and results,
including unresolved provider-executed calls. Choose fallback models that can
understand this history; the Gateway does not translate metadata between
providers.

## Native response values

Use returned sources to display citations and warnings to identify unsupported
settings. See the [source guide](../../ai-gateway/docs/sources.md) for citation
handling. Keep sensitive warning text and response details out of public logs.

Response IDs and model names describe the provider's generation and may differ
from the public model you requested. Use them for diagnostics, not to choose the
model for your next call. Some providers omit these details.

With `GenerateText` and `StreamText`, inspect response metadata after the call
finishes. If you call `model.DoGenerate` directly, native response identity is
available in `Response.Body` rather than its typed ID/model fields. Treat raw
response bodies as sensitive. Reading response details does not enable Gateway
content logging; configure application diagnostics separately.

## Bound work and handle errors

Set a deadline on the context for each generation or stream, and cancel it when
a consumer stops reading. See [Production practices](../best-practices/production.md)
for timeout, retry and stream-ownership patterns.

Use `errors.Is` to recognize cancellation and deadlines. Use `errors.As` with
`*grafana.GatewayError` to inspect the public error category/code, or with
`*provider.APICallError` for retryability. The client does not retry Gateway
requests itself; configure retries deliberately in your application or SDK.

For oversized responses or streams, adjust the client limits for your expected
workload rather than consuming a partial result. Use
[`grafana.DefaultLimits`](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/grafana#DefaultLimits)
as the starting point. Call headers are for application metadata, not overriding
Gateway authentication.

For HTTP errors, `APICallError.Data` and `ResponseBody` retain the complete bounded
response, including server-supplied diagnostics. Stream errors retain the
supplied `error.data` in `APICallError.Data`; their `ResponseBody` is empty.
These opaque extensions do not change classification or retryability, and later
valid stream parts remain consumable. Treat diagnostic payloads as sensitive
and keep them out of public logs.

## Reference

- [Grafana provider](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/grafana)
- [Gateway authentication](../guides/gateway-authentication.md)
- [Generate text from Go](../getting-started/backend-only.md)
- [Security practices](../best-practices/security.md)

---

← [Choose a provider](overview.md) · [Docs index](../README.md) · [Fallback and registry →](../guides/fallback-and-registry.md)
