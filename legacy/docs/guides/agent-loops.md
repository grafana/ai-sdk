# Agent loops

An agent loop lets a model call tools, receive their results, and continue until
it can answer or a stop condition ends the run.

Use direct `StreamText` or `GenerateText` when one call owns its configuration.
Use `ToolLoopAgent` when multiple handlers, jobs, or tools should share the same
model, instructions, tools, and callbacks.

## Add multiple steps to a call

Direct generation defaults to one model step. Opt into a bounded loop:

```go
result := aisdk.StreamText(ctx, model,
	aisdk.WithSystem("Use tools when needed; do not invent tool results."),
	aisdk.WithModelMessages(provider.UserText(prompt)),
	aisdk.WithTools(tools),
	aisdk.WithStopWhen(aisdk.StepCountIs(5)),
)
```

A step limit is a safety boundary, not a quality target. Choose the smallest
limit that supports the workflow. You can also stop when a named tool is called
or supply another `StopCondition`.

Use `WithPrepareStep` when later steps need different active tools, messages,
model settings, or runtime context. Keep ordinary loops simple; step preparation
is most useful for staged workflows and policy enforcement.

## Create a reusable agent

```go
agent := aisdk.NewToolLoopAgent(model,
	aisdk.WithToolLoopAgentID("incident-assistant"),
	aisdk.WithToolLoopAgentOptions(
		aisdk.WithInstructions("Help operators investigate incidents."),
		aisdk.WithTools(tools),
		aisdk.WithStopWhen(aisdk.StepCountIs(8)),
	),
)

result, err := agent.Generate(ctx,
	aisdk.WithAgentPrompt("Investigate the payment API alert."),
)
```

Reusable settings apply to every call. Per-call options can add messages,
headers, callbacks, and runtime context. An Agent defaults to a 20-step limit
when no stop condition is configured; set an explicit lower bound for your
application.

Runtime context reaches tools through `ToolExecutionOptions.Context`. Use it for
request-scoped application state such as tenant services or authorization
objects, not as a replacement for `context.Context` cancellation.

## Stream an agent to `useChat`

```go
if err := aisdk.WriteAgentUIStream(
	w,
	r.Context(),
	agent,
	body.Messages,
); err != nil {
	log.Printf("streaming agent response: %v", err)
}
```

The helper validates and converts UI message history, runs the Agent, and writes
the UI message SSE stream. The [full-stack chat guide](../getting-started/full-stack-chat.md)
shows the corresponding direct `StreamText` endpoint.

## Design bounded agents

- Give every loop an explicit stop condition.
- Keep tools narrow, validated, and cancellable.
- Require approval for consequential actions.
- Treat tool errors as expected workflow outcomes when recovery is possible.
- Observe step count, provider calls, latency, and token usage.
- Scope agent credentials to each tool's approved operations.

A loop can produce several provider calls, and retries or fallback can multiply
that number. Set total and per-step timeouts accordingly.

## Reference

- [`ToolLoopAgent`](https://pkg.go.dev/github.com/grafana/ai-sdk#ToolLoopAgent)
- [`StopCondition`](https://pkg.go.dev/github.com/grafana/ai-sdk#StopCondition)
- [`WithPrepareStep`](https://pkg.go.dev/github.com/grafana/ai-sdk#WithPrepareStep)

---

← [Structured output](structured-output.md) · [Docs index](../README.md) · [Streaming over HTTP →](streaming-http.md)
