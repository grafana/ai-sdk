# Call Grafana AI Gateway from Go

Use the Grafana provider when your server calls Grafana AI Gateway. It uses the same generation, streaming, and registry
interfaces as other Go providers. Install the separate client module:

```bash
go get github.com/grafana/ai-sdk/providers/grafana
```

The client module is Apache-2.0 and does not depend on the Gateway service
module. Use it to generate text and make function-tool calls, with or without
streaming. Available features depend on your Gateway deployment and selected
model; unsupported requests return an invalid-request error.

## Function tools

Define function tools in your application to let a model request actions such as
looking up data or calling an API. Your application executes the functions and
returns their results to the model. Function tools work with both non-streaming
and streaming calls.

For a multi-step conversation, include previous tool calls and their results in
the next request. The Go and Vercel SDKs can manage this tool loop for you. See
[Tools](../guides/tools.md) and [Agent loops](../guides/agent-loops.md) for setup.
The Gateway does not retain conversation state between requests.

Provider-executed tools, including provider-hosted MCP, are not yet available
through the Gateway. Tool approvals, dynamic tools, preliminary or custom tool
results, and generated media responses are also not yet supported.

## Configure provider-specific settings

Use `aisdk.WithProviderOptions` to configure settings for your selected model,
such as Claude thinking or OpenAI reasoning effort. See the [Anthropic](anthropic.md#enable-reasoning-deliberately)
and [OpenAI](openai.md#configure-a-call) guides for examples.

Choose a model that supports the file formats and reasoning settings your
application needs. These capabilities vary by provider and model.

## Native response values

Supported unary/stream warnings preserve their registered variants, active
fields, order and multiplicity. Required empty strings remain meaningful;
optional absent/empty details normalize to the same Go empty string. Native
warning text is no longer replaced with generic prose.

URL/document sources preserve native IDs/title/filename without sequential ID
rewriting, deduplication or `file_path` display substitution. Required IDs and
document titles may be empty; optional empty title/filename values are omitted
by Go-produced responses. See the [source guide](../../ai-gateway/docs/sources.md)
for citation display and current metadata limitations.

Raw unary `response` and streaming `response-metadata` carry supplied native
ID/modelId/timestamp, not requested/canonical route defaults. Native model IDs
need not match public route syntax. Go-produced optional empty identity strings
and zero timestamps are omitted; present identity-free unary responses emit
`{}`, while a nil native response omits the object. Nonzero timestamps preserve
their instant as UTC RFC3339Nano.

Both the registered TS and Go clients replace typed unary request/response with
Gateway-hop transport information. Native unary identity remains nested inside
the bounded `Response.Body`; typed response ID/model/timestamp remain unset.
Streaming identity survives in provider parts. Do not use returned native
identity for route resolution: requested model selection and canonical operator
metrics remain separate.

Independently configured consumer middleware can observe these contracted
values and opt into bounded Gateway response-body logging at its own
destination. Raw-body access alone is not capture, and neither provides full
native transport diagnostics. Central Gateway observations remain metadata-only;
see [text observability](../../ai-gateway/docs/text-observability.md#returned-values-and-consumer-observation).
Response identity/warnings are not universal UI fields; UI message metadata
requires an explicitly configured mapping.

## Authenticate the client

Choose the constructor for your Gateway URL. Use `NewWithCloudCredentials`
with your stack ID and CAP token for the public Grafana Cloud URL. If your
deployment provides a separate JWT-enabled Gateway URL, use
`NewWithTokenExchange` or `NewWithAccessToken` instead.

See [Authenticate to Grafana AI Gateway](../guides/gateway-authentication.md)
for Go and Vercel examples, policy scopes, and URL selection. All constructors
reject redirects and do not discover credentials or endpoints from the
environment.

## Discover and select public models

Use `client.ListModels(ctx)` to build a model picker or find an ID to pass to
`client.LanguageModel(id)`. It returns one row per configured model, with its
name, description and canonical public ID. `Gateway.Aliases` lists other valid
selection IDs; aliases do not create duplicate rows.

When available, `ModelInfo.Gateway` also tells you which providers and models are
configured behind each public ID. Check for nil because some catalogs provide
only public names:

```go
rows, err := client.ListModels(ctx)
if err != nil {
	return err
}
for _, row := range rows {
	if row.Gateway == nil {
		continue
	}
	fmt.Println(row.ID, row.Gateway.Aliases)
	primary := row.Gateway.Primary
	fmt.Println(primary.ProviderInstance, primary.Provider, primary.ProviderModelID)
	for _, fallback := range row.Gateway.Fallbacks {
		fmt.Println(fallback.ProviderInstance, fallback.Provider, fallback.ProviderModelID)
	}
}
```

Use `Primary` and ordered `Fallbacks` to explain configured provider choices,
not to identify which provider served a previous request. Direct models have an
empty fallback list. Listing does not call providers or check availability.
Make requests with the public row ID or one of its aliases, not a target's
`ProviderModelID`.

### Access from TypeScript

`@ai-sdk/gateway`'s `getAvailableModels()` lists canonical public IDs but discards
`gateway`, including aliases and configured provider choices. Alias IDs remain
callable when you already know them. To discover them and provider choices, copy the
[`configured-discovery.ts` helper](../../ai-gateway/examples/configured-discovery.ts)
into your application and call it separately:

```ts
import { fetchConfiguredModels } from "./configured-discovery";

const { models } = await fetchConfiguredModels({
  baseURL,
  headers: { "X-Access-Token": accessToken },
  signal,
});
const route = models[0]?.gateway;
const aliases = route?.aliases;
const primary = route?.primary;
const fallbacks = route?.fallbacks;
```

For Grafana Cloud, select `headers: { Authorization: \`Bearer ${stackID}:${capToken}\` }`
instead; see the [authentication guide](../guides/gateway-authentication.md).
Run this on your server and use HTTPS. Pass the same API-prefix URL and credentials
as your Gateway client; the helper refuses redirects. If you supply a custom
`fetch`, it must honor redirect and cancellation options. Candidate information
may be absent, so keep the optional access shown above.

### Handle large catalogs

Discovery returns a complete catalog or an error, never a partial list. The Go
client accepts up to 4 MiB by default; adjust its discovery limit through
`grafana.DefaultLimits()` if needed. The TypeScript helper also accepts up to
4 MiB; use `maxBytes` to set a smaller limit. Clients decode typed metadata without
revalidating IDs, candidate uniqueness or route consistency. Those rules belong
to server startup validation.

Strings use standard JSON decoding. For escaped lone UTF-16 surrogates, Go
returns U+FFFD while TypeScript retains the decoded surrogate. Do not rely on
identical candidate strings across clients for such malformed Unicode.

The Gateway serves its complete visible configured catalog without a discovery
response-size cap. If your client's read limit rejects it, raise the Go discovery
limit or ask your operator to reduce the catalog. Provider credentials are never
included in discovery; your deployment controls who may see the model list.

To compose the client with other providers, register it as a `registry.Provider`;
see [Fallback and registry](../guides/fallback-and-registry.md).

## Configure fallback

Configure a primary model and ordered backups under the same public model ID:

```yaml
models:
  grafana/assistant:
    name: Assistant
    primary:
      provider: anthropic-primary
      model: claude-sonnet-4-6
    fallback:
      - provider: anthropic-secondary
        model: claude-sonnet-4-6
```

Both provider instances must be declared in `providers` using the existing
environment-variable credential references. Omitting `fallback` creates a direct
route; removing it restores direct routing without changing the public model ID.
Each call starts with the primary model and tries backups in configuration order
after an eligible failure. Follow-up calls in a tool loop also start with the
primary; a backup used for one step does not become the default for later steps.

Choose fallback models that support your tools, file formats, and reasoning
needs. Provider-specific settings are passed unchanged, so configure the settings
for each provider in the chain.

Once a model returns a result or sends its first stream event, the Gateway will
not switch to another model for that call. This includes start and error events,
so a stream can fail without producing visible text and still not try a backup.
This boundary avoids mixing responses from different models or replaying tool
calls after a response has started.

A failed attempt may still incur provider charges. Account for both Gateway
fallback and SDK retries when setting your latency and cost budgets; neither
guarantees that provider work happens only once. The Gateway disables retries in
its native provider clients.

For troubleshooting, ask your Gateway operator to inspect fallback attempts in
private logs. Public model names do not identify which backend served a request.
See [Gateway observability](../../ai-gateway/docs/text-observability.md#inspect-fallback-attempts)
for operator diagnostics and log access requirements.

## Continue conversations with provider metadata

Some models return information that they need on later calls, such as Claude
thinking signatures or OpenAI encrypted reasoning. The Gateway returns this
provider metadata with the response so your application can continue the conversation.

Use the SDK's [agent loops](../guides/agent-loops.md) to manage tool calls and
conversation history. If you build follow-up messages yourself, keep the returned
provider metadata with its content; reconstructing messages from text alone can
lose information the model needs.

Provider metadata is specific to the model that returned it. When configuring
fallback, choose models that can use the conversation history you send. The
Gateway does not translate one provider's metadata for another provider.

## Bound work and handle errors

Use a cancelable context for each generation or stream. Cancel it when a
consumer stops reading. The client closes its response body and stream channel
on cancellation. It does not replay Gateway requests; SDK retry and fallback
orchestration remain the caller's choice. Authlib may retry its token-exchange
request according to its own policy.

`grafana.DefaultLimits()` provides byte bounds for discovery, unary responses,
errors, and streams, plus stream event-size and event-count bounds. Copy that
value, adjust the required limits, and pass its address in the constructor.
Explicit limits must all be positive. Total stream bytes include wire framing;
event bytes count a complete event with CRLF normalized to one line ending.
Call contexts and HTTP client timeouts set latency bounds.

Use `errors.As` with `*grafana.GatewayError` for the public category/code and
with `*provider.APICallError` for retryability. Context cancellation and deadlines
remain identifiable with `errors.Is`. A malformed response is a non-retryable
protocol failure; a transport failure is retryable but never retried internally.
Warnings and public error messages remain server-provided text. Unknown private
metadata is not promoted into model identity or Gateway error fields. A unary
response's bounded raw HTTP body remains available in `Response.Body`.

For a valid HTTP error, `APICallError.Data` and `ResponseBody` retain the complete
bounded response, including any additive diagnostics supplied by the server.
For a committed SSE error, `APICallError.Data` retains exactly the supplied
`error.data`; `ResponseBody` is empty because an SSE event is not an HTTP error
response. These extensions remain opaque and do not change classification or
status-derived retryability. Error envelope and payload fields use standard Go
JSON decoding, including case-insensitive field matching. Later valid stream
parts remain consumable.

Use call headers for application metadata. Credential-bearing call headers are
rejected; configure authentication on the client instead. See the
[package reference](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/grafana)
for configuration and result types.

---

← [Choose a provider](overview.md) · [Docs index](../README.md) · [Fallback and registry →](../guides/fallback-and-registry.md)
