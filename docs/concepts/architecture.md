# How a request runs

Most applications only need one mental model: give a model instructions and
messages, then choose whether to wait for the final result or consume a stream.
The SDK handles provider calls, retries, tool steps, and result aggregation.

## The application flow

```text
application input
      │
      ▼
GenerateText or StreamText
      │
      ├─ calls the selected LanguageModel
      ├─ executes configured local tools
      ├─ continues while a stop condition allows another step
      └─ collects text, usage, sources, files, warnings, and metadata
      │
      ▼
complete result, Go event stream, or UI message stream
```

Use `GenerateText` when the caller can wait for one complete response. Use
`StreamText` when a terminal, server, or browser should receive output while the
model is producing it. Both use the same orchestration engine and options.

## Steps and tools

A step is one model call. Without local tool execution, most requests finish in
one step. With tools, a request can continue:

1. The model asks to call a tool.
2. The SDK validates and executes tools that have an `Execute` function.
3. Tool results become input to the next model call.
4. The loop stops when a stop condition matches or external action is required.

Direct `StreamText` and `GenerateText` calls default to one step. Configure a
stop condition for multi-step behavior, or use `ToolLoopAgent` when the same
model, instructions, and tools should be reused across calls.

See [Tools](../guides/tools.md) and [Agent loops](../guides/agent-loops.md).

## Streaming to React

The orchestration stream contains typed Go events. `WriteUIMessageStream`
converts those events into the UI message stream protocol expected by
`@ai-sdk/react`:

```text
provider output → orchestration events → UI message chunks → SSE → useChat
```

Application code normally uses the HTTP helper to translate events and format
SSE. See [Streaming over HTTP](../guides/streaming-http.md).

## Add capabilities as the application grows

Most application code starts with the root `aisdk` package and one model from a
provider package. Add focused packages when the application needs them:

- `output` and `schema` generate validated Go values;
- `fallback` tries backup models after eligible failures;
- `registry` and `gateway/catalog` select models by configured or public IDs;
- `middleware` adds defaults, logging, metrics, metadata, or policy to model
  calls.

## When internals matter

You need the lower-level event and wire representations when you are:

- filtering or composing UI streams;
- writing a provider;
- serving remote models through provider wire;
- debugging frontend protocol compatibility.

For those cases, continue with [Messages](messages.md),
[UI message stream protocol](wire-protocol.md), or
[Writing a provider](../providers/writing-a-provider.md).

---

← [Understand or debug the SDK](../README.md#understand-or-debug-the-sdk) · [Docs index](../README.md) · [Messages →](messages.md)
