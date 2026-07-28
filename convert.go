package aisdk

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

type convertConfig struct {
	ignoreIncompleteToolCalls bool
	tools                     ToolSet
}

// ConvertOption configures ConvertToModelMessages.
type ConvertOption interface {
	applyConvert(*convertConfig)
	convertOption()
}

type convertOptionFunc func(*convertConfig)

func (f convertOptionFunc) applyConvert(cfg *convertConfig) { f(cfg) }
func (convertOptionFunc) convertOption()                    {}

// WithIgnoreIncompleteToolCalls skips tool calls whose input is not complete
// when converting UI messages to model messages.
func WithIgnoreIncompleteToolCalls() ConvertOption {
	return convertOptionFunc(func(cfg *convertConfig) {
		cfg.ignoreIncompleteToolCalls = true
	})
}

func buildConvertConfig(opts []ConvertOption) convertConfig {
	var cfg convertConfig
	for _, opt := range opts {
		if opt != nil {
			opt.applyConvert(&cfg)
		}
	}
	return cfg
}

// ConvertToModelMessages converts UIMessages to provider.Message slice
// suitable for passing to provider.LanguageModel.DoStream/DoGenerate.
func ConvertToModelMessages(messages []UIMessage, opts ...ConvertOption) ([]provider.Message, error) {
	opt := buildConvertConfig(opts)

	var result []provider.Message

	for _, msg := range messages {
		switch msg.Role {
		case RoleSystem:
			text, opts := extractSystemContent(msg.Parts)
			if text != "" || len(opts) > 0 {
				result = append(result, provider.Message{
					Role: provider.RoleSystem,
					Content: []provider.ContentPart{
						{Type: provider.ContentPartTypeText, Text: text},
					},
					ProviderOptions: opts,
				})
			}

		case RoleUser:
			var parts []provider.ContentPart
			for _, p := range msg.Parts {
				switch v := p.(type) {
				case TextPart:
					parts = append(parts, provider.ContentPart{
						Type:            provider.ContentPartTypeText,
						Text:            v.Text,
						ProviderOptions: providerMetadataToOptions(v.ProviderMetadata),
					})
				case FilePart:
					data, mediaType, err := filePartData(v.URL, v.MediaType, v.ProviderReference)
					if err != nil {
						return nil, fmt.Errorf("converting user file part: %w", err)
					}
					parts = append(parts, provider.ContentPart{
						Type:            provider.ContentPartTypeFile,
						Data:            &data,
						MediaType:       mediaType,
						Filename:        v.Filename,
						ProviderOptions: providerMetadataToOptions(v.ProviderMetadata),
					})
				}
			}
			if len(parts) > 0 {
				result = append(result, provider.NewUserMessage(parts...))
			}

		case RoleAssistant:
			var assistParts []provider.ContentPart
			var toolMsgParts []provider.ContentPart

			// flushBlock emits the accumulated assistant and tool messages,
			// mirroring the TypeScript SDK's step-based block splitting.
			// Each step produces at most one assistant message followed by
			// one tool message, ensuring tool_use/tool_result ordering.
			flushBlock := func() {
				if len(assistParts) > 0 {
					result = append(result, provider.NewAssistantMessage(assistParts...))
					assistParts = nil
				}
				if len(toolMsgParts) > 0 {
					result = append(result, provider.NewToolMessage(toolMsgParts...))
					toolMsgParts = nil
				}
			}

			// processToolPart handles both ToolInvocationPart and DynamicToolUIPart,
			// which share identical fields and conversion logic.
			//
			// Mirrors the upstream convert-to-model-messages.ts step block:
			// emit the tool-call assistant part, optionally an inline
			// provider-executed tool-result, and on the tool message side
			// optionally a tool-approval-response and either a regular or
			// synthetic execution-denied tool-result.
			processToolPart := func(tp toolPartFields) error {
				if opt.ignoreIncompleteToolCalls && isToolCallInputIncomplete(tp.State) {
					return nil
				}
				callOpts := providerMetadataToOptions(tp.CallProviderMetadata)
				assistParts = append(assistParts, provider.ContentPart{
					Type:             provider.ContentPartTypeToolCall,
					ToolCallID:       tp.ToolCallID,
					ToolName:         tp.ToolName,
					Input:            tp.Input,
					ProviderExecuted: tp.ProviderExecuted,
					ProviderOptions:  callOpts,
				})
				if tp.Approval != nil && tp.Approval.ID != "" {
					assistParts = append(assistParts, provider.ContentPart{
						Type:        provider.ContentPartTypeToolApprovalRequest,
						ApprovalID:  tp.Approval.ID,
						ToolCallID:  tp.ToolCallID,
						Signature:   tp.Approval.Signature,
						IsAutomatic: tp.Approval.IsAutomatic,
					})
				}
				// Upstream falls back to callProviderMetadata when the
				// result-side metadata is absent (convert-to-model-messages.ts:231-232).
				resultOpts := providerMetadataToOptions(tp.ResultProviderMetadata)
				if resultOpts == nil {
					resultOpts = callOpts
				}
				if tp.ProviderExecuted {
					r, err := providerExecutedToolResult(tp, resultOpts, opt.tools)
					if err != nil {
						return err
					}
					if r != nil {
						assistParts = append(assistParts, *r)
					}
				}
				// Approval response on the tool message side (only when the
				// user has actually responded). Upstream emits this for both
				// provider-executed and client-executed tools whenever
				// approval.approved is set (convert-to-model-messages.ts:293-301).
				if tp.Approval != nil && tp.Approval.Approved != nil {
					approvedCopy := *tp.Approval.Approved
					toolMsgParts = append(toolMsgParts, provider.ContentPart{
						Type:             provider.ContentPartTypeToolApprovalResponse,
						ApprovalID:       tp.Approval.ID,
						Approved:         &approvedCopy,
						Reason:           tp.Approval.Reason,
						ProviderExecuted: tp.ProviderExecuted,
					})
				}
				// Synthetic execution-denied tool-result for denied tool
				// approvals (provider-executed and client-executed alike;
				// upstream gates on state==approval-responded && approved==false).
				if tp.State == ToolStateApprovalResponded && tp.Approval != nil && tp.Approval.Approved != nil && !*tp.Approval.Approved {
					toolMsgParts = append(toolMsgParts, provider.ContentPart{
						Type:            provider.ContentPartTypeToolResult,
						ToolCallID:      tp.ToolCallID,
						ToolName:        tp.ToolName,
						ProviderOptions: callOpts,
						Output: &provider.ToolResultOutput{
							Type:   provider.ToolOutputExecutionDenied,
							Reason: tp.Approval.Reason,
						},
					})
				}
				if !tp.ProviderExecuted {
					var err error
					toolMsgParts, err = appendToolResult(toolMsgParts, tp, resultOpts, opt.tools)
					if err != nil {
						return err
					}
				}
				return nil
			}

			for _, p := range msg.Parts {
				switch v := p.(type) {
				case StepStartPart:
					flushBlock()
				case TextPart:
					if v.Text != "" {
						assistParts = append(assistParts, provider.ContentPart{
							Type:            provider.ContentPartTypeText,
							Text:            v.Text,
							ProviderOptions: providerMetadataToOptions(v.ProviderMetadata),
						})
					}
				case ReasoningPart:
					assistParts = append(assistParts, provider.ContentPart{
						Type:            provider.ContentPartTypeReasoning,
						Text:            v.Text,
						ProviderOptions: providerMetadataToOptions(v.ProviderMetadata),
					})
				case FilePart:
					data := provider.DataContent{URL: v.URL}
					if v.ProviderReference != nil {
						var err error
						data, err = providerReferenceFileData(v.ProviderReference)
						if err != nil {
							return nil, fmt.Errorf("converting assistant file part: %w", err)
						}
					}
					assistParts = append(assistParts, provider.ContentPart{
						Type:            provider.ContentPartTypeFile,
						Data:            &data,
						MediaType:       v.MediaType,
						Filename:        v.Filename,
						ProviderOptions: providerMetadataToOptions(v.ProviderMetadata),
					})
				case ReasoningFilePart:
					assistParts = append(assistParts, provider.ContentPart{
						Type:            provider.ContentPartTypeReasoningFile,
						Data:            &provider.DataContent{URL: v.URL},
						MediaType:       v.MediaType,
						ProviderOptions: providerMetadataToOptions(v.ProviderMetadata),
					})
				case ToolInvocationPart:
					if err := processToolPart(toolPartFields(v)); err != nil {
						return nil, err
					}
				case DynamicToolUIPart:
					if err := processToolPart(toolPartFields(v)); err != nil {
						return nil, err
					}
				}
			}

			flushBlock()
		}
	}

	return result, nil
}

func filePartData(url string, mediaType string, reference map[string]string) (provider.DataContent, string, error) {
	if reference != nil {
		data, err := providerReferenceFileData(reference)
		return data, mediaType, err
	}

	const prefix = "data:"
	const base64Marker = ";base64,"

	if strings.HasPrefix(url, prefix) {
		if base64Index := strings.Index(url, base64Marker); base64Index > len(prefix) {
			if mediaType == "" || mediaType == "image/*" {
				mediaType = url[len(prefix):base64Index]
			}
			return provider.DataContent{Base64: url[base64Index+len(base64Marker):]}, mediaType, nil
		}
	}

	return provider.DataContent{URL: url}, mediaType, nil
}

func providerReferenceFileData(reference map[string]string) (provider.DataContent, error) {
	referenceData, err := json.Marshal(reference)
	if err != nil {
		return provider.DataContent{}, fmt.Errorf("marshaling provider reference: %w", err)
	}
	return provider.DataContent{Reference: referenceData}, nil
}

func extractSystemContent(parts []Part) (string, provider.ProviderOptions) {
	var texts []string
	var opts provider.ProviderOptions
	for _, p := range parts {
		tp, ok := p.(TextPart)
		if !ok {
			continue
		}
		texts = append(texts, tp.Text)
		if len(tp.ProviderMetadata) > 0 {
			if opts == nil {
				opts = make(provider.ProviderOptions)
			}
			for k, v := range tp.ProviderMetadata {
				opts[k] = provider.RawProviderOption{Key: k, Raw: v}
			}
		}
	}
	return strings.Join(texts, ""), opts
}

// toolPartFields holds the common fields shared by ToolInvocationPart and
// DynamicToolUIPart, used to avoid duplicating conversion logic.
type toolPartFields struct {
	ToolCallID             string
	ToolName               string
	State                  ToolInvocationState
	Input                  json.RawMessage
	Output                 json.RawMessage
	ErrorText              string
	ProviderExecuted       bool
	Approval               *ToolApproval
	CallProviderMetadata   provider.ProviderMetadata
	ResultProviderMetadata provider.ProviderMetadata
}

func isToolCallInputIncomplete(state ToolInvocationState) bool {
	return state == ToolStateInputStreaming || state == ToolStateInputAvailable
}

// providerExecutedToolResult creates an inline tool-result ContentPart for
// provider-executed tools. These go directly into the assistant message
// content, not into a separate tool message.
func providerExecutedToolResult(tp toolPartFields, resultOpts provider.ProviderOptions, tools ToolSet) (*provider.ContentPart, error) {
	if tp.State == ToolStateOutputAvailable && tp.Output != nil {
		output, err := createToolModelOutput(tp, tools)
		if err != nil {
			return nil, err
		}
		return &provider.ContentPart{
			Type:            provider.ContentPartTypeToolResult,
			ToolCallID:      tp.ToolCallID,
			ToolName:        tp.ToolName,
			ProviderOptions: resultOpts,
			Output:          output,
		}, nil
	}
	if tp.State == ToolStateOutputError {
		errorJSON, err := json.Marshal(tp.ErrorText)
		if err != nil {
			errorJSON = []byte(`"error"`)
		}
		return &provider.ContentPart{
			Type:            provider.ContentPartTypeToolResult,
			ToolCallID:      tp.ToolCallID,
			ToolName:        tp.ToolName,
			ProviderOptions: resultOpts,
			Output: &provider.ToolResultOutput{
				Type: provider.ToolOutputErrorJSON,
				JSON: errorJSON,
			},
		}, nil
	}
	return nil, nil
}

// appendToolResult appends a tool-result ContentPart for non-provider-executed
// tools. These go into a separate tool role message.
func appendToolResult(parts []provider.ContentPart, tp toolPartFields, resultOpts provider.ProviderOptions, tools ToolSet) ([]provider.ContentPart, error) {
	switch tp.State {
	case ToolStateOutputAvailable:
		if tp.Output == nil {
			return parts, nil
		}
		output, err := createToolModelOutput(tp, tools)
		if err != nil {
			return nil, err
		}
		return append(parts, provider.ContentPart{
			Type:            provider.ContentPartTypeToolResult,
			ToolCallID:      tp.ToolCallID,
			ToolName:        tp.ToolName,
			ProviderOptions: resultOpts,
			Output:          output,
		}), nil
	case ToolStateOutputError:
		return append(parts, provider.ContentPart{
			Type:            provider.ContentPartTypeToolResult,
			ToolCallID:      tp.ToolCallID,
			ToolName:        tp.ToolName,
			ProviderOptions: resultOpts,
			Output: &provider.ToolResultOutput{
				Type: provider.ToolOutputErrorText,
				Text: tp.ErrorText,
			},
		}), nil
	case ToolStateOutputDenied:
		reason := "Tool execution denied."
		if tp.Approval != nil && tp.Approval.Reason != "" {
			reason = tp.Approval.Reason
		}
		return append(parts, provider.ContentPart{
			Type:            provider.ContentPartTypeToolResult,
			ToolCallID:      tp.ToolCallID,
			ToolName:        tp.ToolName,
			ProviderOptions: resultOpts,
			Output: &provider.ToolResultOutput{
				Type: provider.ToolOutputErrorText,
				Text: reason,
			},
		}), nil
	default:
		return parts, nil
	}
}

func createToolModelOutput(tp toolPartFields, tools ToolSet) (*provider.ToolResultOutput, error) {
	if tool, ok := tools[tp.ToolName]; ok && tool.ToModelOutput != nil {
		output, err := tool.ToModelOutput(ToolOutputContext{
			ToolCallID: tp.ToolCallID,
			Input:      tp.Input,
			Output:     tp.Output,
		})
		if err != nil {
			return nil, fmt.Errorf("converting output for tool %q: %w", tp.ToolName, err)
		}
		return output, nil
	}

	var text *string
	if err := json.Unmarshal(tp.Output, &text); err == nil && text != nil {
		return &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: *text}, nil
	}
	return &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: tp.Output}, nil
}

// toolSetToProviderTools converts a ToolSet map to a sorted slice of provider.Tool.
// Tools are sorted by name for deterministic provider calls.
func toolSetToProviderTools(tools ToolSet) ([]provider.Tool, []provider.Warning) {
	if tools == nil {
		return nil, nil
	}
	names := slices.Sorted(maps.Keys(tools))

	result := make([]provider.Tool, 0, len(tools))
	var warnings []provider.Warning
	for _, name := range names {
		t := tools[name]
		switch t.Type {
		case UserToolProvider:
			result = append(result, provider.Tool{
				Type:            provider.ToolTypeProvider,
				Name:            name,
				ID:              t.ID,
				Args:            t.Args,
				ProviderOptions: t.ProviderOptions,
			})
		case "", UserToolFunction, UserToolDynamic:
			examples := make([]provider.InputExample, len(t.InputExamples))
			for i, raw := range t.InputExamples {
				examples[i] = provider.InputExample{Input: raw}
			}
			result = append(result, provider.Tool{
				Type:            provider.ToolTypeFunction,
				Name:            name,
				Description:     t.Description,
				InputSchema:     t.InputSchema.JSON(),
				InputExamples:   examples,
				Strict:          t.Strict,
				ProviderOptions: t.ProviderOptions,
			})
		default:
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: fmt.Sprintf("tool %s", name),
				Details: fmt.Sprintf("unsupported tool type %q, skipping", t.Type),
			})
		}
	}
	return result, warnings
}

func providerMetadataToOptions(meta provider.ProviderMetadata) provider.ProviderOptions {
	if len(meta) == 0 {
		return nil
	}
	opts := make(provider.ProviderOptions, len(meta))
	for k, v := range meta {
		opts[k] = provider.RawProviderOption{Key: k, Raw: v}
	}
	return opts
}

func optionsToProviderMetadata(opts provider.ProviderOptions) provider.ProviderMetadata {
	if len(opts) == 0 {
		return nil
	}
	meta := make(provider.ProviderMetadata, len(opts))
	for k, v := range opts {
		if raw, ok := v.(provider.RawProviderOption); ok {
			meta[k] = raw.Raw
		}
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}

func systemToMessages(system []SystemModelMessage) []provider.Message {
	var msgs []provider.Message
	for _, s := range system {
		msgs = append(msgs, provider.Message{
			Role: provider.RoleSystem,
			Content: []provider.ContentPart{
				{Type: provider.ContentPartTypeText, Text: s.Content},
			},
			ProviderOptions: s.ProviderOptions,
		})
	}
	return msgs
}
