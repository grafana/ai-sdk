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

Self-hosted users and service accounts use the configured catalog. In cloud
mode only allowlisted internal service callers can use this adapter; cloud
user and service-account requests lack the required BYOK controls and receive
HTTP 400 from shared selection, without native provider I/O. A caller rejected
by the host access policy receives HTTP 403. Internal service access tokens use
the platform's `X-Access-Token` header without an `Authorization` header; configure
the SDK's request headers for that caller type. Request-scoped provider keys
(BYOK) are not supported. Provider credentials come from the Gateway's configured
providers. See [Gateway authentication](gateway-authentication.md) for the
platform credential contract.

The adapter supports text, client-executed function tools, tool-result
continuations, JSON output, and streaming for one choice. It does not execute
functions, persist conversations, or provide files or hosted tools. Unsupported
request fields fail before model invocation. The adapter supplies a call-scoped translator to the shared execution runtime.
Chat defaults are applied to each native fallback candidate, including
`store: false` and function strictness;
they do not change ProviderWire requests.

Response IDs and model names describe the selected native response. Streaming
identity is frozen once public content starts. Late native metadata and warnings
can appear in the `grafana` extension on individual chunks. SDK accumulators may
omit those extensions, so inspect individual chunks when they matter.

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
