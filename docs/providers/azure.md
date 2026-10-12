# Microsoft Azure

Use the Azure provider for Claude through Microsoft Foundry while keeping
canonical model IDs separate from Azure deployment names.

## Install

```bash
go get github.com/grafana/ai-sdk/providers/azure
```

## Call Claude through Microsoft Foundry

Pass the canonical Claude model ID and the Azure deployment name separately:

```go
model, err := azure.NewAnthropic(foundry.Config{
	BaseURL: "https://my-resource.services.ai.azure.com/anthropic",
	APIKey:  os.Getenv("AZURE_FOUNDRY_API_KEY"),
}, "claude-sonnet-4-6", "my-sonnet-deployment")
if err != nil {
	return err
}
```

Import `foundry` from
`github.com/grafana/ai-sdk/providers/azure/foundry`. The canonical ID drives
model capabilities and is returned in generation and stream metadata, with
provider identity `azure`. Only the request's model field uses the deployment
name. Configure the canonical ID to match the model deployed in Azure.

Configuration is explicit: ambient Anthropic or Foundry settings cannot supply
the endpoint or credential. API keys use `x-api-key`; Entra tokens use bearer
authentication. For rotating secrets or Entra tokens, supply a credential
callback that returns exactly one credential on every HTTP attempt, including
retries. The application owns token caching, refresh, and secret-file reads.
The default HTTP client refuses redirects to avoid forwarding credentials.

Applications using `anthropic-sdk-go` directly can use
[`foundry.Config.RequestOptions`](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/azure/foundry#Config.RequestOptions)
to share the same endpoint and authentication contract. Calls use the native
Messages endpoint and Anthropic version header, without the older preview
`api-version` query parameter.

Generation, tools, reasoning and usage reuse the Anthropic adapter. Feature
availability depends on the pinned adapter, model and deployment hosting version; see [Claude in Microsoft Foundry](https://platform.claude.com/docs/en/build-with-claude/claude-in-microsoft-foundry).
Approved deployments, provider fallback, external tool access, and residency
restrictions remain application and infrastructure policy. Creating this model
does not establish a residency guarantee.

## Compatibility

This module composes the published Anthropic provider and supports its existing
LanguageModel contract. Streaming identity normalization lives in this provider;
applications do not need custom middleware or stream forwarding. The module can
be adopted without upgrading the core SDK across unrelated Gateway changes.

## Reference

- [`providers/azure`](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/azure)
- [`foundry.Config`](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/azure/foundry#Config)

---

← [Anthropic](anthropic.md) · [Docs index](../README.md) · [Amazon Bedrock →](bedrock.md)
