# Messages and conversation history

The SDK has two message types because browser UIs and model APIs need different
representations. Choose the type at the boundary you are working with; the SDK
handles the common conversion path.

## Choose the message type

### `UIMessage`

Use `UIMessage` for chat history that crosses the frontend boundary. It is
parts-based and represents the supported AI SDK UI protocol parts, including
text, reasoning, tools, files, sources, typed data, and step boundaries.

A `useChat` endpoint normally decodes `[]UIMessage` and passes it directly with
`WithMessages`:

```go
result := aisdk.StreamText(ctx, model,
	aisdk.WithMessages(messages...),
)
```

### `provider.Message`

Use `provider.Message` for Go-only calls or when you are constructing model
input yourself:

```go
result, err := aisdk.GenerateText(ctx, model,
	aisdk.WithSystem("You are a concise assistant."),
	aisdk.WithModelMessages(
		provider.UserText("Summarize this incident."),
	),
)
```

Provider messages are closer to the role and content structure expected by LLM
APIs. Do not convert frontend JSON into provider messages by hand.

## Convert only when you need control

`WithMessages` performs UI-to-model conversion automatically. Call
`ConvertToModelMessages` explicitly when application policy must inspect,
filter, or modify the model input first:

```go
modelMessages, err := aisdk.ConvertToModelMessages(
	uiMessages,
	aisdk.WithTools(tools),
)
if err != nil {
	return err
}

result := aisdk.StreamText(ctx, model,
	aisdk.WithModelMessages(modelMessages...),
)
```

Pass the applicable tool set when persisted messages can contain completed tool
results. Conversion then applies each tool's `ToModelOutput` hook before sending
the result to the provider. Add `WithIgnoreIncompleteToolCalls` when incomplete
tool calls should be omitted during explicit conversion.

If both message options are supplied, `WithModelMessages` takes precedence.

## Persist conversations

For a React chat application, persist complete `UIMessage` values. They retain
tool state, approval decisions, reasoning, files, sources, metadata, and custom
data needed to render or resume the next request correctly.

A typical flow is:

1. Load the stored UI messages.
2. Append the new user message.
3. Pass the complete history to `StreamText` or an Agent helper.
4. Store the final assistant message after the stream finishes.

When the server owns conversation storage, pass the loaded history to the UI
stream and save the completed history in its finish callback:

```go
stream := result.ToUIMessageStream(
	aisdk.WithUIMessageStreamOriginalMessages(messages...),
	aisdk.OnUIMessageStreamFinish(func(state aisdk.UIMessageStreamOnFinishState) {
		if err := persistMessages(state.Messages); err != nil {
			reportPersistenceFailure(err)
		}
	}),
)
if err := aisdk.PipeUIMessageStreamToResponse(w, stream); err != nil {
	return err
}
```

The finish callback has no error return and may run after response output begins,
so route storage failures to the application's retry and alerting path. Validate that the
authenticated caller owns the conversation before loading or saving it. Decide
whether persisted history should retain reasoning, files, tool payloads, and
metadata under the application's data policy. Tool approvals use the same
persisted history; see [Tool approval](../guides/tool-approval.md).

## Continue Go-only model conversations

When building lower-level loops, `ToResponseMessages` converts collected model
response content into assistant and tool messages suitable for a later call. It
preserves provider information required by reasoning and tool round trips.
`StreamText` handles this conversation loop automatically for most applications.

## Reference

- [`UIMessage`](https://pkg.go.dev/github.com/grafana/ai-sdk#UIMessage)
- [`ConvertToModelMessages`](https://pkg.go.dev/github.com/grafana/ai-sdk#ConvertToModelMessages)
- [`provider.Message`](https://pkg.go.dev/github.com/grafana/ai-sdk/provider#Message)

---

← [How a request runs](architecture.md) · [Docs index](../README.md) · [UI message stream protocol →](wire-protocol.md)
