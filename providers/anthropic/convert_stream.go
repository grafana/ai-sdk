package anthropic

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/grafana/ai-sdk/provider"
)

type blockState struct {
	blockType        string
	toolCallID       string
	toolName         string
	accumulatedInput string
	providerExecuted bool
	callerType       string
	callerToolID     string
	firstDelta       bool
	providerToolName string
	citations        []json.RawMessage
}

type mcpToolCallInfo struct {
	ToolName         string
	ProviderMetadata provider.ProviderMetadata
}

type streamAdapter struct {
	blocks                 map[int64]*blockState
	mapping                toolNameMapping
	serverToolCalls        map[string]string
	mcpToolCalls           map[string]mcpToolCallInfo
	usesJsonResponseTool   bool
	isJsonResponseFromTool bool
	usage                  anthropicUsage
	metadataFields         map[string]json.RawMessage
	messageOpen            bool
	activeMessageID        string
	invalidMessageSequence bool

	// markCodeExecutionDynamic mirrors upstream
	// hasWebTool20260209WithoutCodeExecution: when set, code_execution
	// server_tool_use blocks are flagged with dynamic: true on the
	// tool-input-start and tool-call events, so the strict tool-validation
	// layer accepts them when a 20260209 web tool implicitly triggers them.
	markCodeExecutionDynamic bool

	citationDocuments []citationDocument
	generateID        func() string
	providerName      string
}

func blockID(idx int64) string {
	return strconv.FormatInt(idx, 10)
}

func (a *streamAdapter) handleEvent(event anthropic.BetaRawMessageStreamEventUnion, ch chan<- provider.StreamPart) error {
	if a.invalidMessageSequence {
		return nil
	}

	switch e := event.AsAny().(type) {
	case anthropic.BetaRawMessageStartEvent:
		msg := e.Message
		if a.messageOpen {
			if a.activeMessageID == msg.ID {
				return nil
			}
			a.invalidMessageSequence = true
			return fmt.Errorf("received message_start for message %q while message %q is still open", msg.ID, a.activeMessageID)
		}
		a.messageOpen = true
		a.activeMessageID = msg.ID
		if err := a.resetUsage(msg.Usage); err != nil {
			return err
		}
		a.metadataFields = messageMetadataFields(msg.RawJSON())
		usage := convertAnthropicUsage(a.usage)
		ch <- provider.StreamPart{
			Type:       provider.PartResponseMeta,
			ResponseID: msg.ID,
			ModelID:    string(msg.Model),
			Provider:   a.providerName,
			Usage:      &usage,
		}

		for _, block := range msg.Content {
			if block.Type != "tool_use" {
				continue
			}
			inputStr := "{}"
			if block.Input != nil {
				b, err := json.Marshal(block.Input)
				if err != nil {
					return fmt.Errorf("marshaling pre-populated tool input: %w", err)
				}
				inputStr = string(b)
			}
			ch <- provider.StreamPart{
				Type:     provider.PartToolInputStart,
				ID:       block.ID,
				ToolName: block.Name,
			}
			ch <- provider.StreamPart{
				Type:  provider.PartToolInputDelta,
				ID:    block.ID,
				Delta: inputStr,
			}
			ch <- provider.StreamPart{
				Type: provider.PartToolInputEnd,
				ID:   block.ID,
			}
			var meta provider.ProviderMetadata
			if block.Caller.Type != "" {
				var err error
				meta, err = marshalCallerMetadata(block.Caller.Type, block.Caller.ToolID)
				if err != nil {
					return err
				}
			}
			ch <- provider.StreamPart{
				Type:             provider.PartToolCall,
				ToolCallID:       block.ID,
				ToolName:         block.Name,
				Input:            inputStr,
				ProviderMetadata: meta,
			}
		}

	case anthropic.BetaRawContentBlockStartEvent:
		idx := e.Index
		cb := e.ContentBlock
		switch cb.Type {
		case "text":
			if a.usesJsonResponseTool {
				return nil
			}
			a.blocks[idx] = &blockState{blockType: "text"}
			ch <- provider.StreamPart{Type: provider.PartTextStart, ID: blockID(idx)}
		case "thinking":
			a.blocks[idx] = &blockState{blockType: "thinking"}
			ch <- provider.StreamPart{Type: provider.PartReasoningStart, ID: blockID(idx)}
		case "redacted_thinking":
			a.blocks[idx] = &blockState{blockType: "reasoning"}
			metaJSON, err := json.Marshal(map[string]string{"redactedData": cb.Data})
			if err != nil {
				return fmt.Errorf("marshaling redacted thinking metadata: %w", err)
			}
			meta := provider.ProviderMetadata{"anthropic": metaJSON}
			ch <- provider.StreamPart{Type: provider.PartReasoningStart, ID: blockID(idx), ProviderMetadata: meta}
		case "compaction":
			a.blocks[idx] = &blockState{blockType: "text"}
			metaJSON, err := json.Marshal(map[string]string{"type": "compaction"})
			if err != nil {
				return fmt.Errorf("marshaling compaction metadata: %w", err)
			}
			meta := provider.ProviderMetadata{"anthropic": metaJSON}
			ch <- provider.StreamPart{Type: provider.PartTextStart, ID: blockID(idx), ProviderMetadata: meta}
		case "tool_use":
			if a.usesJsonResponseTool && cb.Name == jsonResponseToolName {
				a.isJsonResponseFromTool = true
				a.blocks[idx] = &blockState{blockType: "json_response"}
				ch <- provider.StreamPart{Type: provider.PartTextStart, ID: blockID(idx)}
			} else {
				initialInput := serializePrePopulatedInput(cb.Input)
				bs := &blockState{
					blockType:        "tool_use",
					toolCallID:       cb.ID,
					toolName:         cb.Name,
					accumulatedInput: initialInput,
					firstDelta:       initialInput == "",
				}
				if cb.Caller.Type != "" {
					bs.callerType = cb.Caller.Type
					bs.callerToolID = cb.Caller.ToolID
				}
				a.blocks[idx] = bs
				ch <- provider.StreamPart{
					Type:     provider.PartToolInputStart,
					ID:       cb.ID,
					ToolName: cb.Name,
				}
			}
		case "server_tool_use":
			wireName := cb.Name
			a.serverToolCalls[cb.ID] = wireName

			providerToolName := wireName
			if wireName == "bash_code_execution" || wireName == "text_editor_code_execution" {
				wireName = "code_execution"
			}

			initialInput := serializePrePopulatedInput(cb.Input)
			bs := &blockState{
				blockType:        "tool_use",
				toolCallID:       cb.ID,
				toolName:         wireName,
				accumulatedInput: initialInput,
				providerExecuted: true,
				firstDelta:       true,
				providerToolName: providerToolName,
			}
			a.blocks[idx] = bs

			startPart := provider.StreamPart{
				Type:             provider.PartToolInputStart,
				ID:               cb.ID,
				ToolName:         a.mapping.toCustomToolName(wireName),
				ProviderExecuted: true,
			}
			// Mirrors upstream anthropic-language-model.ts:1710-1713: when
			// a 20260209 web tool is configured without an explicit
			// code_execution tool, mark implicit code_execution
			// server_tool_use blocks as dynamic on tool-input-start so the
			// tool-validation layer accepts them.
			if a.markCodeExecutionDynamic && wireName == "code_execution" {
				startPart.Dynamic = ptrBool(true)
			}
			ch <- startPart
		case "web_search_tool_result":
			wsResult := cb.AsWebSearchToolResult()
			if err := a.emitWebSearchResult(wsResult, ch); err != nil {
				return err
			}
		case "web_fetch_tool_result":
			wfResult := cb.AsWebFetchToolResult()
			if err := a.emitWebFetchResult(wfResult, ch); err != nil {
				return err
			}
		case "tool_search_tool_result":
			tsResult := cb.AsToolSearchToolResult()
			if err := a.emitToolSearchResult(tsResult, ch); err != nil {
				return err
			}
		case "advisor_tool_result":
			advisorResult := cb.AsAdvisorToolResult()
			if err := a.emitAdvisorResult(advisorResult, ch); err != nil {
				return err
			}
		case "code_execution_tool_result":
			ceResult := cb.AsCodeExecutionToolResult()
			if err := a.emitCodeExecutionResult(ceResult, ch); err != nil {
				return err
			}
		case "bash_code_execution_tool_result":
			bceResult := cb.AsBashCodeExecutionToolResult()
			a.emitBashCodeExecutionResult(bceResult, ch)
		case "text_editor_code_execution_tool_result":
			teceResult := cb.AsTextEditorCodeExecutionToolResult()
			a.emitTextEditorCodeExecutionResult(teceResult, ch)
		case "mcp_tool_use":
			mtu := cb.AsMCPToolUse()
			inputJSON, err := json.Marshal(mtu.Input)
			if err != nil {
				return fmt.Errorf("marshaling mcp tool use input: %w", err)
			}
			mcpMeta, err := json.Marshal(map[string]string{
				"type":       "mcp-tool-use",
				"serverName": mtu.ServerName,
			})
			if err != nil {
				return fmt.Errorf("marshaling mcp tool use metadata: %w", err)
			}
			meta := provider.ProviderMetadata{"anthropic": mcpMeta}
			a.mcpToolCalls[mtu.ID] = mcpToolCallInfo{
				ToolName:         mtu.Name,
				ProviderMetadata: meta,
			}
			ch <- provider.StreamPart{
				Type:             provider.PartToolCall,
				ToolCallID:       mtu.ID,
				ToolName:         mtu.Name,
				Input:            string(inputJSON),
				ProviderExecuted: true,
				Dynamic:          ptrBool(true),
				ProviderMetadata: meta,
			}
		case "mcp_tool_result":
			mtr := cb.AsMCPToolResult()
			contentJSON := json.RawMessage(mtr.Content.RawJSON())
			info, ok := a.mcpToolCalls[mtr.ToolUseID]
			var toolName string
			var meta provider.ProviderMetadata
			if ok {
				toolName = info.ToolName
				meta = info.ProviderMetadata
			}
			ch <- provider.StreamPart{
				Type:             provider.PartToolResult,
				ToolCallID:       mtr.ToolUseID,
				ToolName:         toolName,
				Result:           contentJSON,
				IsError:          mtr.IsError,
				Dynamic:          ptrBool(true),
				ProviderMetadata: meta,
			}
		}

	case anthropic.BetaRawContentBlockDeltaEvent:
		idx := e.Index
		bs := a.blocks[idx]
		if bs == nil {
			return nil
		}
		delta := e.Delta
		switch delta.Type {
		case "text_delta":
			if a.usesJsonResponseTool {
				return nil
			}
			ch <- provider.StreamPart{
				Type:  provider.PartTextDelta,
				ID:    blockID(idx),
				Delta: delta.Text,
			}
		case "thinking_delta":
			ch <- provider.StreamPart{
				Type:  provider.PartReasoningDelta,
				ID:    blockID(idx),
				Delta: delta.Thinking,
			}
		case "signature_delta":
			if bs.blockType == "thinking" || bs.blockType == "reasoning" {
				metaJSON, err := json.Marshal(map[string]string{"signature": delta.Signature})
				if err != nil {
					return fmt.Errorf("marshaling signature metadata: %w", err)
				}
				meta := provider.ProviderMetadata{"anthropic": metaJSON}
				ch <- provider.StreamPart{
					Type:             provider.PartReasoningDelta,
					ID:               blockID(idx),
					ProviderMetadata: meta,
				}
			}
		case "compaction_delta":
			if delta.Content != "" {
				ch <- provider.StreamPart{
					Type:  provider.PartTextDelta,
					ID:    blockID(idx),
					Delta: delta.Content,
				}
			}
		case "input_json_delta":
			if bs.blockType == "json_response" {
				if delta.PartialJSON == "" {
					return nil
				}
				ch <- provider.StreamPart{
					Type:  provider.PartTextDelta,
					ID:    blockID(idx),
					Delta: delta.PartialJSON,
				}
			} else {
				partialJSON := delta.PartialJSON
				if bs.firstDelta && partialJSON != "" &&
					(bs.providerToolName == "bash_code_execution" || bs.providerToolName == "text_editor_code_execution") {
					partialJSON = fmt.Sprintf(`{"type": "%s",%s`, bs.providerToolName, partialJSON[1:])
					bs.firstDelta = false
				} else if bs.firstDelta && partialJSON != "" && bs.providerToolName == "code_execution" {
					partialJSON = injectProgrammaticToolCallTypeIntoDelta(partialJSON)
					bs.firstDelta = false
				} else if partialJSON != "" {
					bs.firstDelta = false
				}
				bs.accumulatedInput += partialJSON
				if partialJSON == "" {
					return nil
				}
				ch <- provider.StreamPart{
					Type:  provider.PartToolInputDelta,
					ID:    bs.toolCallID,
					Delta: partialJSON,
				}
			}
		case "citations_delta":
			cd := delta.AsCitationsDelta()
			citation := cd.Citation.AsAny()
			if bs.blockType == "text" {
				if _, ok := citation.(anthropic.BetaCitationsWebSearchResultLocation); ok {
					bs.citations = append(bs.citations, json.RawMessage(cd.Citation.RawJSON()))
				}
			}
			src, err := createCitationSource(citation, a.citationDocuments, a.generateID)
			if err != nil {
				return fmt.Errorf("creating citation source: %w", err)
			}
			if src != nil {
				ch <- provider.StreamPart{
					Type:   provider.PartSource,
					Source: src,
				}
			}
		}

	case anthropic.BetaRawContentBlockStopEvent:
		idx := e.Index
		bs := a.blocks[idx]
		if bs == nil {
			return nil
		}
		switch bs.blockType {
		case "text":
			metadata, err := marshalWebSearchCitationMetadata(bs.citations)
			if err != nil {
				return err
			}
			ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: blockID(idx), ProviderMetadata: metadata}
		case "thinking", "reasoning":
			ch <- provider.StreamPart{Type: provider.PartReasoningEnd, ID: blockID(idx)}
		case "json_response":
			ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: blockID(idx)}
		case "tool_use":
			ch <- provider.StreamPart{
				Type: provider.PartToolInputEnd,
				ID:   bs.toolCallID,
			}
			input := bs.accumulatedInput
			if input == "" {
				input = "{}"
			}
			if bs.providerExecuted && bs.providerToolName == "code_execution" {
				input = injectProgrammaticToolCallType(input)
			}
			toolName := bs.toolName
			if bs.providerExecuted {
				toolName = a.mapping.toCustomToolName(toolName)
			}
			var meta provider.ProviderMetadata
			if bs.callerType != "" {
				var err error
				meta, err = marshalCallerMetadata(bs.callerType, bs.callerToolID)
				if err != nil {
					return err
				}
			}
			callPart := provider.StreamPart{
				Type:             provider.PartToolCall,
				ToolCallID:       bs.toolCallID,
				ToolName:         toolName,
				Input:            input,
				ProviderExecuted: bs.providerExecuted,
				ProviderMetadata: meta,
			}
			// Mirrors upstream anthropic-language-model.ts:2110-2113: emit
			// dynamic: true on the final tool-call event for an implicit
			// code_execution server_tool_use triggered by a 20260209 web
			// tool.
			if a.markCodeExecutionDynamic && bs.providerExecuted && bs.toolName == "code_execution" {
				callPart.Dynamic = ptrBool(true)
			}
			ch <- callPart
		}
		delete(a.blocks, idx)

	case anthropic.BetaRawMessageDeltaEvent:
		if err := a.updateUsage(e.Usage); err != nil {
			return err
		}
		a.metadataFields = mergeMessageDeltaMetadata(a.metadataFields, e.RawJSON())
		usage := convertAnthropicUsage(a.usage)
		fr := mapFinishReason(e.Delta.StopReason)
		if a.isJsonResponseFromTool && e.Delta.StopReason == anthropic.BetaStopReasonToolUse {
			fr = provider.FinishReason{Unified: provider.FinishReasonStop, Raw: string(e.Delta.StopReason)}
		}
		providerMetadata, err := buildAnthropicProviderMetadata(a.metadataFields, a.usage.raw)
		if err != nil {
			return err
		}
		ch <- provider.StreamPart{
			Type:             provider.PartFinish,
			FinishReason:     &fr,
			Usage:            &usage,
			ProviderMetadata: providerMetadata,
		}

	case anthropic.BetaRawMessageStopEvent:
		a.messageOpen = false
		a.activeMessageID = ""
	}
	return nil
}

func (a *streamAdapter) emitWebSearchResult(block anthropic.BetaWebSearchToolResultBlock, ch chan<- provider.StreamPart) error {
	content := block.Content
	if content.Type == "web_search_tool_result_error" {
		errData, err := marshalToolResultError("web_search_tool_result_error", string(content.ErrorCode))
		if err != nil {
			return fmt.Errorf("marshaling web search error: %w", err)
		}
		ch <- provider.StreamPart{
			Type:             provider.PartToolResult,
			ToolCallID:       block.ToolUseID,
			ToolName:         a.mapping.toCustomToolName("web_search"),
			Result:           errData,
			IsError:          true,
			ProviderExecuted: true,
		}
		return nil
	}

	resultJSON, err := json.Marshal(marshalWebSearchResults(content.OfBetaWebSearchResultBlockArray))
	if err != nil {
		return fmt.Errorf("marshaling web search results: %w", err)
	}
	ch <- provider.StreamPart{
		Type:             provider.PartToolResult,
		ToolCallID:       block.ToolUseID,
		ToolName:         a.mapping.toCustomToolName("web_search"),
		Result:           resultJSON,
		ProviderExecuted: true,
	}

	for _, result := range content.OfBetaWebSearchResultBlockArray {
		pageAgeMeta, err := json.Marshal(map[string]any{"pageAge": nilIfEmpty(result.PageAge)})
		if err != nil {
			return fmt.Errorf("marshaling web search page age: %w", err)
		}
		ch <- provider.StreamPart{
			Type: provider.PartSource,
			Source: &provider.SourceInfo{
				ID:         a.generateID(),
				SourceType: provider.SourceTypeURL,
				URL:        result.URL,
				Title:      result.Title,
				ProviderMetadata: provider.ProviderMetadata{
					"anthropic": pageAgeMeta,
				},
			},
		}
	}
	return nil
}

func (a *streamAdapter) emitWebFetchResult(block anthropic.BetaWebFetchToolResultBlock, ch chan<- provider.StreamPart) error {
	content := block.Content
	if content.Type == "web_fetch_tool_result_error" {
		errData, err := json.Marshal(map[string]any{
			"type":      "web_fetch_tool_result_error",
			"errorCode": string(content.ErrorCode),
		})
		if err != nil {
			return fmt.Errorf("marshaling web fetch error: %w", err)
		}
		ch <- provider.StreamPart{
			Type:             provider.PartToolResult,
			ToolCallID:       block.ToolUseID,
			ToolName:         a.mapping.toCustomToolName("web_fetch"),
			Result:           errData,
			IsError:          true,
			ProviderExecuted: true,
		}
		return nil
	}

	title := content.Content.Title
	if title == "" {
		title = content.URL
	}
	a.citationDocuments = append(a.citationDocuments, citationDocument{
		title:     title,
		mediaType: content.Content.Source.MediaType,
	})

	resultData, err := json.Marshal(buildWebFetchResultOutput(content))
	if err != nil {
		return fmt.Errorf("marshaling web fetch result: %w", err)
	}

	ch <- provider.StreamPart{
		Type:             provider.PartToolResult,
		ToolCallID:       block.ToolUseID,
		ToolName:         a.mapping.toCustomToolName("web_fetch"),
		Result:           resultData,
		ProviderExecuted: true,
	}
	return nil
}

func marshalWebSearchResults(results []anthropic.BetaWebSearchResultBlock) []map[string]any {
	out := make([]map[string]any, len(results))
	for i, r := range results {
		out[i] = map[string]any{
			"url":              r.URL,
			"title":            nilIfEmpty(r.Title),
			"pageAge":          nilIfEmpty(r.PageAge),
			"encryptedContent": r.EncryptedContent,
			"type":             string(r.Type),
		}
	}
	return out
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func buildWebFetchResultOutput(content anthropic.BetaWebFetchToolResultBlockContentUnion) map[string]any {
	contentMap := map[string]any{
		"type":  content.Content.Type,
		"title": nilIfEmpty(content.Content.Title),
		"source": map[string]any{
			"type":      content.Content.Source.Type,
			"mediaType": content.Content.Source.MediaType,
			"data":      content.Content.Source.Data,
		},
	}
	if content.Content.Citations.RawJSON() != "" {
		contentMap["citations"] = content.Content.Citations
	}
	return map[string]any{
		"type":        "web_fetch_result",
		"url":         content.URL,
		"retrievedAt": nilIfEmpty(content.RetrievedAt),
		"content":     contentMap,
	}
}

func (a *streamAdapter) emitToolSearchResult(block anthropic.BetaToolSearchToolResultBlock, ch chan<- provider.StreamPart) error {
	providerToolName := resolveToolSearchProviderToolName(a.mapping, a.serverToolCalls, block.ToolUseID)
	content := block.Content
	if content.Type == "tool_search_tool_result_error" {
		errData, err := json.Marshal(map[string]any{
			"type":      "tool_search_tool_result_error",
			"errorCode": string(content.ErrorCode),
		})
		if err != nil {
			return fmt.Errorf("marshaling tool search error: %w", err)
		}
		ch <- provider.StreamPart{
			Type:             provider.PartToolResult,
			ToolCallID:       block.ToolUseID,
			ToolName:         a.mapping.toCustomToolName(providerToolName),
			Result:           errData,
			IsError:          true,
			ProviderExecuted: true,
		}
		return nil
	}

	resultJSON, err := json.Marshal(marshalToolSearchReferences(content.ToolReferences))
	if err != nil {
		return fmt.Errorf("marshaling tool search results: %w", err)
	}
	ch <- provider.StreamPart{
		Type:             provider.PartToolResult,
		ToolCallID:       block.ToolUseID,
		ToolName:         a.mapping.toCustomToolName(providerToolName),
		Result:           resultJSON,
		ProviderExecuted: true,
	}
	return nil
}

func marshalToolSearchReferences(refs []anthropic.BetaToolReferenceBlock) []map[string]any {
	out := make([]map[string]any, len(refs))
	for i, r := range refs {
		out[i] = map[string]any{
			"type":     string(r.Type),
			"toolName": r.ToolName,
		}
	}
	return out
}

func (a *streamAdapter) emitAdvisorResult(block anthropic.BetaAdvisorToolResultBlock, ch chan<- provider.StreamPart) error {
	resultJSON, isError, err := marshalAdvisorResult(block.Content)
	if err != nil {
		return err
	}
	ch <- provider.StreamPart{
		Type:             provider.PartToolResult,
		ToolCallID:       block.ToolUseID,
		ToolName:         a.mapping.toCustomToolName("advisor"),
		Result:           resultJSON,
		IsError:          isError,
		ProviderExecuted: true,
	}
	return nil
}

func (a *streamAdapter) emitCodeExecutionResult(block anthropic.BetaCodeExecutionToolResultBlock, ch chan<- provider.StreamPart) error {
	content := block.Content
	part := provider.StreamPart{
		Type:             provider.PartToolResult,
		ToolCallID:       block.ToolUseID,
		ToolName:         a.mapping.toCustomToolName("code_execution"),
		ProviderExecuted: true,
	}

	switch content.Type {
	case "code_execution_tool_result_error":
		resultJSON, err := json.Marshal(map[string]any{
			"type":      "code_execution_tool_result_error",
			"errorCode": string(content.ErrorCode),
		})
		if err != nil {
			return fmt.Errorf("marshaling code execution error: %w", err)
		}
		part.IsError = true
		part.Result = resultJSON
	case "code_execution_result":
		resultJSON, err := json.Marshal(map[string]any{
			"type":        content.Type,
			"stdout":      content.Stdout,
			"stderr":      content.Stderr,
			"return_code": content.ReturnCode,
			"content":     ensureSlice(content.Content),
		})
		if err != nil {
			return fmt.Errorf("marshaling code execution result: %w", err)
		}
		part.Result = resultJSON
	case "encrypted_code_execution_result":
		resultJSON, err := json.Marshal(map[string]any{
			"type":             content.Type,
			"encrypted_stdout": content.EncryptedStdout,
			"stderr":           content.Stderr,
			"return_code":      content.ReturnCode,
			"content":          ensureSlice(content.Content),
		})
		if err != nil {
			return fmt.Errorf("marshaling encrypted code execution result: %w", err)
		}
		part.Result = resultJSON
	default:
		part.Result = json.RawMessage(content.RawJSON())
	}

	ch <- part
	return nil
}

func (a *streamAdapter) emitBashCodeExecutionResult(block anthropic.BetaBashCodeExecutionToolResultBlock, ch chan<- provider.StreamPart) {
	resultJSON := json.RawMessage(block.Content.RawJSON())
	ch <- provider.StreamPart{
		Type:             provider.PartToolResult,
		ToolCallID:       block.ToolUseID,
		ToolName:         a.mapping.toCustomToolName("code_execution"),
		Result:           resultJSON,
		ProviderExecuted: true,
	}
}

func (a *streamAdapter) emitTextEditorCodeExecutionResult(block anthropic.BetaTextEditorCodeExecutionToolResultBlock, ch chan<- provider.StreamPart) {
	resultJSON := json.RawMessage(block.Content.RawJSON())
	ch <- provider.StreamPart{
		Type:             provider.PartToolResult,
		ToolCallID:       block.ToolUseID,
		ToolName:         a.mapping.toCustomToolName("code_execution"),
		Result:           resultJSON,
		ProviderExecuted: true,
	}
}

func ensureSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func serializePrePopulatedInput(input any) string {
	if input == nil {
		return ""
	}
	inputMap, ok := input.(map[string]any)
	if !ok || len(inputMap) == 0 {
		return ""
	}
	b, err := json.Marshal(input)
	if err != nil {
		return ""
	}
	return string(b)
}

func injectProgrammaticToolCallType(input string) string {
	var parsed map[string]any
	if json.Unmarshal([]byte(input), &parsed) != nil {
		return input
	}
	if _, hasCode := parsed["code"]; !hasCode {
		return input
	}
	if _, hasType := parsed["type"]; hasType {
		return input
	}
	parsed["type"] = "programmatic-tool-call"
	b, err := json.Marshal(parsed)
	if err != nil {
		return input
	}
	return string(b)
}

func injectProgrammaticToolCallTypeIntoDelta(input string) string {
	if len(input) == 0 || input[0] != '{' {
		return input
	}
	var parsed map[string]any
	if json.Unmarshal([]byte(input), &parsed) == nil {
		if _, hasCode := parsed["code"]; !hasCode {
			return input
		}
		if _, hasType := parsed["type"]; hasType {
			return input
		}
		return fmt.Sprintf(`{"type": "programmatic-tool-call",%s`, input[1:])
	}
	if !partialObjectStartsWithField(input, "code") {
		return input
	}
	return fmt.Sprintf(`{"type": "programmatic-tool-call",%s`, input[1:])
}

func partialObjectStartsWithField(input, field string) bool {
	rest := strings.TrimLeft(input[1:], " \t\r\n")
	return strings.HasPrefix(rest, strconv.Quote(field)+":")
}

type webSearchResult struct {
	URL              string  `json:"url"`
	Title            string  `json:"title"`
	PageAge          *string `json:"pageAge"`
	EncryptedContent string  `json:"encryptedContent"`
	Type             string  `json:"type"`
}

type webFetchResult struct {
	URL         string          `json:"url"`
	RetrievedAt *string         `json:"retrievedAt"`
	Content     webFetchContent `json:"content"`
}

type webFetchContent struct {
	Title     *string            `json:"title"`
	Citations *webFetchCitations `json:"citations,omitempty"`
	Source    webFetchSource     `json:"source"`
}

type webFetchCitations struct {
	Enabled bool `json:"enabled"`
}

type webFetchSource struct {
	Type      string `json:"type"`
	MediaType string `json:"mediaType"`
	Data      string `json:"data"`
}
