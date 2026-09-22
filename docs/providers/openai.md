# OpenAI

Use the OpenAI provider for OpenAI's Responses API. Choose it over the
OpenAI-compatible provider when you need Responses features such as native
reasoning, conversation continuation, built-in tools, and citations.

## Install

```bash
go get github.com/grafana/ai-sdk/providers/openai
```

## Create a model

```go
model := openai.NewResponses(
	os.Getenv("OPENAI_API_KEY"),
	"gpt-5",
)
```

Continue with [Generate text from Go](../getting-started/backend-only.md) or
[Full-stack chat](../getting-started/full-stack-chat.md). The same model also
works with tools, structured output, middleware, and Agents.

## Reuse Responses in a provider integration

Provider packages whose APIs implement OpenAI Responses semantics can call
`NewResponsesWithClient` with a preconfigured official OpenAI SDK client. The
model preserves the client's endpoint, authentication, headers, retries, and
transport. Set the integrating provider's model identity with
`WithProviderName`; for a custom identity this changes `Provider()` while
resolving the Responses metadata namespace once when the model is constructed.
Identities containing `"azure"` use the `"azure"` namespace; other custom
identities use `"openai"`. Empty names are ignored so optional configuration
cannot erase the default `"openai"` identity.

This is a composition seam, not a generic endpoint authentication feature. The
integrating provider owns its client configuration and policy. OpenAI-specific
call options and response metadata remain under that stable `"openai"` or
`"azure"` provider-options key because request conversion still follows OpenAI
Responses semantics. Keeping the namespace stable across calls preserves item
references, encrypted reasoning, tool namespaces, and approval correlation even
when a continuation call has no top-level provider options. For compatibility,
the existing `NewResponses` path with the default `"openai"` identity retains
its per-call OpenAI-first, Azure-fallback option resolution.

## Configure a call

Set request-scoped headers with `aisdk.WithHeaders`. Headers configured through
`openai.WithRequestOptions` apply to every request; a per-call header overrides
a configured header with the same name.

Use typed OpenAI options for behavior that is not part of the common model
contract:

```go
store := false
result := aisdk.StreamText(ctx, model,
	aisdk.WithProviderOptions(openai.OpenAIResponsesOptions{
		Store:            &store,
		ReasoningEffort:  "high",
		ReasoningSummary: "auto",
	}),
)
```

Not every option applies to every model. Provider options cover conversation
IDs, response continuation, reasoning, storage, metadata, service tier, tool
controls, text verbosity, and other Responses settings.

## Choose conversation storage behavior

The Responses API can reference stored response items or resend prior content
inline. Set `Store` to `false` for stateless or Zero Data Retention workflows.
The provider preserves encrypted reasoning content where needed for stateless
multi-turn reasoning.

This provider behavior does not replace application chat persistence. Continue
to store the `UIMessage` history needed by your frontend.

## Use built-in tools

Built-in tools are requested through provider tools keyed by their OpenAI ID,
including `openai.web_search`, `openai.code_interpreter`, `openai.file_search`,
`openai.mcp`, `openai.computer`, and `openai.programmatic_tool_calling`.
Hosted-tool calls and results are surfaced as provider-executed parts, and
web-search citations become source parts.

`openai.computer` is client-executed. Its tool-call input contains ordered
computer actions and pending safety checks. Return a JSON tool result containing
a `computer_screenshot` by `imageUrl` or `fileId`, plus any acknowledged safety
checks; the provider converts it to a Responses `computer_call_output`.

`openai.programmatic_tool_calling` lets OpenAI execute a hosted JavaScript
program that can invoke function tools. Function-tool `OpenAIToolOptions` can
restrict `AllowedCallers` and declare an `OutputSchema`; the provider preserves
program fingerprints, caller correlation, and program outputs across stateless
and stored continuations.

Provider-executed tools are different from Go tools with an `Execute` function:
they run in OpenAI's environment and follow OpenAI's data and security model.

## Handle unsupported generic settings

Responses does not accept every Chat Completions parameter. Unsupported generic
settings are dropped with warnings where the request can proceed. Inspect
`Warnings` in tests and operational telemetry when portable behavior matters.

## Reference

- [`providers/openai`](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/openai)
- [`OpenAIResponsesOptions`](https://pkg.go.dev/github.com/grafana/ai-sdk/providers/openai#OpenAIResponsesOptions)

---

← [Amazon Bedrock](bedrock.md) · [Docs index](../README.md) · [OpenAI-compatible →](openai-compatible.md)
