# Production checklist

Before shipping a model-backed endpoint, bound its work, constrain its authority,
and make failures observable without exposing sensitive content.

## Bound every operation

- Pass the request or job `context.Context`.
- Set a total timeout for the user-facing latency budget.
- Set a per-step timeout for individual provider calls.
- Set first-content and inter-content timeouts for stalled streams.
- Use explicit stop conditions for tool loops.
- Limit request bodies, output tokens, tool output, and concurrent work.

```go
result := aisdk.StreamText(r.Context(), model,
	aisdk.WithMaxOutputTokens(2_000),
	aisdk.WithTimeout(aisdk.TimeoutConfig{
		Total:      45 * time.Second,
		Step:       20 * time.Second,
		FirstChunk: 15 * time.Second,
		Chunk:      10 * time.Second,
	}),
	aisdk.WithStopWhen(aisdk.StepCountIs(5)),
)
```

Choose values from the endpoint's requirements and provider latency. See
[Retry and timeout](../guides/retry-and-timeout.md).

## Control retries and fallback

Know which layer retries. Provider clients, SDK orchestration, HTTP
infrastructure, and job queues can all repeat work. Avoid stacked retry policies.
The SDK retry policy applies to provider calls. Make side-effecting tools
idempotent so retries in surrounding infrastructure remain safe.

Monitor the number of provider attempts per application request. Tool steps,
retries, and fallback multiply latency, cost, and rate-limit usage.

## Consume or cancel every stream

A `StreamTextResult` runs asynchronously. Choose one consumer and drain it:

```go
if err := aisdk.WriteUIMessageStream(w, result); err != nil {
	return err
}
```

or:

```go
for range result.FullStream() {
}
if err := result.Err(); err != nil {
	return err
}
```

Do not read `FullStream` and also pass the same result to an HTTP writer. If the
caller stops consuming, cancel the context.

## Secure the request boundary

Authenticate and authorize before starting SSE output. Validate model IDs,
message count, body size, uploaded content, and requested tools. Apply the same
model-entitlement policy to resolution and listing.

Treat model-generated tool input and externally retrieved content as untrusted.
Require approval and signed persisted approval state for consequential actions.
See [Security](security.md).

## Handle data deliberately

Decide whether prompts, reasoning, tool data, files, and outputs may be sent to:

- the selected provider;
- provider-executed tools or remote MCP servers;
- logs, metrics, traces, and Agent Observability;
- browser clients;
- conversation storage.

Start observability with metadata only. Enable content capture only with explicit
retention, redaction, tenancy, and access controls.

## Observe both operation and provider calls

Use orchestration callbacks for complete operations and model middleware for
provider attempts. At minimum, monitor latency, cancellations, timeout source,
provider errors, retry/fallback attempts, step count, token usage, and finish
reason.

Available integrations include [structured logging](../middleware/structured-logging.md),
[Prometheus metrics](../middleware/prometheus.md), and
[Agent Observability](../middleware/agent-observability.md).

## Verify the deployed streaming path

Test through the real proxy and load balancer. Confirm SSE is not buffered,
client disconnects cancel work, idle timeouts are compatible, errors remain
well-formed after headers are committed, and shutdown drains or cancels active
requests.

---

← [Operate in production](../README.md#operate-in-production) · [Docs index](../README.md) · [Error handling →](error-handling.md)
