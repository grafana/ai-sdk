package aisdk

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

func validateAgentUIMessages(messages []UIMessage, tools ToolSet) ([]UIMessage, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("aisdk: validating UI messages: empty history")
	}
	normalized := make([]UIMessage, len(messages))
	for msgIdx, msg := range messages {
		if msg.Role != RoleAssistant && msg.Role != RoleUser && msg.Role != RoleSystem {
			return nil, fmt.Errorf("aisdk: validating UI message %d: unsupported role %q", msgIdx, msg.Role)
		}
		if msg.Role != RoleAssistant && len(msg.Parts) == 0 {
			return nil, fmt.Errorf("aisdk: validating UI message %d: empty parts", msgIdx)
		}
		if len(msg.Metadata) > 0 && !json.Valid(msg.Metadata) {
			return nil, fmt.Errorf("aisdk: validating UI message %d: invalid metadata JSON", msgIdx)
		}
		normalized[msgIdx] = cloneUIMessage(msg)
		for partIdx, part := range normalized[msgIdx].Parts {
			converted, err := validateAgentUIPart(part, tools)
			if err != nil {
				return nil, fmt.Errorf("aisdk: validating UI message %d part %d: %w", msgIdx, partIdx, err)
			}
			normalized[msgIdx].Parts[partIdx] = converted
		}
	}
	return normalized, nil
}

func validateAgentUIPart(part Part, tools ToolSet) (Part, error) {
	if _, err := marshalPart(part); err != nil {
		return nil, err
	}
	switch p := part.(type) {
	case ToolInvocationPart:
		fields := toolPartFields(p)
		if err := validateToolUIState(fields); err != nil {
			return nil, err
		}
		tool, exists := tools[p.ToolName]
		terminal := p.State == ToolStateOutputAvailable || p.State == ToolStateOutputError || p.State == ToolStateOutputDenied
		if !exists {
			if terminal {
				return DynamicToolUIPart(fields), nil
			}
			return nil, fmt.Errorf("tool %q is not configured on agent", p.ToolName)
		}
		toDynamic := false
		if tool.InputSchema.JSON() != nil && p.State != ToolStateInputStreaming && (p.State != ToolStateOutputError || p.Input != nil) {
			if err := tool.InputSchema.Validate(p.Input); err != nil {
				if p.State == ToolStateOutputError || (p.State == ToolStateOutputAvailable && isEmptyJSONObj(p.Input)) {
					toDynamic = true
				} else {
					return nil, fmt.Errorf("validating tool %q input: %w", p.ToolName, err)
				}
			}
		}
		if p.State == ToolStateOutputAvailable && tool.OutputSchema.JSON() != nil {
			if err := tool.OutputSchema.Validate(p.Output); err != nil {
				return nil, fmt.Errorf("validating tool %q output: %w", p.ToolName, err)
			}
		}
		if toDynamic {
			return DynamicToolUIPart(fields), nil
		}
	case DynamicToolUIPart:
		if p.State != ToolStateOutputError {
			p.RawInput = nil
		}
		if err := validateToolUIState(toolPartFields(p)); err != nil {
			return nil, err
		}
		return p, nil
	case TextPart:
		if err := validateUIContentState(p.State); err != nil {
			return nil, err
		}
		if err := validateProviderMetadata(p.ProviderMetadata); err != nil {
			return nil, err
		}
	case ReasoningPart:
		if err := validateUIContentState(p.State); err != nil {
			return nil, err
		}
		if err := validateProviderMetadata(p.ProviderMetadata); err != nil {
			return nil, err
		}
	case DataPart:
		if p.DataName == "" || len(p.Data) == 0 {
			return nil, fmt.Errorf("data part is missing name or data")
		}
	case FilePart:
		if err := validateProviderMetadata(p.ProviderMetadata); err != nil {
			return nil, err
		}
	case ReasoningFilePart:
		if err := validateProviderMetadata(p.ProviderMetadata); err != nil {
			return nil, err
		}
	case SourceURLPart:
		if err := validateProviderMetadata(p.ProviderMetadata); err != nil {
			return nil, err
		}
	case SourceDocumentPart:
		if err := validateProviderMetadata(p.ProviderMetadata); err != nil {
			return nil, err
		}
	case CustomPart:
		if err := validateProviderMetadata(p.ProviderMetadata); err != nil {
			return nil, err
		}
	case StepStartPart:
	default:
		return nil, fmt.Errorf("unsupported UI part %T", part)
	}
	return part, nil
}

func validateUIContentState(state string) error {
	if state != "" && state != "streaming" && state != "done" {
		return fmt.Errorf("unknown content state %q", state)
	}
	return nil
}

func isEmptyJSONObj(raw json.RawMessage) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && object != nil && len(object) == 0
}

func validateToolUIState(part toolPartFields) error {
	for _, metadata := range []provider.ProviderMetadata{part.CallProviderMetadata, part.ResultProviderMetadata} {
		if err := validateProviderMetadata(metadata); err != nil {
			return err
		}
	}
	if part.ToolCallID == "" || part.ToolName == "" {
		return fmt.Errorf("tool invocation has empty tool call ID or tool name")
	}
	if !isKnownToolInvocationState(part.State) {
		return fmt.Errorf("unknown tool invocation state %q", part.State)
	}
	if toolInvocationStateRequiresInput(part.State) && len(part.Input) == 0 {
		return fmt.Errorf("tool invocation %q in state %q is missing input", part.ToolCallID, part.State)
	}
	available := part.State == ToolStateOutputAvailable
	failed := part.State == ToolStateOutputError
	if available != (len(part.Output) > 0) {
		return fmt.Errorf("tool invocation %q has invalid output for state %q", part.ToolCallID, part.State)
	}
	if failed != (part.ErrorText != nil) {
		return fmt.Errorf("tool invocation %q has invalid error text for state %q", part.ToolCallID, part.State)
	}
	if !available && part.Preliminary != nil {
		return fmt.Errorf("preliminary is only valid on output-available")
	}
	if !failed && part.RawInput != nil {
		return fmt.Errorf("raw input is only valid on output-error")
	}
	if !available && !failed && part.ResultProviderMetadata != nil {
		return fmt.Errorf("result metadata is not valid in state %q", part.State)
	}
	approval := part.Approval
	switch part.State {
	case ToolStateInputStreaming, ToolStateInputAvailable:
		if approval != nil {
			return fmt.Errorf("approval is not valid in state %q", part.State)
		}
	case ToolStateApprovalRequested:
		if approval == nil || approval.ID == "" || approval.Approved != nil || approval.Reason != nil {
			return fmt.Errorf("tool invocation %q is missing approval request or has response fields", part.ToolCallID)
		}
	case ToolStateApprovalResponded, ToolStateOutputDenied:
		if approval == nil || approval.ID == "" || approval.Approved == nil {
			return fmt.Errorf("tool invocation %q is missing approval response", part.ToolCallID)
		}
		if part.State == ToolStateOutputDenied && *approval.Approved {
			return fmt.Errorf("output-denied requires a denied approval")
		}
	case ToolStateOutputAvailable, ToolStateOutputError:
		if approval != nil && (approval.ID == "" || approval.Approved == nil || !*approval.Approved) {
			return fmt.Errorf("tool output requires an approved approval when present")
		}
	}
	return nil
}
