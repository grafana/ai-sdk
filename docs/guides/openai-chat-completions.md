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
continuations, JSON output, and streaming for one choice. It does not execute
functions, persist conversations, upload files, or provide hosted tools. Unsupported
request fields fail before model invocation. The adapter supplies a call-scoped translator to the shared execution runtime.
Chat defaults are applied to each native fallback candidate, including
`store: false` and function strictness;
they do not change ProviderWire requests.

Messages accept developer instructions, named participants, assistant refusal
history and multipart text tool results. Developer instructions map to system
instructions and refusal history maps to assistant text for Responses and
Anthropic; compatible Chat candidates retain native role, name and refusal
fields. Participant names have no corresponding common provider field outside
compatible Chat. Message order is preserved.

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

## Images, audio and files

User content arrays accept `image_url`, `input_audio` and `file` parts alongside
text, preserving their order. Images accept HTTP(S) URLs or base64 data URLs.
OpenAI and compatible candidates retain `detail`; Anthropic has no corresponding
resolution control. Inline files accept a base64 data URL or raw base64 with a
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

---

[Prev: Gateway authentication](gateway-authentication.md) · [Docs index](../README.md) · [Next: Gateway architecture](../concepts/gateway-architecture.md)

When a native provider emits several response segments, the adapter keeps one
public completion identity and emits one terminal finish chunk after the runtime
confirms the stream ended. Native raw transport events do not interrupt delivery.
Usage is summed across segments; if a segment omits token totals, the final usage
is unknown instead of an incomplete total. Later native response identities remain
available in `grafana.native_response`.

Execution attribution is exposed in `grafana.provider_metadata.gateway`, using
the same runtime projection and credential redaction as ProviderWire. Native
provider metadata is preserved, with a colliding native `gateway` value relocated
to `gateway.nativeMetadata`. Stream-native failures may include a redacted
`grafana.native_error`; the standard OpenAI error fields remain stable.

Retrieved sources no longer invalidate an otherwise valid completion. They are
preserved under `grafana.sources`; citations are preserved under
`grafana.citations`. URL citations with native start/end positions additionally
appear as standard `message.annotations` or `delta.annotations`. Sources without
positions do not receive fabricated text spans. Positions retain the native
provider's indexing convention.

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
