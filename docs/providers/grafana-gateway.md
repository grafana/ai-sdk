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

Provider-defined tools are selected by the model provider. A returned call with
`providerExecuted: true` belongs to the provider; do not execute it in your
application. Provider-defined tools can also return client-executed calls, so use
the returned ownership marker rather than the definition to decide who runs them.

Provider calls and correlated results work with unary and streaming requests.
Preliminary results must be followed by a final result. Include unresolved
provider calls in assistant history when continuing on a later request; the
Gateway does not store them for you. Tool metadata is preserved as opaque
provider data, including extension fields and explicit empty objects. Returning
that data does not enable telemetry capture.

Anthropic-hosted MCP requires separate Gateway support. Tool approvals, custom
tool results and generated media responses are not yet supported.

## Configure provider-specific settings

Use `aisdk.WithProviderOptions` to configure settings for your selected model,
such as Claude thinking or OpenAI reasoning effort. See the [Anthropic](anthropic.md#enable-reasoning-deliberately)
and [OpenAI](openai.md#configure-a-call) guides for examples.

Choose a model that supports the file formats and reasoning settings your
application needs. These capabilities vary by provider and model.

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

`client.ListModels(ctx)` returns public model IDs, names, optional descriptions,
and their specification identifiers. Aliases remain ordinary independent rows
in server order. The client neither caches this catalog nor infers backend or
fallback topology. A malformed or oversized response returns no partial catalog.

Use the returned ID with `client.LanguageModel(id)`, or register the client as a
`registry.Provider`; see [Fallback and registry](../guides/fallback-and-registry.md).

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

Use call headers for application metadata. Credential-bearing call headers are
rejected; configure authentication on the client instead. See the
[package reference](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/grafana)
for configuration and result types.

---

← [Choose a provider](overview.md) · [Docs index](../README.md) · [Fallback and registry →](../guides/fallback-and-registry.md)
