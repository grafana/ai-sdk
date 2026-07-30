# Generate text from Go

Starting from the model created in [Installation](installation.md), use
`GenerateText` when application code needs one complete result and `StreamText`
when a caller should receive text as it arrives. Both accept the same
instructions, messages, tools, provider options, and reliability settings.

A system instruction defines how the model should behave. A user message carries
the request to answer.

## Generate a complete response

`GenerateText` blocks until the operation finishes:

```go
result, err := aisdk.GenerateText(ctx, model,
	aisdk.WithSystem("You write concise incident summaries."),
	aisdk.WithModelMessages(
		provider.UserText("Summarize the payment API incident."),
	),
)
if err != nil {
	return err
}

fmt.Println(result.Text)
fmt.Println(result.TotalUsage)
```

Use this path for background jobs, API endpoints that return one JSON response,
classification pipelines, and other work that does not need incremental output.
The result also exposes tool calls, sources, files, warnings, finish reason, and
per-step information.

## Stream text as it arrives

`StreamText` returns immediately. Consume its stream exactly once:

```go
result := aisdk.StreamText(ctx, model,
	aisdk.WithModelMessages(provider.UserText("Explain this stack trace.")),
)

for part := range result.FullStream() {
	switch part := part.(type) {
	case aisdk.StreamTextDelta:
		fmt.Print(part.Text)
	case aisdk.StreamError:
		log.Printf("model stream: %v", part.Error)
	}
}
if err := result.Err(); err != nil {
	return err
}
```

Reading `FullStream`, converting with `ToUIMessageStream`, or passing the result
to an HTTP writer all consume the same underlying stream. Choose one consumer.
Blocking accessors such as `Text()` and `TotalUsage()` are populated after the
stream finishes.

The loop above is the complete terminal-consumption pattern. For larger
application outcomes, continue to the runnable [examples](../../examples/).

## Choose the next capability

- Need typed JSON? Use [structured output](../guides/structured-output.md).
- Need the model to call application code? Add [tools](../guides/tools.md).
- Need several model/tool turns? Configure an [agent loop](../guides/agent-loops.md).
- Need a React client? Build a [full-stack chat](full-stack-chat.md).

For exact result fields and options, see
[`GenerateText`](https://pkg.go.dev/github.com/grafana/ai-sdk#GenerateText) and
[`StreamText`](https://pkg.go.dev/github.com/grafana/ai-sdk#StreamText).

---

← [Installation](installation.md) · [Docs index](../README.md) · [Full-stack chat →](full-stack-chat.md)
