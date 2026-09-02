# Documentation

Build with the Grafana AI SDK for Go by starting with the result you want. Add
streaming, tools, reliability, observability, and platform infrastructure as the
application grows.

## Start building

1. [Install the SDK](getting-started/installation.md) from an empty Go project
   and make a verified model call.
2. [Generate text from Go](getting-started/backend-only.md) when a service, job,
   or command needs a model response.
3. Continue with the application you are building:
   - [Full-stack chat](getting-started/full-stack-chat.md) for a streaming React
     client and Go backend.
   - [Structured output](guides/structured-output.md) for validated Go values.
   - [Tools](guides/tools.md) when a model needs live data or application actions.

## Build application workflows

### Stream and manage conversations

- [Streaming over HTTP](guides/streaming-http.md) — choose the response helper
  for `useChat`, `useCompletion`, `useObject`, or a Go consumer.
- [Messages and conversation history](concepts/messages.md) — choose the message
  representation to send, render, and persist.
- [UI message stream protocol](concepts/wire-protocol.md) — inspect or customize
  the browser stream after the standard HTTP helper is insufficient.

### Add tools and agents

- [Tools](guides/tools.md) — expose narrow Go functions or external actions to a
  model.
- [Tool approval](guides/tool-approval.md) — pause consequential actions for a
  user or policy decision.
- [Agent loops](guides/agent-loops.md) — continue through model and tool steps
  until the task finishes or a stop condition is reached.

### Return typed data

- [Structured output](guides/structured-output.md) — generate schema-validated
  objects, arrays, choices, or JSON.

### Add reliability

- [Retry and timeout](guides/retry-and-timeout.md) — bound latency and recover
  from brief provider failures.
- [Fallback and registry](guides/fallback-and-registry.md) — try backup models or
  select configured models by ID.
- [Testing model-backed code](guides/testing.md) — test applications with
  deterministic models and focused integration coverage.

## Add logging, metrics, and policy

Start with the [middleware overview](middleware/overview.md) to choose where a
cross-cutting behavior belongs.

- [Structured logging](middleware/structured-logging.md) — record provider-call
  lifecycle, latency, outcome, and optional bounded content.
- [Prometheus metrics](middleware/prometheus.md) — collect local call volume,
  failures, latency, stream timing, and token usage.
- [Context enrichment](middleware/context-enrichment.md) — attach an approved set
  of request metadata for a provider or gateway.
- [Agent Observability](middleware/agent-observability.md) — investigate model and
  agent behavior in Grafana and apply optional preflight policy.

## Choose a model service

Start with [Choose a provider](providers/overview.md), then follow the setup for
the service your application calls:

- [Anthropic](providers/anthropic.md) for Claude through Anthropic or Vertex AI.
- [Amazon Bedrock](providers/bedrock.md) for models through Bedrock Converse.
- [OpenAI](providers/openai.md) for the Responses API.
- [OpenAI-compatible APIs](providers/openai-compatible.md) for local or hosted
  Chat Completions-compatible servers.

## Extend model infrastructure

- [Writing a provider](providers/writing-a-provider.md) — adapt another model
  service to the common SDK behavior.

## Operate in production

- [Production checklist](best-practices/production.md) — bound work, secure the
  request boundary, and verify the deployed streaming path.
- [Error handling](best-practices/error-handling.md) — handle failures before,
  during, and after streaming.
- [Security](best-practices/security.md) — constrain model authority, tools,
  credentials, content, and resource use.

## Understand or debug the SDK

- [How a request runs](concepts/architecture.md) — follow generation, streaming,
  tools, and result collection through one request.
- [Providers and models](concepts/providers.md) — understand what stays common
  and what varies between model services.
- [Messages and conversation history](concepts/messages.md) — understand browser
  and provider message representations.
- [UI message stream protocol](concepts/wire-protocol.md) — inspect frontend
  chunks, Server-Sent Event framing, and custom data.

## Runnable examples

- [agent-chat](../examples/agent-chat) — run a typed-tool agent behind a
  `useChat`-compatible Go endpoint.
- [structured-extraction](../examples/structured-extraction) — turn an alert
  into a validated Go value.

See the [examples index](../examples/README.md) for run and test commands.

---

[Project overview](../README.md) · [Examples](../examples/) · [API reference](https://pkg.go.dev/github.com/grafana/ai-sdk)
