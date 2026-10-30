# Call Grafana AI Gateway from Go

Use the Grafana provider to call Grafana AI Gateway from your application
server. You select a model, then use the same generation, streaming and tool
APIs as with any other provider. Which models and features you get depends on
how your Gateway is configured.

```bash
go get github.com/grafana/ai-sdk github.com/grafana/ai-sdk/providers/grafana
```

Keep Gateway and model-provider credentials on your server, not in browser code.

For the server-side flow, see [Gateway architecture](../concepts/gateway-architecture.md)
and the [glossary](../concepts/gateway-glossary.md).

## Inspect raw provider events

Pass `aisdk.WithIncludeRawChunks()` to `StreamText` (or set `IncludeRawChunks`
on direct `DoStream` calls) to receive the selected provider's native stream
events as `aisdk.StreamRaw` parts, in order with the normalized parts. Without
the option, the Gateway sends no raw events. Unary calls accept the option and
return no raw output.

Raw events are for debugging. Their shape is whatever the provider sends and can
change without notice, so do not build application logic on them. Events keep
their JSON values but not their exact bytes: whitespace, key order and escaping
can differ from the provider's. An event that is not valid JSON, or that repeats
an object key, is skipped. Provider error events pass through, so they can
contain more detail than the Gateway's own errors. Treat raw events as
sensitive. The Gateway does not log or export them.

The Gateway removes MCP tool credentials that OpenAI echoes from your request
before it sends an event: headers, the authorization value, and credential
query parameters in the server URL.

Raw events are never truncated, and their size does not end a stream. The
Gateway does not limit the size of a provider stream; the request deadline and
the stream idle limit bound it.

## Authenticate the client

Use your Gateway URL as the base URL. The Gateway's overview page in Grafana
shows it, for example
`https://grafana.example.com/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-123`.
Then choose the constructor for your credential:

- `NewWithServiceAccountToken` for a Grafana service account token.
- `NewWithTokenExchange` for a Grafana service with a Cloud access policy
  token. Add `WithOnBehalfOf` to a call's context to make it on behalf of a
  user.
- `NewWithAccessToken` for an access token you obtain and refresh yourself.

```go
client, err := grafana.NewWithServiceAccountToken(grafana.ServiceAccountConfig{
	Token:   os.Getenv("GRAFANA_SERVICE_ACCOUNT_TOKEN"),
	BaseURL: gatewayURL,
})
if err != nil {
	return err
}
```

[Authenticate to Grafana AI Gateway](../guides/gateway-authentication.md)
explains which credential to use and what access it gets, with Go and
TypeScript examples. Use HTTPS, and let the client add authentication rather
than setting headers on individual calls.

## Generate or stream a response

Pass a model ID from your Gateway's model list to `client.LanguageModel`:

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

Use `aisdk.StreamText` to receive text as it arrives. See
[Generate text from Go](../getting-started/backend-only.md#stream-text-as-it-arrives)
for how to consume a stream, or [Full-stack chat](../getting-started/full-stack-chat.md)
to stream a response to your frontend. Always consume or cancel a stream.

If you bring your own provider key, select models differently; see
[Use your own provider credentials](#use-your-own-provider-credentials).

## Discover and select public models

List the configured models to build a model picker:

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

Pass a row's ID or one of its aliases to `client.LanguageModel`. An alias is
another name for the same model, so it has no row of its own.

`row.Gateway` can also list the model's primary provider and its fallbacks.
Check for nil before using it. It describes the configuration, not which
provider answered a past call, and it does not mean the providers are
available. Always call the model by its ID, not by a provider's model name.

Callers that can only bring their own provider key, such as users in Grafana
Cloud, get an invalid request error instead, because their provider account
decides which models exist.

### List models from TypeScript

Use `getAvailableModels()` on the Vercel Gateway client to list model IDs. To
also get aliases and provider choices, copy
[`configured-discovery.ts`](../../ai-gateway/examples/configured-discovery.ts)
into your server application:

```ts
import { fetchConfiguredModels } from "./configured-discovery";

const { models } = await fetchConfiguredModels({
  baseURL,
  headers: { Authorization: `Bearer ${serviceAccountToken}` },
  signal,
});
const aliases = models[0]?.gateway?.aliases;
```

A Grafana service can send its exchanged token in `X-Access-Token` instead of
`Authorization`, but never both.

### Handle large catalogs

Listing returns the whole catalog or an error, never part of it. The Go client
does not limit the catalog size. The TypeScript helper's `maxBytes` can only
lower its limit, so if a catalog is too large for it, ask your Gateway
administrator to publish fewer models.

To combine the Gateway with other providers, see
[Fallback and registry](../guides/fallback-and-registry.md).

## Configure fallback

Your Gateway administrator can put a primary model and ordered backups behind
one model ID. Your application keeps using that ID, and the Gateway tries the
backups when the primary fails. Each call starts with the primary again,
including follow-up calls in a tool loop.

Once a provider has started answering, the Gateway does not switch providers
for that call. A stream can therefore fail without trying a backup. A failed
attempt may already have cost money or run a provider-hosted tool, so fallback
and your own retries can repeat work. Avoid automatic retries for operations
that must not run twice.

To see which models were tried, read the
[execution overview](#inspect-execution-overviews-and-failures).

## Use your own provider credentials

When you bring your own key (BYOK), the Gateway calls the provider with your
provider account. This is how users and service accounts call the Gateway in
Grafana Cloud. Select the provider's own model as `provider/model`, such as
`openai/gpt-5`, and send your key with every call:

```go
model, err := client.LanguageModel("openai/gpt-5")
if err != nil {
	return err
}
result, err := aisdk.GenerateText(ctx, model,
	aisdk.WithModelMessages(provider.UserText("Summarize this incident.")),
	aisdk.WithProviderOptions(grafana.GatewayOptions{
		BYOK: map[string][]grafana.BYOKAccount{
			"openai": {{APIKey: apiKey}},
		},
	}),
)
if err != nil {
	return err
}
fmt.Println(result.Text)
```

From a Vercel server, pass the same data:

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

Read `apiKey` from your server's secret store. Other provider settings still go
under the provider's own options, such as `openai`.

- Send the key on every call, including follow-up calls in a conversation.
- List several accounts in priority order to fall back between them. An
  authentication failure stops the fallback.
- Leave out `baseURL` to use the provider's own endpoint; other endpoints are
  refused.
- OpenAI accounts can also set `organization` and `project`.

Calls with your own key do not use the Gateway's configured models or aliases.
To see which of your accounts were tried, read the
[execution overview](#inspect-execution-overviews-and-failures).

## Route a call to other models and providers

For one call, you can name fallback models and prefer or restrict providers.
These controls use the same names as the Vercel AI Gateway, under
`providerOptions.gateway`:

- `models` lists fallback model IDs. The Gateway tries the model you selected
  first, with all of its providers, then each listed model in order.
- `order` lists providers to try first. Within each model, matching providers
  move to the front in that order, and the others follow in their usual order.
  It does not remove providers.
- `only` lists the only providers to use. It applies to each model before
  `order`. A model with none of these providers is skipped.

```go
result, err := aisdk.GenerateText(ctx, model,
	aisdk.WithModelMessages(provider.UserText("Summarize this incident.")),
	aisdk.WithProviderOptions(grafana.GatewayOptions{
		Models: []string{"grafana/backup"},
		Order:  []string{"anthropic"},
	}),
)
```

```ts
import type { GatewayProviderOptions } from "@ai-sdk/gateway";
import { generateText } from "ai";

const result = await generateText({
  model: gateway("grafana/assistant"),
  prompt: "Summarize this incident.",
  providerOptions: {
    gateway: { models: ["grafana/backup"], order: ["anthropic"] } satisfies GatewayProviderOptions,
  },
});
```

Which names you can use depends on how you call the Gateway:

- With configured models, `models` entries are model IDs or aliases from
  [model discovery](#discover-and-select-public-models). Provider names are the
  `provider` values discovery reports for each model, such as `anthropic`,
  `openai` or a compatible provider's name, not the names of configured
  provider accounts.
- With your own keys, `models` entries are `provider/model` selectors, and you
  must send keys under `byok` for each provider you name. The Gateway never
  uses its own provider accounts for your calls.

The controls never add destinations: you can only reach models and accounts
you could already call directly. A call tries at most 8 providers in total;
each provider and model is tried once, even when several routes share it.
Everything else about the call still applies to every attempt: each provider
reads its own options, such as `anthropic` or `openai`, and the
[fallback rules](#configure-fallback) about committed output are the same.

The Gateway checks the controls before calling any provider, and rejects:

- with configured models, a `models` entry it cannot find, with a model not
  found error;
- with your own keys, a `models` entry that is not a supported
  `provider/model` selector or names a provider you sent no keys for, with an
  invalid request error;
- an `only` list that leaves no provider, a list with an entry that is not a
  non-empty string, a conditional `models` entry, or a call that would try more
  than 8 providers, with an invalid request error;
- any other control, such as `sort`, `user`, `tags`, `zeroDataRetention` or
  `providerTimeouts`, with an invalid request error, rather than ignoring it.

The Go client does not check these values itself, and its `GatewayOptions`
has only the controls the Gateway supports. To see how a call was routed, read
the [routing summary](#inspect-execution-overviews-and-failures).

## Keep credentials out of logs

Call options and request details can contain the provider keys you send. Do
not log them, and do not send them to browsers or telemetry.

The [structured logging middleware](../middleware/structured-logging.md)
redacts fields such as `apiKey` and authentication headers. Add your own
sensitive fields with `DefaultRedactorWithExtraKeys` or `RedactorFunc`.
Redaction does not catch a key that a provider repeats in its response text,
so start with metadata-only logging.

## Configure provider-specific settings

Use `aisdk.WithProviderOptions` for settings such as Claude thinking or OpenAI
reasoning effort; see the [Anthropic](anthropic.md#enable-reasoning-deliberately)
and [OpenAI](openai.md#configure-a-call) examples. Choose a model that supports
the files, tools and settings your application needs. With fallback, include
settings for each provider that may receive the call.

## Structured output

The SDK's [structured output helpers](../guides/structured-output.md) work with
the Gateway in generated and streamed calls. Schemas need an object at the
root. The model returns JSON text, and the SDK parses and validates it.

Schema support depends on the provider. OpenAI and Anthropic models follow the
schema natively. Models from OpenAI-compatible providers follow the schema when
the operator sets `supportsStructuredOutputs: true` on that provider. Otherwise
they use JSON mode and return a warning when you supply a schema.

`provider.ResponseFormat.Name` is a pointer: use `new("answer")` to set a name.
See the [provider reference](https://pkg.go.dev/github.com/grafana/ai-sdk/provider#ResponseFormat).

## Function tools

Define tools in your application to let the model look up data or call APIs.
Your application runs them and returns their results to the model. See
[Tools](../guides/tools.md) and [Agent loops](../guides/agent-loops.md). The
Gateway keeps no conversation state, so send the history each call needs.

## Provider-defined tools

Some tools run on the provider. Check `providerExecuted` on returned tool
calls, and do not run a call your application received already executed. A
preliminary tool result is a preview; wait for the final result.

Configure provider-hosted MCP tools through the provider's options; see the
[Anthropic](anthropic.md) and [OpenAI](openai.md#use-built-in-tools) guides.
For Anthropic MCP, give each server a distinct name and an HTTPS URL without
credentials. Send the server configuration again on each follow-up call,
together with earlier calls, results and provider metadata.

When a provider asks for approval, approve or deny it and continue the
conversation, in generated or streamed calls. The approval ID is different
from the tool call ID. The Gateway does not store approvals.

For OpenAI hosted MCP, link the approval to its tool call before you continue:
find the provider-executed tool call whose ID matches the approval's
`toolCallId`, and set that call's `providerOptions.openai.approvalRequestId` to
the `approvalId`. The SDK does not do this for you.

`ToolResultOutput.Reason` is a pointer, so a denied tool call can tell an
absent reason (nil) from an empty one (`new("")`).

## Continue conversations with provider metadata

Models can return data they need on later calls, such as Claude thinking
signatures or OpenAI encrypted reasoning. Keep that metadata with its content
when you send the next request; history rebuilt from text alone loses it.

[Agent loops](../guides/agent-loops.md) manage this for you. If you build
messages yourself, keep earlier tool calls and results, including
provider-executed calls. With fallback, choose models that understand each
other's history; the Gateway does not translate metadata between providers.

### Preserve custom content

Some providers return content without a standard type, such as OpenAI
compaction items. It arrives as custom content with a `kind`, such as
`openai.compaction`, and provider metadata. Send these parts back in the
history, with the metadata as their provider options, so the provider can
continue the conversation. Other providers ignore custom content they don't
recognize, so it is safe with fallback. If no model can accept custom content
in a tool result, the call fails with an invalid request error.

## Receive generated files

When a model generates files, the Gateway returns them in generated and
streamed responses. The SDK downloads files that arrive as URLs; the Gateway
does not. Once a model
has returned a file, the Gateway does not fall back for that call.

## Native response values

Use returned sources and text citations to display references, and warnings to
identify unsupported settings. See the [source guide](../../ai-gateway/docs/sources.md) for citation
handling. Keep sensitive warning text and response details out of public logs.

One stream can carry several `response-metadata` and `finish` events when the
provider returns several messages or responses in one call. The stream ends
when the provider closes it, not at the first `finish`.

Response IDs and model names come from the provider and can differ from the
model you selected. Use them to diagnose a call, not to pick the next model.
Some providers leave them out. With `GenerateText` and `StreamText`, read them
from the response metadata after the call. With `model.DoGenerate`, they are in
`Response.Body`. Treat response details as sensitive.

## Inspect execution overviews and failures

Calls return a `gateway.execution` entry in the provider metadata of the
result or each stream finish. It names the model you selected and lists each
attempt in order, with its provider, model, outcome and, for a failure, the
provider's error message, type, code and status. In Go, read it from
`ProviderMetadata`; in TypeScript, from `providerMetadata`. Each call has its
own overview, so retries and tool loop steps start new ones.

For configured models, the overview also has the resolved `canonicalModelId`
and each attempt's `providerInstance`. For calls with your own key,
`requestedModelId` is the `provider/model` you sent, there is no
`canonicalModelId` or `providerInstance`, and the attempts follow the order of
your accounts. An account that failed before a later one answered keeps its
own error summary. The overview has no account or key field; use the
attempt's position to match it to your list. A provider's error text can still
repeat your key, as described below.

Next to it, `gateway.routing` groups the same attempts by model, using field
names from the Vercel AI Gateway: `originalModelId`, `canonicalSlug` for
configured models, `resolvedProvider` (the first provider tried),
`finalProvider` (the one that answered, absent when none did), and
`modelAttempts`. Each model attempt has its `modelId` as you sent it, a
`success` flag and its `providerAttempts`, each with a `provider`, a `success`
flag and, for a failure, the provider's `statusCode`. Models the call never
reached are not listed. Unlike Vercel's, it has no timings, costs, credential
types or providers that were not tried.

Errors carry the same information:

- An HTTP error has it in `providerMetadata` in the error body. In Go, read
  `*provider.APICallError.Data`.
- A stream error has `error.data.providerMetadata` and, when available, the
  provider's error in `error.data.nativeError`. In Go, read the part's
  `APICallError.Data`. The stream's later parts remain readable.

Overviews are best-effort: a missing overview does not mean no provider was
tried. A call the Gateway rejects before trying any account, such as one with
missing or malformed `gateway.byok` credentials, unsupported options or
invalid routing controls, has no overview. A selected attempt, or a provider
attempt with `success`, means the provider started answering, not that the
stream completed.

The Gateway replaces its own configured provider keys with `[REDACTED]` in
error summaries. It does not change your own provider key if a provider repeats
it in an error, or any other content. Treat these details as sensitive, and
choose deliberately which ones your frontend shows.

## Bound work and handle errors

Set a deadline on each call's context, and cancel it when nobody reads the
stream anymore. See [Production practices](../best-practices/production.md).

Use `errors.Is` to detect cancellation and deadlines. Use `errors.As` with
`*grafana.GatewayError` for the Gateway's error category and code, or with
`*provider.APICallError` to check whether a call can be retried. The client
does not retry on its own; configure retries in your application.

The Gateway limits provider response sizes and stream idle time, and Grafana
limits request size and duration, so the client has no size limits to
configure. To bound a call yourself, use the
context deadline above, and your own `http.Client` for transport-level limits
such as timeouts.

`APICallError.Data` and `ResponseBody` contain the error response, which can
include provider details; keep it out of public logs.

## Reference

- [Grafana provider](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/grafana)
- [Gateway authentication](../guides/gateway-authentication.md)
- [Generate text from Go](../getting-started/backend-only.md)
- [Security practices](../best-practices/security.md)

---

← [Choose a provider](overview.md) · [Docs index](../README.md) · [Fallback and registry →](../guides/fallback-and-registry.md)
