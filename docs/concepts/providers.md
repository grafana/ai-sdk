# Providers and models

A provider lets your application call models through a service such as
Anthropic, Amazon Bedrock, OpenAI, or an OpenAI-compatible endpoint. Internally
provisioned Grafana services can also use Grafana's hosted endpoint. After
construction, every model works with the same generation, streaming, tool,
retry, and middleware APIs.

## What application code depends on

Construct a model with a provider package, then pass it to the core APIs:

```go
model := anthropic.New(apiKey, modelID)

result, err := aisdk.GenerateText(ctx, model,
	aisdk.WithModelMessages(provider.UserText("Hello")),
)
```

Switching providers changes model construction and provider-specific options;
the surrounding orchestration code can stay the same.

## Choose direct or routed access

Most applications should start with one provider model:

- Anthropic or Vertex AI
- Amazon Bedrock
- OpenAI Responses API
- Grafana's hosted endpoint for internal services
- an OpenAI-compatible Chat Completions endpoint

See the [provider overview](../providers/overview.md) for the selection guide and
setup links.

Use model composition only when the application needs it:

- `fallback` tries another model after eligible failures.
- `registry` resolves construction-oriented IDs such as `provider:model`.
- `gateway/catalog` exposes a controlled public model namespace with aliases
  and listing metadata.
- `middleware` adds behavior around any model.

A registry and a catalog solve different problems. A registry helps server code
construct a model. A catalog defines which stable names clients are allowed to
see and use.

## Provider-specific behavior

The common interface covers shared language-model behavior, but services still
differ in authentication, model IDs, supported inputs, built-in tools,
reasoning, structured output, and request options. Configure those differences
through the provider package so application code stays vendor-independent.

Unsupported generic settings are normally surfaced as warnings so applications
can observe provider differences without making every request fail.

## For provider authors

Provider implementations follow the registered LanguageModelV4 contract. To
adapt another service, see [Writing a provider](../providers/writing-a-provider.md).

## Reference

- [`provider.LanguageModel`](https://pkg.go.dev/github.com/grafana/ai-sdk/provider#LanguageModel)
- [Provider overview](../providers/overview.md)
- [Fallback and registry](../guides/fallback-and-registry.md)
- [Gateway model catalog](../guides/gateway-model-catalog.md)

---

← [UI message stream protocol](wire-protocol.md) · [Docs index](../README.md) · [Understand or debug the SDK](../README.md#understand-or-debug-the-sdk)
