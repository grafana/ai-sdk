package aisdk

import (
	"bytes"
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
	convertDataPart           func(DataPart) (*provider.ContentPart, error)
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

// WithConvertDataPart converts user and assistant data parts to text or file content.
// A nil result skips the data part; callback errors abort conversion.
func WithConvertDataPart(fn func(DataPart) (*provider.ContentPart, error)) ConvertOption {
	return convertOptionFunc(func(cfg *convertConfig) { cfg.convertDataPart = fn })
}

func convertDataPart(part DataPart, fn func(DataPart) (*provider.ContentPart, error)) (*provider.ContentPart, error) {
	if fn == nil {
		return nil, nil
	}
	converted, err := fn(part)
	if err != nil {
		return nil, fmt.Errorf("aisdk: converting data part %q: %w", part.DataName, err)
	}
	if converted != nil && converted.Type != provider.ContentPartTypeText && converted.Type != provider.ContentPartTypeFile {
		return nil, fmt.Errorf("aisdk: data part %q converter returned unsupported content type %q", part.DataName, converted.Type)
	}
	return converted, nil
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
				case DataPart:
					converted, err := convertDataPart(v, opt.convertDataPart)
					if err != nil {
						return nil, err
					}
					if converted != nil {
						parts = append(parts, *converted)
					}
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
			var toolParts []toolPartFields

			flushBlock := func() error {
				var toolMsgParts []provider.ContentPart
				for _, tp := range toolParts {
					if tp.Approval != nil && tp.Approval.Approved != nil {
						toolMsgParts = append(toolMsgParts, provider.ContentPart{
							Type:             provider.ContentPartTypeToolApprovalResponse,
							ApprovalID:       tp.Approval.ID,
							Approved:         new(*tp.Approval.Approved),
							Reason:           stringValue(tp.Approval.Reason),
							ProviderExecuted: tp.ProviderExecuted,
						})
					}
					if tp.State == ToolStateApprovalResponded && tp.Approval != nil && tp.Approval.Approved != nil && !*tp.Approval.Approved {
						toolMsgParts = append(toolMsgParts, provider.ContentPart{
							Type:            provider.ContentPartTypeToolResult,
							ToolCallID:      tp.ToolCallID,
							ToolName:        tp.ToolName,
							ProviderOptions: providerMetadataToOptions(tp.CallProviderMetadata),
							Output: &provider.ToolResultOutput{
								Type:   provider.ToolOutputExecutionDenied,
								Reason: stringValue(tp.Approval.Reason),
							},
						})
					}
					if !tp.ProviderExecuted {
						var err error
						toolMsgParts, err = appendToolResult(toolMsgParts, tp, providerMetadataToOptions(tp.CallProviderMetadata), opt.tools)
						if err != nil {
							return err
						}
					}
				}
				if len(assistParts) > 0 {
					result = append(result, provider.NewAssistantMessage(assistParts...))
				}
				if len(toolMsgParts) > 0 {
					result = append(result, provider.NewToolMessage(toolMsgParts...))
				}
				assistParts, toolParts = nil, nil
				return nil
			}

			processToolPart := func(tp toolPartFields) error {
				if tp.State == ToolStateInputStreaming || (opt.ignoreIncompleteToolCalls && (!isCompleteToolCallState(tp.State) || (tp.State == ToolStateOutputAvailable && tp.Preliminary != nil && *tp.Preliminary))) {
					return nil
				}
				input := tp.Input
				callMetadata := tp.CallProviderMetadata
				if tp.State == ToolStateOutputError {
					if len(input) == 0 || bytes.Equal(bytes.TrimSpace(input), []byte("null")) {
						input = tp.RawInput
					}
					if callMetadata == nil {
						callMetadata = tp.ResultProviderMetadata
					}
				}
				callOpts := providerMetadataToOptions(callMetadata)
				assistParts = append(assistParts, provider.ContentPart{
					Type:             provider.ContentPartTypeToolCall,
					ToolCallID:       tp.ToolCallID,
					ToolName:         tp.ToolName,
					Input:            input,
					ProviderExecuted: tp.ProviderExecuted,
					ProviderOptions:  callOpts,
				})
				if tp.Approval != nil && tp.Approval.ID != "" {
					assistParts = append(assistParts, provider.ContentPart{
						Type:        provider.ContentPartTypeToolApprovalRequest,
						ApprovalID:  tp.Approval.ID,
						ToolCallID:  tp.ToolCallID,
						Signature:   tp.Approval.Signature,
						Reason:      stringValue(tp.Approval.RequestReason),
						IsAutomatic: tp.Approval.IsAutomatic,
					})
				}
				// Upstream falls back to callProviderMetadata when the
				// result-side metadata is absent (convert-to-model-messages.ts:231-232).
				resultOpts := providerMetadataToOptions(tp.ResultProviderMetadata)
				if tp.ResultProviderMetadata == nil {
					resultOpts = providerMetadataToOptions(tp.CallProviderMetadata)
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
				toolParts = append(toolParts, tp)
				return nil
			}

			for _, p := range msg.Parts {
				switch v := p.(type) {
				case StepStartPart:
					if err := flushBlock(); err != nil {
						return nil, err
					}
				case DataPart:
					converted, err := convertDataPart(v, opt.convertDataPart)
					if err != nil {
						return nil, err
					}
					if converted != nil {
						assistParts = append(assistParts, *converted)
					}
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
				case CustomPart:
					assistParts = append(assistParts, provider.ContentPart{
						Type:            provider.ContentPartTypeCustom,
						Kind:            v.Kind,
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

			if err := flushBlock(); err != nil {
				return nil, err
			}
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
			return provider.Base64DataContent(url[base64Index+len(base64Marker):]), mediaType, nil
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
type toolPartFields ToolInvocationPart

func isCompleteToolCallState(state ToolInvocationState) bool {
	switch state {
	case ToolStateApprovalResponded, ToolStateOutputAvailable, ToolStateOutputError, ToolStateOutputDenied:
		return true
	default:
		return false
	}
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
		errorJSON, err := json.Marshal(stringValue(tp.ErrorText))
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
				Text: stringValue(tp.ErrorText),
			},
		}), nil
	case ToolStateOutputDenied:
		reason := "Tool call execution denied."
		if tp.Approval != nil && tp.Approval.Reason != nil {
			reason = *tp.Approval.Reason
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

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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
			var examples []provider.InputExample
			if t.InputExamples != nil {
				examples = make([]provider.InputExample, len(t.InputExamples))
				for i, raw := range t.InputExamples {
					examples[i] = provider.InputExample{Input: raw}
				}
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
