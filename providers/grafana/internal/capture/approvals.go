package main

import (
	"context"
	"encoding/json"
	"fmt"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func captureApprovalReplay(ctx context.Context, model provider.LanguageModel, options provider.CallOptions, mode string, decisions []bool) error {
	executions := 0
	local, err := aisdk.TypedTool(aisdk.TypedToolDef[map[string]any, string]{Name: "local", Execute: func(context.Context, map[string]any, aisdk.ToolExecutionOptions) (string, error) {
		executions++
		return "local-result", nil
	}})
	if err != nil {
		return err
	}
	local.NeedsApproval = &aisdk.ToolApprovalConfig{Required: true}
	hosted := local
	hosted.NeedsApproval = nil
	tools := aisdk.ToolSet{"local": local, "mcp.lookup": hosted}
	history := append([]provider.Message(nil), options.Prompt...)
	var messages []provider.Message
	high := mode == "approval-high-generate" || mode == "approval-high-stream"
	streaming := mode == "approval-stream" || mode == "approval-high-stream"
	invokeHigh := func(prompt []provider.Message) ([]provider.Message, error) {
		opts := []aisdk.Option{aisdk.WithModelMessages(prompt...), aisdk.WithMaxRetries(0)}
		for _, option := range options.ProviderOptions {
			opts = append(opts, aisdk.WithProviderOptions(option))
		}
		if streaming {
			result := aisdk.StreamText(ctx, model, func() []aisdk.StreamOption {
				out := make([]aisdk.StreamOption, len(opts))
				for i, o := range opts {
					out[i] = o
				}
				return append(out, aisdk.WithTools(tools))
			}()...)
			for range result.FullStream() {
			}
			return result.ResponseMessages(), result.Err()
		}
		result, err := aisdk.GenerateText(ctx, model, func() []aisdk.GenerateOption {
			out := make([]aisdk.GenerateOption, len(opts))
			for i, o := range opts {
				out[i] = o
			}
			return append(out, aisdk.WithTools(tools))
		}()...)
		if err != nil {
			return nil, err
		}
		return result.ResponseMessages, nil
	}
	if high {
		var err error
		messages, err = invokeHigh(history)
		if err != nil {
			return err
		}
	} else {
		var content []provider.GenerateContentPart
		if streaming {
			result, err := model.DoStream(ctx, options)
			if err != nil {
				return err
			}
			for p := range result.Stream {
				if p.Type == provider.PartError {
					return fmt.Errorf("approval stream: %v", p.APICallError)
				}
				if p.Type == provider.PartToolCall {
					content = append(content, provider.GenerateContentPart{Type: provider.ContentToolCall, ToolCallID: p.ToolCallID, ToolName: p.ToolName, Input: json.RawMessage(p.Input), ProviderExecuted: p.ProviderExecuted, ProviderMetadata: p.ProviderMetadata})
				}
				if p.Type == provider.PartToolApprovalRequest {
					content = append(content, provider.GenerateContentPart{Type: provider.ContentToolApprovalRequest, ToolCallID: p.ToolCallID, ApprovalID: p.ApprovalID, ProviderMetadata: p.ProviderMetadata})
				}
			}
		} else {
			result, err := model.DoGenerate(ctx, options)
			if err != nil {
				return err
			}
			content = result.Content
		}
		var parts []provider.ContentPart
		for _, p := range content {
			if p.Type == provider.ContentToolCall {
				part := provider.ToolCallPart(p.ToolCallID, p.ToolName, p.Input)
				part.ProviderExecuted = p.ProviderExecuted
				part.ProviderOptions = approvalMetadata(p.ProviderMetadata)
				parts = append(parts, part)
			}
			if p.Type == provider.ContentToolApprovalRequest {
				part := provider.ToolApprovalRequestPart(p.ApprovalID, p.ToolCallID, true)
				part.ProviderOptions = approvalMetadata(p.ProviderMetadata)
				parts = append(parts, part)
			}
		}
		messages = []provider.Message{provider.NewAssistantMessage(parts...)}
	}
	for _, message := range messages {
		for _, approval := range message.Content {
			if approval.Type != provider.ContentPartTypeToolApprovalRequest {
				continue
			}
			for mi := range messages {
				for pi := range messages[mi].Content {
					call := &messages[mi].Content[pi]
					if call.Type == provider.ContentPartTypeToolCall && call.ToolCallID == approval.ToolCallID && call.ProviderExecuted {
						if call.ProviderOptions == nil {
							call.ProviderOptions = provider.ProviderOptions{}
						}
						raw, _ := json.Marshal(map[string]string{"approvalRequestId": approval.ApprovalID})
						call.ProviderOptions["openai"] = provider.RawProviderOption{Key: "openai", Raw: raw}
					}
				}
			}
		}
	}
	before, _ := json.Marshal(messages)
	calls := map[string]provider.ContentPart{}
	for _, msg := range messages {
		for _, part := range msg.Content {
			if part.Type == provider.ContentPartTypeToolCall {
				calls[part.ToolCallID] = part
			}
		}
	}
	var responses []provider.ContentPart
	approvalIDs := map[string]string{}
	for _, msg := range messages {
		for _, part := range msg.Content {
			if part.Type == provider.ContentPartTypeToolApprovalRequest {
				approvalIDs[part.ToolCallID] = part.ApprovalID
				if len(responses) >= len(decisions) {
					return fmt.Errorf("too many approvals")
				}
				response := provider.ToolApprovalResponsePart(part.ApprovalID, decisions[len(responses)], "")
				response.ProviderExecuted = calls[part.ToolCallID].ProviderExecuted
				responses = append(responses, response)
			}
		}
	}
	if len(responses) != len(decisions) {
		return fmt.Errorf("got %d approvals, want %d", len(responses), len(decisions))
	}
	history = append(history, messages...)
	history = append(history, provider.NewToolMessage(responses...))
	if high {
		resumed, err := invokeHigh(history)
		if err != nil {
			return err
		}
		approvedCount, deniedCount := 0, 0
		for i, d := range decisions {
			if i < 2 && d {
				approvedCount++
			}
			if i < 2 && !d && !streaming {
				deniedCount++
			}
		}
		resultCount, denialCount := 0, 0
		for _, msg := range resumed {
			for _, part := range msg.Content {
				if part.Type == provider.ContentPartTypeToolResult && part.ToolName == "mcp.lookup" {
					if _, ok := calls[part.ToolCallID]; !ok {
						return fmt.Errorf("result lost original tool call id")
					}
					if part.Output == nil || part.Output.Type != provider.ToolOutputExecutionDenied {
						resultCount++
						continue
					}
					denialCount++
					correlation, _, err := provider.ResolveOption[struct {
						ApprovalID string `json:"approvalId"`
					}](part.Output.ProviderOptions, "openai")
					if err != nil {
						return err
					}
					if correlation.ApprovalID != approvalIDs[part.ToolCallID] {
						return fmt.Errorf("denied provider result approval id %q, want %q", correlation.ApprovalID, approvalIDs[part.ToolCallID])
					}
				}
			}
		}
		if resultCount != approvedCount {
			return fmt.Errorf("got %d provider results, want %d", resultCount, approvedCount)
		}
		if denialCount != deniedCount {
			return fmt.Errorf("got %d denied provider results, want %d", denialCount, deniedCount)
		}
		history = append(history, resumed...)
		history = append(history, provider.NewUserMessage(provider.TextPart("third-turn")))
		if _, err := invokeHigh(history); err != nil {
			return err
		}
	} else {
		var prompt []provider.Message
		for _, msg := range history {
			copy := msg
			copy.Content = nil
			for _, p := range msg.Content {
				if p.Type == provider.ContentPartTypeToolApprovalRequest {
					continue
				}
				p.ProviderExecuted = p.ProviderExecuted && p.Type == provider.ContentPartTypeToolCall
				copy.Content = append(copy.Content, p)
			}
			prompt = append(prompt, copy)
		}
		options.Prompt = prompt
		if streaming {
			r, err := model.DoStream(ctx, options)
			if err != nil {
				return err
			}
			for p := range r.Stream {
				if p.Type == provider.PartError {
					return fmt.Errorf("approval continuation: %v", p.APICallError)
				}
			}
		} else {
			if _, err := model.DoGenerate(ctx, options); err != nil {
				return err
			}
		}
	}
	after, _ := json.Marshal(messages)
	if string(before) != string(after) {
		return fmt.Errorf("caller history mutated")
	}
	emit(map[string]any{"approvals": responses, "messages": messages, "executions": executions})
	return nil
}
func approvalMetadata(metadata provider.ProviderMetadata) provider.ProviderOptions {
	if metadata == nil {
		return nil
	}
	result := provider.ProviderOptions{}
	for key, raw := range metadata {
		result[key] = provider.RawProviderOption{Key: key, Raw: raw}
	}
	return result
}
