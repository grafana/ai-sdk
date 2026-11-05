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

## Reuse uploaded files

When your server accepts uploaded file IDs, pass a user file part with
[`ReferenceDataContent`](https://pkg.go.dev/github.com/grafana/ai-sdk/provider#ReferenceDataContent):

```go
file := provider.FilePart("application/pdf", provider.ReferenceDataContent(
	json.RawMessage(`{"custom-provider":"file-123"}`),
))
```

For a model configured with `WithProviderName("custom-provider.chat")`, the
adapter selects the exact `custom-provider` entry and sends `file.file_id`.
It uses the trimmed segment before the first dot; it does not camel-case or
otherwise normalize reference keys. The default namespace is `openai-compatible`.
A missing entry fails before the request is sent, while an explicitly empty
ID remains empty for the server to validate. Media type and filename do not
change the reference representation.

This supports user messages; it does not upload files or enable multipart tool
results. Synthetic request tests cover conversion, but each target server must
support the emitted file-ID format.

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
