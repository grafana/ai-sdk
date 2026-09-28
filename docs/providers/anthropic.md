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

Anthropic-specific options also cover effort, beta features, remote MCP servers,
containers, task budgets, and tool streaming. Enable only options supported by
the chosen model.

## Request dangerous-tool-use classification

The safeguard classifier is opt-in. Confirm with Anthropic that the
`dangerous-tool-use-2026-09-03` beta is available for your credential, model,
and endpoint before using it in production; availability through the direct
API or Vertex AI has not been established by local tests. A rejected beta
request returns an API error rather than retrying without safeguards.

```go
result := aisdk.StreamText(ctx, model,
	aisdk.WithModelMessages(provider.UserText("Use the available tools")),
	aisdk.WithProviderOptions(anthropic.AnthropicOptions{
		Safeguards: []anthropic.AnthropicSafeguard{{
			Type: anthropic.AnthropicSafeguardDangerousToolUse,
		}},
	}),
)
for range result.FullStream() {
}
if err := result.Err(); err != nil {
	return err
}
verdictMetadata := result.ProviderMetadata()["anthropic"]
```

The provider sends the safeguard request and beta only when the list is
nonempty. When Anthropic returns verdicts, `safeguardResults` in Anthropic
provider metadata retains the API's nested `tool_uses` keys, including tool
call IDs. Streaming exposes the last non-null verdict. Classifier context is
optional and forwarded as `classifier_context`; provide it only when needed.
The verdict is data for your application, **not** a tool-execution policy or
automatic permission check. Focused fake-HTTP tests verify serialization and
mapping, but no live provider safeguard recording is available yet.

## Avoid duplicate retry policy

The underlying Anthropic Go client retries by default, and the core SDK has its
own retry layer. Choose one owner so attempts and latency do not multiply. See
[Retry and timeout](../guides/retry-and-timeout.md).

## Reference

- [`providers/anthropic`](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/anthropic)
- [`AnthropicOptions`](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/anthropic#AnthropicOptions)

---

← [Provider overview](overview.md) · [Docs index](../README.md) · [Amazon Bedrock →](bedrock.md)
