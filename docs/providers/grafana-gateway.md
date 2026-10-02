# Call Grafana AI Gateway from Go

Use the Grafana provider when your server calls Grafana AI Gateway. It uses the same generation, streaming, and registry
interfaces as other Go providers. Install the separate client module:

```bash
go get github.com/grafana/ai-sdk/providers/grafana
```

The client module is Apache-2.0 and does not depend on the Gateway service
module. Its executable response family includes text and unary/streaming function calls. Requests preserve
representable provider options, files, tools, and structured-output settings;
the deployed Gateway decides which capabilities it can execute and returns a
public invalid-request error for unsupported calls.

## Function tools

Direct routes support unary client-executed function tools. Definitions preserve
`strict: false`, examples, object schemas, and ordinary provider options;
Gateway-reserved option namespaces remain rejected. History accepts assistant
calls and text, JSON (including null), error-text, error-JSON, and content
results with text or supported file entries, preserving selected empty values.
The application executes tools and supplies call/result history on a later independent request.
The Gateway never executes a tool. Provider-executed/dynamic tools, approvals,
preliminary results, custom tool-result content, generated media responses, and
reasoning-file input remain unsupported. Logical telemetry
removes tool-bearing definitions, choices, inputs and outputs before export.

Streaming direct routes additionally support input start/delta/end, calls and
matching non-null JSON results. IDs, ordering and empty deltas are preserved.
Vercel and Go clients own the multi-step orchestration; each HTTP generation
remains stateless. Ordered fallback routes continue rejecting tool definitions,
choice and history before any physical invocation.

## Native provider options

Direct routes carry ordinary provider-option objects to the native adapter at
supported call, message, content-part, function-tool, and nested tool-result
file-entry scopes. The Gateway does not maintain an allowed-field inventory or
remove irrelevant namespaces. Empty objects and nested JSON remain intact;
the adapter selects its own namespaces, applies native precedence, and decides
which values it consumes. Forwarding an unknown field does not promise that a
native adapter emits it in its HTTP request.

Concrete credential, account, destination, mapped-content, tool-ownership and
transport protections still apply. For example, compatible metadata cannot
rewrite a mapped role or function call, and Anthropic options cannot activate
MCP history, server tools or native model fallback. Safe contextual controls,
such as assistant compaction history and local assistant-call caller metadata,
remain available. Ordinary Anthropic tool-role result caller metadata reaches
the adapter but is currently ignored by its native result converter. Supplied
history is request-path support, not a promise of output-derived continuation.

Gateway-owned routing/BYOK/operator namespaces remain explicitly unsupported
unless an owning feature consumes them. Outer Gateway authentication stays
separate from body-carried provider call headers. Operator capture settings do
not filter native request options or change request/result semantics; this
behavior does not enable server payload capture or alter consumer capture.

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
Models without fallback accept supported file inputs. Models configured with
fallback are text-only: files, nonempty tools, tool-call/result history,
active provider options, and tool choices other than plain auto are rejected
before any candidate, including primary, runs. Message-level namespaces
containing only empty objects retain text fallback eligibility without losing
their original representation. Active options under irrelevant namespaces are
also preserved and therefore rejected by the existing fallback guard rather
than silently removed. This is an interim fallback support boundary, separate
from direct native-option forwarding; broader mapped fallback support remains
separately scoped. A stream's
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
