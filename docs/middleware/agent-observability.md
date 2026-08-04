# Agent Observability

Grafana Agent Observability helps teams investigate model and agent behavior,
including inputs, outputs, usage, errors, multi-step relationships, and the
provider that served a routed call. Add `middleware/agentobservability` to send
those generations to Grafana and optionally evaluate policy before a provider
call.

## Install and wrap a model

```bash
go get github.com/grafana/ai-sdk/middleware/agentobservability
```

```go
import (
	"github.com/grafana/ai-sdk/middleware/agentobservability"
	"github.com/grafana/agento11y/go/agento11y"
)

model := agentobservability.Wrap(baseModel, agentobservability.WrapOptions{
	ClientResolver: func(ctx context.Context) *agento11y.Client {
		return clientForRequest(ctx)
	},
	ContextProvider: contextInfoForRequest,
	Hooks: agentobservability.HooksOptions{
		Enabled: func(ctx context.Context) bool {
			return hooksEnabled(ctx)
		},
		MaxLatency: 5 * time.Second,
	},
})
```

See the official [Agent Observability Go setup guide](https://grafana.com/docs/grafana-cloud/machine-learning/ai-observability/get-started/go/)
for creating and configuring the underlying SDK client.

## Resolve clients per request

`ClientResolver` chooses the SDK client from the call context. This supports
multi-tenant routing without coupling the middleware to application tenant
types. Returning `nil` makes recording and hooks no-ops for that request.

`ContextProvider` supplies approved user, tag, agent, and metadata fields. Keep
sensitive and high-cardinality values aligned with retention, access, and tenant
policies.

## Apply preflight policy

Hooks can:

- allow the provider call unchanged;
- deny it before the model is called;
- transform supported input before continuing.

Handle `ErrHookDenied` as a policy outcome. Give hook evaluation a bounded
latency and decide how the application should handle policy-service
unavailability.

## Record media

Recording maps provider `file` and `reasoning-file` content to Agent
Observability media for prompts, generated results, and streams. This does not
add provider content types or UI message chunks, or change model requests and
responses. Hook evaluation excludes media to avoid widening the preflight
disclosure boundary.

Only image and video media are recorded. The mapper accepts valid data URLs and
HTTP(S) URLs, or converts byte and base64 payloads to data URLs. It determines a
concrete MIME type from the declared media type, data URL, filename, URL path, or
sniffed inline bytes, in that order. Conflicting, malformed, ambiguous, and
unsupported media is skipped, as are provider file references, inline text file
data, URL credentials, and non-HTTP URL schemes. Streamed assistant parts retain
the order of their first provider events.

Remote URLs are stored verbatim, including signed query parameters, and are not
fetched by the middleware. Inline bytes grow to approximately 4/3 of their
original size when base64 encoded. The middleware applies no additional size
cap before conversion. Provider limits constrain source payloads and the
agento11y exporter limit constrains uploads, but neither prevents local
validation, cloning, or base64 encoding from consuming memory first. Configure
the agento11y client's content capture mode and retention policy before
recording sensitive media. Metadata-only capture strips media URLs before
export.

## Record generation relationships

Recording captures unary and streaming generations, including results, usage,
and errors. Streaming usage combines every usage-bearing part and preserves the
greatest value observed for each normalized counter. Context helpers relate
parent, child, and linked generations across agents and tool workflows. Use
these helpers for generation relationships and reserve generic enrichment for
other provider-bound metadata.

For gateway providers, response metadata can identify the backend model while
transport metadata retains the gateway identity. This supports provider and cost
attribution for routed calls.

## Compose middleware deliberately

`agentobservability.Stack` returns the standard hooks-then-recording order. Use
the individual middleware constructors when composing with logging, Prometheus,
or enrichment.

Ordering determines whether another middleware sees denied attempts,
transformed parameters, or only calls that passed policy. Test both allowed and
denied paths.

## Reference

- [`middleware/agentobservability`](https://pkg.go.dev/github.com/grafana/ai-sdk/middleware/agentobservability)
- [Grafana Agent Observability documentation](https://grafana.com/docs/grafana-cloud/monitor-applications/ai-observability/)
- [Middleware overview](overview.md)

---

← [Prometheus metrics](prometheus.md) · [Docs index](../README.md) · [Production checklist →](../best-practices/production.md)
