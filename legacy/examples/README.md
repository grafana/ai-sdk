# Examples

The runnable examples are complete Go application outcomes rather than one
program for every SDK function. Use the README and guides for focused API
recipes such as a first `GenerateText` call or consuming `FullStream` directly.

## Choose an application

| Example | Use it when you are building |
|---|---|
| [agent-chat](agent-chat) | A Go agent backend that executes typed tools and streams UI messages to `@ai-sdk/react` `useChat` |
| [structured-extraction](structured-extraction) | A job or service that converts unstructured input into a schema-validated Go value |

Both examples use Anthropic when run normally. Their tests use deterministic
local models and require no credentials or network access.

## Run agent chat

Start the backend:

```bash
(cd examples/agent-chat && ANTHROPIC_API_KEY=sk-... go run .)
```

Connect the React client from the
[full-stack chat guide](../docs/getting-started/full-stack-chat.md), or inspect
the stream directly:

```bash
curl -N http://localhost:8080/api/chat \
  -H 'content-type: application/json' \
  -d '{"messages":[{"id":"user-1","role":"user","parts":[{"type":"text","text":"What is the weather in Paris?"}]}]}'
```

The agent can call its typed weather tool, feed the result into another model
step, and stream both tool state and the final answer to the client. The tool
returns deterministic sample data so the example stays focused on orchestration;
replace it with a real weather client in an application.

## Run structured extraction

```bash
(cd examples/structured-extraction && ANTHROPIC_API_KEY=sk-... go run .)
```

The program turns a sample alert into a validated `AlertTriage` value and prints
fields that ordinary Go application code can consume.

## Test the examples

Run every example test without credentials:

```bash
mise run test-examples
```

Build every runnable command:

```bash
mise run build-examples
```
