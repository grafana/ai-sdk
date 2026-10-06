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

## Avoid duplicate retry policy

The underlying Anthropic Go client retries by default, and the core SDK has its
own retry layer. Choose one owner so attempts and latency do not multiply. See
[Retry and timeout](../guides/retry-and-timeout.md).

## Reference

- [`providers/anthropic`](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/anthropic)
- [`AnthropicOptions`](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/anthropic#AnthropicOptions)

---

← [Provider overview](overview.md) · [Docs index](../README.md) · [Amazon Bedrock →](bedrock.md)
