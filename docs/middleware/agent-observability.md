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

## Export generations through OpenTelemetry

The pinned agento11y v0.19.0 supports generation export through an
OpenTelemetry (OTel) span. Select the protocol on the client returned by
`ClientResolver`, not on the middleware. Applications own the exporter, tracer
provider, sampling, routing, and shutdown.

Configure a client once, outside the request path. Given an application-owned
`sdktrace.SpanExporter` named `exporter`:

```go
import (
	"github.com/grafana/agento11y/go/agento11y"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

generationProvider := sdktrace.NewTracerProvider(
	sdktrace.WithBatcher(exporter),
	sdktrace.WithSampler(sdktrace.AlwaysSample()),
)
cfg := agento11y.DefaultConfig()
cfg.EnableExperimentalFeatures = new(true)
cfg.GenerationExport.Protocol = agento11y.GenerationExportProtocolOTel
cfg.TracerProvider = generationProvider
cfg.Flusher = generationProvider
cfg.ContentCapture = agento11y.ContentCaptureModeFull
client := agento11y.NewClient(cfg)
```

Return this client from `ClientResolver`. The middleware neither constructs an
exporter nor flushes it per call. After requests and streams finish, call
`client.Shutdown(ctx)` before `generationProvider.Shutdown(ctx)`. Check both
errors. A stream's recorder finishes before its consumer channel closes.

Set `Flusher` explicitly when client `Flush` or `Shutdown` must invoke and wait
for `ForceFlush`. Setting `TracerProvider` alone does not configure flushing.
A successful flush does not confirm remote ingestion or acknowledge each
generation.

A dedicated `AlwaysSample` provider prevents request-trace sampling from dropping
generation records. If you omit `TracerProvider` or `MeterProvider`, agento11y
uses the corresponding global provider. `Config.Tracer` and `Config.Meter` do
not select the OTel generation pipeline. When using a dedicated generation
provider, set `Config.Tracer` separately if normal tool and embedding spans
should use the application's trace pipeline.

The OTLP destination must route spans marked `agento11y.record="true"` to Agent
Observability. Sending them only to Tempo does not create generation records.
Selecting OTel without enabling experimental features exports no generation and
does not fall back to gRPC or HTTP. Sampling and span attribute limits can drop
records or remove content; the middleware cannot verify the destination.

With gRPC or HTTP, the client exports a generation payload and emits a
`generateText <model>` or `streamText <model>` metadata span. With OTel, the
`chat <model>` CLIENT span is the generation transport. It includes the
generation ID, mapped attributes, and permitted content. Streaming spans include
`gen_ai.request.stream=true` and remain open until upstream closes or the request
is cancelled. Forwarding stops on cancellation. No second generation span is
created.

In OTel mode, `full_with_metadata_spans` resolves to `full` because the span is
the only generation copy. Use `metadata_only` to omit system instructions,
input/output messages, tool definitions, and media. Metadata-only capture still
retains non-content fields such as tags and custom metadata.

Requested identity and provided-only context controls also apply to OTel
recording. A `GenerationFilter` decides which mapped request and response fields
reach the final span, including on immediate provider errors. It cannot remove
context fields: the generation start includes the user ID, agent name and
version, conversation ID, parent generation IDs, tags, and metadata, and
agento11y restores them when the filter omits them. Agento11y v0.19 exposes
operation, provider, and request model to samplers, but not request controls.
Retained controls appear on the finished span.

## Apply preflight policy

Hooks can allow the provider call unchanged or deny it before the model is
called. Input transforms are temporarily disabled with the pinned agento11y v0.19.0.

Handle `ErrHookDenied` as a policy outcome. Give hook evaluation a bounded
latency and decide how the application should handle policy-service
unavailability.

`HooksMiddleware` rejects every non-nil decoded `TransformedInput` with
`ErrHookTransformFailed` before calling the provider, regardless of `FailOpen`.
This includes valid assistant, system-prompt, tool-message, tool-list, and user
changes, as well as unchanged replacements. Denials take precedence over
transforms.

Agento11y v0.19.0 converts unsupported response roles to `user` and drops malformed
parts before middleware validation. It also recovers encoded JSON payloads and
discards part metadata. The dependency exposes no raw response API, so validation
of decoded input cannot detect all lost data. Transform support must remain
disabled until the decoder preserves invalid data for validation or rejects it.

An empty HTTP `transformed_input:{}` is different: the dependency normalizes it
to nil. An allow response with that value, or without a transform, passes the
original request through unchanged. The client adds available conversation and
trace correlation to hook requests; media remains excluded.

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

Remote URLs are stored unchanged, including signed query parameters, and are not
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
and errors. Streaming usage comes from the last `finish`: each counter it
reports wins, and counters it omits keep the greatest value seen in that
response. Before any `finish`, each counter keeps its greatest observed value.
Reported input totals are marked as cache-inclusive in exported generations,
spans, and token metrics unless reported token buckets contradict the total.
Cache reads and writes are already included and are not added again. Context
helpers relate parent, child, and linked generations across agents and tool
workflows. Use these helpers for generation relationships and reserve generic enrichment for
other provider-bound metadata.

A stream can carry several responses or messages, each ending in its own
`finish`. Recording covers all of them in one generation, with the last finish
reason and the last message's usage, as upstream `streamText` reports it. Usage
is not summed across messages, so it can understate a multi-message total. The
generation is recorded when the stream closes or its context ends, not at the
first `finish`. Drain the stream to the end, or wait for `OnRecordComplete`,
before you close a shared client. A consumer that stops at `finish` and cancels
still records the finished messages without a cancellation error. If no provider error follows the last
`finish`, context end records the snapshot from that finish, including its
completion time. Otherwise, recording retains the state observed so far and the
latest provider error, not the cancellation.

For gateway providers, response metadata can identify the backend model while
transport metadata retains the gateway identity. This supports provider and cost
attribution for routed calls. With the default response-preferred identity,
OTel span names and request-model attributes use the backend identity when it
is complete. `IdentityRequested` keeps the wrapped model identity instead.
Recording stores `amazon-bedrock` as `bedrock` and `anthropic.vertex` as
`vertex` with every export protocol, and hook requests use the same names.
Hook rules and queries that match the AI SDK names must use the stored names.
OTel generation spans and the hooks preflight span use `aws.bedrock` and
`gcp.vertex_ai`. `ai_sdk.transport.provider` keeps the AI SDK name. Consistent
reported input totals, including cache tokens, use
`gen_ai.token.semantics="inclusive"`.

## Compose middleware deliberately

`agentobservability.Stack` returns the standard hooks-then-recording order. Use
the individual middleware constructors when composing with logging, Prometheus,
or enrichment.

Ordering determines whether another middleware sees denied attempts, rejected
transforms, or only calls that passed policy. In the standard stack, denied
requests and rejected transforms never reach recording or the provider. Test
allowed, denied, and rejected-transform paths.

## Reference

- [`middleware/agentobservability`](https://pkg.go.dev/github.com/grafana/ai-sdk/middleware/agentobservability)
- [Grafana Agent Observability documentation](https://grafana.com/docs/grafana-cloud/monitor-applications/ai-observability/)
- [Middleware overview](overview.md)

---

← [Prometheus metrics](prometheus.md) · [Docs index](../README.md) · [Production checklist →](../best-practices/production.md)
