# Security

Treat the model as an untrusted decision-maker. It can propose actions and
content, but application code remains responsible for authentication,
authorization, validation, and containment.

## Give tools least authority

A tool should expose one narrow capability with the minimum credentials it
needs. Validate model-generated input before using it in SQL, shell commands,
file paths, URLs, cloud APIs, or account operations.

- Allowlist reachable hosts, paths, resources, and operations.
- Use parameterized database APIs for query values.
- Apply tenant and user authorization inside `Execute`.
- Bound output size and execution time.
- Make side effects idempotent where requests can be repeated.
- Avoid general-purpose shell or unrestricted HTTP tools.

Schema validation protects shape, not authorization or intent.

## Require approval for consequences

Use [tool approval](../guides/tool-approval.md) for destructive, external, or
high-value actions. When approval state travels through a browser, sign it with
`WithToolApprovalSecret` and verify it on resume.

Approval is not a substitute for authorization. Recheck current permissions and
resource state immediately before execution, even after a user approved the
proposal.

## Defend against prompt injection

User messages, retrieved documents, websites, files, tool outputs, and remote MCP
responses can contain instructions intended to redirect the model. Treat them as
data, not trusted policy.

Keep authorization and tool restrictions in application code. Do not rely on a
system prompt to prevent access to a tool or secret. Separate trusted
instructions from untrusted content, and require deterministic checks around
sensitive actions.

## Protect credentials and content

- Store provider keys and CAP tokens in a secret manager or environment.
- Never send server credentials to the browser or place them in prompts.
- Avoid logging prompts, reasoning, tool payloads, files, and outputs by default.
- Review provider retention and training settings for the selected service.
- Consider where provider-executed tools and remote MCP servers send data.
- Apply retention and deletion policy to persisted `UIMessage` history.

Redaction reduces accidental exposure but does not make unnecessary capture
safe.

## Restrict model selection

Do not pass arbitrary client model IDs directly to a provider constructor. Map
allowed names through a custom provider or gateway catalog and apply the same
entitlement policy to model listing and resolution.

```go
models := registry.NewCustomProvider(
	registry.WithLanguageModels(map[string]provider.LanguageModel{
		"fast":    fastModel,
		"quality": qualityModel,
	}),
)
```

A model allowlist also limits unexpected cost and capability changes.

## Bound resource consumption

Limit body size, message history, uploaded files, output tokens, steps, tool
concurrency, retries, fallback candidates, and total duration. Apply rate limits
and budget controls at an authenticated application boundary.

A single user request can trigger several provider calls. Monitor token usage,
step count, and provider attempts for each request; HTTP counts alone do not
capture the work performed.

## Return safe failures

Map internal errors to stable client messages after a stream starts. Keep
provider bodies, policy rules, stack traces, and internal identifiers in
protected telemetry. See [Error handling](error-handling.md).

---

← [Error handling](error-handling.md) · [Docs index](../README.md) · [Operate in production](../README.md#operate-in-production)
