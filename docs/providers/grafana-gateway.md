# Call Grafana AI Gateway from Go

Use the Grafana provider when your server calls Grafana AI Gateway. It uses the same generation, streaming, and registry
interfaces as other Go providers. Install the separate client module:

```bash
go get github.com/grafana/ai-sdk/providers/grafana
```

The client module is Apache-2.0 and does not depend on the Gateway service
module. Its executable response family includes text and unary/streaming function calls. Requests preserve
representable provider options, files, tools, and structured-output settings;
the deployed Gateway decides which capabilities it can execute and returns a
public invalid-request error for unsupported calls.

## Function tools

Direct routes support unary client-executed function tools. Definitions preserve
`strict: false`, examples, object schemas, and ordinary provider options;
Gateway-reserved option namespaces remain rejected. History accepts assistant
calls and text, JSON (including null), error-text, error-JSON, and content
results with text or supported file entries, preserving selected empty values.
The application executes tools and supplies call/result history on a later independent request.
The Gateway never executes a tool. Provider-executed/dynamic tools, approvals,
preliminary results, custom tool-result content, generated media responses, and
reasoning-file input remain unsupported. Logical telemetry
removes tool-bearing definitions, choices, inputs and outputs before export.

Streaming direct routes additionally support input start/delta/end, calls and
matching non-null JSON results. IDs, ordering and empty deltas are preserved.
Vercel and Go clients own the multi-step orchestration; each HTTP generation
remains stateless. Ordered fallback routes continue rejecting tool definitions,
choice and history before any physical invocation.

## Configure provider-specific settings

Use `aisdk.WithProviderOptions` to configure settings for your selected model,
such as Claude thinking or OpenAI reasoning effort. See the [Anthropic](anthropic.md#enable-reasoning-deliberately)
and [OpenAI](openai.md#configure-a-call) guides for examples.

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

## Bound work and handle errors

Operators can configure ordered text fallback using direct provider references:

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
Candidates retain configuration order and each new call starts at primary.
Models without fallback accept supported file inputs. Models configured with
fallback are text-only: files, nonempty tools, tool-call/result history,
active provider options, and tool choices other than plain auto are rejected
before any candidate, including primary, runs. Empty message-level provider
option objects are accepted. A stream's first part commits its candidate,
including an error part. No later failure
restarts on another provider. Client retries can multiply physical attempts;
the Gateway disables native-provider retries.

Private physical attribution writes newline-delimited `gateway_physical_attempt`
records to the existing operator stderr destination through a separate bounded
worker. Records contain the logical correlation ID when available, candidate
index, configured provider instance/backend model, start/decision timestamps,
selected/failed/canceled outcome, decision-time fallback intent, and winner.
They exclude payloads, credentials, headers, endpoint URLs, and raw errors.
These private records are separate from the canonical logical generation export.

The worker has a 256-record queue, 100 ms write deadline, 4096-byte record limit,
and one-second shutdown budget. It supports Linux stderr sockets and pipes,
plus sockets already configured nonblocking on other Unix platforms. A blocking
macOS socket is rejected because its send operation can ignore the per-call
nonblocking flag. The sink never changes the shared stderr descriptor flags;
unsupported destinations (including ordinary files or terminals) disable this
output while calls continue. Queue saturation and output failures drop records
and increment `grafana_ai_gateway_physical_attempt_dropped_total` with a closed
class label. Operators must verify their runtime stderr transport and private
log access policy before activation. Production enablement and rollback smoke
remain WP10 work; local fallback tests do not establish deployment acceptance.
FIFO deadline tests run only on Linux, matching the production output policy;
macOS tests verify nonblocking sockets, rejection of blocking sockets without
descriptor mutation, and portable queue/worker bounds.

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

Configured headers are applied before call headers, then client-owned auth,
content negotiation, and model protocol headers. Call headers also remain in
the request body, matching the registered client; a text-only Gateway may reject
them. See [package reference](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/grafana)
for configuration and result types.

---

← [Choose a provider](overview.md) · [Docs index](../README.md) · [Fallback and registry →](../guides/fallback-and-registry.md)
