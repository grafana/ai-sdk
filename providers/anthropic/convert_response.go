package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/grafana/ai-sdk/provider"
)

func convertResponse(msg *anthropic.BetaMessage, mapping toolNameMapping, usesJsonResponseTool bool, citDocs []citationDocument, generateID func() string, providerName string, markCodeExecutionDynamic bool) (*provider.GenerateResult, error) {
	var content []provider.GenerateContentPart
	serverToolCalls := make(map[string]string)
	mcpToolCalls := make(map[string]mcpToolCallInfo)

	var isJsonResponseFromTool bool

	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			if !usesJsonResponseTool {
				var webSearchCitations []json.RawMessage
				for _, citation := range block.Citations {
					if _, ok := citation.AsAny().(anthropic.BetaCitationsWebSearchResultLocation); ok {
						webSearchCitations = append(webSearchCitations, json.RawMessage(citation.RawJSON()))
					}
				}
				metadata, err := marshalWebSearchCitationMetadata(webSearchCitations)
				if err != nil {
					return nil, err
				}
				content = append(content, provider.GenerateContentPart{
					Type:             provider.ContentText,
					Text:             block.Text,
					ProviderMetadata: metadata,
				})
				for _, cit := range block.Citations {
					src, err := createCitationSource(cit.AsAny(), citDocs, generateID)
					if err != nil {
						return nil, fmt.Errorf("creating citation source: %w", err)
					}
					if src != nil {
						content = append(content, provider.GenerateContentPart{
							Type:             provider.ContentSource,
							ID:               src.ID,
							SourceType:       src.SourceType,
							URL:              src.URL,
							Text:             src.Title,
							MediaType:        src.MediaType,
							Filename:         src.Filename,
							ProviderMetadata: src.ProviderMetadata,
						})
					}
				}
			}
		case "tool_use":
			if usesJsonResponseTool && block.Name == jsonResponseToolName {
				isJsonResponseFromTool = true
				inputJSON, err := json.Marshal(block.Input)
				if err != nil {
					return nil, fmt.Errorf("marshaling json response tool input: %w", err)
				}
				content = append(content, provider.GenerateContentPart{
					Type: provider.ContentText,
					Text: string(inputJSON),
				})
				continue
			}
			part := provider.GenerateContentPart{
				Type:       provider.ContentToolCall,
				ToolCallID: block.ID,
				ToolName:   block.Name,
				Input:      block.Input,
			}
			if block.Caller.Type != "" {
				callerMeta, err := marshalCallerMetadata(block.Caller.Type, block.Caller.ToolID)
				if err != nil {
					return nil, err
				}
				part.ProviderMetadata = callerMeta
			}
			content = append(content, part)
		case "thinking":
			// Preserve the signature in provider metadata so the reasoning
			// block can round-trip through a follow-up turn (the request
			// path in convert_request.go reads back this signature via
			// extractSignature). Mirrors upstream
			// anthropic-language-model.ts:934-944.
			part := provider.GenerateContentPart{
				Type: provider.ContentReasoning,
				Text: block.Thinking,
			}
			if block.Signature != "" {
				meta, err := json.Marshal(map[string]string{"signature": block.Signature})
				if err != nil {
					return nil, fmt.Errorf("marshaling thinking signature: %w", err)
				}
				part.ProviderMetadata = provider.ProviderMetadata{"anthropic": meta}
			}
			content = append(content, part)
		case "redacted_thinking":
			// Map redacted thinking to a reasoning block with empty text and
			// the redacted data preserved in anthropic provider metadata.
			// Mirrors upstream anthropic-language-model.ts:946-956.
			redacted := block.AsRedactedThinking()
			meta, err := json.Marshal(map[string]string{"redactedData": redacted.Data})
			if err != nil {
				return nil, fmt.Errorf("marshaling redacted thinking metadata: %w", err)
			}
			content = append(content, provider.GenerateContentPart{
				Type:             provider.ContentReasoning,
				Text:             "",
				ProviderMetadata: provider.ProviderMetadata{"anthropic": meta},
			})
		case "server_tool_use":
			stu := block.AsServerToolUse()
			wireName := string(stu.Name)
			serverToolCalls[stu.ID] = wireName

			resolvedName := wireName
			var inputJSON json.RawMessage

			switch wireName {
			case "bash_code_execution", "text_editor_code_execution":
				resolvedName = "code_execution"
				if inputMap, ok := stu.Input.(map[string]any); ok {
					wrapped := make(map[string]any, len(inputMap)+1)
					wrapped["type"] = wireName
					for k, v := range inputMap {
						wrapped[k] = v
					}
					var err error
					inputJSON, err = json.Marshal(wrapped)
					if err != nil {
						return nil, fmt.Errorf("marshaling server tool use input: %w", err)
					}
				} else {
					var err error
					inputJSON, err = json.Marshal(stu.Input)
					if err != nil {
						return nil, fmt.Errorf("marshaling server tool use input: %w", err)
					}
				}
			case "code_execution":
				var err error
				inputJSON, err = json.Marshal(stu.Input)
				if err != nil {
					return nil, fmt.Errorf("marshaling server tool use input: %w", err)
				}
				inputJSON = json.RawMessage(injectProgrammaticToolCallType(string(inputJSON)))
			default:
				var err error
				inputJSON, err = json.Marshal(stu.Input)
				if err != nil {
					return nil, fmt.Errorf("marshaling server tool use input: %w", err)
				}
			}

			part := provider.GenerateContentPart{
				Type:             provider.ContentToolCall,
				ToolCallID:       stu.ID,
				ToolName:         mapping.toCustomToolName(resolvedName),
				Input:            inputJSON,
				ProviderExecuted: true,
			}
			// Mirrors upstream anthropic-language-model.ts:1043-1047: when
			// a 20260209 web tool is configured without an explicit
			// code_execution tool, mark the implicit code_execution
			// server_tool_use as dynamic so the tool-validation layer accepts
			// the call.
			if markCodeExecutionDynamic && resolvedName == "code_execution" {
				part.Dynamic = ptrBool(true)
			}
			content = append(content, part)
		case "web_search_tool_result":
			ws := block.AsWebSearchToolResult()
			wsContent := ws.Content
			if wsContent.Type == "web_search_tool_result_error" {
				errData, err := marshalToolResultError("web_search_tool_result_error", string(wsContent.ErrorCode))
				if err != nil {
					return nil, fmt.Errorf("marshaling web search error: %w", err)
				}
				content = append(content, provider.GenerateContentPart{
					Type:       provider.ContentToolResult,
					ToolCallID: ws.ToolUseID,
					ToolName:   mapping.toCustomToolName("web_search"),
					IsError:    true,
					Result:     errData,
				})
			} else {
				resultJSON, err := json.Marshal(marshalWebSearchResults(wsContent.OfBetaWebSearchResultBlockArray))
				if err != nil {
					return nil, fmt.Errorf("marshaling web search results: %w", err)
				}
				content = append(content, provider.GenerateContentPart{
					Type:       provider.ContentToolResult,
					ToolCallID: ws.ToolUseID,
					ToolName:   mapping.toCustomToolName("web_search"),
					Result:     resultJSON,
				})
				for _, result := range wsContent.OfBetaWebSearchResultBlockArray {
					pageAgeMeta, err := json.Marshal(map[string]any{"pageAge": nilIfEmpty(result.PageAge)})
					if err != nil {
						return nil, fmt.Errorf("marshaling web search page age: %w", err)
					}
					content = append(content, provider.GenerateContentPart{
						Type:       provider.ContentSource,
						ID:         generateID(),
						SourceType: provider.SourceTypeURL,
						URL:        result.URL,
						Text:       result.Title,
						ProviderMetadata: provider.ProviderMetadata{
							"anthropic": pageAgeMeta,
						},
					})
				}
			}
		case "web_fetch_tool_result":
			wf := block.AsWebFetchToolResult()
			wfContent := wf.Content
			if wfContent.Type == "web_fetch_tool_result_error" {
				errData, err := json.Marshal(map[string]any{
					"type":      "web_fetch_tool_result_error",
					"errorCode": string(wfContent.ErrorCode),
				})
				if err != nil {
					return nil, fmt.Errorf("marshaling web fetch error: %w", err)
				}
				content = append(content, provider.GenerateContentPart{
					Type:       "tool-result",
					ToolCallID: wf.ToolUseID,
					ToolName:   mapping.toCustomToolName("web_fetch"),
					IsError:    true,
					Result:     errData,
				})
			} else {
				title := wfContent.Content.Title
				if title == "" {
					title = wfContent.URL
				}
				citDocs = append(citDocs, citationDocument{
					title:     title,
					mediaType: wfContent.Content.Source.MediaType,
				})

				resultData, err := json.Marshal(buildWebFetchResultOutput(wfContent))
				if err != nil {
					return nil, fmt.Errorf("marshaling web fetch result: %w", err)
				}
				content = append(content, provider.GenerateContentPart{
					Type:       "tool-result",
					ToolCallID: wf.ToolUseID,
					ToolName:   mapping.toCustomToolName("web_fetch"),
					Result:     resultData,
				})
			}
		case "tool_search_tool_result":
			ts := block.AsToolSearchToolResult()
			providerToolName := resolveToolSearchProviderToolName(mapping, serverToolCalls, ts.ToolUseID)
			tsContent := ts.Content
			if tsContent.Type == "tool_search_tool_result_error" {
				errData, err := json.Marshal(map[string]any{
					"type":         "tool_search_tool_result_error",
					"errorCode":    string(tsContent.ErrorCode),
					"errorMessage": tsContent.ErrorMessage,
				})
				if err != nil {
					return nil, fmt.Errorf("marshaling tool search error: %w", err)
				}
				content = append(content, provider.GenerateContentPart{
					Type:       provider.ContentToolResult,
					ToolCallID: ts.ToolUseID,
					ToolName:   mapping.toCustomToolName(providerToolName),
					IsError:    true,
					Result:     errData,
				})
			} else {
				resultJSON, err := json.Marshal(marshalToolSearchReferences(tsContent.ToolReferences))
				if err != nil {
					return nil, fmt.Errorf("marshaling tool search results: %w", err)
				}
				content = append(content, provider.GenerateContentPart{
					Type:       provider.ContentToolResult,
					ToolCallID: ts.ToolUseID,
					ToolName:   mapping.toCustomToolName(providerToolName),
					Result:     resultJSON,
				})
			}
		case "code_execution_tool_result":
			ceResult := block.AsCodeExecutionToolResult()
			ceContent := ceResult.Content
			switch ceContent.Type {
			case "code_execution_tool_result_error":
				errData, err := json.Marshal(map[string]any{
					"type":      "code_execution_tool_result_error",
					"errorCode": string(ceContent.ErrorCode),
				})
				if err != nil {
					return nil, fmt.Errorf("marshaling code execution error: %w", err)
				}
				content = append(content, provider.GenerateContentPart{
					Type:             provider.ContentToolResult,
					ToolCallID:       ceResult.ToolUseID,
					ToolName:         mapping.toCustomToolName("code_execution"),
					IsError:          true,
					ProviderExecuted: true,
					Result:           errData,
				})
			case "code_execution_result":
				resultJSON, err := json.Marshal(map[string]any{
					"type":        ceContent.Type,
					"stdout":      ceContent.Stdout,
					"stderr":      ceContent.Stderr,
					"return_code": ceContent.ReturnCode,
					"content":     ensureSlice(ceContent.Content),
				})
				if err != nil {
					return nil, fmt.Errorf("marshaling code execution result: %w", err)
				}
				content = append(content, provider.GenerateContentPart{
					Type:             provider.ContentToolResult,
					ToolCallID:       ceResult.ToolUseID,
					ToolName:         mapping.toCustomToolName("code_execution"),
					ProviderExecuted: true,
					Result:           resultJSON,
				})
			case "encrypted_code_execution_result":
				resultJSON, err := json.Marshal(map[string]any{
					"type":             ceContent.Type,
					"encrypted_stdout": ceContent.EncryptedStdout,
					"stderr":           ceContent.Stderr,
					"return_code":      ceContent.ReturnCode,
					"content":          ensureSlice(ceContent.Content),
				})
				if err != nil {
					return nil, fmt.Errorf("marshaling encrypted code execution result: %w", err)
				}
				content = append(content, provider.GenerateContentPart{
					Type:             provider.ContentToolResult,
					ToolCallID:       ceResult.ToolUseID,
					ToolName:         mapping.toCustomToolName("code_execution"),
					ProviderExecuted: true,
					Result:           resultJSON,
				})
			default:
				resultJSON := json.RawMessage(ceContent.RawJSON())
				content = append(content, provider.GenerateContentPart{
					Type:             provider.ContentToolResult,
					ToolCallID:       ceResult.ToolUseID,
					ToolName:         mapping.toCustomToolName("code_execution"),
					ProviderExecuted: true,
					Result:           resultJSON,
				})
			}
		case "bash_code_execution_tool_result":
			bceResult := block.AsBashCodeExecutionToolResult()
			resultJSON := json.RawMessage(bceResult.Content.RawJSON())
			content = append(content, provider.GenerateContentPart{
				Type:             provider.ContentToolResult,
				ToolCallID:       bceResult.ToolUseID,
				ToolName:         mapping.toCustomToolName("code_execution"),
				ProviderExecuted: true,
				Result:           resultJSON,
			})
		case "text_editor_code_execution_tool_result":
			teceResult := block.AsTextEditorCodeExecutionToolResult()
			resultJSON := json.RawMessage(teceResult.Content.RawJSON())
			content = append(content, provider.GenerateContentPart{
				Type:             provider.ContentToolResult,
				ToolCallID:       teceResult.ToolUseID,
				ToolName:         mapping.toCustomToolName("code_execution"),
				ProviderExecuted: true,
				Result:           resultJSON,
			})
		case "mcp_tool_use":
			mtu := block.AsMCPToolUse()
			inputJSON, err := json.Marshal(mtu.Input)
			if err != nil {
				return nil, fmt.Errorf("marshaling mcp tool use input: %w", err)
			}
			mcpMeta, err := json.Marshal(map[string]string{
				"type":       "mcp-tool-use",
				"serverName": mtu.ServerName,
			})
			if err != nil {
				return nil, fmt.Errorf("marshaling mcp tool use metadata: %w", err)
			}
			meta := provider.ProviderMetadata{
				"anthropic": mcpMeta,
			}
			mcpToolCalls[mtu.ID] = mcpToolCallInfo{
				ToolName:         mtu.Name,
				ProviderMetadata: meta,
			}
			content = append(content, provider.GenerateContentPart{
				Type:             provider.ContentToolCall,
				ToolCallID:       mtu.ID,
				ToolName:         mtu.Name,
				Input:            inputJSON,
				ProviderExecuted: true,
				Dynamic:          ptrBool(true),
				ProviderMetadata: meta,
			})
		case "mcp_tool_result":
			mtr := block.AsMCPToolResult()
			contentJSON := json.RawMessage(mtr.Content.RawJSON())
			info, ok := mcpToolCalls[mtr.ToolUseID]
			var toolName string
			var meta provider.ProviderMetadata
			if ok {
				toolName = info.ToolName
				meta = info.ProviderMetadata
			}
			content = append(content, provider.GenerateContentPart{
				Type:             provider.ContentToolResult,
				ToolCallID:       mtr.ToolUseID,
				ToolName:         toolName,
				IsError:          mtr.IsError,
				Dynamic:          ptrBool(true),
				ProviderMetadata: meta,
				Result:           contentJSON,
			})
		}
	}

	usage := convertAnthropicUsage(anthropicUsage{
		inputTokens:              msg.Usage.InputTokens,
		outputTokens:             msg.Usage.OutputTokens,
		cacheCreationInputTokens: msg.Usage.CacheCreationInputTokens,
		cacheReadInputTokens:     msg.Usage.CacheReadInputTokens,
		reasoningTokens:          thinkingTokenCount(msg.Usage.OutputTokensDetails),
		iterations:               msg.Usage.Iterations,
		raw:                      json.RawMessage(msg.Usage.RawJSON()),
	})

	fr := mapFinishReason(msg.StopReason)
	if isJsonResponseFromTool && msg.StopReason == anthropic.BetaStopReasonToolUse {
		fr = provider.FinishReason{Unified: provider.FinishReasonStop, Raw: string(msg.StopReason)}
	}

	return &provider.GenerateResult{
		Content:      content,
		FinishReason: fr,
		Usage:        usage,
		Response: &provider.GenerateResponse{
			ResponseMetadata: provider.ResponseMetadata{
				ID:       msg.ID,
				ModelID:  string(msg.Model),
				Provider: providerName,
			},
		},
	}, nil
}

func marshalCallerMetadata(callerType, callerToolID string) (provider.ProviderMetadata, error) {
	caller := map[string]string{"type": callerType}
	if callerToolID != "" {
		caller["toolId"] = callerToolID
	}
	callerJSON, err := json.Marshal(map[string]any{"caller": caller})
	if err != nil {
		return nil, fmt.Errorf("marshaling caller metadata: %w", err)
	}
	return provider.ProviderMetadata{
		"anthropic": callerJSON,
	}, nil
}

func mapFinishReason(reason anthropic.BetaStopReason) provider.FinishReason {
	raw := string(reason)
	var unified provider.UnifiedFinishReason
	switch reason {
	case anthropic.BetaStopReasonEndTurn:
		unified = provider.FinishReasonStop
	case anthropic.BetaStopReasonStopSequence:
		unified = provider.FinishReasonStop
	case anthropic.BetaStopReasonMaxTokens:
		unified = provider.FinishReasonLength
	case anthropic.BetaStopReasonToolUse:
		unified = provider.FinishReasonToolCalls
	default:
		if raw == "content_filter" || raw == "refusal" {
			unified = provider.FinishReasonContentFilter
		} else {
			unified = provider.FinishReasonOther
		}
	}
	return provider.FinishReason{Unified: unified, Raw: raw}
}

func ptrBool(b bool) *bool { return &b }

func marshalToolResultError(errorType, errorCode string) (json.RawMessage, error) {
	return json.Marshal(struct {
		Type      string `json:"type"`
		ErrorCode string `json:"errorCode"`
	}{Type: errorType, ErrorCode: errorCode})
}
