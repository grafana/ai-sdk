# Gateway glossary

Use this alongside [How a request crosses AI Gateway](gateway-architecture.md).
Aliases below explain how a term appears in conversation and code. Directional
terms use the Gateway server's point of view unless stated otherwise.

## Components and direction

| Term and aliases | Meaning and why it matters | Code or further reading |
| --- | --- | --- |
| Client; caller; consumer | The application making a request. A browser, application server, and Gateway's outbound HTTP client occupy different positions; always identify which hop is meant. Gateway credentials belong on the application server. | [Go client](../providers/grafana-gateway.md) |
| Adapter; translator | Converts between representations. An **inbound** adapter accepts a client protocol; an **outbound** adapter turns common model calls into a native provider protocol. Supporting one direction does not establish support for the other. | [Component diagram](gateway-architecture.md#the-pieces-and-their-boundaries) |
| Provider; model provider; provider adapter | Can mean the remote vendor, its Go adapter package, or a configured account instance. `provider.LanguageModel` is the common model interface. A public model name is not necessarily a native model ID. | [Providers and models](providers.md) |
| Native; upstream | The provider's own API, payloads, or metadata, beyond Gateway's common representation. In parity discussions, **upstream** instead means the pinned Vercel source; it does not mean the live model service. | [Parity baseline](../../test/conformance/upstream.yaml) |
| Ingress; inbound; frontend API | The boundary receiving client traffic. Here, “frontend” can mean the server's public API, not a browser UI. The command exposes ProviderWire ingress. | [Routes](../../ai-gateway/cmd/grafana-ai-gateway/internal/gateway/routes.go) |
| Egress; outbound | Traffic from Gateway to a provider or other dependency. An inbound authenticated request does not by itself establish safe outbound network access. | [Gateway plugin](../../ai-gateway/README.md) |
| ProviderWire; ProviderWire V4 | HTTP representation of model calls and provider results, consumed by Gateway clients. It carries JSON for unary results and provider events over SSE for streams. It is distinct from the UI message protocol consumed by React hooks. | [ProviderWire handler](../../ai-gateway/providerwire/v4/handler.go) |
| Transport; HTTP client | Moves requests and responses across a hop. Provider translation sits above transport; a successful TCP/HTTP exchange need not be a successful model invocation. | [Native model composition](../../ai-gateway/cmd/grafana-ai-gateway/internal/nativemodel) |
| Middleware; wrapper | Behavior surrounding another handler or model. HTTP middleware covers routing/authentication/HTTP observation; model middleware surrounds `DoGenerate` and `DoStream`. Their positions determine what failures they can see. | [Middleware overview](../middleware/overview.md) |

## Identity, selection, and control

| Term and aliases | Meaning and why it matters | Code or further reading |
| --- | --- | --- |
| Authentication; authn | Establishes who the caller is. The plugin host verifies a platform-delegated token against the platform signing keys; there is one verified-token path, not separate JWT/static-key or trusted-proxy paths. It precedes the access decision and ProviderWire decoding on protected routes. | [Authentication guide](../guides/gateway-authentication.md) |
| Authorization; authz; scope | Decides what an identity may do. A valid token alone does not imply access; `Decide` maps the token and route namespace to configured, BYOK, or refused access under the `access_mode`, and the token namespace must cover the route namespace. | [Authentication guide](../guides/gateway-authentication.md) |
| Catalog; discovery; public model inventory | Maps public names and aliases to configured model objects and describes available choices. Discovery lists public models; it is not a dump of credentials or all vendor models. | [Catalog guide](../../ai-gateway/docs/model-catalog.md) |
| Alias; canonical ID; native model ID | An alias resolves to a canonical public model such as `assistant`; that model's candidates carry native provider model IDs. Keeping these identities separate makes fallback understandable. | [Catalog implementation](../../ai-gateway/catalog/static_catalog.go) |
| Routing; selection; candidate | Chooses the configured model and, when applicable, an ordered primary/backup candidate. Selection is not proof of generation completion. | [Catalog construction](../../ai-gateway/cmd/grafana-ai-gateway/internal/service/catalog.go) |
| Guard; policy; preflight | A rule checked before some action, such as validating native options before a provider call. These names do not imply one global policy layer; check the actual placement and whether the command installs it. | [Native option guards](../../ai-gateway/cmd/grafana-ai-gateway/internal/nativeoptions) |
| BYOK; bring your own key; request account | Provider credentials supplied for a request, distinct from the delegated token authenticating the caller. BYOK access (`cloud` mode) uses request-local Anthropic or OpenAI accounts; configured access uses configured catalog models and cannot switch to BYOK. | [Capability boundary](gateway-architecture.md#existing-code-versus-deployed-capabilities) |
| Admission control; aggregate capacity limit | Deciding whether to accept more work based on shared capacity, such as concurrent requests. Per-request byte and duration limits are not aggregate admission control. The command does not implement a general aggregate controller. | [Resource boundaries](gateway-architecture.md#streaming-failures-and-the-point-of-no-return) |

## Bytes and stream lifecycle

| Term and aliases | Meaning and why it matters | Code or further reading |
| --- | --- | --- |
| Unary; non-streaming | One complete model result returned as JSON. The handler can validate and encode it before committing success. A high-level SDK call that collects a stream is a different execution path. | [Request walkthrough](gateway-architecture.md#follow-one-request) |
| Serialization; encoding; decoding | Converts values to bytes or bytes back to values. JSON escaping and binary encoding can change size, so source-value size and encoded-response size are different measurements. | [Response encoding](../../ai-gateway/providerwire/v4/response.go) |
| Framing; SSE; Server-Sent Events | Delimits serialized events on a stream. ProviderWire uses `data: <JSON>` followed by a blank line. Sharing SSE framing does not make ProviderWire and UI message events interchangeable. | [Stream encoder](../../ai-gateway/providerwire/v4/stream.go) |
| Buffering; accumulation | Holding values or bytes before forwarding them. The handler buffers a unary result and individual encoded frames. That is different from buffering a complete provider stream to enable failover. | [Fallback guide](../guides/fallback-and-registry.md) |
| Decompression; content decoding | Expands compressed HTTP bytes, usually in a transport or provider client. Compressed length, decoded body length, and encoded Gateway frame length are different measurements; the model transport limits each provider response by its decoded length. | [Transport and resource boundaries](gateway-architecture.md#streaming-failures-and-the-point-of-no-return) |
| Response commitment; headers sent | The HTTP status and headers have been written. A subsequent failure cannot replace HTTP 200 with an error status; it needs a stream error if writing is still possible. | [Stream lifecycle](gateway-architecture.md#streaming-failures-and-the-point-of-no-return) |
| Candidate commitment; first-part selection | Default fallback accepts a provider on its first part, before Gateway's HTTP commitment. Even a start or error part selects it; later errors do not trigger replay. | [Fallback implementation](../../fallback/fallback.go) |
| Cancellation; abort; deadline; timeout | Cancellation asks work to stop; a deadline bounds elapsed work. The request deadline, set by Grafana's route deadline, covers selection and execution, while stream idle time limits gaps between parts. Cancellation cannot reverse remote effects. | [Stream lifecycle](gateway-architecture.md#streaming-failures-and-the-point-of-no-return) |
| Drain; cleanup | Consumes leftover channel values after stopping normal output, so producers can release resources. Gateway bounds its post-terminal draining by time. | [Cleanup tests](../../ai-gateway/providerwire/v4/stream_test.go) |
| Retry; fallback; replay | Retry repeats a call, often to the same provider. Fallback tries another configured candidate after an eligible failure. Replay repeats already-started work or output. None guarantees exactly-once remote execution; retries may also occur inside native clients. | [Retry guide](../guides/retry-and-timeout.md), [fallback guide](../guides/fallback-and-registry.md) |
| Error event; terminal failure | A provider error event can precede more content and finish. A handler-terminal failure ends processing. An HTTP success status does not distinguish these stream outcomes. | [Streaming tests](../../ai-gateway/providerwire/v4/stream_test.go) |

## Observation and accounting

| Term and aliases | Meaning and why it matters | Code or further reading |
| --- | --- | --- |
| Logical call; invocation | One call to the selected public model. It may contain multiple physical candidate attempts. Tool continuation and application retries create new invocations. | [Operator observation](../../ai-gateway/docs/text-observability.md) |
| Physical attempt; candidate attempt | An observed try of a configured candidate. It is not necessarily one network request: native client retries or provider-side execution can add work beneath it. | [Attempt observation](../../ai-gateway/cmd/grafana-ai-gateway/internal/service/physical_attempts.go) |
| Diagnostics; execution overview; native error summary | Optional evidence about decisions and failures. `gateway.execution` describes observed configured attempts; it is not a full trace, completion guarantee, or replay authorization. Its absence does not prove no attempts occurred. | [Execution overviews](../providers/grafana-gateway.md#inspect-execution-overviews-and-failures) |
| Usage; token accounting | Provider-reported input/output and related token counts. Counts may be unavailable on failure and do not establish total billing across hidden retries or remote effects. | [Usage transport](../../ai-gateway/providerwire/v4/response.go) |
| Provider metadata; namespace; opaque data | Provider-specific values carried alongside common output. A namespace is a map key, not authority: native values named `gateway` do not automatically become trusted Gateway diagnostics. | [Metadata boundaries](../providers/grafana-gateway.md#inspect-execution-overviews-and-failures) |
| Observability; telemetry; capture | Logs, metrics, traces, or optional content recording for operators. This is separate from client-facing diagnostics; enabling one does not imply the other, nor permission to expose content to a browser. | [Operator observation](../../ai-gateway/docs/text-observability.md) |

---

← [Gateway architecture](gateway-architecture.md) · [Docs index](../README.md) · [Call Grafana AI Gateway →](../providers/grafana-gateway.md)
