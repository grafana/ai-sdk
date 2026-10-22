# Anthropic Messages API

The Gateway accepts authenticated Anthropic Messages requests at
`POST /v1/messages`. It resolves the public `model` value through the configured
catalog and invokes the selected provider model. Client credentials are checked
by the Gateway's configured authenticator and are never sent to the provider.

## Supported request subset

The initial adapter supports:

- A model name, required `max_tokens`, optional system text, and ordered user
  and assistant text history.
- Function tools with JSON input schemas, tool choice, assistant `tool_use`
  blocks, and user `tool_result` blocks. Tools run in the client; the Gateway
  passes calls and results through without executing them.
- `temperature`, `top_p`, `top_k`, and `stop_sequences` where the selected
  provider supports them.
- Unary JSON replies and `stream: true` incremental Anthropic SSE events.
- Provider-reported stop reasons and token usage. If a provider does not report
  a token count, the response uses zero for that count.
- `disable_parallel_tool_use` is passed through the existing Anthropic provider
  option when supplied with `tool_choice`.

Non-text blocks, server-side tools, thinking/signature replay, cache controls,
and other requests that cannot be represented by the canonical provider call
are rejected with a fixed `invalid_request_error`. Unknown additive JSON fields
are ignored using Go's ordinary JSON decoding behavior. Unsupported known
fields are rejected rather than silently removed.

Provider-specific request features depend on the configured provider and its
canonical options. The current adapter does not claim complete Anthropic API or
Claude Code compatibility. The Gateway does not expose `/v1/messages/count_tokens`.
The canonical provider contract carries a stop reason but not the matched stop
sequence text, so `stop_sequence` may be returned with a null `stop_sequence`
value.

## Client authentication

The official Anthropic Go and TypeScript SDKs send `x-api-key` by default; both
also support bearer authentication. On the private listener, supply a Gateway
JWT or configured static key as the SDK API key or bearer token. Both forms use
the same configured Gateway verifier. Conflicting credentials are rejected.
Provider credentials remain in server-side Gateway configuration.

The endpoint requires configured-account access and is available only on the
private listener. Cloud BYOK and operational listeners do not expose it. See the
[authentication guide](../../docs/guides/gateway-authentication.md) for private
JWT, static-key, and development-only unsafe verification setup.

The endpoint uses the same model catalog, provider execution, request telemetry,
and lifecycle as the Gateway's other protected routes. The adapter does not add
its own byte or item budgets. The shared server currently enforces request read
timeouts and header limits, but does not yet enforce shared request-body or
unary response byte limits for this route. Those shared resource limits are
tracked in [issue #394](https://github.com/grafana/ai-sdk/issues/394).

## Related guides

- [Model catalog](model-catalog.md)
- [Cloud authentication](cloud-authentication.md)
- [Gateway client guide](../../docs/providers/grafana-gateway.md)

---

← [AI Gateway](../README.md) · [SDK documentation](../../docs/README.md)
