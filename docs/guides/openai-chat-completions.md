# Use official OpenAI clients with AI Gateway

The Gateway's Chat Completions adapter lets an existing OpenAI client call a
Gateway model. The Grafana App Platform owns authentication and the public
route. For a [stored stack catalog](../../ai-gateway/README.md#call-a-stored-catalog),
use a Grafana service-account token with model invoker access as the client API
key. Set the base URL to the catalog's prefix ending in `/v1`:

```text
https://<grafana-host>/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/<namespace>/modelcatalogs/<name>/v1
```

The SDK appends `/chat/completions`. Use a catalog model ID or alias for
`model`. For the official Go client, retain a trailing slash in the base URL:

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

Catalog Chat uses only the named catalog's models and stored provider keys.
Editors and Admins have the model invoker grant; Viewers cannot invoke. The
catalog route works in either Gateway access mode when the platform supplies the
catalog and decrypted keys. Grafana Cloud does not yet store secure values for
plugin kinds, so this does not enable catalog Chat on deployed Grafana Cloud.

To use the operator catalog instead, set the base URL to
`.../namespaces/<namespace>/v1`. Self-hosted users and service accounts and
allowlisted internal services in either mode can call that namespace route.
Cloud-mode users and service accounts get HTTP 400 on the namespace route
because Chat does not accept request-scoped provider keys (BYOK).
Internal service access tokens use `X-Access-Token` without an `Authorization`
header. See [Gateway authentication](gateway-authentication.md) for credentials.

The adapter supports text, client-executed function tools, tool-result
continuations, JSON output, and streaming for one choice. It does not execute
functions, persist conversations, or provide files or hosted tools. Unsupported
request fields fail before model invocation. The adapter supplies a call-scoped translator to the shared execution runtime.
Caller values, including explicit zero, take precedence. Enabled
[tenant defaults](gateway-stack-defaults.md) fill omitted or null settings and
prepend default instructions only if the caller supplied no system message.
An explicit valid stop sequence wins; an empty stop list is not accepted on
Chat. The adapter applies Chat-specific defaults such as `store: false` and
omitted function strictness separately to each native fallback candidate,
without overwriting the resolved prompt or settings. These Chat defaults do
not change ProviderWire requests.

Structured response and function schemas support self-contained JSON Schema,
including local references and unions produced by SDK helpers such as
`zodResponseFormat`. Schema compilation never loads external files or URLs.
Omitting function parameters declares a function with an empty parameter list.

Response IDs and model names describe the selected native response. Streaming
identity is frozen once public content starts. Late native metadata and warnings
can appear in the `grafana` extension on individual chunks. SDK accumulators may
omit those extensions, so inspect individual chunks when they matter. Chat does
not include the per-call execution overview returned by ProviderWire.

Empty completions are valid. A token limit or content filter can leave function
arguments incomplete: inspect `finish_reason` before parsing or executing a tool
call. Successful complete strict tool calls remain schema-validated.

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
Grafana can refuse catalog access with 403 or report a missing catalog with 404
before the call reaches the plugin. A saved catalog that the plugin cannot
use (for example, when a decrypted key is missing) returns a fixed 424 Chat
error. A Gateway extension can deny a call (403) or fail closed (503) before
stream setup; after output starts, it can end the stream with an in-band
Chat error instead of `[DONE]`.

## Verify the client contract

Run `mise run test-openai-chat-completions-adapter` from the repository root.
The harness builds the candidate plugin and runs it through
`test/platformhost/cmd/plugin-host`, using signed host delegation and a test
service account. It exercises official `openai-go/v3` (the module's pinned
version) and npm `openai@6.27.0` against deterministic local native-provider
fixtures. These fixtures are synthetic tests, not provider recordings.

The suite covers unary and streaming text, tools and continuations, native
identity, usage, strictness/defaults, fallback, SDK errors, cancellation and
access policy. The Go harness also calls a stored catalog in both Gateway
access modes. This is not deployed Grafana or live-provider evidence. The
pinned Grafana Playwright suite separately checks route mounting, catalog
permissions and stored key use.

---

[Prev: Gateway authentication](gateway-authentication.md) · [Docs index](../README.md) · [Next: Gateway architecture](../concepts/gateway-architecture.md)
