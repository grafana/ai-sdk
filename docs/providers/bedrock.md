# Amazon Bedrock

Use the Bedrock provider when your application calls models through the AWS
Bedrock Converse API. It supports model families including Anthropic, Mistral,
Amazon Nova, and OpenAI, subject to Bedrock availability and each family's
capabilities.

## Install

```bash
go get github.com/grafana/ai-sdk/providers/bedrock
```

## Create a model

```go
model := bedrock.New(
	"us.anthropic.claude-haiku-4-5-20251001-v1:0",
	bedrock.WithRegion("us-east-1"),
)
```

Use the Bedrock model or inference-profile ID provisioned for your account and
region. Cross-region inference profiles commonly include a region-group prefix.
Pass the resulting model to [Generate text from Go](../getting-started/backend-only.md)
or [Full-stack chat](../getting-started/full-stack-chat.md).

## Authenticate

By default, the provider uses the AWS SDK v2 credential chain, including
environment variables, shared configuration, and workload identity. Credentials
are resolved lazily on the first call.

Supply a credential provider when the application already owns AWS
configuration:

```go
awsConfig, err := config.LoadDefaultConfig(ctx)
if err != nil {
	return err
}

model := bedrock.New(modelID,
	bedrock.WithRegion("us-east-1"),
	bedrock.WithCredentials(awsConfig.Credentials),
)
```

For deployments configured for Bedrock bearer-token authentication, use
`WithBearerToken`. Do not combine application credentials and user-controlled
model IDs without an authorization boundary.

## Account for model-family differences

The provider translates common AI SDK messages and tools into Converse requests,
then applies family-specific behavior based on the Bedrock model ID. Reasoning,
structured output, cache controls, and other provider options may be supported
by one family and ignored with a warning by another.

Validate the capabilities required by your workflow before putting unlike model
families in the same fallback chain.

## Resolve Bedrock IDs through a registry

`bedrock.NewProvider` implements registry-based construction:

```go
models := registry.NewProviderRegistry(map[string]registry.Provider{
	"bedrock": bedrock.NewProvider(bedrock.WithRegion("us-east-1")),
})

model, err := models.LanguageModel("bedrock:" + modelID)
```

The `bedrock:` route is a server-side construction detail. Use a
[AI Gateway model catalog](../../ai-gateway/docs/model-catalog.md) when clients should
see a stable provider-neutral name.

## Scope

This package covers language-model text generation through Converse and
ConverseStream. Embeddings, image generation, reranking, and other Bedrock APIs
are outside its current scope.

## Reference

- [`providers/bedrock`](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/bedrock)
- [AWS Bedrock Converse API](https://docs.aws.amazon.com/bedrock/latest/APIReference/API_runtime_Converse.html)

---

← [Anthropic](anthropic.md) · [Docs index](../README.md) · [OpenAI →](openai.md)
