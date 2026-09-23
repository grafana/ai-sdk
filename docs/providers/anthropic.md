# Anthropic

Use the Anthropic provider for Claude through the direct Anthropic API or Google
Vertex AI. It supports the common generation, streaming, tool, reasoning, and
structured-output workflows of the core SDK.

## Install

```bash
go get github.com/grafana/ai-sdk/providers/anthropic
```

## Call the direct API

```go
model := anthropic.New(
	os.Getenv("ANTHROPIC_API_KEY"),
	"claude-sonnet-5",
)
```

Pass the model to [Generate text from Go](../getting-started/backend-only.md) or
[Full-stack chat](../getting-started/full-stack-chat.md). Credential errors
appear on the first model call. Create the model once and reuse its underlying
HTTP resources across requests.

## Configure call headers

Use `aisdk.WithHeaders` for request-scoped headers. Ordinary call headers override
configured headers with the same name. Anthropic beta headers instead combine
configured, per-call, and feature-required tokens into a normalized, deduplicated
union. The same behavior applies to direct API and Vertex calls.

Native call results retain outbound JSON and response headers for diagnostics.
These can contain sensitive prompt or backend data; do not automatically log or
forward them. Raw streaming events are opt-in and remain separate from normalized
content and frontend UI streams.

## Use Vertex AI

`NewVertex` resolves Google Application Default Credentials and can fail during
setup:

```go
model, err := anthropic.NewVertex(
	ctx,
	"us-east5",
	"my-project",
	"claude-sonnet-5",
)
if err != nil {
	return err
}
```

Use the model IDs supported by the selected Anthropic or Vertex endpoint. The
package exposes model-ID helpers for discovery; availability still depends on
your account and region.

Vertex uses native `output_config.format` for JSON-schema responses on models
that support structured output, rather than forcing a synthetic tool call.
Your Google Cloud organization must allow the `structured_outputs` feature in
`constraints/vertexai.allowedPartnerModelFeatures`; see
[Google Cloud's structured-output guide](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/partner-models/claude/structured-outputs).
The explicit `StructuredOutputJSONTool` mode remains available for models that
support forced tool choice.

## Enable reasoning deliberately

```go
result := aisdk.StreamText(ctx, model,
	aisdk.WithProviderOptions(anthropic.AnthropicOptions{
		Thinking: &anthropic.ThinkingConfig{
			Type:         anthropic.ThinkingEnabled,
			BudgetTokens: 10_000,
		},
	}),
)
```

Reasoning increases token usage and latency. Decide whether reasoning content
should be forwarded to a frontend; UI streams include it by default unless
configured otherwise.

Some models always think and reject forced tool use. On `claude-sonnet-5-5`,
disabled and budget-based thinking, a `required` tool choice, and a named tool
choice all fail at the API. The provider rewrites them instead: root reasoning
`none` sends `between_tools` thinking, the model's lowest setting, and a forced
tool choice is sent as `auto` with an `unsupported` warning. Tell the model in
the prompt to call the tool, and check the result for the tool call.

Anthropic-specific options also cover effort, beta features, remote MCP servers,
containers, task budgets, and tool streaming. Enable only options supported by
the chosen model. For non-streaming calls with a large model-default output
budget, set a request context deadline: `DoGenerate` uses its remaining time
as the Anthropic SDK attempt timeout, and the context still bounds the call.
An explicit SDK request timeout supplied through model options takes
precedence. Without either deadline or explicit timeout, the SDK may require
streaming rather than issuing a long-running unary request. The Gateway adds
its configured model-duration deadline automatically.

MCP server authorization tokens and the tool-configuration
`enabled` flag are presence-aware: nil omits them; pointers to `""` or `false`
forward those explicit values. A non-nil empty `allowedTools` slice forwards an
empty array rather than omitting the list. Callers previously constructing
these Go structs with string/bool fields must supply pointers for explicit
values. Configure remote servers only for callers and models authorized to use
them; tokens can appear in caller-owned request metadata. These native-provider
options do not imply that the Gateway service accepts MCP configuration.

## Avoid duplicate retry policy

The underlying Anthropic Go client retries by default, and the core SDK has its
own retry layer. Choose one owner so attempts and latency do not multiply. See
[Retry and timeout](../guides/retry-and-timeout.md).

## Reference

- [`providers/anthropic`](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/anthropic)
- [`AnthropicOptions`](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/anthropic#AnthropicOptions)

---

← [Provider overview](overview.md) · [Docs index](../README.md) · [Amazon Bedrock →](bedrock.md)
