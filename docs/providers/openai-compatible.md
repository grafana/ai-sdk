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

## Control input image detail

Attach [`ImageOptions`](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/openai-compatible#ImageOptions)
to an image file part to select its native resolution. This Go extension writes
`image_url.detail` while keeping the file's URL intact. It accepts the native
`auto`, `low`, `high` and `original` modes; model support remains server-specific.
The semantic `imageDetail` option is consumed locally, rather than sent as an
extra content-part field. Raw metadata overlays retain their existing standalone
provider behavior; Gateway policy still refuses overrides of `image_url`.

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

## Preserving Chat-specific output

Set `OpenAIOptions.PreserveChatCompletionMetadata` to `true` when a protocol
codec needs refusal, log probabilities or detailed Chat token accounting. This
local option is not sent in the native request. The default remains the pinned
upstream LanguageModel behavior.

Validated fields are available as `openaiCompatible.chatCompletion` in provider
metadata. Decode that value as `ChatCompletionMetadata` and validate it before
projection. Unary results contain complete refusal/logprobs/usage values.
Streaming emits `custom` parts with kind `openai-compatible.chat-completion` for
refusal and logprob deltas, and final usage on the `finish` part. Missing counters
remain absent; unknown native fields are not copied. Existing metadata under the
configured provider namespace remains available.

This opt-in Go extension does not add multiple choices.
Its focused tests use synthetic native responses, not live provider recordings.

### Native Chat audio

`OpenAIOptions.Modalities` and `OpenAIOptions.Audio` request audio generation on
compatible endpoints that support it. `ChatCompletionVoice` accepts a voice name
or custom voice ID. Provider model availability remains native-server policy.
Use `PreserveChatCompletionMetadata` to retain the generated audio under
`chatCompletion.audio`: complete unary responses carry ID, expiration, base64 data
and transcript; streaming custom parts retain only fields supplied in each delta.
Base64 data, timestamps and text are validated before publication.

For subsequent turns, attach `ChatCompletionMessageOptions` containing an
`Audio` reference to the assistant message's `ProviderOptions`. Only the native
audio ID is sent in history, with null content for audio-only messages. This
extension deliberately preserves data omitted by the pinned upstream compatible
normalizer; ordinary requests without audio retain their existing behavior.

The opt-in Chat metadata projection also preserves native service tier, system
fingerprint and URL annotations. Citation spans must be present and nonnegative;
malformed annotations fail typed decoding instead of receiving invented indices.

### Custom text tools

A provider-defined tool with ID `openai.custom` maps to Chat's `type: custom`
declaration, including text or Lark/regex grammar format. Stream input parts use
`InputEncoding: text`; final tool calls contain the canonical JSON-encoded string
used by the existing Responses provider. Function-tool JSON inputs are unchanged.

For custom history without a current declaration, the tool-call part's
`openaiCompatible.customTool: true` metadata marks that canonical string as native
custom input. The marker is consumed locally and does not override the common
call ID or tool name. A current custom declaration supplies the same information
when the marker is absent. This is an intentional extension over pinned compatible
3.0.82; its synthetic tests do not claim live grammar-engine conformance.

Native Chat predicted output and search controls are available through typed
`OpenAIOptions.Prediction` and `WebSearchOptions`. Prediction accepts text or a
nonempty array of validated text parts, including explicit cache breakpoints;
search retains context-size and approximate-location controls. These extensions
are validated before transport and do not change requests that omit them. Model
support remains a native service decision.
