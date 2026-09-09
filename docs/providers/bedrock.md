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

### Bedrock Mantle Responses

Bedrock Mantle is a separate AWS service with OpenAI-compatible API surfaces.
Use the nested `bedrock/mantle` package for the Responses API rather than
pointing the Converse provider at a Mantle host:

```go
model, err := mantle.NewResponses(
	ctx,
	"openai.gpt-5.6-luna",
	mantle.Config{
		AWSRegion:              "us-east-1",
		AWSCredentialsProvider: awsConfig.Credentials,
	},
)
if err != nil {
	return err
}
```

Most Mantle models use the regional `/v1/responses` route. Some newer models
use AWS's documented `/openai/v1/responses` compatibility route instead. The
maintained exception set currently covers GPT-5.4, GPT-5.5, the documented
GPT-5.6 variants, Grok 4.3 and 4.6, and Gemma 4, including 26B-A4B; the
constructor selects the route from the exact model ID. The allowlist is
intentionally exact because AWS assigns this route per model rather than by
family. When AWS adds a model, verify the Programmatic Access endpoint on its
model card and update both the allowlist and its route test. Requests are signed
with SigV4 service `bedrock-mantle`, using the standard AWS credential chain
when credentials are not supplied explicitly.

For a controlled bearer-authentication rollback, set `Config.APIKey` or
`AWS_BEARER_TOKEN_BEDROCK`. Explicit bearer and AWS credential modes are
mutually exclusive and fail closed when combined.

A custom `Config.BaseURL` must include the intended API prefix, such as
`https://proxy.example.com/v1` or `https://proxy.example.com/openai/v1`. HTTP
clients, retry settings, and static headers can be passed using `openai-go`
request options.

The model reports provider identity `bedrock-mantle.responses`; Responses call
options and continuation metadata remain under the `openai` namespace. Mantle
Chat Completions, including safeguard models, are not yet implemented.

The root `bedrock` package remains a Converse provider. Although it can infer
the `bedrock-mantle` signing service for a Mantle host, it still emits
Converse-shaped requests and must not be used for the Mantle OpenAI-compatible
surface.

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
[gateway model catalog](../guides/gateway-model-catalog.md) when clients should
see a stable provider-neutral name.

## Scope

The root package covers language-model text generation through Converse and
ConverseStream. The nested `mantle` package covers OpenAI-compatible Responses.
Mantle Chat, embeddings, image generation, reranking, and other Bedrock APIs are
outside the current scope.

## Reference

- [`providers/bedrock`](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/bedrock)
- [`providers/bedrock/mantle`](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/bedrock/mantle)
- [AWS Bedrock Converse API](https://docs.aws.amazon.com/bedrock/latest/APIReference/API_runtime_Converse.html)
- [AWS Bedrock Responses API](https://docs.aws.amazon.com/bedrock/latest/userguide/bedrock-mantle.html)
- [AWS Gemma 4 26B-A4B model card](https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-google-gemma-4-26b-a4b.html)

---

← [Anthropic](anthropic.md) · [Docs index](../README.md) · [OpenAI →](openai.md)
