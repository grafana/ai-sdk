package aisdk

import "fmt"

// InvalidToolApprovalError is returned when a tool-approval-response cannot
// be correlated with a prior assistant approval request, or when the response
// itself is structurally invalid (for example, missing the required `approved`
// field).
//
// Mirrors upstream's `InvalidToolApprovalError` from the Vercel AI SDK so
// callers can type-assert via [errors.As] and inspect [InvalidToolApprovalError.ApprovalID].
type InvalidToolApprovalError struct {
	// ApprovalID identifies the approval response that triggered the error.
	ApprovalID string
	// Reason is non-empty when the response is structurally invalid (for
	// example, missing `approved`) and empty when the approval ID has no
	// matching prior assistant request.
	Reason string
}

func (e *InvalidToolApprovalError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("aisdk: invalid tool approval %q: %s", e.ApprovalID, e.Reason)
	}
	return fmt.Sprintf("aisdk: invalid tool approval %q", e.ApprovalID)
}

// InvalidToolApprovalSignatureError is returned when a signed tool approval
// request is replayed without a valid signature.
type InvalidToolApprovalSignatureError struct {
	ApprovalID string
	ToolCallID string
	Reason     string
}

func (e *InvalidToolApprovalSignatureError) Error() string {
	return fmt.Sprintf("aisdk: tool approval signature verification failed for approval %q (tool call %q): %s", e.ApprovalID, e.ToolCallID, e.Reason)
}

// ToolCallNotFoundForApprovalError is returned when an approval request's
// referenced tool call cannot be found in any prior assistant message.
//
// Mirrors upstream's `ToolCallNotFoundForApprovalError` from the Vercel AI SDK
// so callers can type-assert via [errors.As] and inspect both
// [ToolCallNotFoundForApprovalError.ToolCallID] and
// [ToolCallNotFoundForApprovalError.ApprovalID].
type ToolCallNotFoundForApprovalError struct {
	ToolCallID string
	ApprovalID string
}

func (e *ToolCallNotFoundForApprovalError) Error() string {
	return fmt.Sprintf("aisdk: tool call %q not found for approval %q", e.ToolCallID, e.ApprovalID)
}

// ToolNotExecutableError is returned when an approved tool call cannot be
// executed locally because the tool is unknown to the current invocation or
// has no `Execute` function configured.
type ToolNotExecutableError struct {
	ToolName string
}

func (e *ToolNotExecutableError) Error() string {
	return fmt.Sprintf("aisdk: approved tool %q is not executable", e.ToolName)
}
