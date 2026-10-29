package execution

import "github.com/grafana/ai-sdk/provider"

type unaryTool struct {
	historical       bool
	providerExecuted bool
	approvalID       string
	name             string
	preview          bool
	final            bool
}

// ValidateGenerateResult checks the output lifecycle of a unary result against
// the prompt's [ToolHistory], and returns [ErrInvalidOutput] for a nil result
// or one that breaks a rule. The rules are: only tool calls and results may be
// provider-executed, dynamic or preliminary; a tool call has an ID and a name,
// is not preliminary, and repeats an ID only as one provider-executed
// re-emission, with the same name, of a history call with no preview or final
// result; an approval names a provider-executed call awaiting its first result
// and not yet approved, with an unused approval ID; a result names a known call
// and its tool, is not repeated after a final result, and holds a JSON value
// other than null; no preview is left without a final result; and the unified
// finish reason is registered. It does not modify history.
func ValidateGenerateResult(result *provider.GenerateResult, history ToolHistory) error {
	if result == nil {
		return ErrInvalidOutput
	}
	tools := make(map[string]unaryTool, len(history))
	for id, name := range history {
		tools[id] = unaryTool{name: name, providerExecuted: true, historical: true}
	}
	approvals := make(map[string]struct{})
	for _, part := range result.Content {
		if part.Type != provider.ContentToolCall && part.Type != provider.ContentToolResult && (part.ProviderExecuted || isTrue(part.Dynamic) || isTrue(part.Preliminary)) {
			return ErrInvalidOutput
		}
		switch part.Type {
		case provider.ContentSource, provider.ContentText, provider.ContentCustom, provider.ContentReasoning, provider.ContentReasoningFile, provider.ContentFile:
		case provider.ContentToolCall:
			if part.ToolCallID == "" || part.ToolName == "" || isTrue(part.Preliminary) {
				return ErrInvalidOutput
			}
			prior, exists := tools[part.ToolCallID]
			if exists && (!prior.historical || prior.final || prior.preview || !part.ProviderExecuted || prior.name != part.ToolName) {
				return ErrInvalidOutput
			}
			tools[part.ToolCallID] = unaryTool{name: part.ToolName, providerExecuted: part.ProviderExecuted, approvalID: prior.approvalID}
		case provider.ContentToolApprovalRequest:
			tool, exists := tools[part.ToolCallID]
			_, duplicate := approvals[part.ApprovalID]
			if !exists || !tool.providerExecuted || tool.final || tool.preview || tool.approvalID != "" || duplicate || part.ApprovalID == "" {
				return ErrInvalidOutput
			}
			tool.approvalID = part.ApprovalID
			tools[part.ToolCallID] = tool
			approvals[part.ApprovalID] = struct{}{}
		case provider.ContentToolResult:
			tool, exists := tools[part.ToolCallID]
			if !exists || tool.name != part.ToolName || tool.final || part.ToolCallID == "" || part.ToolName == "" || !validToolResult(part.Result) {
				return ErrInvalidOutput
			}
			tool.preview = isTrue(part.Preliminary)
			tool.final = !tool.preview
			tools[part.ToolCallID] = tool
		default:
			return ErrInvalidOutput
		}
	}
	for _, tool := range tools {
		if tool.preview {
			return ErrInvalidOutput
		}
	}
	if !registeredFinishReason(result.FinishReason.Unified) {
		return ErrInvalidOutput
	}
	return nil
}
