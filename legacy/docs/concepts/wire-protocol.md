# UI message stream protocol

The UI message stream protocol is the contract between a Go endpoint and AI SDK
frontend hooks. It sends typed `UIMessageChunk` objects as Server-Sent Events
and ends with `data: [DONE]`.

Most endpoints can use `WriteUIMessageStream` without handling frames directly.
Work with individual chunks when filtering a stream, adding custom data, or
debugging frontend interoperability.

## Use the HTTP helper

For `useChat`, convert a `StreamTextResult` with `WriteUIMessageStream`:

```go
result := aisdk.StreamText(r.Context(), model,
	aisdk.WithMessages(messages...),
)
if err := aisdk.WriteUIMessageStream(w, result); err != nil {
	return err
}
```

The helper sets response headers, translates orchestration events, flushes SSE
frames, and writes the completion sentinel.

A simplified response looks like:

```text
data: {"type":"start","messageId":"msg_123"}

data: {"type":"text-start","id":"text_1"}

data: {"type":"text-delta","id":"text_1","delta":"Hello"}

data: {"type":"text-end","id":"text_1"}

data: {"type":"finish"}

data: [DONE]
```

Text, reasoning, tool calls, approvals, sources, files, metadata, errors, and
custom data each have typed chunk variants. See the
[`UIMessageChunk` reference](https://pkg.go.dev/github.com/grafana/ai-sdk#UIMessageChunk)
for the complete schema.

## Filter what reaches the client

Call `ToUIMessageStream` when you need stream options before writing the
response:

```go
stream := result.ToUIMessageStream(
	aisdk.WithUIMessageStreamReasoning(false),
	aisdk.WithUIMessageStreamSources(true),
)
if err := aisdk.PipeUIMessageStreamToResponse(w, stream); err != nil {
	return err
}
```

Reasoning is sent by default; sources are opt-in. Decide deliberately whether
those values are appropriate for the user and application.

## Compose custom UI streams

`CreateUIMessageStream` and `UIMessageStreamWriter` let a server merge model
output with custom data chunks. Use them for application progress, status, or
other typed UI state that belongs in the same stream. Keep business data typed
and avoid exposing secrets or internal errors.

For server-side consumers, `StreamUIMessage` produces progressive message
snapshots and `AssembleUIMessage` returns one final message.

## Distinguish UI streams from provider wire

The UI message stream connects application endpoints to browser hooks such as
`useChat` and ends with the `[DONE]` sentinel. `gateway/providerwire` transports
`provider.LanguageModel` calls between services with provider-level framing.

Frontend compatibility follows the upstream
[UI message stream protocol](https://ai-sdk.dev/docs/ai-sdk-ui/stream-protocol).

---

← [Messages](messages.md) · [Docs index](../README.md) · [Providers and models →](providers.md)
