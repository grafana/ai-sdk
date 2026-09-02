# OpenAI-compatible APIs

Use this provider for servers implementing the OpenAI Chat Completions shape at
`/v1/chat/completions`, including vLLM, LM Studio, Kimi/Moonshot, and similar
hosted or local endpoints.

The [OpenAI provider](openai.md) supports OpenAI's Responses API.

## Install

```bash
go get github.com/grafana/ai-sdk/providers/openai-compatible
```

## Connect to a server

```go
model := openaicompatible.New(
	"moonshotai/Kimi-K2-Instruct",
	openaicompatible.WithProviderName("vllm"),
	openaicompatible.WithBaseURL("http://localhost:8000/v1"),
)
```

Add `WithAPIKey` when the endpoint requires bearer authentication. It is
optional so local servers do not need dummy credentials.

The base URL normally includes `/v1`; the provider adds the Chat Completions
path. Set a stable provider name so logs and metrics identify the actual
service. Continue with [Generate text from Go](../getting-started/backend-only.md)
or [Full-stack chat](../getting-started/full-stack-chat.md).

## Enable optional compatibility features

Compatible servers vary beyond the common request shape. Enable features only
after verifying the target server:

```go
model := openaicompatible.New(
	modelID,
	openaicompatible.WithBaseURL(baseURL),
	openaicompatible.WithAPIKey(apiKey),
	openaicompatible.WithIncludeUsage(true),
	openaicompatible.WithStructuredOutputs(true),
)
```

Some servers reject streaming `stream_options`; usage metadata is therefore
opt-in. JSON-schema response formats are also opt-in because not every server
implements them correctly.

## Adapt small request differences

Use static headers and query parameters for service configuration. Keep
`WithRequestTransform` focused on small, well-tested JSON shape differences. A
server with substantially different messages, streaming events, or tool
semantics needs a dedicated provider.

Per-call `OpenAIOptions` cover compatible reasoning and schema settings where
the server supports them. Unsupported values may be ignored or rejected by the
server, so include the target implementation in integration tests.

## Reference

- [`providers/openai-compatible`](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/openai-compatible)
- [`OpenAIOptions`](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/openai-compatible#OpenAIOptions)

---

← [OpenAI](openai.md) · [Docs index](../README.md) · [Writing a provider →](writing-a-provider.md)
