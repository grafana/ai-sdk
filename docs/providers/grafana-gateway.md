# Call Grafana AI Gateway from Go

Use the Grafana provider when your server calls Grafana AI Gateway. It uses the same generation, streaming, and registry
interfaces as other Go providers. Install the separate client module:

```bash
go get github.com/grafana/ai-sdk/providers/grafana
```

The client module is Apache-2.0 and does not depend on the Gateway service
module. Its executable response family includes text, function calls, and
bounded provider-defined tool calls/results in unary and streaming modes. Requests preserve
representable provider options, files, tools, and structured-output settings;
the deployed Gateway decides which capabilities it can execute and returns a
public invalid-request error for unsupported calls.

## Function tools

Direct routes support unary client-executed function tools. Definitions preserve
`strict: false`, examples, object schemas, and ordinary provider options;
Gateway-reserved option namespaces remain rejected. History accepts assistant
calls and text, JSON (including null), error-text, error-JSON, and text-only
content results, preserving required selected empty values. The application
executes tools and supplies call/result history on a later independent request.
The Gateway never executes application tools. Direct routes also support
provider-defined tools and provider-executed results. A provider-tool definition
has only a registered ID, name and object args; function-only fields, including
provider options on the definition, are rejected. Direct Go callers may leave
provider args nil for an empty object; the HTTP request still requires `args: {}`.
`providerExecuted: true` on a returned call means the client must not run the
application tool. A provider-defined tool can also return a client-executed call;
its definition alone does not determine execution ownership.

Streaming direct routes support input start/delta/end, calls, non-null JSON
results and preliminary results after a correlated call followed by a final
result. Early image previews emitted before a tool call remain deferred with
generated media (WP16). IDs, ordering and empty deltas are preserved. A provider-executed call can finish without a result;
its result may arrive on a later independent HTTP request when the client sends
the unresolved call in assistant history. Tool-part provider options carry
reviewed continuation metadata; response metadata is allowlisted, not arbitrary
provider passthrough. Tool approvals, file/source and other media output,
structured output, and general root provider options remain unsupported.
Vercel and Go clients own multi-step orchestration; each Gateway generation
remains stateless. Ordered fallback routes reject tool definitions, choices,
and history before any physical invocation. Logical
telemetry omits tool-bearing definitions, names, IDs, inputs, outputs and
provider metadata before export.

Anthropic-hosted MCP is not enabled by provider-tool support. Nonempty root
provider options, including `providerOptions.anthropic.mcpServers`, and MCP
continuation metadata remain rejected. The separate `gateway-anthropic-mcp`
change owns that capability and its routing and privacy controls. See the
[Gateway operator guide](../../ai-gateway/docs/provider-tools.md).

## Authenticate the client

Choose the constructor for your Gateway URL. Use `NewWithCloudCredentials`
with your stack ID and CAP token for the public Grafana Cloud URL. If your
deployment provides a separate JWT-enabled Gateway URL, use
`NewWithTokenExchange` or `NewWithAccessToken` instead.

See [Authenticate to Grafana AI Gateway](../guides/gateway-authentication.md)
for Go and Vercel examples, policy scopes, and URL selection. All constructors
reject redirects and do not discover credentials or endpoints from the
environment.

## Discover and select public models

`client.ListModels(ctx)` returns public model IDs, names, optional descriptions,
and their specification identifiers. Aliases remain ordinary independent rows
in server order. The client neither caches this catalog nor infers backend or
fallback topology. A malformed or oversized response returns no partial catalog.

Use the returned ID with `client.LanguageModel(id)`, or register the client as a
`registry.Provider`; see [Fallback and registry](../guides/fallback-and-registry.md).

## Bound work and handle errors

Operators can configure ordered text fallback using direct provider references:

```yaml
models:
  grafana/assistant:
    name: Assistant
    primary:
      provider: anthropic-primary
      model: claude-sonnet-4-6
    fallback:
      - provider: anthropic-secondary
        model: claude-sonnet-4-6
```

Both provider instances must be declared in `providers` using the existing
environment-variable credential references. Omitting `fallback` creates a direct
route; removing it restores direct routing without changing the public model ID.
Candidates retain configuration order and each new call starts at primary.
Fallback routes accept text only: tools, tool choice, tool-call/result history,
and other effectful content are rejected before any candidate runs. A stream's
first part commits its candidate, including an error part. No later failure
restarts on another provider. Client retries can multiply physical attempts;
the Gateway disables native-provider retries.

Private physical attribution writes newline-delimited `gateway_physical_attempt`
records to the existing operator stderr destination through a separate bounded
worker. Records contain the logical correlation ID when available, candidate
index, configured provider instance/backend model, start/decision timestamps,
selected/failed/canceled outcome, decision-time fallback intent, and winner.
They exclude payloads, credentials, headers, endpoint URLs, and raw errors.
These private records are separate from the canonical logical generation export.

The worker has a 256-record queue, 100 ms write deadline, 4096-byte record limit,
and one-second shutdown budget. It supports Linux stderr sockets and pipes,
plus sockets already configured nonblocking on other Unix platforms. A blocking
macOS socket is rejected because its send operation can ignore the per-call
nonblocking flag. The sink never changes the shared stderr descriptor flags;
unsupported destinations (including ordinary files or terminals) disable this
output while calls continue. Queue saturation and output failures drop records
and increment `grafana_ai_gateway_physical_attempt_dropped_total` with a closed
class label. Operators must verify their runtime stderr transport and private
log access policy before activation. Production enablement and rollback smoke
remain WP10 work; local fallback tests do not establish deployment acceptance.
FIFO deadline tests run only on Linux, matching the production output policy;
macOS tests verify nonblocking sockets, rejection of blocking sockets without
descriptor mutation, and portable queue/worker bounds.

Use a cancelable context for each generation or stream. Cancel it when a
consumer stops reading. The client closes its response body and stream channel
on cancellation. It does not replay Gateway requests; SDK retry and fallback
orchestration remain the caller's choice. Authlib may retry its token-exchange
request according to its own policy.

`grafana.DefaultLimits()` provides byte bounds for discovery, unary responses,
errors, and streams, plus stream event-size and event-count bounds. Copy that
value, adjust the required limits, and pass its address in the constructor.
Explicit limits must all be positive. Total stream bytes include wire framing;
event bytes count a complete event with CRLF normalized to one line ending.
Call contexts and HTTP client timeouts set latency bounds.

Use `errors.As` with `*grafana.GatewayError` for the public category/code and
with `*provider.APICallError` for retryability. Context cancellation and deadlines
remain identifiable with `errors.Is`. A malformed response is a non-retryable
protocol failure; a transport failure is retryable but never retried internally.
Warnings and public error messages remain server-provided text. Unknown private
metadata is not promoted into model identity or Gateway error fields. A unary
response's bounded raw HTTP body remains available in `Response.Body`.

Configured headers are applied before call headers, then client-owned auth,
content negotiation, and model protocol headers. Call headers also remain in
the request body, matching the registered client; a text-only Gateway may reject
them. See [package reference](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/grafana)
for configuration and result types.

---

← [Choose a provider](overview.md) · [Docs index](../README.md) · [Fallback and registry →](../guides/fallback-and-registry.md)
