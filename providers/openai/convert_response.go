package openai

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/responses"
)

// convertResponse converts a non-streaming Responses response into a
// provider.GenerateResult, mapping every output item to provider content.
func convertResponse(resp *responses.Response, br buildResult, generateID func() string, providerName string) (*provider.GenerateResult, error) {
	var content []provider.GenerateContentPart
	hasFunctionCall := false
	var hostedToolSearchCallIDs []string

	for _, item := range resp.Output {
		switch v := item.AsAny().(type) {
		case responses.ResponseOutputMessage:
			for _, c := range v.Content {
				if t := c.AsAny(); t != nil {
					if ot, ok := t.(responses.ResponseOutputText); ok {
						content = append(content, provider.GenerateContentPart{
							Type:             provider.ContentText,
							Text:             ot.Text,
							ProviderMetadata: textMeta(providerName, v.ID, string(v.Phase), rawList(ot.Annotations)),
						})
						content = append(content, convertAnnotations(ot.Annotations, generateID, providerName)...)
					}
				}
			}

		case responses.ResponseReasoningItem:
			summaries := v.Summary
			if len(summaries) == 0 {
				summaries = []responses.ResponseReasoningItemSummary{{Text: ""}}
			}
			for _, s := range summaries {
				content = append(content, provider.GenerateContentPart{
					Type:             provider.ContentReasoning,
					Text:             s.Text,
					ProviderMetadata: reasoningMeta(providerName, v.ID, v.EncryptedContent),
				})
			}

		case responses.ResponseFunctionToolCall:
			hasFunctionCall = true
			content = append(content, provider.GenerateContentPart{
				Type:             provider.ContentToolCall,
				ToolCallID:       v.CallID,
				ToolName:         v.Name,
				Input:            json.RawMessage(v.Arguments),
				ProviderMetadata: itemIDNamespaceCallerMeta(providerName, v.ID, v.Namespace, v.Caller.Type, v.Caller.CallerID),
			})

		case responses.ResponseOutputItemProgram:
			toolName := br.toolNameMapping.toCustomToolName("programmatic_tool_calling")
			input, _ := json.Marshal(map[string]any{"code": v.Code, "fingerprint": v.Fingerprint})
			content = append(content, provider.GenerateContentPart{
				Type:             provider.ContentToolCall,
				ToolCallID:       v.CallID,
				ToolName:         toolName,
				Input:            input,
				ProviderExecuted: true,
				ProviderMetadata: itemIDMeta(providerName, v.ID),
			})

		case responses.ResponseOutputItemProgramOutput:
			toolName := br.toolNameMapping.toCustomToolName("programmatic_tool_calling")
			result, _ := json.Marshal(map[string]any{"result": v.Result, "status": v.Status})
			content = append(content, provider.GenerateContentPart{
				Type:             provider.ContentToolResult,
				ToolCallID:       v.CallID,
				ToolName:         toolName,
				Result:           result,
				ProviderMetadata: itemIDMeta(providerName, v.ID),
			})

		case responses.ResponseFunctionWebSearch:
			toolName := br.webSearchCustomToolName()
			content = append(content, providerExecutedCall(v.ID, toolName, "{}")...)
			content = append(content, provider.GenerateContentPart{
				Type:       provider.ContentToolResult,
				ToolCallID: v.ID,
				ToolName:   toolName,
				Result:     webSearchOutput(v.Action),
			})

		case responses.ResponseFileSearchToolCall:
			toolName := br.toolNameMapping.toCustomToolName("file_search")
			content = append(content, providerExecutedCall(v.ID, toolName, "{}")...)
			content = append(content, provider.GenerateContentPart{
				Type:       provider.ContentToolResult,
				ToolCallID: v.ID,
				ToolName:   toolName,
				Result:     fileSearchOutput(v.Queries, v.Results),
			})

		case responses.ResponseCodeInterpreterToolCall:
			toolName := br.toolNameMapping.toCustomToolName("code_interpreter")
			input, _ := json.Marshal(map[string]any{"code": v.Code, "containerId": v.ContainerID})
			content = append(content, providerExecutedCall(v.ID, toolName, string(input))...)
			content = append(content, provider.GenerateContentPart{
				Type:       provider.ContentToolResult,
				ToolCallID: v.ID,
				ToolName:   toolName,
				Result:     codeInterpreterOutput(v.Outputs),
			})

		case responses.ResponseOutputItemMcpCall:
			toolCallID := v.ID
			if v.ApprovalRequestID != "" {
				if mapped := br.approvalRequestToolCallIDs[v.ApprovalRequestID]; mapped != "" {
					toolCallID = mapped
				}
			}
			toolName := "mcp." + v.Name
			dyn := true
			content = append(content, provider.GenerateContentPart{
				Type:             provider.ContentToolCall,
				ToolCallID:       toolCallID,
				ToolName:         toolName,
				Input:            json.RawMessage(orEmptyObject(v.Arguments)),
				ProviderExecuted: true,
				Dynamic:          &dyn,
			})
			result, err := mcpCallResult(v)
			if err != nil {
				return nil, err
			}
			content = append(content, provider.GenerateContentPart{
				Type:             provider.ContentToolResult,
				ToolCallID:       toolCallID,
				ToolName:         toolName,
				Result:           result,
				ProviderMetadata: itemIDMeta(providerName, v.ID),
			})

		case responses.ResponseOutputItemMcpApprovalRequest:
			toolName := "mcp." + v.Name
			dummyID := generateID()
			approvalID := v.ID
			dyn := true
			content = append(content, provider.GenerateContentPart{
				Type:             provider.ContentToolCall,
				ToolCallID:       dummyID,
				ToolName:         toolName,
				Input:            json.RawMessage(orEmptyObject(v.Arguments)),
				ProviderExecuted: true,
				Dynamic:          &dyn,
			})
			content = append(content, provider.GenerateContentPart{
				Type:       provider.ContentToolApprovalRequest,
				ApprovalID: approvalID,
				ToolCallID: dummyID,
			})

		case responses.ResponseOutputItemMcpListTools:
			// skip

		case responses.ResponseCustomToolCall:
			hasFunctionCall = true
			input, _ := json.Marshal(v.Input)
			content = append(content, provider.GenerateContentPart{
				Type:             provider.ContentToolCall,
				ToolCallID:       v.CallID,
				ToolName:         v.Name,
				Input:            input,
				ProviderMetadata: itemIDMeta(providerName, v.ID),
			})

		case responses.ResponseOutputItemImageGenerationCall:
			toolName := br.toolNameMapping.toCustomToolName("image_generation")
			content = append(content, providerExecutedCall(v.ID, toolName, "{}")...)
			result, _ := json.Marshal(map[string]any{"result": v.Result})
			content = append(content, provider.GenerateContentPart{
				Type:       provider.ContentToolResult,
				ToolCallID: v.ID,
				ToolName:   toolName,
				Result:     result,
			})

		case responses.ResponseComputerToolCall:
			if v.CallID == "" {
				toolName := br.toolNameMapping.toCustomToolName("computer_use")
				content = append(content, providerExecutedCall(v.ID, toolName, "")...)
				result, _ := json.Marshal(map[string]any{
					"type":   "computer_use_tool_result",
					"status": orDefaultStatus(string(v.Status)),
				})
				content = append(content, provider.GenerateContentPart{
					Type:       provider.ContentToolResult,
					ToolCallID: v.ID,
					ToolName:   toolName,
					Result:     result,
				})
				break
			}

			hasFunctionCall = true
			input, err := mapComputerCallInput(v)
			if err != nil {
				return nil, err
			}
			content = append(content, provider.GenerateContentPart{
				Type:             provider.ContentToolCall,
				ToolCallID:       v.CallID,
				ToolName:         br.toolNameMapping.toCustomToolName("computer"),
				Input:            input,
				ProviderMetadata: itemIDMeta(providerName, v.ID),
			})

		case responses.ResponseFunctionShellToolCall:
			toolName := br.toolNameMapping.toCustomToolName("shell")
			input := shellInput(v.Action.RawJSON(), v.Action.Commands)
			part := provider.GenerateContentPart{
				Type:             provider.ContentToolCall,
				ToolCallID:       v.CallID,
				ToolName:         toolName,
				Input:            input,
				ProviderMetadata: itemIDMeta(providerName, v.ID),
			}
			if br.isShellProviderExecuted {
				part.ProviderExecuted = true
			}
			content = append(content, part)

		case responses.ResponseFunctionShellToolCallOutput:
			toolName := br.toolNameMapping.toCustomToolName("shell")
			result, _ := json.Marshal(map[string]any{"output": v.Output})
			content = append(content, provider.GenerateContentPart{
				Type:       provider.ContentToolResult,
				ToolCallID: v.CallID,
				ToolName:   toolName,
				Result:     result,
			})

		case responses.ResponseOutputItemLocalShellCall:
			toolName := br.toolNameMapping.toCustomToolName("local_shell")
			input := localShellInput(v.RawJSON())
			content = append(content, provider.GenerateContentPart{
				Type:             provider.ContentToolCall,
				ToolCallID:       v.CallID,
				ToolName:         toolName,
				Input:            input,
				ProviderMetadata: itemIDMeta(providerName, v.ID),
			})

		case responses.ResponseApplyPatchToolCall:
			toolName := br.toolNameMapping.toCustomToolName("apply_patch")
			input, _ := json.Marshal(map[string]any{"callId": v.CallID, "operation": v.Operation})
			content = append(content, provider.GenerateContentPart{
				Type:             provider.ContentToolCall,
				ToolCallID:       v.CallID,
				ToolName:         toolName,
				Input:            input,
				ProviderMetadata: itemIDMeta(providerName, v.ID),
			})

		case responses.ResponseToolSearchCall:
			toolName := br.toolNameMapping.toCustomToolName("tool_search")
			toolCallID := v.CallID
			if toolCallID == "" {
				toolCallID = v.ID
			}
			if v.Execution == "server" {
				hostedToolSearchCallIDs = append(hostedToolSearchCallIDs, toolCallID)
			}
			input, _ := json.Marshal(toolSearchInput(v.Arguments, v.CallID))
			part := provider.GenerateContentPart{
				Type:             provider.ContentToolCall,
				ToolCallID:       toolCallID,
				ToolName:         toolName,
				Input:            input,
				ProviderMetadata: itemIDMeta(providerName, v.ID),
			}
			if v.Execution == "server" {
				part.ProviderExecuted = true
			}
			content = append(content, part)

		case responses.ResponseToolSearchOutputItem:
			toolName := br.toolNameMapping.toCustomToolName("tool_search")
			toolCallID := v.CallID
			if toolCallID == "" {
				if len(hostedToolSearchCallIDs) > 0 {
					toolCallID = hostedToolSearchCallIDs[0]
					hostedToolSearchCallIDs = hostedToolSearchCallIDs[1:]
				} else {
					toolCallID = v.ID
				}
			}
			result, _ := json.Marshal(map[string]any{"tools": v.Tools})
			content = append(content, provider.GenerateContentPart{
				Type:             provider.ContentToolResult,
				ToolCallID:       toolCallID,
				ToolName:         toolName,
				Result:           result,
				ProviderMetadata: itemIDMeta(providerName, v.ID),
			})

		case responses.ResponseCompactionItem:
			b, _ := json.Marshal(map[string]any{
				"type":             "compaction",
				"itemId":           v.ID,
				"encryptedContent": v.EncryptedContent,
			})
			content = append(content, provider.GenerateContentPart{
				Type:             provider.ContentCustom,
				Kind:             "openai.compaction",
				ProviderMetadata: provider.ProviderMetadata{providerName: b},
			})
		}
	}

	usageRaw := json.RawMessage(resp.Usage.RawJSON())
	result := &provider.GenerateResult{
		Content:          content,
		FinishReason:     mapFinishReason(resp.IncompleteDetails.Reason, hasFunctionCall),
		Usage:            convertUsage(resp.Usage, usageRaw),
		ProviderMetadata: responseMeta(providerName, resp),
		Response: &provider.GenerateResponse{
			ResponseMetadata: provider.ResponseMetadata{
				ID:        resp.ID,
				ModelID:   resp.Model,
				Provider:  providerName,
				Timestamp: time.Unix(int64(resp.CreatedAt), 0),
			},
			Body: json.RawMessage(resp.RawJSON()),
		},
	}
	return result, nil
}

// providerExecutedCall builds a provider-executed tool-call content part.
func providerExecutedCall(callID, toolName, input string) []provider.GenerateContentPart {
	return []provider.GenerateContentPart{{
		Type:             provider.ContentToolCall,
		ToolCallID:       callID,
		ToolName:         toolName,
		Input:            json.RawMessage(input),
		ProviderExecuted: true,
	}}
}

func (br buildResult) webSearchCustomToolName() string {
	if br.webSearchToolName != "" {
		return br.webSearchToolName
	}
	return br.toolNameMapping.toCustomToolName("web_search")
}

func shellInput(raw string, commands []string) json.RawMessage {
	var value struct {
		Commands        []string     `json:"commands"`
		TimeoutMs       *json.Number `json:"timeout_ms"`
		MaxOutputLength *json.Number `json:"max_output_length"`
	}
	if json.Unmarshal([]byte(raw), &value) != nil {
		value.Commands = commands
	}
	action := map[string]any{"commands": value.Commands}
	if value.TimeoutMs != nil {
		action["timeoutMs"] = *value.TimeoutMs
	}
	if value.MaxOutputLength != nil {
		action["maxOutputLength"] = *value.MaxOutputLength
	}
	input, _ := json.Marshal(map[string]any{"action": action})
	return input
}

func localShellInput(raw string) json.RawMessage {
	var item struct {
		Action struct {
			Type             string             `json:"type"`
			Command          []string           `json:"command"`
			Env              *map[string]string `json:"env"`
			TimeoutMs        *json.Number       `json:"timeout_ms"`
			User             *string            `json:"user"`
			WorkingDirectory *string            `json:"working_directory"`
		} `json:"action"`
	}
	if json.Unmarshal([]byte(raw), &item) != nil {
		return json.RawMessage(`{"action":{}}`)
	}
	action := map[string]any{
		"type":    item.Action.Type,
		"command": item.Action.Command,
	}
	if item.Action.Env != nil {
		action["env"] = *item.Action.Env
	}
	if item.Action.TimeoutMs != nil {
		action["timeoutMs"] = *item.Action.TimeoutMs
	}
	if item.Action.User != nil {
		action["user"] = *item.Action.User
	}
	if item.Action.WorkingDirectory != nil {
		action["workingDirectory"] = *item.Action.WorkingDirectory
	}
	input, _ := json.Marshal(map[string]any{"action": action})
	return input
}

func mcpCallResult(call responses.ResponseOutputItemMcpCall) (json.RawMessage, error) {
	result := map[string]any{
		"type":        "call",
		"serverLabel": call.ServerLabel,
		"name":        call.Name,
		"arguments":   call.Arguments,
	}
	if call.JSON.Output.Valid() {
		result["output"] = call.Output
	}
	if call.JSON.Error.Valid() {
		result["error"] = call.Error
	} else if raw := call.JSON.Error.Raw(); raw != "" && raw != "null" {
		var structuredError map[string]any
		if err := json.Unmarshal([]byte(raw), &structuredError); err != nil {
			return nil, fmt.Errorf("openai: decoding mcp call error: %w", err)
		}
		if structuredError != nil {
			result["error"] = json.RawMessage(raw)
		}
	}
	b, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("openai: marshaling mcp call result: %w", err)
	}
	return b, nil
}

func toolSearchInput(arguments any, callID string) map[string]any {
	input := map[string]any{"arguments": arguments}
	if callID != "" {
		input["call_id"] = callID
	}
	return input
}

func toolSearchStreamInput(arguments any, hosted bool, callID string) map[string]any {
	input := map[string]any{"arguments": arguments}
	if hosted {
		input["call_id"] = nil
	} else {
		input["call_id"] = callID
	}
	return input
}

func itemIDMeta(providerName, itemID string) provider.ProviderMetadata {
	if itemID == "" {
		return nil
	}
	b, _ := json.Marshal(map[string]any{"itemId": itemID})
	return provider.ProviderMetadata{providerName: b}
}

func itemIDAndNamespaceMeta(providerName, itemID, namespace string) provider.ProviderMetadata {
	return itemIDNamespaceCallerMeta(providerName, itemID, namespace, "", "")
}

func itemIDNamespaceCallerMeta(providerName, itemID, namespace, callerType, callerID string) provider.ProviderMetadata {
	if itemID == "" && namespace == "" && callerType == "" {
		return nil
	}
	m := map[string]any{}
	if itemID != "" {
		m["itemId"] = itemID
	}
	if namespace != "" {
		m["namespace"] = namespace
	}
	if callerType != "" {
		caller := map[string]any{"type": callerType}
		if callerID != "" {
			caller["callerId"] = callerID
		}
		m["caller"] = caller
	}
	b, _ := json.Marshal(m)
	return provider.ProviderMetadata{providerName: b}
}

func textMeta(providerName, itemID, phase string, annotations json.RawMessage) provider.ProviderMetadata {
	if itemID == "" && phase == "" && len(annotations) == 0 {
		return nil
	}
	m := map[string]any{}
	if itemID != "" {
		m["itemId"] = itemID
	}
	if phase != "" {
		m["phase"] = phase
	}
	if len(annotations) > 0 && string(annotations) != "[]" {
		m["annotations"] = annotations
	}
	b, _ := json.Marshal(m)
	return provider.ProviderMetadata{providerName: b}
}

// textEndMeta builds the text-end provider metadata, including the accumulated
// annotations when present, mirroring upstream's ongoingAnnotations handling.
func textEndMeta(providerName, itemID, phase string, annotations []json.RawMessage) provider.ProviderMetadata {
	if itemID == "" && phase == "" && len(annotations) == 0 {
		return nil
	}
	m := map[string]any{"itemId": itemID}
	if phase != "" {
		m["phase"] = phase
	}
	if len(annotations) > 0 {
		m["annotations"] = annotations
	}
	b, _ := json.Marshal(m)
	return provider.ProviderMetadata{providerName: b}
}

func reasoningMeta(providerName, itemID, encrypted string) provider.ProviderMetadata {
	m := map[string]any{"itemId": itemID}
	if encrypted != "" {
		m["reasoningEncryptedContent"] = encrypted
	} else {
		m["reasoningEncryptedContent"] = nil
	}
	b, _ := json.Marshal(m)
	return provider.ProviderMetadata{providerName: b}
}

func responseMeta(providerName string, resp *responses.Response) provider.ProviderMetadata {
	m := map[string]any{"responseId": resp.ID}
	if resp.ServiceTier != "" {
		m["serviceTier"] = string(resp.ServiceTier)
	}
	if context := reasoningContext(resp); context != "" {
		m["reasoningContext"] = context
	}
	b, _ := json.Marshal(m)
	return provider.ProviderMetadata{providerName: b}
}

func reasoningContext(resp *responses.Response) string {
	if !resp.Reasoning.JSON.Context.Valid() {
		return ""
	}
	return string(resp.Reasoning.Context)
}

// rawList marshals a slice of RawJSON-able SDK values using their original wire
// JSON so zero-valued struct fields are not re-serialized. Falls back to
// json.Marshal per element.
func rawList[T any](items []T) json.RawMessage {
	raws := make([]json.RawMessage, 0, len(items))
	for i := range items {
		var raw json.RawMessage
		if r, ok := any(items[i]).(interface{ RawJSON() string }); ok {
			if s := r.RawJSON(); s != "" {
				raw = json.RawMessage(s)
			}
		}
		if raw == nil {
			raw, _ = json.Marshal(items[i])
		}
		raws = append(raws, raw)
	}
	b, _ := json.Marshal(raws)
	return b
}

// codeInterpreterOutput wraps code interpreter outputs in { "outputs": [...] }
// using each output's original wire JSON.
func codeInterpreterOutput[T any](outputs []T) json.RawMessage {
	b, _ := json.Marshal(map[string]json.RawMessage{"outputs": rawList(outputs)})
	return b
}

func fileSearchOutput(queries []string, results []responses.ResponseFileSearchToolCallResult) json.RawMessage {
	var normalizedResults []map[string]any
	if results != nil {
		normalizedResults = make([]map[string]any, 0, len(results))
		for _, result := range results {
			var attributes map[string]any
			if result.Attributes != nil {
				attributes = make(map[string]any, len(result.Attributes))
				for key, attribute := range result.Attributes {
					switch {
					case attribute.JSON.OfString.Valid():
						attributes[key] = attribute.AsString()
					case attribute.JSON.OfFloat.Valid():
						attributes[key] = attribute.AsFloat()
					case attribute.JSON.OfBool.Valid():
						attributes[key] = attribute.AsBool()
					}
				}
			}
			normalizedResults = append(normalizedResults, map[string]any{
				"attributes": attributes,
				"fileId":     result.FileID,
				"filename":   result.Filename,
				"score":      result.Score,
				"text":       result.Text,
			})
		}
	}
	b, _ := json.Marshal(map[string]any{"queries": queries, "results": normalizedResults})
	return b
}

func webSearchOutput(action responses.ResponseFunctionWebSearchActionUnion) json.RawMessage {
	out := map[string]any{}
	switch action.Type {
	case "search":
		act := map[string]any{"type": "search"}
		if action.Query != "" {
			act["query"] = action.Query
		}
		if len(action.Queries) > 0 {
			act["queries"] = action.Queries
		}
		out["action"] = act
		if len(action.Sources) > 0 {
			out["sources"] = action.Sources
		}
	case "open_page":
		out["action"] = map[string]any{"type": "openPage", "url": action.URL}
	case "find_in_page":
		out["action"] = map[string]any{"type": "findInPage", "url": action.URL, "pattern": action.Pattern}
	}
	b, _ := json.Marshal(out)
	return b
}

func orEmptyObject(s string) string {
	if s == "" {
		return "{}"
	}
	return s
}

func orDefaultStatus(s string) string {
	if s == "" {
		return "completed"
	}
	return s
}
