# Use official OpenAI clients with AI Gateway

The Gateway's Chat Completions adapter lets an existing OpenAI client call a
configured Gateway model. The Grafana App Platform owns authentication and the
public route. Use a Grafana service-account token as the client API key, and set the base
URL to your namespace's route:

```text
https://<grafana-host>/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/<namespace>/v1
```

For the official Go client, retain a trailing slash in the base URL:

```go
client := openai.NewClient(
    option.WithBaseURL(baseURL + "/"),
    option.WithAPIKey(grafanaToken),
    option.WithMaxRetries(0),
)
result, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
    Model: "configured-model-alias",
    Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Hello")},
})
```

For server-side TypeScript:

```typescript
import OpenAI from "openai";

const client = new OpenAI({ baseURL, apiKey: grafanaToken, maxRetries: 0 });
const result = await client.chat.completions.create({
  model: "configured-model-alias",
  messages: [{ role: "user", content: "Hello" }],
});
```

Retries are disabled in these examples so callers can distinguish one client
request from the Gateway's configured fallback attempts. Choose any client
retry policy with the application's side effects and overall deadline in mind.

## Access and supported behavior

Self-hosted users and allowlisted internal cloud services use the configured
catalog. Cloud users and service accounts can provide request-scoped provider
accounts through the `providerOptions.gateway` extension. The shared selector
validates those accounts and applies the same routing, fallback and credential
redaction as ProviderWire. Credentials are removed before native model calls;
the host identity still comes from the platform token. A caller rejected by the
host access policy receives HTTP 403. Internal service access tokens use the
platform's `X-Access-Token` header without an `Authorization` header.
See [Gateway authentication](gateway-authentication.md) for the access contract.

In TypeScript, pass the extension alongside the standard SDK request fields:

```typescript
const request = {
  model: "openai/gpt-4.1",
  messages: [{ role: "user" as const, content: "Hello" }],
  providerOptions: {
    gateway: { byok: { openai: [{ apiKey: process.env.OPENAI_API_KEY }] } },
  },
};
const result = await client.chat.completions.create(request);
```

The Go client accepts the same object with
`option.WithJSONSet("providerOptions", providerOptions)` as a per-call option.
Each namespace must be a JSON object. Provider-specific options such as
`openai.user` or `anthropic.container` remain available, subject to the shared
native guards. Chat's explicit fields and defaults take precedence over the
corresponding native options. The `gateway` namespace is case-sensitive;
configured-account callers cannot use it to supply BYOK credentials.

The adapter supports text, client-executed function tools, tool-result
continuations, JSON output, and streaming. Compatible Chat candidates also accept native `n` from 1 to 128.
Multiple choices use one native request, preserve their original indices and
finish reasons, and report native usage once. Responses and Anthropic candidates
reject `n > 1` with HTTP 400 before provider I/O because those APIs have no
equivalent native operation. It does not execute
application functions, persist conversations, or upload files. Native compatible
search is available through `web_search_options`; the selected service executes
that search. Unsupported
request fields fail before model invocation. The adapter supplies a call-scoped translator to the shared execution runtime.
Chat defaults are applied to each native fallback candidate, including
`store: false` and function strictness;
they do not change ProviderWire requests.

Messages accept developer instructions, named participants, assistant refusal
history and multipart text tool results. Developer instructions map to system
instructions for Responses and Anthropic. Compatible Chat candidates retain
native role, name and refusal fields. Named participants or nonempty refusal
history require a compatible Chat route; other transports reject them before I/O
instead of discarding those semantics. Null and empty refusal values remain
accepted. Message order is preserved.

Existing clients using deprecated `functions` and `function_call` can continue
using those fields, including assistant function-call history and `role: "function"`
results. The adapter converts them to common function tools for native execution,
then returns `message.function_call` or `delta.function_call` with the legacy
`function_call` finish reason. Legacy mode permits one function call per choice.
Mixing legacy request controls with `tools`, `tool_choice` or parallel calls is
rejected instead of choosing one silently. Prefer the current tool fields for
new clients.

Structured response and function schemas support self-contained JSON Schema,
including local references and unions produced by SDK helpers such as
`zodResponseFormat`. Schema compilation never loads external files or URLs.
Omitting function parameters declares a function with an empty parameter list.

Response IDs and model names describe the selected native response. Streaming
identity is frozen once public content starts. Late native metadata and warnings
can appear in the `grafana` extension on individual chunks. SDK accumulators may
omit those extensions, so inspect individual chunks when they matter.

Empty completions are valid. A token limit or content filter can leave function
arguments incomplete: inspect `finish_reason` before parsing or executing a tool
call. Successful complete strict tool calls remain schema-validated.

## Native transport capabilities

The table describes Gateway translation and projection. The selected native model
must also support the requested feature. A capability error occurs before native
I/O when the transport has no mapping; configured fallback does not make an
unsupported capability available on every candidate.

| Feature | Compatible Chat | OpenAI Responses | Anthropic |
| --- | --- | --- | --- |
| Text, streaming, function tools and continuation | Supported | Supported | Supported |
| JSON output and self-contained schemas | Supported | Supported | Supported |
| Images and PDF/file references | Supported | Supported | Supported |
| Image detail | Preserved | Preserved | No equivalent control |
| WAV/MP3 input and native audio output/history | Supported | Capability error | Capability error |
| Multiple alternatives (`n > 1`) | Native, usage once | Capability error | Capability error |
| Custom text tools | Supported | Supported | Capability error |
| Log probabilities | Native Chat fields | Projected token records | Capability error |
| Verbosity | Native setting | Native text setting | Capability error |
| Prediction and `web_search_options` | Native settings | Capability error | Capability error |
| Inline moderation | Native settings | Capability error | Capability error |
| `max_tokens` / `max_completion_tokens` | Native key distinction retained | Common output budget | Common output budget |
| `allowed_tools` | Native subset, full definitions | Native subset, full definitions | Filtered function definitions; history retained |
| Structured-output refusal | Standard refusal fields | Standard refusal fields | Native transport behavior |
| Prompt-cache options and part breakpoints | Native controls | Capability error | Capability error |
| User/safety identifiers, cache key/retention | Native controls | Native equivalents | Capability error |
| Metadata and service tier | Native controls | Native equivalents; `scale` rejected | Capability error |
| Nonempty logit bias | Native control | Capability error | Capability error |
| Extended usage details | Native counters retained | Available native counters | Available native counters |
| Streaming obfuscation | Gateway padding | Gateway padding | Gateway padding |
| Request-scoped accounts and routing | Shared selector policy | Shared selector policy | Shared selector policy |

The adapter does not manage uploaded-file ownership or transfer files between
accounts. Provider-specific settings remain subject to native guards and model
capabilities. Conversation persistence and file upload are separate operations
from this completion endpoint.

## Images, audio and files

User content arrays accept `image_url`, `input_audio` and `file` parts alongside
text, preserving their order. Images accept HTTP(S) URLs or base64 data URLs.
OpenAI and compatible candidates retain `detail`; Anthropic has no corresponding
resolution control and rejects explicit low, high or original detail before I/O;
auto uses its normal image handling. Inline files accept a base64 data URL or raw base64 with a
filename identifying the media type. PDF inputs and uploaded `file_id` values
map to the native file representation on all three backends. The Gateway does
not download URLs or upload files.

A `file_id` belongs to the native account selected for that attempt. It must be
valid for every candidate that may receive the request; the Gateway cannot move
an uploaded file across providers or accounts. Filenames remain attached to
inline files where supported by the native API.

WAV and MP3 `input_audio` parts require a compatible Chat candidate. The current
Responses and Anthropic transports cannot represent those audio inputs; the
Chat translator refuses them before native I/O instead of dropping them.
Likewise, unsupported inline media is refused for the selected native transport.
These capability checks do not establish that every configured model accepts
images, files or audio. Synthetic native-request tests cover all three backends;
no live provider acceptance is claimed.

## Streaming and failures

Use `Chat.Completions.NewStreaming` in Go or `stream: true` in TypeScript.
Request `stream_options.include_usage` to receive the final usage chunk, which
has no choices. Consume through the normal end of the stream. Cancel the request
context or abort the TypeScript stream when the caller stops consuming it.

Fallback can select another configured provider before the first public effect.
There is no replay after public content has started. The platform owns request
body admission and route deadlines; the Gateway owns native transport,
stream idle detection and bounded draining. Structured output and tool arguments
need retained content for final validation, so streaming is not a claim of
constant memory consumption.

Errors produced inside the adapter use Chat error envelopes. Authentication,
undeclared-route, admission, or deadline failures produced before the plugin
can use platform error documents. Handle the HTTP status as well as the SDK
error type; do not assume every platform failure has an OpenAI error code.

## Verify the client contract

Run `mise run test-openai-chat-completions-adapter` from the repository root.
The harness builds the candidate plugin and runs it through
`test/platformhost/cmd/plugin-host`, using signed host delegation and a test
service account. It exercises official `openai-go/v3` (the module's pinned
version) and npm `openai@6.27.0` against deterministic local native-provider
fixtures. These fixtures are synthetic tests, not provider recordings.

The suite covers unary and streaming text, tools and continuations, native
identity, usage, strictness/defaults, fallback, SDK errors, cancellation and
access policy. This evidence does not establish deployment compatibility with a
real Grafana host or live commercial provider behavior. Plugin artifact
validation and Grafana end-to-end testing remain separate gates.

When a native provider emits several response segments, the adapter keeps one
public completion identity and emits one terminal finish chunk after the runtime
confirms the stream ended. Native raw transport events do not interrupt delivery.
Usage is summed across segments; if a segment omits token totals, the final usage
is unknown instead of an incomplete total. Later native response identities remain
available in `grafana.native_response`.

Execution attribution is exposed in `grafana.provider_metadata.gateway`, using
the same runtime projection and credential redaction as ProviderWire. Native
provider metadata is preserved as provider output, not passed through the failure
summary redactor, with a colliding native `gateway` value relocated
to `gateway.nativeMetadata`. Stream-native failures may include a redacted
`grafana.native_error`; the standard OpenAI error fields remain stable.

Retrieved sources no longer invalidate an otherwise valid completion. They are
preserved under `grafana.sources`; citations are preserved under
`grafana.citations`. URL citations with native start/end positions additionally
appear as standard `message.annotations` or `delta.annotations`. Sources without
positions do not receive fabricated text spans. Native local spans remain in the extension; standard Chat annotation offsets
follow the character convention below.

The compatible backend preserves native refusal and token log-probability fields
in their standard Chat locations. Refusals are valid responses to structured-output
requests and are not parsed as JSON. Native audio/prediction token counters remain
available in usage details, preserving absent counters versus reported zero.

Validation applies to the complete public stream: structured JSON must remain valid
after concatenation, and `parallel_tool_calls:false` permits only one call across
all native segments. The shared runtime rejects duplicate native tool IDs. A response containing calls ends with `tool_calls` unless it was interrupted
by a token limit or content filter.
URL citation annotations use character offsets in the concatenated Chat text.
Preceding text is counted in Unicode code points, including one for each emoji;
native block-local indices are shifted without conversion. The `grafana.citations`
extension retains the original local spans. This convention also applies across
sequential native stream segments.
Audio-capable compatible endpoints accept `modalities` and `audio` output
parameters, including named or custom-ID voices. Assistant history can carry
`audio: {"id": "..."}` alongside text, name and refusal. The native audio ID is
preserved. Requests requiring audio output or audio history fail before provider
I/O on Responses and Anthropic targets, whose current transports do not implement
that Chat audio capability. Explicit text-only modalities remain usable there.

For compatible audio models, unary output preserves `message.audio` and streaming
preserves `delta.audio`, including IDs, expiration, encoded data and transcript.
Audio-only unary responses retain `content:null`. The official Go client PoC covers
audio request parameters and both output forms through native synthetic HTTP/SSE.

Chat request controls include user/safety identifiers, service tier, metadata,
prompt-cache key/retention, logit bias and log-probability selection. The adapter
maps supported equivalents to each native protocol and rejects unsupported
capabilities before I/O. Responses logprobs are flattened into Chat token records;
alternative counts are trimmed to the client's requested `top_logprobs` (zero by
default), without mutating provider metadata.

Native compatible `system_fingerprint`, actual `service_tier`, and positioned URL
annotations are preserved in standard response fields. Responses' reported
service tier is also projected. Unknown values remain absent; request parameters
are not substituted for native execution evidence.
Custom tools accept `type: "custom"` with text or Lark/regex grammar formats and
named custom tool choices. Compatible and Responses targets preserve their text
inputs, streamed fragments and final custom call shape. Applications execute the
tool themselves and return its result using the ordinary tool-message call ID;
custom call history remains valid even when the follow-up omits tool declarations.
Anthropic targets reject this capability before native invocation. Grammar
compilation and generation constraints are enforced by the selected native API.

`verbosity` maps to the native text verbosity setting on compatible and Responses
backends. Native compatible backends also accept validated `prediction` (text or
text parts) and `web_search_options` (search context and approximate location).
These options are forwarded with their native semantics; unsupported models can
still reject them. Responses and Anthropic return a capability error for these
Chat-only prediction/search controls before I/O; Anthropic also rejects verbosity.

Streaming includes transport-local random `obfuscation` padding by default. Set
`stream_options.include_obfuscation` to `false` to omit it. Each public chunk,
including the final usage chunk, is padded to a 64-byte boundary with fresh
cryptographic randomness; no provider padding or content is reused.

Inline `moderation` requests are supported on native compatible routes. The
configured model and input/output policies are validated and passed to the native
service. Other backends reject the option before I/O rather than skipping it.

Native compatible routes support `prompt_cache_options` and explicit
`prompt_cache_breakpoint` boundaries, including text, image, audio and file user parts and tool-result text arrays. The
adapter retains original part boundaries without changing text or identities.
Responses and Anthropic reject these Chat cache controls before I/O because the
selected provider transports do not preserve their exact semantics.

Native usage details preserve prompt cache-write, image and text counts and
completion text counts. Missing counters remain absent, explicit zeros remain
zero, and counters exceeding their corresponding prompt/completion total are
rejected as invalid provider output.

For compatible targets, Chat preserves the distinction between `max_tokens` and
`max_completion_tokens`. Both use the common validated output budget; only the
native field name differs. Shared native-option guards reject raw overrides of
either exact native key. Native JSON keys remain case-sensitive.

Chat `tool_choice` supports `allowed_tools` with `auto` or `required` mode and
function/custom references matching declared tools. Duplicate or undeclared
references are rejected. Compatible and Responses targets retain full tool
definitions and send a native allowed subset. Anthropic filters definitions for
the current attempt and retains message history; this can change prompt context
and cache reuse compared with sending all definitions. Custom tools remain
unsupported on Anthropic. An empty `auto` subset disables tool calls; an empty
`required` subset is rejected. Output membership is checked before emitting tool
start or argument deltas, as well as on complete tool calls.

Responses-backed Chat requests preserve native refusals automatically. Unary
responses expose `message.refusal` with null content when no text was generated;
streams expose refusal deltas exactly once, including when the refusal arrives
only in the final native snapshot. A refusal can validly replace requested JSON
schema output or required tool calls and does not become a gateway output error.

Chat requests fail with HTTP 400 before native I/O if the selected provider
would discard an option as unsupported. Responses rejects `stop`, `seed`,
`presence_penalty`, and `frequency_penalty`; Anthropic rejects the latter three.
Model-specific restrictions also apply, including unsupported sampling controls
with reasoning and Anthropic's mutually exclusive `temperature` and `top_p`.
Use a compatible Chat target when the native Chat API supports the requested
controls. Other adapters retain their provider warning behavior.

The current HTTP surface implements creation and streaming only. `store: true`
is rejected; retrieve, list, update, delete, and stored-message routes are not
implemented. Supporting those operations requires a host storage and ownership
contract, not only forwarding `store` to a native provider. Unsupported create
fields are rejected rather than silently ignored. Native search is available
through compatible `web_search_options`; application tools
are executed by the client.

Unary compatible responses preserve the native top-level `metadata` map for one
or multiple choices. The returned native value is used even if it differs from
the submitted request metadata; an empty returned map remains empty.

---

[Prev: Gateway authentication](gateway-authentication.md) · [Docs index](../README.md) · [Next: Gateway architecture](../concepts/gateway-architecture.md)
