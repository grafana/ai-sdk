# Choose a provider

A provider lets your application call a language-model service through the same
Go generation and streaming APIs. Choose the provider for the endpoint and
authentication your application uses.

## Provider guide

| Provider | Use it when |
|---|---|
| [Anthropic](anthropic.md) | You call Claude through the Anthropic API or Google Vertex AI |
| [Amazon Bedrock](bedrock.md) | You call models through AWS Bedrock Converse |
| [OpenAI](openai.md) | You call OpenAI's Responses API |
| [Grafana Cloud](grafana-cloud.md) | An internal Grafana service calls provisioned models through the hosted AI SDK endpoint |
| [OpenAI-compatible](openai-compatible.md) | You call a Chat Completions-compatible `/v1/chat/completions` server |

The OpenAI provider targets OpenAI's current Responses API. The OpenAI-compatible
provider targets vLLM, LM Studio, Kimi/Moonshot, and other servers implementing
the Chat Completions shape.

## What changes between providers

The core generation flow stays the same:

```go
result, err := aisdk.GenerateText(ctx, model,
	aisdk.WithModelMessages(provider.UserText(prompt)),
)
```

Provider setup determines:

- authentication and base URL;
- model IDs;
- supported input types and tools;
- reasoning and structured-output behavior;
- provider-specific request options;
- underlying client retry behavior.

Choose based on the authentication model, deployment environment, model
capabilities, and provider-specific behavior your application requires. After
constructing a model, continue with [Generate text from Go](../getting-started/backend-only.md)
or [Full-stack chat](../getting-started/full-stack-chat.md).

## Install only what you use

Each provider is a separate Go module. For example:

```bash
go get github.com/grafana/ai-sdk/providers/openai
```

This keeps AWS, Google Cloud, OpenAI, and other vendor dependencies out of
applications that do not need them.

## Compose models later

Start with one model. Add composition only for a concrete requirement:

- improve availability with [fallback](../guides/fallback-and-registry.md);
- resolve server-side IDs with a [registry](../guides/fallback-and-registry.md);
- expose stable public IDs with a [gateway catalog](../guides/gateway-model-catalog.md);
- apply common behavior with [middleware](../middleware/overview.md).

If the service is not supported, see [Writing a provider](writing-a-provider.md).

---

← [Choose a model service](../README.md#choose-a-model-service) · [Docs index](../README.md) · [Anthropic →](anthropic.md)
