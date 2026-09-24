# Native Chat Completions

`POST /v1/chat/completions` is an intentionally bounded OpenAI-compatible subset,
not a full implementation of the OpenAI platform. The adapter is AGPL, independent
of ProviderWire codecs, and uses the same catalog, outbound transports, logical
model middleware, authentication verification and process shutdown.

## Authority and proof

Native JSON/SSE authority is the official OpenAI API contract and exact official
SDKs: Go `openai-go/v3 v3.48.0` and JavaScript `openai 6.27.0` (workspace lockfile).
The JavaScript pin is an explicit compatibility witness, not a claim to be latest.
See [Chat create](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create),
[function calling](https://developers.openai.com/api/docs/guides/function-calling)
and [Responses migration](https://developers.openai.com/api/docs/guides/migrate-to-responses).
Responses storage and function strict defaults are explicitly translated; the
Gateway intentionally rejects persistence even if an upstream account defaults
to storing completions.

Provider-domain conversion remains on the registered Vercel baseline in
`test/conformance/upstream.yaml`: commit `08ae5ad05bc12496dd1ffcf64e34419e0831300d`.
The AGPL module uses its immutable published Go module versions, not root-workspace
replacements. Synthetic local upstreams in `test/native` prove mappings and SDK
consumption, not live-provider parity. No recorded fixture has been invented.

## Authentication and routing

Access-token mode accepts exactly one `Authorization: Bearer <token>`; scheme
matching is case-insensitive, token whitespace/comma and duplicate values are
rejected. X-Access-Token, X-Grafana-Id, and X-Scope-* conflicts are rejected, even
when empty. The existing verifier validates JWT signatures/audiences/namespace.
Optional Grafana ID tokens are deliberately unsupported on this native route.

Cloud mode requires the trusted edge to strip Authorization, X-Access-Token and
X-Grafana-Id; the existing positive X-Scope-OrgID assertion contract is unchanged.
Never expose a cloud-auth listener directly to untrusted clients. SDKs sending
Bearer credentials must go through the credential-stripping edge.

Native unknown routes (including `/v1/models`) receive a safe JSON 404. Wrong
Chat methods receive JSON 405 plus Allow: POST. Raw-path aliases, query strings,
non-JSON bodies and encoded request bodies are not accepted. Authentication
precedes body decoding. Error bodies never include upstream messages, request
content, credentials or backend identity. Organization/project headers are not
forwarded or interpreted as authorization.

## Finite backend matrix

Policies are frozen from configuration before middleware wraps backend identity.
Public aliases resolve to canonical public response model IDs. Other configured
backend IDs remain usable through ProviderWire but receive native 400.

| Profile / exact model IDs | Text/scalars | Functions/history | Structured output | Reasoning |
| --- | --- | --- | --- | --- |
| `openai`: `gpt-4.1`, `gpt-4.1-mini`, `gpt-4o`, `gpt-4o-mini` | temperature, top_p, token bound; no seed/penalties/stop | non-strict or strict, auto/none/required/named; parallel bool | json_object, json_schema | rejected |
| `openai`: `o3`, `o3-mini`, `o4-mini` | token bound; no temperature/top_p/seed/penalties/stop | rejected, including history | json_object, json_schema | omitted provider default; low/medium/high |
| `anthropic`: `claude-sonnet-4-20250514`, `claude-3-5-haiku-20241022` | temperature 0..1, top_p, stop; tokens 1..4096 | non-strict only, auto/none/required/named; no parallel bool | rejected | rejected |
| `openai-compatible`: `gpt-4o`, `gpt-4o-mini`, default providerName only | temperature 0..2, top_p, penalties, seed, stop, token bound | strict/non-strict, auto/none/required/named; no parallel bool | rejected | rejected |
| Ordered fallback containing only the above Anthropic profiles | common text/scalars; tokens 1..4096 | rejected, including history and non-auto choice | rejected | rejected |

Compatible providerName must be empty or `openai-compatible`; custom namespaces
are not certified. An arbitrary compatible endpoint still must implement the
documented protocol; a model name alone is not provider attestation. Fallbacks
containing Responses or compatible candidates are rejected before invocation:
their explicit storage override cannot pass the shared text-only fallback guard.
That guard is not weakened.

## Defaults and request boundaries

- `n`: omitted/null/1 only. `stream`: omitted/null/false is unary.
- `store`: omitted/null/false only. Responses and compatible requests explicitly
  send false; the adapter has no persistence. This does not waive upstream abuse
  monitoring or retention policies outside the store parameter.
- Function `strict`: omitted/null/false means non-strict. Responses and compatible
  receive false explicitly; Anthropic omits its unsupported strict setting.
- Tools absent: no tool choice is generated. Tools present: choice defaults auto.
  Explicit none, required, or one declared function is supported where listed.
- Sampling scalars are omitted when absent/null; backend defaults apply. Anthropic
  and fallback use explicit max output tokens 4096 when absent/null; other profiles
  leave it unset. `max_tokens` and `max_completion_tokens` cannot appear together;
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
Stop is one string or 1..4 strings. Unknown fields, duplicate JSON object keys and
case-altered field names are rejected instead of ignored.
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
prediction, metadata, user, service_tier, response retrieval/deletion, seed on
Responses/Anthropic, and streaming obfuscation. Unsupported provider-domain output arms fail
safely rather than disappear. Private reasoning content is deliberately not
exposed as assistant text; signed reasoning tool replay is not supported.
Successful stop without represented nonempty text or function calls is rejected,
including refusal-only output discarded by a lower converter. The adapter does
not reconstruct native refusal semantics or metadata absent from the shared
provider-domain result.

## Streaming and lifecycle

One opaque chatcmpl ID, creation timestamp and canonical model remain stable.
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

## Verification ownership

`mise run test-native-chat` runs adapter/command Go tests, TypeScript typechecking
and the official JavaScript SDK command suite. `mise run test-ai-gateway` includes
all native Go tests; `mise run test-integration` also owns the native SDK task.
Run `GOWORK=off go test -race ./openai/chatcompletions ./test/native` in ai-gateway
for native race coverage. Tests use local signed JWKS, synthetic Responses,
compatible and Anthropic servers, no live credentials. Existing parity and
AGPL/Apache boundary checks remain required; native compatibility does not expand
ProviderWire or UIMessage parity claims.
