package aisdk

import (
	"sort"

	"github.com/grafana/ai-sdk/provider"
)

// ToResponseMessages converts collected response content parts into the
// assistant + tool messages that should be fed into the next call.
//
// It mirrors the upstream Vercel AI SDK helper at
// packages/ai/src/generate-text/to-response-messages.ts. The conversion
// preserves provider-specific metadata on every variant (notably reasoning
// signatures used by Anthropic extended thinking), and routes tool results
// the same way upstream does:
//
//   - text: assistant text part (empty text is skipped)
//   - reasoning: assistant reasoning part with [provider.ContentPart.ProviderOptions]
//     copied from the input (this preserves Anthropic's "signature")
//   - reasoning-file: assistant reasoning-file part (passthrough)
//   - file: assistant file part
//   - custom: assistant custom part (Kind + ProviderOptions)
//   - tool-call: assistant tool-call part (provider-executed flag + options
//     copied through)
//   - tool-result (provider-executed): assistant tool-result part emitted at
//     its input position
//   - tool-result (client-executed): goes in the tool message
//   - tool-approval-request: assistant tool-approval-request part emitted at
//     its input position
//   - tool-approval-response: tool message; if Approved == false, a
//     synthetic execution-denied tool-result is appended for the matching call
//   - sources / unknown types: skipped
//
// The order of parts in the assistant message follows the input order:
// every provider-executed tool-result appears at its own position rather
// than being inlined immediately after the matching tool-call. This
// matches upstream's main-loop traversal and lets public callers control
// the on-wire ordering of interleaved text / reasoning / call / result
// sequences.
//
// Tool-call inputs are preserved verbatim. [StreamText] and [GenerateText]
// sanitize invalid non-object inputs before calling this helper.
//
// The function does not perform any I/O and never returns an error: tool
// output normalization happens earlier via [ToolResult.ModelOutput] and the
// existing [provider.ToolResultOutput] shape, populated during execution
// in [StreamText] by dispatching the per-tool [Tool.ToModelOutput] hook.
//
// The returned slice contains zero, one, or two messages: an assistant
// message (if any assistant content was produced) and a tool message (if
// any tool-results or approval responses were produced), in that order.
//
// Compared to upstream's signature, this function does not take a
// [ToolSet]: the Go port runs [Tool.ToModelOutput] eagerly during tool
// execution and stores the result on [ToolResult.ModelOutput], so by the
// time content reaches this helper the conversion is already done. Public
// callers constructing parts from raw tool output can call
// [Tool.ToModelOutput] directly.
func ToResponseMessages(parts []provider.ContentPart) []provider.Message {
	var messages []provider.Message
	toolCallOrder := make(map[string]int)

	// Build assistant content in input order. Provider-executed tool
	// results stay in the assistant message at their original position;
	// non-provider-executed tool results are deferred to the tool message
	// pass below. This mirrors upstream's main-loop skip filter
	// (`(tool-result || tool-error) && !providerExecuted -> continue`).
	var assistantContent []provider.ContentPart
	for _, p := range parts {
		if p.Type == provider.ContentPartTypeToolResult && !p.ProviderExecuted {
			continue
		}

		switch p.Type {
		case provider.ContentPartTypeText:
			if p.Text == "" {
				continue
			}
			assistantContent = append(assistantContent, provider.ContentPart{
				Type:            provider.ContentPartTypeText,
				Text:            p.Text,
				ProviderOptions: p.ProviderOptions,
			})

		case provider.ContentPartTypeReasoning:
			assistantContent = append(assistantContent, provider.ContentPart{
				Type:            provider.ContentPartTypeReasoning,
				Text:            p.Text,
				ProviderOptions: p.ProviderOptions,
			})

		case provider.ContentPartTypeReasoningFile:
			assistantContent = append(assistantContent, provider.ContentPart{
				Type:            provider.ContentPartTypeReasoningFile,
				Data:            p.Data,
				MediaType:       p.MediaType,
				ProviderOptions: p.ProviderOptions,
			})

		case provider.ContentPartTypeFile:
			assistantContent = append(assistantContent, provider.ContentPart{
				Type:            provider.ContentPartTypeFile,
				Data:            p.Data,
				MediaType:       p.MediaType,
				Filename:        p.Filename,
				ProviderOptions: p.ProviderOptions,
			})

		case provider.ContentPartTypeCustom:
			assistantContent = append(assistantContent, provider.ContentPart{
				Type:            provider.ContentPartTypeCustom,
				Kind:            p.Kind,
				ProviderOptions: p.ProviderOptions,
			})

		case provider.ContentPartTypeToolCall:
			if _, ok := toolCallOrder[p.ToolCallID]; !ok {
				toolCallOrder[p.ToolCallID] = len(toolCallOrder)
			}
			assistantContent = append(assistantContent, provider.ContentPart{
				Type:             provider.ContentPartTypeToolCall,
				ToolCallID:       p.ToolCallID,
				ToolName:         p.ToolName,
				Input:            p.Input,
				ProviderExecuted: p.ProviderExecuted,
				ProviderOptions:  p.ProviderOptions,
			})

		case provider.ContentPartTypeToolResult:
			// Filtered above: only provider-executed results reach here.
			assistantContent = append(assistantContent, provider.ContentPart{
				Type:            provider.ContentPartTypeToolResult,
				ToolCallID:      p.ToolCallID,
				ToolName:        p.ToolName,
				Output:          p.Output,
				ProviderOptions: p.ProviderOptions,
			})

		case provider.ContentPartTypeToolApprovalRequest:
			assistantContent = append(assistantContent, provider.ContentPart{
				Type:            provider.ContentPartTypeToolApprovalRequest,
				ApprovalID:      p.ApprovalID,
				ToolCallID:      p.ToolCallID,
				ToolName:        p.ToolName,
				Signature:       p.Signature,
				Reason:          p.Reason,
				IsAutomatic:     p.IsAutomatic,
				ProviderOptions: p.ProviderOptions,
			})

		default:
			// Unknown / unhandled types (sources, tool-approval-response): skip
			// at the assistant pass.
		}
	}

	if len(assistantContent) > 0 {
		messages = append(messages, provider.NewAssistantMessage(assistantContent...))
	}

	// Second pass: build tool content (non-provider-executed results +
	// approval responses, with denied-approval execution-denied synthesis).
	var toolContent []provider.ContentPart
	for _, p := range parts {
		switch p.Type {
		case provider.ContentPartTypeToolResult:
			if p.ProviderExecuted {
				continue
			}
			toolContent = append(toolContent, provider.ContentPart{
				Type:            provider.ContentPartTypeToolResult,
				ToolCallID:      p.ToolCallID,
				ToolName:        p.ToolName,
				Output:          p.Output,
				ProviderOptions: p.ProviderOptions,
			})

		case provider.ContentPartTypeToolApprovalResponse:
			toolContent = append(toolContent, provider.ContentPart{
				Type:             provider.ContentPartTypeToolApprovalResponse,
				ApprovalID:       p.ApprovalID,
				ToolCallID:       p.ToolCallID,
				ToolName:         p.ToolName,
				Approved:         p.Approved,
				Reason:           p.Reason,
				ProviderExecuted: p.ProviderExecuted,
				ProviderOptions:  p.ProviderOptions,
			})
			if p.Approved != nil && !*p.Approved {
				toolContent = append(toolContent, provider.ContentPart{
					Type:       provider.ContentPartTypeToolResult,
					ToolCallID: p.ToolCallID,
					ToolName:   p.ToolName,
					Output: &provider.ToolResultOutput{
						Type:   provider.ToolOutputExecutionDenied,
						Reason: p.Reason,
					},
				})
			}

		default:
			// Other types are handled in the assistant pass or skipped.
		}
	}

	if len(toolContent) > 0 {
		messages = append(messages, provider.NewToolMessage(sortToolContentByToolCallOrder(toolContent, toolCallOrder)...))
	}

	return messages
}

func sortToolContentByToolCallOrder(content []provider.ContentPart, toolCallOrder map[string]int) []provider.ContentPart {
	if len(content) < 2 || len(toolCallOrder) == 0 {
		return content
	}

	type indexedPart struct {
		part  provider.ContentPart
		index int
	}

	toolResults := make([]indexedPart, 0, len(content))
	for i, part := range content {
		if part.Type == provider.ContentPartTypeToolResult {
			toolResults = append(toolResults, indexedPart{part: part, index: i})
		}
	}
	if len(toolResults) < 2 {
		return content
	}

	sort.SliceStable(toolResults, func(i, j int) bool {
		left, leftOK := toolCallOrder[toolResults[i].part.ToolCallID]
		right, rightOK := toolCallOrder[toolResults[j].part.ToolCallID]
		switch {
		case leftOK && rightOK:
			if left == right {
				return toolResults[i].index < toolResults[j].index
			}
			return left < right
		case leftOK:
			return true
		case rightOK:
			return false
		default:
			return toolResults[i].index < toolResults[j].index
		}
	})

	out := append([]provider.ContentPart(nil), content...)
	resultIndex := 0
	for i, part := range out {
		if part.Type == provider.ContentPartTypeToolResult {
			out[i] = toolResults[resultIndex].part
			resultIndex++
		}
	}
	return out
}
