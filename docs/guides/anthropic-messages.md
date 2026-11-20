# Use Anthropic clients and Claude Code with AI Gateway

The Gateway's Messages adapter lets an Anthropic client call a configured
Gateway model. The adapter serves text, client-executed tools, tool-result
continuations and streaming. It accepts, but does not yet apply, the fields that
tune only the Anthropic wire, such as thinking and cache control. The
[limitations](#what-is-accepted-and-not-applied) section lists each one.
Do not treat the route as Claude Code compatible until those fields are applied
and a compatibility pass is recorded.

## Base URL and credentials

Use a stored catalog's API prefix as the Anthropic base URL. The client appends
`/v1/messages`:

```text
https://<grafana-host>/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/<namespace>/modelcatalogs/<catalog>
```

Send a Grafana service-account token as a **bearer token**. The Grafana platform
authenticates the request before the plugin runs and rejects `x-api-key` with
HTTP 403, so an Anthropic client's default API-key option does not work. Use the
client's bearer or auth-token option instead. The token is never used as, or
forwarded as, a provider credential; provider keys come from the Gateway's
configuration. Catalog callers need `get` access to the catalog and its
`messages` subresource. The `aigateway:modelinvoker` role, bound to Editor,
grants both. Calls use only the named catalog's models and keys.

For the operator-configured catalog, omit `/modelcatalogs/<catalog>` from the
base URL. That namespace route keeps the Cloud restrictions on user and
service-account callers described in the
[Chat Completions guide](openai-chat-completions.md#access-and-supported-behavior).
See [Gateway authentication](gateway-authentication.md) for the credential
contract.

The `model` field takes a catalog model ID or alias. The response reports the
canonical ID.

## Claude Code

```bash
export ANTHROPIC_BASE_URL="https://<grafana-host>/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/<namespace>/modelcatalogs/<catalog>"
export ANTHROPIC_AUTH_TOKEN="<grafana service-account token>"
export ANTHROPIC_MODEL="<catalog alias>"
claude
```

Claude Code also sends requests for its smaller background model. Point that at a
catalog alias too (Claude Code documents `ANTHROPIC_DEFAULT_HAIKU_MODEL` for it),
or those calls fail with a 404 for an unknown model. Claude Code appends
`?beta=true` and sends `anthropic-beta` headers; the adapter accepts both and
ignores them.

Evidence so far: Claude Code 2.1.326 completed a multi-turn read-and-edit task
through a local Gateway against a live Anthropic model, with every
`POST v1/messages` call returning 200. That was one manual session, not an
automated test.

## Official Go client

```go
client := anthropic.NewClient(
    option.WithBaseURL(baseURL),
    option.WithAuthToken(grafanaToken),
    option.WithMaxRetries(0),
)
msg, err := client.Messages.New(ctx, anthropic.MessageNewParams{
    Model:     "configured-model-alias",
    MaxTokens: 1024,
    Messages: []anthropic.MessageParam{
        anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
    },
})
```

Retries are disabled so callers can tell one client request from the Gateway's
configured fallback attempts.

## What is supported

- `model`, `max_tokens` (required, positive) and `messages`.
- System text as a string or text blocks. A `system`-role message after the first
  message stays at its position. Each nonempty system text block becomes a
  separate system message.
- User and assistant turns as a string or content blocks: `text`, `tool_use` and
  `tool_result`. Multiple text blocks within a tool result are joined with
  newlines.
- Client function tools, `tool_choice` (`auto`, `any`, `tool`, `none`),
  `stop_sequences`, `temperature`, `top_p`, `top_k` and `stream`.
- `disable_parallel_tool_use`, applied per target: Anthropic's own option, and the
  inverse of `parallelToolCalls` or `parallel_tool_calls` for OpenAI and
  OpenAI-compatible targets.
- `output_config.effort` (`low`, `medium`, `high`, `xhigh`, `max`), forwarded
  unchanged only to Anthropic targets. It does not enable thinking. OpenAI
  Responses and OpenAI-compatible targets ignore this inbound field and retain
  any existing provider effort option, including on fallback attempts.
  Anthropic models can reject unsupported effort levels with HTTP 400; that
  non-retryable error stops fallback. Choose a model that supports the requested
  level. Cross-provider effort handling is deferred to
  [#427](https://github.com/grafana/ai-sdk/issues/427).
- Unary messages and SSE with text and tool-use blocks, stop reasons and
  provider-reported usage, including cache read and creation tokens. Parallel
  tool inputs keep separate block indexes, including interleaved arguments.
- Stop reasons: `end_turn`, `tool_use`, `max_tokens`, `pause_turn`,
  `stop_sequence` and `refusal`. Provider content-filter finishes map to
  `refusal` and complete normally, without an upstream error.
- OpenAI and OpenAI-compatible targets receive `store: false`, including on
  fallback attempts.

The Gateway runs no tools. It returns `tool_use` blocks and the client executes
them.

## What is accepted and not applied

The adapter accepts these and does nothing with them. The request succeeds; the
provider call does not carry them:

- `cache_control` on the request, system blocks, content blocks and tools:
  ignored. No cache breakpoints reach the provider.
- `thinking`: ignored. The model is not asked to think.
- `thinking` and `redacted_thinking` blocks in history: dropped.
- `context_management`: ignored.
- `output_config` fields other than `effort`: ignored.
- `metadata`, including `user_id`, and `service_tier`: ignored.
- `anthropic-beta`, `anthropic-version` and `?beta=true`: accepted, no effect.

Responses never contain thinking blocks. `stop_sequence` is reported as `null`
even when a stop sequence ended the turn.

## What is refused

These fail with a fixed `invalid_request_error` before any model is called:
image and document blocks, server or built-in tools, and unknown content block
types or tool-choice types. `POST v1/messages/count_tokens` is not served, and
neither are model listing or batch endpoints.

## Errors

Failures return `{"type":"error","error":{"type":...,"message":...}}` with a
fixed message and no provider text. A deadline during stream setup returns
HTTP 504 with `timeout_error`. A failure after a stream has started ends the
stream with an `error` event and no `message_stop`. Failures the platform
produces before the plugin runs, such as a rejected `x-api-key`, use the
platform's own format.

---

[Prev: Use official OpenAI clients](openai-chat-completions.md) · [Docs index](../README.md) · [Next: Gateway architecture](../concepts/gateway-architecture.md)
