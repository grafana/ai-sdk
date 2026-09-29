# Use provider-defined tools through Grafana AI Gateway

A direct Gateway model route can send provider-defined tools to the model and
return their calls and results to your application. Configure the tools in your
SDK request as you would for a direct provider call. The Gateway does not run
application tools or keep a conversation between requests.

See [Tools](../../docs/guides/tools.md) for the SDK tool lifecycle and
[Call Grafana AI Gateway from Go](../../docs/providers/grafana-gateway.md) for
client setup.

## Decide who executes a call

A provider-tool definition contains an ID, name, and JSON object of arguments.
It cannot contain function-tool fields such as an input schema or definition-level
provider options. Direct Go callers can leave arguments nil for an empty object;
a serialized Gateway request must contain `args: {}`.

Check `ProviderExecuted` on each returned call. When true, the provider owns
execution; do not run the tool in your application. When false, your application
handles the call as an ordinary tool. The definition's type does not determine
who executes a particular call.

## Continue after a provider-owned call

A provider-owned call may finish a response without a result. To continue, send
the unresolved assistant call in the next request's conversation history. Keep
its ID, name, input, execution marker, and returned provider metadata intact.
The provider can return the matching result in that later response without
repeating the call. The Gateway does not correlate calls across requests unless
you supply their history.

In a stream, a preliminary result is only a preview. Wait for its final result
before treating the tool as complete. A completed provider-owned call can also
have no result yet; that is different from a preliminary result awaiting its
final value.

## Supported routes and limits

Provider tools and their history work on direct model routes. Fallback routes
reject tool definitions, tool choices, and tool-call/result history before
calling a provider. Ordinary provider options follow the configured route's
policy; reserved Gateway controls and fields that override a validated tool
call are rejected.

Anthropic-hosted MCP server configuration and MCP continuation are not supported
by this route. Tool approvals, sources, generated media, and image previews
emitted before a tool call are also unsupported. File inputs and supported
file entries in application tool-result history remain available on direct
routes.

Gateway telemetry records tool-bearing requests without exporting tool names,
IDs, inputs, results, or provider metadata. Your application still receives the
supported tool data needed for execution and continuation; validate and handle
it as untrusted input. See [Text observability](text-observability.md) for
telemetry and privacy details.

---

← [AI Gateway](../README.md) · [SDK documentation](../../docs/README.md)
