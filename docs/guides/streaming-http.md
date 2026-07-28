# Streaming over HTTP

Use the HTTP helpers to turn a model stream into a response for AI SDK frontend
hooks. They handle protocol headers, SSE framing, flushing, and stream
termination.

## Choose a response helper

| Client or use case | Helper |
|---|---|
| `useChat` or a UI message stream | `WriteUIMessageStream` |
| Reusable `Agent` behind `useChat` | `WriteAgentUIStream` |
| `useCompletion` or `useObject` | `WriteTextStream` |
| Filter or inspect UI chunks first | `ToUIMessageStream` then `PipeUIMessageStreamToResponse` |
| Consume progressive messages in Go | `StreamUIMessage` |
| Build one final message in Go | `AssembleUIMessage` |

## Serve a chat stream

```go
result := aisdk.StreamText(r.Context(), model,
	aisdk.WithMessages(messages...),
)
if err := aisdk.WriteUIMessageStream(w, result); err != nil {
	log.Printf("streaming response: %v", err)
}
```

Start streaming only after request authentication, validation, and any work that
must still be able to return a normal HTTP status. Once SSE output begins, encode
later failures in the stream; the HTTP status and response format are already
committed.

## Control client-visible content

```go
stream := result.ToUIMessageStream(
	aisdk.WithUIMessageStreamReasoning(false),
	aisdk.WithUIMessageStreamSources(true),
	aisdk.OnUIMessageStreamError(func(err error) string {
		return "The model request failed."
	}),
)
if err := aisdk.PipeUIMessageStreamToResponse(w, stream); err != nil {
	return err
}
```

Do not send internal provider errors directly to users. Decide explicitly
whether reasoning, sources, metadata, and custom data are safe and useful for
the frontend.

Pass original messages to preserve continuation IDs and collect the final
history:

```go
stream := result.ToUIMessageStream(
	aisdk.WithUIMessageStreamOriginalMessages(messages...),
	aisdk.OnUIMessageStreamFinish(func(state aisdk.UIMessageStreamOnFinishState) {
		persistMessages(state.Messages)
	}),
)
```

## Write plain text

`WriteTextStream` forwards text deltas without UI-message SSE framing. Use it
with `useCompletion` by selecting the text stream protocol:

```tsx
const completion = useCompletion({
  api: "/api/completion",
  streamProtocol: "text",
});
```

The Go endpoint writes the plain text stream:

```go
result := aisdk.StreamText(r.Context(), model, opts...)
if err := aisdk.WriteTextStream(w, result); err != nil {
	return err
}
```

`useObject` consumes streamed JSON through the same helper; see
[Structured output](structured-output.md#stream-json-to-useobject).

## Compose a stream

Use `CreateUIMessageStream` when an endpoint needs to merge model output with
application-generated chunks such as progress or typed data. The supplied
`UIMessageStreamWriter` can write chunks or merge another UI stream. Keep one
owner responsible for closing and error mapping.

## Consume streams safely

A `StreamTextResult` has one underlying stream consumer. Do not call
`FullStream`, `ToUIMessageStream`, and an HTTP writer on the same result.
Whichever path you choose must drain the stream or cancel its context.

Use `r.Context()` so client disconnects stop provider calls and tools. Configure
total, step, and chunk timeouts for stalled connections. Verify streaming in the
deployed path, including reverse proxies that may buffer SSE.

## Reference

- [`WriteUIMessageStream`](https://pkg.go.dev/github.com/grafana/ai-sdk#WriteUIMessageStream)
- [`CreateUIMessageStream`](https://pkg.go.dev/github.com/grafana/ai-sdk#CreateUIMessageStream)
- [UI message stream protocol](../concepts/wire-protocol.md)

---

← [Agent loops](agent-loops.md) · [Docs index](../README.md) · [Retry and timeout →](retry-and-timeout.md)
