# OpenAI Chat Completions adapter

`POST /v1/chat/completions` is an intentionally bounded OpenAI-compatible subset,
not a full implementation of the OpenAI platform. The adapter is AGPL, independent
of ProviderWire codecs, and uses the same catalog, outbound transports, logical
model middleware, authentication verification and process shutdown. Its sole
responsibility is translating the public Chat Completions wire contract to and
from the canonical provider domain. Backend selection, fallback and upstream
serialization remain host/catalog and provider responsibilities.

## Authority and proof

The adapter's JSON/SSE authority is the official OpenAI API contract and exact official
SDKs: Go `openai-go/v3 v3.66.0` and JavaScript `openai 6.27.0` (workspace lockfile).
The JavaScript pin is an explicit compatibility witness, not a claim to be latest.
See [Chat create](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create),
[function calling](https://developers.openai.com/api/docs/guides/function-calling)
and [Responses migration](https://developers.openai.com/api/docs/guides/migrate-to-responses).
Responses storage and function strict defaults are explicitly translated; the
Gateway intentionally rejects persistence even if an upstream account defaults
to storing completions.

Provider-domain conversion remains on the registered Vercel baseline in
`test/conformance/upstream.yaml`: commit `5d12eaa6caa193d3901cbab98a734403eb6bf622`.
Gateway source tests and the container build use the explicit `go.gateway.work`
workspace so the adapter runs with candidate SDK/provider source. Standalone
module checks separately verify the immutable published dependency versions. Synthetic local upstreams in `test/openai-chat-completions-adapter` prove mappings and SDK
consumption, not live-provider parity. No recorded fixture has been invented.

## Authentication and routing

Access-token mode accepts exactly one `Authorization: Bearer <token>`; scheme
matching is case-insensitive, token whitespace/comma and duplicate values are
rejected. X-Access-Token, X-Grafana-Id, and X-Scope-* conflicts are rejected, even
when empty. The existing verifier validates JWT signatures/audiences/namespace.
Optional Grafana ID tokens are deliberately unsupported on this adapter route.

Cloud mode requires the trusted edge to strip Authorization, X-Access-Token and
X-Grafana-Id; the existing positive X-Scope-OrgID assertion contract is unchanged.
Never expose a cloud-auth listener directly to untrusted clients. SDKs sending
Bearer credentials must go through the credential-stripping edge.

Unknown adapter routes (including `/v1/models`) receive a safe JSON 404. Wrong
Chat methods receive JSON 405 plus Allow: POST. Raw-path aliases, query strings,
non-JSON bodies and encoded request bodies are not accepted. Authentication
precedes body decoding. Fixed error envelopes do not include upstream messages, request
content or credentials. Native response identity and warnings are developer-visible. Organization/project headers are not
forwarded or interpreted as authorization.

## Configured routes and native capabilities

Every configured OpenAI Responses, Anthropic or OpenAI-compatible model is eligible,
including new model IDs and custom compatible provider names. Aliases select the
canonical route; returned identity comes from the provider where available.
The catalog composes its ordinary ordered fallback for every mapped request,
including function tools/history, structured output and reasoning. It does not
intersect candidate options or require matching provider families.

Chat defaults are translated at each candidate's native boundary before invocation.
Only Chat requests activate this translation; ProviderWire options remain unchanged.
Responses receives `store: false`, the Chat schema strictness and `parallelToolCalls`.
Compatible receives storage and `parallel_tool_calls` in its configured namespace,
including names that coincide with another provider's namespace. Anthropic receives
`disableParallelToolUse` when requested. Each call gets its own option map.

Native adapters own model capability checks and defaults. Unsupported native settings
can produce warnings rather than fail a successful generation. The adapter still
rejects unsupported Chat protocol fields and output shapes it cannot represent.
Selected output or encoding failure never triggers another fallback attempt.
Functions execute in the consumer application; continuation starts a new request.

## Defaults and request boundaries

- `n`: omitted/null/1 only. `stream`: omitted/null/false is unary.
- `store`: omitted/null/false only. Responses and compatible requests explicitly
  send false; the adapter has no persistence. This does not waive upstream abuse
  monitoring or retention policies outside the store parameter.
- Function `strict`: omitted/null/false means non-strict. Native adapters receive explicit false and apply their own capability handling.
- Tools absent: no tool choice is generated. Tools present: choice defaults auto.
  Explicit none, required, or one declared function maps to the provider contract.
- Sampling scalars are omitted when absent/null; backend defaults apply. Native token defaults apply when the bound is absent/null; there is no adapter-specific 4096-token cap. `max_tokens` and `max_completion_tokens` cannot appear together;
  either maps to the provider output-token bound (including reasoning where supported).
- JSON schema strict omitted/null/false remains false. Only explicit true enforces
  the schema locally on stop. JSON syntax is checked for successful JSON output;
  length/content_filter partial content is returned without claiming schema success.
- `stream_options.include_usage` defaults false and requires stream true. A usage
  chunk has choices `[]`; totals are omitted/unknown, never fabricated. Missing
  totals, cached tokens and reasoning tokens are not double-counted.

Messages accept system/user/assistant text strings or arrays of text parts. Only
leading system messages are accepted, avoiding backend-dependent reordering. Only
assistant messages may contain function calls; null content requires calls. Tool
results are strings, paired by unique call IDs with a preceding assistant call;
all pending calls must be resolved before another assistant/user/system message.
Functions require object parameters and JSON-object arguments. Limits include
1024 messages, 128 tools, 256-byte IDs, and names matching `[A-Za-z0-9_-]{1,64}`.
Stop is one string or 1..4 strings. A closed explicit request schema rejects unknown and case-altered protocol fields.
Standard JSON decoding uses the last duplicate key, including nested objects; escaped
lone surrogates follow Go replacement semantics. Schema validation and typed mapping
consume the same normalized document. Raw invalid UTF-8 and trailing JSON fail.
Explicit strict function outputs are checked against the supplied schema before
unary success or streaming terminal success; streamed argument fragments can
precede a terminal validation error.

Schema input is at most 32 KiB, depth 16 and 512 visited nodes, root type object.
Allowed keywords: type, title, description, enum, const, required, properties,
items, boolean additionalProperties, minimum/maximum, minLength/maxLength,
minItems/maxItems. References, remote loading, regex, combinators and all other
keywords are rejected. This is narrower than general JSON Schema by design.

Unsupported: developer messages, named messages, multimodal/file/audio content,
refusals, hosted/custom tools and execution, approvals, logprobs/logit_bias,
prediction, metadata, user, service_tier, response retrieval/deletion and streaming obfuscation. Unsupported provider-domain output arms fail
safely rather than disappear. Private reasoning content is deliberately not
exposed as assistant text; signed reasoning tool replay is not supported.
Successful stop without represented nonempty text or function calls is rejected,
including refusal-only output discarded by a lower converter. The adapter does
not reconstruct OpenAI refusal semantics or metadata absent from the shared
provider-domain result.

## Streaming and lifecycle

The first content/finish opens the stream after consuming preceding metadata.
Available native ID, model and timestamp are used. Missing fields use one generated
`chatcmpl-` ID, request timestamp or resolved route model, respectively; a route
fallback is not a claim of actual selected backend identity. These three values
remain stable for all chunks. Later identity is retained in the extension below.
This adds no wait after content arrives and does not change fallback selection at
the first provider part.
The first chunk declares assistant role; text follows immediately. Tool indices
follow first-seen order; IDs/type/name are emitted once, argument fragments once.
The final provider call must match accumulated fragments. A finish chunk, optional
usage-only chunk and `[DONE]` conclude success. Premature EOF, contradictory
lifecycle, unsupported content or bounds violations produce a safe SSE error when
writable and never manufacture `[DONE]`.

The existing `providerwire.*` scalar flags currently configure both adapters:
request 1 MiB; response cumulative 8 MiB; frame 1 MiB; 100000 provider parts;
model total 120s; visible-progress idle 30s; cleanup drain 1s. Durations are Go
duration values, not unscaled integers. A small response-byte reserve permits a
terminal safe error. Discarded metadata/reasoning never resets idle time. Socket
write deadlines bound total slow-client backpressure. Custom response writers must
support `http.ResponseController` write deadlines and flushing.

Cancellation has one channel owner; claimed streams transfer to an asynchronous
bounded drain without delaying response completion. Late setup retains its sole
bounded cleanup owner. Cleanup consumers exit on channel closure or drain expiry,
including when a producer continually supplies discarded parts.
Non-cooperative synchronous provider methods cannot be forcibly killed in Go and
may outlive the handler; configured HTTP providers cooperate with cancellation.
These are per-request bounds, not global concurrency, memory, goroutine admission
or throughput guarantees. Deployments need edge/process admission policies for
aggregate load. Tests exercise finite 16-way real-command load and 32-way adapter
load, eight concurrent ProviderWire calls with operational endpoint checks, and
a 32-client streaming cancellation storm, not an unbounded-load claim.

## Grafana diagnostics extension

Successful responses can include the optional Grafana-owned `grafana` object:

```json
{
  "grafana": {
    "warnings": [{"type":"unsupported","feature":"seed","details":"..."}],
    "native_response": {"id":"late-id","model":"actual-model","created":123}
  }
}
```

`warnings` preserves native warning type, feature/setting, message/details and order.
Warnings are returned with unary output or the next stream chunk after they arrive;
they do not convert a successful generation into HTTP 502. `native_response` is
stream-only and carries metadata received after public identity was fixed, without
rewriting earlier chunks. Its absent fields remain absent. These fields are an
extension of the Chat contract, not an OpenAI feature or a complete attempt history.

Go clients read `result.JSON.ExtraFields["grafana"].Raw()` or the equivalent field
on each `stream.Current()` chunk. JavaScript clients can access `result.grafana`
through a local extension type and inspect every streamed chunk. Ordinary SDK
accumulators are used for text/tools; preservation of extension fields in their
final assembled result is not promised, so collect diagnostics during iteration.

Diagnostics obey response/frame budgets and UTF-8 validation. At most 1024 pending
warnings are retained; aggregate diagnostic input and encoded output are bounded.
Oversized or malformed diagnostics fail explicitly without truncation. Raw transport
bodies/headers and credentials are not added by this extension. Operator capture
settings do not control whether these ordinary developer diagnostics are returned.

## Verification ownership

`mise run test-openai-chat-completions-adapter` runs adapter/command Go tests, TypeScript typechecking
and the official JavaScript SDK command suite. `mise run test-ai-gateway` includes
all adapter Go tests; `mise run test-integration` also owns the SDK adapter task.
The task selects `go.gateway.work` for both the Go tests and the binaries built
by the official SDK contract tests. `GATEWAY_TEST_GOWORK` can explicitly select
the workspace for those subprocesses; otherwise they inherit `GOWORK`. Tests use local signed JWKS, synthetic Responses,
compatible and Anthropic servers, no live credentials. Existing parity and
AGPL/Apache boundary checks remain required; adapter compatibility does not expand
ProviderWire or UIMessage parity claims.
