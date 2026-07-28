# Examples

Complete programs for making a model call, streaming a response, adding tools,
generating typed data, and serving React chat. Start with the result you want
and run each example with an Anthropic API key.

## Learning path

| # | Example | What you will build |
|---|---|---|
| 1 | [generate-text](generate-text) | Make one model call and print the complete response |
| 2 | [streaming-cli](streaming-cli) | Print text as it arrives and inspect stream events and usage |
| 3 | [tools-agent](tools-agent) | Let a model call typed Go functions across several steps |
| 4 | [structured-output](structured-output) | Convert model output into a validated Go value |
| 5 | [chat-server](chat-server) | Serve a streaming endpoint for `@ai-sdk/react` `useChat` |

```text
generate-text      make the first call
      │
streaming-cli      receive output as it arrives
      │
tools-agent        let the model call Go functions
      │
structured-output  return validated data
      │
chat-server        connect a React frontend
```

## Run the examples

Make one complete model call:

```bash
(cd examples/generate-text && \
  ANTHROPIC_API_KEY=sk-... go run . "Explain channels in one sentence.")
```

Stream text in the terminal:

```bash
(cd examples/streaming-cli && \
  ANTHROPIC_API_KEY=sk-... go run . "Explain channels in two sentences.")
```

Run a multi-step tool workflow:

```bash
(cd examples/tools-agent && \
  ANTHROPIC_API_KEY=sk-... go run . "Weather in Tokyo in Fahrenheit, plus 10?")
```

Generate a typed object:

```bash
(cd examples/structured-output && ANTHROPIC_API_KEY=sk-... go run .)
```

Start the chat backend:

```bash
(cd examples/chat-server && ANTHROPIC_API_KEY=sk-... go run .)
```

Connect the React client from the
[full-stack chat guide](../docs/getting-started/full-stack-chat.md), or inspect
the stream directly:

```bash
curl -N http://localhost:8080/api/chat \
  -H 'content-type: application/json' \
  -d '{"messages":[{"id":"user-1","role":"user","parts":[{"type":"text","text":"Explain goroutines briefly."}]}]}'
```

Credentials are needed only when running examples. Every example builds without
credentials.
