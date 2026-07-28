# Tool approval

Approval pauses a tool call before execution so a person or application policy
can accept or deny it. Use it for actions with meaningful side effects: changing
access, deleting data, sending messages, spending money, or operating outside a
safe read-only scope.

Approval is different from an external tool. An external tool always runs
somewhere else; an approval-gated tool still has a local executor, but cannot run
until a decision is present.

## Require approval on a tool

Require every invocation:

```go
transfer := aisdk.Tool{
	Description:   "Transfer money between accounts.",
	InputSchema:   transferSchema,
	NeedsApproval: aisdk.ApprovalRequired(),
	Execute:       executeTransfer,
}
```

Or decide from the proposed input:

```go
deleteFile.NeedsApproval = aisdk.ApprovalIf(
	func(input json.RawMessage, _ aisdk.ToolExecutionOptions) (bool, error) {
		var request struct {
		Path string `json:"path"`
		}
		if err := json.Unmarshal(input, &request); err != nil {
			return false, err
		}
		return !strings.HasPrefix(request.Path, "/tmp/"), nil
	},
)
```

Call-level policy set with `WithToolApproval` overrides the tool's own
`NeedsApproval` rule. Use call-level policy for request-, tenant-, or
user-specific authorization; keep inherent safety requirements on the tool.

## Understand the resume flow

Approval spans two model invocations:

```text
first call
  model requests tool
  → SDK emits approval request
  → tool does not run
  → invocation finishes

application records approve/deny decision
  → decision is appended to the same UI message history

second call
  SDK reads the decision
  → approved local tools execute
  → denied tools receive a denial result
  → model continues with the outcome
```

For `useChat`, persist the complete `UIMessage` parts and submit the updated
history after the user decides. Pending approvals intentionally stop the current
invocation; do not keep the HTTP request open while waiting for a person.

## Protect persisted approvals

When approval state crosses an untrusted client, sign requests with a server
secret:

```go
result := aisdk.StreamText(ctx, model,
	aisdk.WithMessages(messages...),
	aisdk.WithTools(tools),
	aisdk.WithToolApprovalSecret(os.Getenv("TOOL_APPROVAL_SECRET")),
)
```

Use the same secret when creating and resuming the approval. The signature binds
the approval ID, tool call ID, tool name, and proposed input so modified client
state is rejected. Keep the secret server-side and rotate it with a deliberate
migration strategy for in-flight approvals.

Signing prevents message tampering; it does not replace authorization. Recheck
the current user, tenant, resource permissions, and business invariants inside
`Execute` immediately before the side effect.

## Handle denial as a normal outcome

A denial is not an infrastructure error. Preserve the optional reason and let the
model explain or offer a safe alternative. Log the decision without recording
sensitive tool input unless your retention policy permits it.

## Reference

- [`ApprovalRequired` and `ApprovalIf`](https://pkg.go.dev/github.com/grafana/ai-sdk#ApprovalRequired)
- [`WithToolApproval`](https://pkg.go.dev/github.com/grafana/ai-sdk#WithToolApproval)
- [`WithToolApprovalSecret`](https://pkg.go.dev/github.com/grafana/ai-sdk#WithToolApprovalSecret)

---

← [Tools](tools.md) · [Docs index](../README.md) · [Structured output →](structured-output.md)
