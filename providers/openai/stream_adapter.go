package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/responses"
)

// ongoingToolCall tracks per-output-index tool call state during streaming.
type ongoingToolCall struct {
	toolName          string
	toolCallID        string
	containerID       string
	codeInputDone     bool
	toolCallEmitted   bool
	applyPatchHasDiff bool
	applyPatchDone    bool
}

type reasoningSummaryState string

const (
	reasoningSummaryActive      reasoningSummaryState = "active"
	reasoningSummaryCanConclude reasoningSummaryState = "can-conclude"
	reasoningSummaryConcluded   reasoningSummaryState = "concluded"
)

type activeReasoningState struct {
	encryptedContent string
	summaryParts     map[int64]reasoningSummaryState
}

// streamAdapter holds mutable per-stream state and converts Responses SSE
// events into provider.StreamParts.
type streamAdapter struct {
	warnings     []provider.Warning
	br           buildResult
	requestBody  responses.ResponseNewParams
	response     *http.Response
	generateID   func() string
	providerName string

	startEmitted           bool
	encounteredStreamError bool
	finishEmitted          bool

	ongoingToolCalls          map[int64]*ongoingToolCall
	ongoingAnnotations        []json.RawMessage
	activeReasoning           map[string]*activeReasoningState
	activeOutputItemIDs       map[int64]string
	hasFunctionCall           bool
	responseID                string
	activeMessagePhase        string
	hostedToolSearchCallIDs   []string
	streamApprovalToolCallIDs map[string]string
}

// newStreamAdapter constructs a streamAdapter with initialized maps.
func newStreamAdapter(warnings []provider.Warning, br buildResult, requestBody responses.ResponseNewParams, response *http.Response, generateID func() string, providerName string) *streamAdapter {
	return &streamAdapter{
		warnings:                  warnings,
		br:                        br,
		requestBody:               requestBody,
		response:                  response,
		generateID:                generateID,
		providerName:              providerName,
		ongoingToolCalls:          make(map[int64]*ongoingToolCall),
		activeReasoning:           make(map[string]*activeReasoningState),
		activeOutputItemIDs:       make(map[int64]string),
		streamApprovalToolCallIDs: make(map[string]string),
	}
}

// handleEvent dispatches a single Responses stream event to provider.StreamParts.
func (a *streamAdapter) handleEvent(event responses.ResponseStreamEventUnion, ch chan<- provider.StreamPart) {
	if !a.startEmitted {
		a.startEmitted = true
		ch <- provider.StreamPart{Type: provider.PartStreamStart, Warnings: a.warnings}
	}

	switch e := event.AsAny().(type) {
	case responses.ResponseCreatedEvent:
		a.responseID = e.Response.ID
		ch <- provider.StreamPart{
			Type:       provider.PartResponseMeta,
			ResponseID: e.Response.ID,
			ModelID:    e.Response.Model,
			Provider:   a.providerName,
			Timestamp:  time.Unix(int64(e.Response.CreatedAt), 0).UTC(),
		}

	case responses.ResponseOutputItemAddedEvent:
		a.handleOutputItemAdded(e, ch)

	case responses.ResponseTextDeltaEvent:
		itemID := a.resolveOutputItemID(e.ItemID, e.OutputIndex)
		ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: itemID, Delta: e.Delta}

	case responses.ResponseOutputTextAnnotationAddedEvent:
		// Accumulate the raw annotation; attached to the text-end metadata.
		if raw := e.JSON.Annotation.Raw(); raw != "" {
			a.ongoingAnnotations = append(a.ongoingAnnotations, json.RawMessage(raw))
		} else if e.Annotation != nil {
			if b, err := json.Marshal(e.Annotation); err == nil {
				a.ongoingAnnotations = append(a.ongoingAnnotations, b)
			}
		}
		a.emitAnnotationSource(e.JSON.Annotation.Raw(), e.Annotation, ch)

	case responses.ResponseFunctionCallArgumentsDeltaEvent:
		tc := a.ongoingToolCalls[e.OutputIndex]
		id := e.ItemID
		if tc != nil {
			id = tc.toolCallID
		}
		ch <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: id, Delta: e.Delta}

	case responses.ResponseReasoningSummaryTextDeltaEvent:
		itemID := a.resolveOutputItemID(e.ItemID, e.OutputIndex)
		ch <- provider.StreamPart{
			Type:             provider.PartReasoningDelta,
			ID:               fmt.Sprintf("%s:%d", itemID, e.SummaryIndex),
			Delta:            e.Delta,
			ProviderMetadata: itemIDMeta(a.providerName, itemID),
		}

	case responses.ResponseReasoningSummaryPartAddedEvent:
		itemID := a.resolveOutputItemID(e.ItemID, e.OutputIndex)
		if e.SummaryIndex > 0 {
			state := a.reasoningState(itemID)
			for index, status := range state.summaryParts {
				if status == reasoningSummaryCanConclude {
					ch <- provider.StreamPart{
						Type:             provider.PartReasoningEnd,
						ID:               fmt.Sprintf("%s:%d", itemID, index),
						ProviderMetadata: itemIDMeta(a.providerName, itemID),
					}
					state.summaryParts[index] = reasoningSummaryConcluded
				}
			}
			state.summaryParts[e.SummaryIndex] = reasoningSummaryActive
			ch <- provider.StreamPart{
				Type:             provider.PartReasoningStart,
				ID:               fmt.Sprintf("%s:%d", itemID, e.SummaryIndex),
				ProviderMetadata: reasoningMeta(a.providerName, itemID, state.encryptedContent),
			}
		}

	case responses.ResponseReasoningSummaryPartDoneEvent:
		itemID := a.resolveOutputItemID(e.ItemID, e.OutputIndex)
		state := a.reasoningState(itemID)
		if a.br.storeExplicitlyEnabled {
			ch <- provider.StreamPart{
				Type:             provider.PartReasoningEnd,
				ID:               fmt.Sprintf("%s:%d", itemID, e.SummaryIndex),
				ProviderMetadata: itemIDMeta(a.providerName, itemID),
			}
			state.summaryParts[e.SummaryIndex] = reasoningSummaryConcluded
		} else {
			state.summaryParts[e.SummaryIndex] = reasoningSummaryCanConclude
		}

	case responses.ResponseCustomToolCallInputDeltaEvent:
		id := e.ItemID
		if tc := a.ongoingToolCalls[e.OutputIndex]; tc != nil {
			id = tc.toolCallID
		}
		ch <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: id, Delta: e.Delta}

	case responses.ResponseCodeInterpreterCallCodeDeltaEvent:
		id := e.ItemID
		if tc := a.ongoingToolCalls[e.OutputIndex]; tc != nil {
			id = tc.toolCallID
		}
		ch <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: id, Delta: jsonEscape(e.Delta)}

	case responses.ResponseCodeInterpreterCallCodeDoneEvent:
		if tc := a.ongoingToolCalls[e.OutputIndex]; tc != nil {
			a.emitCodeInterpreterToolCall(tc, e.Code, ch)
		}

	case responses.ResponseImageGenCallPartialImageEvent:
		preliminary := true
		result, _ := json.Marshal(map[string]any{"result": e.PartialImageB64})
		ch <- provider.StreamPart{
			Type:        provider.PartToolResult,
			ToolCallID:  e.ItemID,
			ToolName:    a.br.toolNameMapping.toCustomToolName("image_generation"),
			Result:      result,
			Preliminary: &preliminary,
		}

	case responses.ResponseOutputItemDoneEvent:
		a.handleOutputItemDone(e, ch)

	case responses.ResponseCompletedEvent:
		a.emitFinish(e.Response, ch)

	case responses.ResponseIncompleteEvent:
		a.emitFinish(e.Response, ch)

	case responses.ResponseFailedEvent:
		if !a.encounteredStreamError {
			if apiErr := openAIStreamEventError(event, a.requestBody, a.response); apiErr != nil {
				a.encounteredStreamError = true
				ch <- provider.StreamPart{Type: provider.PartError, APICallError: apiErr}
			}
		}
		a.emitFailedFinish(e.Response, ch)

	case responses.ResponseErrorEvent:
		a.encounteredStreamError = true
		apiErr := openAIStreamEventError(event, a.requestBody, a.response)
		if apiErr == nil {
			apiErr = provider.NewAPICallError(provider.APICallErrorOptions{Message: e.Message})
		}
		ch <- provider.StreamPart{Type: provider.PartError, APICallError: apiErr}

	default:
		a.handleRawEvent(event.RawJSON(), ch)
	}
}

// handleOutputItemAdded emits start parts based on the added item type.
func (a *streamAdapter) handleOutputItemAdded(e responses.ResponseOutputItemAddedEvent, ch chan<- provider.StreamPart) {
	switch v := e.Item.AsAny().(type) {
	case responses.ResponseOutputMessage:
		a.activeOutputItemIDs[e.OutputIndex] = v.ID
		a.ongoingAnnotations = nil
		a.activeMessagePhase = string(v.Phase)
		ch <- provider.StreamPart{Type: provider.PartTextStart, ID: v.ID, ProviderMetadata: textMeta(a.providerName, v.ID, string(v.Phase), nil)}

	case responses.ResponseFunctionToolCall:
		a.ongoingToolCalls[e.OutputIndex] = &ongoingToolCall{toolName: v.Name, toolCallID: v.CallID}
		ch <- provider.StreamPart{Type: provider.PartToolInputStart, ID: v.CallID, ToolName: v.Name}

	case responses.ResponseReasoningItem:
		a.activeOutputItemIDs[e.OutputIndex] = v.ID
		a.activeReasoning[v.ID] = &activeReasoningState{
			encryptedContent: v.EncryptedContent,
			summaryParts:     map[int64]reasoningSummaryState{0: reasoningSummaryActive},
		}
		ch <- provider.StreamPart{
			Type:             provider.PartReasoningStart,
			ID:               fmt.Sprintf("%s:0", v.ID),
			ProviderMetadata: reasoningMeta(a.providerName, v.ID, v.EncryptedContent),
		}

	case responses.ResponseFunctionWebSearch:
		name := a.br.webSearchCustomToolName()
		a.ongoingToolCalls[e.OutputIndex] = &ongoingToolCall{toolName: name, toolCallID: v.ID}
		ch <- provider.StreamPart{Type: provider.PartToolInputStart, ID: v.ID, ToolName: name, ProviderExecuted: true}
		ch <- provider.StreamPart{Type: provider.PartToolInputEnd, ID: v.ID}
		ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: v.ID, ToolName: name, Input: "{}", ProviderExecuted: true}

	case responses.ResponseFileSearchToolCall:
		name := a.br.toolNameMapping.toCustomToolName("file_search")
		ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: v.ID, ToolName: name, Input: "{}", ProviderExecuted: true}

	case responses.ResponseCustomToolCall:
		toolName := v.Name
		a.ongoingToolCalls[e.OutputIndex] = &ongoingToolCall{toolName: toolName, toolCallID: v.CallID}
		ch <- provider.StreamPart{Type: provider.PartToolInputStart, ID: v.CallID, ToolName: toolName}

	case responses.ResponseComputerToolCall:
		if v.CallID == "" {
			name := a.br.toolNameMapping.toCustomToolName("computer")
			a.ongoingToolCalls[e.OutputIndex] = &ongoingToolCall{toolName: name, toolCallID: v.ID}
			ch <- provider.StreamPart{Type: provider.PartToolInputStart, ID: v.ID, ToolName: name}
			break
		}
		name := a.br.toolNameMapping.toCustomToolName("computer")
		a.ongoingToolCalls[e.OutputIndex] = &ongoingToolCall{toolName: name, toolCallID: v.CallID}
		ch <- provider.StreamPart{Type: provider.PartToolInputStart, ID: v.CallID, ToolName: name}

	case responses.ResponseCodeInterpreterToolCall:
		name := a.br.toolNameMapping.toCustomToolName("code_interpreter")
		a.ongoingToolCalls[e.OutputIndex] = &ongoingToolCall{toolName: name, toolCallID: v.ID, containerID: v.ContainerID}
		ch <- provider.StreamPart{Type: provider.PartToolInputStart, ID: v.ID, ToolName: name, ProviderExecuted: true}
		// Seed the input with the container id and an open "code" string, matching
		// upstream so the accumulated tool input is valid JSON once code deltas land.
		ch <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: v.ID, Delta: `{"containerId":"` + v.ContainerID + `","code":"`}

	case responses.ResponseOutputItemImageGenerationCall:
		name := a.br.toolNameMapping.toCustomToolName("image_generation")
		ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: v.ID, ToolName: name, Input: "{}", ProviderExecuted: true}

	case responses.ResponseToolSearchCall:
		name := a.br.toolNameMapping.toCustomToolName("tool_search")
		toolCallID := v.ID
		a.ongoingToolCalls[e.OutputIndex] = &ongoingToolCall{toolName: name, toolCallID: toolCallID}
		if v.Execution == "server" {
			ch <- provider.StreamPart{Type: provider.PartToolInputStart, ID: toolCallID, ToolName: name, ProviderExecuted: true}
		}

	case responses.ResponseApplyPatchToolCall:
		name := a.br.toolNameMapping.toCustomToolName("apply_patch")
		tc := &ongoingToolCall{toolName: name, toolCallID: v.CallID}
		a.ongoingToolCalls[e.OutputIndex] = tc
		ch <- provider.StreamPart{Type: provider.PartToolInputStart, ID: v.CallID, ToolName: name}
		if v.Operation.Type == "delete_file" {
			input := applyPatchInput(v.CallID, v.Operation)
			ch <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: v.CallID, Delta: string(input)}
			ch <- provider.StreamPart{Type: provider.PartToolInputEnd, ID: v.CallID}
			tc.applyPatchDone = true
		} else {
			ch <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: v.CallID, Delta: `{"callId":"` + jsonEscape(v.CallID) + `","operation":{"type":"` + jsonEscape(v.Operation.Type) + `","path":"` + jsonEscape(v.Operation.Path) + `","diff":"`}
		}

	case responses.ResponseFunctionShellToolCall:
		name := a.br.toolNameMapping.toCustomToolName("shell")
		a.ongoingToolCalls[e.OutputIndex] = &ongoingToolCall{toolName: name, toolCallID: v.CallID}

	case responses.ResponseOutputItemLocalShellCall:
	}
}

// handleOutputItemDone emits end/call/result parts based on the done item type.
func (a *streamAdapter) handleOutputItemDone(e responses.ResponseOutputItemDoneEvent, ch chan<- provider.StreamPart) {
	switch v := e.Item.AsAny().(type) {
	case responses.ResponseOutputMessage:
		itemID := a.resolveOutputItemID(v.ID, e.OutputIndex)
		phase := string(v.Phase)
		if phase == "" {
			phase = a.activeMessagePhase
		}
		a.activeMessagePhase = ""
		ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: itemID, ProviderMetadata: textEndMeta(a.providerName, itemID, phase, a.ongoingAnnotations)}
		a.ongoingAnnotations = nil
		delete(a.activeOutputItemIDs, e.OutputIndex)

	case responses.ResponseFunctionToolCall:
		a.hasFunctionCall = true
		delete(a.ongoingToolCalls, e.OutputIndex)
		ch <- provider.StreamPart{Type: provider.PartToolInputEnd, ID: v.CallID, ProviderMetadata: itemIDAndNamespaceMeta(a.providerName, "", v.Namespace)}
		ch <- provider.StreamPart{
			Type:             provider.PartToolCall,
			ToolCallID:       v.CallID,
			ToolName:         v.Name,
			Input:            orEmptyObject(v.Arguments),
			ProviderMetadata: itemIDNamespaceCallerMeta(a.providerName, v.ID, v.Namespace, v.Caller.Type, v.Caller.CallerID),
		}

	case responses.ResponseOutputItemProgram:
		name := a.br.toolNameMapping.toCustomToolName("programmatic_tool_calling")
		input, _ := json.Marshal(map[string]any{"code": v.Code, "fingerprint": v.Fingerprint})
		ch <- provider.StreamPart{
			Type:             provider.PartToolCall,
			ToolCallID:       v.CallID,
			ToolName:         name,
			Input:            string(input),
			ProviderExecuted: true,
			ProviderMetadata: itemIDMeta(a.providerName, v.ID),
		}

	case responses.ResponseOutputItemProgramOutput:
		name := a.br.toolNameMapping.toCustomToolName("programmatic_tool_calling")
		result, _ := json.Marshal(map[string]any{"result": v.Result, "status": v.Status})
		ch <- provider.StreamPart{
			Type:             provider.PartToolResult,
			ToolCallID:       v.CallID,
			ToolName:         name,
			Result:           result,
			ProviderMetadata: itemIDMeta(a.providerName, v.ID),
		}

	case responses.ResponseReasoningItem:
		itemID := a.resolveOutputItemID(v.ID, e.OutputIndex)
		encrypted := v.EncryptedContent
		state := a.activeReasoning[itemID]
		if encrypted == "" && state != nil {
			encrypted = state.encryptedContent
		}
		if state == nil {
			state = &activeReasoningState{summaryParts: map[int64]reasoningSummaryState{0: reasoningSummaryActive}}
		}
		for index, status := range state.summaryParts {
			if status != reasoningSummaryActive && status != reasoningSummaryCanConclude {
				continue
			}
			ch <- provider.StreamPart{
				Type:             provider.PartReasoningEnd,
				ID:               fmt.Sprintf("%s:%d", itemID, index),
				ProviderMetadata: reasoningMeta(a.providerName, itemID, encrypted),
			}
		}
		delete(a.activeReasoning, itemID)
		delete(a.activeOutputItemIDs, e.OutputIndex)

	case responses.ResponseFunctionWebSearch:
		delete(a.ongoingToolCalls, e.OutputIndex)
		name := a.br.webSearchCustomToolName()
		ch <- provider.StreamPart{
			Type:       provider.PartToolResult,
			ToolCallID: v.ID,
			ToolName:   name,
			Result:     webSearchOutput(v.Action),
		}

	case responses.ResponseFileSearchToolCall:
		name := a.br.toolNameMapping.toCustomToolName("file_search")
		ch <- provider.StreamPart{
			Type:       provider.PartToolResult,
			ToolCallID: v.ID,
			ToolName:   name,
			Result:     fileSearchOutput(v.Queries, v.Results),
		}

	case responses.ResponseCustomToolCall:
		a.hasFunctionCall = true
		delete(a.ongoingToolCalls, e.OutputIndex)
		input, _ := json.Marshal(v.Input)
		ch <- provider.StreamPart{Type: provider.PartToolInputEnd, ID: v.CallID}
		ch <- provider.StreamPart{
			Type:             provider.PartToolCall,
			ToolCallID:       v.CallID,
			ToolName:         v.Name,
			Input:            string(input),
			ProviderMetadata: itemIDMeta(a.providerName, v.ID),
		}

	case responses.ResponseComputerToolCall:
		delete(a.ongoingToolCalls, e.OutputIndex)
		if v.CallID == "" {
			name := a.br.toolNameMapping.toCustomToolName("computer_use")
			ch <- provider.StreamPart{Type: provider.PartToolInputEnd, ID: v.ID}
			ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: v.ID, ToolName: name, Input: "", ProviderExecuted: true}
			result, _ := json.Marshal(map[string]any{"type": "computer_use_tool_result", "status": orDefaultStatus(string(v.Status))})
			ch <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: v.ID, ToolName: name, Result: result}
			break
		}

		a.hasFunctionCall = true
		input, err := mapComputerCallInput(v)
		if err != nil {
			ch <- provider.StreamPart{Type: provider.PartError, APICallError: wrapAsAPICallError(err)}
			return
		}
		name := a.br.toolNameMapping.toCustomToolName("computer")
		ch <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: v.CallID, Delta: string(input)}
		ch <- provider.StreamPart{Type: provider.PartToolInputEnd, ID: v.CallID}
		ch <- provider.StreamPart{
			Type:             provider.PartToolCall,
			ToolCallID:       v.CallID,
			ToolName:         name,
			Input:            string(input),
			ProviderMetadata: itemIDMeta(a.providerName, v.ID),
		}

	case responses.ResponseCodeInterpreterToolCall:
		name := a.br.toolNameMapping.toCustomToolName("code_interpreter")
		tc := a.ongoingToolCalls[e.OutputIndex]
		if tc == nil {
			tc = &ongoingToolCall{toolName: name, toolCallID: v.ID, containerID: v.ContainerID}
		}
		a.emitCodeInterpreterToolCall(tc, v.Code, ch)
		delete(a.ongoingToolCalls, e.OutputIndex)
		ch <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: v.ID, ToolName: name, Result: codeInterpreterOutput(v.Outputs)}

	case responses.ResponseOutputItemImageGenerationCall:
		name := a.br.toolNameMapping.toCustomToolName("image_generation")
		result, _ := json.Marshal(map[string]any{"result": v.Result})
		ch <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: v.ID, ToolName: name, Result: result}

	case responses.ResponseToolSearchCall:
		name := a.br.toolNameMapping.toCustomToolName("tool_search")
		tc := a.ongoingToolCalls[e.OutputIndex]
		delete(a.ongoingToolCalls, e.OutputIndex)
		toolCallID := v.CallID
		if toolCallID == "" {
			toolCallID = v.ID
		}
		if v.Execution == "server" {
			if tc != nil {
				toolCallID = tc.toolCallID
			}
			a.hostedToolSearchCallIDs = append(a.hostedToolSearchCallIDs, toolCallID)
			ch <- provider.StreamPart{Type: provider.PartToolInputEnd, ID: toolCallID}
		} else {
			ch <- provider.StreamPart{Type: provider.PartToolInputStart, ID: toolCallID, ToolName: name}
			ch <- provider.StreamPart{Type: provider.PartToolInputEnd, ID: toolCallID}
		}
		input, _ := json.Marshal(toolSearchStreamInput(v.Arguments, v.Execution == "server", toolCallID))
		part := provider.StreamPart{Type: provider.PartToolCall, ToolCallID: toolCallID, ToolName: name, Input: string(input), ProviderMetadata: itemIDMeta(a.providerName, v.ID)}
		if v.Execution == "server" {
			part.ProviderExecuted = true
		}
		ch <- part

	case responses.ResponseToolSearchOutputItem:
		name := a.br.toolNameMapping.toCustomToolName("tool_search")
		toolCallID := v.CallID
		if toolCallID == "" {
			if len(a.hostedToolSearchCallIDs) > 0 {
				toolCallID = a.hostedToolSearchCallIDs[0]
				a.hostedToolSearchCallIDs = a.hostedToolSearchCallIDs[1:]
			} else {
				toolCallID = v.ID
			}
		}
		result := toolSearchOutput(v.RawJSON())
		ch <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: toolCallID, ToolName: name, Result: result, ProviderMetadata: itemIDMeta(a.providerName, v.ID)}

	case responses.ResponseApplyPatchToolCall:
		name := a.br.toolNameMapping.toCustomToolName("apply_patch")
		tc := a.ongoingToolCalls[e.OutputIndex]
		if tc != nil && !tc.applyPatchDone && v.Operation.Type != "delete_file" {
			if !tc.applyPatchHasDiff {
				ch <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: tc.toolCallID, Delta: jsonEscape(v.Operation.Diff)}
			}
			ch <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: tc.toolCallID, Delta: `"}}`}
			ch <- provider.StreamPart{Type: provider.PartToolInputEnd, ID: tc.toolCallID}
			tc.applyPatchDone = true
		}
		input := applyPatchInput(v.CallID, v.Operation)
		ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: v.CallID, ToolName: name, Input: string(input), ProviderMetadata: itemIDMeta(a.providerName, v.ID)}
		delete(a.ongoingToolCalls, e.OutputIndex)

	case responses.ResponseFunctionShellToolCall:
		name := a.br.toolNameMapping.toCustomToolName("shell")
		delete(a.ongoingToolCalls, e.OutputIndex)
		input := shellInput(v.Action.RawJSON(), v.Action.Commands)
		part := provider.StreamPart{Type: provider.PartToolCall, ToolCallID: v.CallID, ToolName: name, Input: string(input), ProviderMetadata: itemIDMeta(a.providerName, v.ID)}
		if a.br.isShellProviderExecuted {
			part.ProviderExecuted = true
		}
		ch <- part

	case responses.ResponseFunctionShellToolCallOutput:
		name := a.br.toolNameMapping.toCustomToolName("shell")
		result := shellOutput(v.RawJSON())
		ch <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: v.CallID, ToolName: name, Result: result}

	case responses.ResponseCompactionItem:
		ch <- provider.StreamPart{
			Type:             provider.PartCustom,
			Kind:             "openai.compaction",
			ProviderMetadata: compactionMetadata(a.providerName, v.ID, v.EncryptedContent),
		}

	case responses.ResponseOutputItemLocalShellCall:
		name := a.br.toolNameMapping.toCustomToolName("local_shell")
		input := localShellInput(v.RawJSON())
		ch <- provider.StreamPart{
			Type:             provider.PartToolCall,
			ToolCallID:       v.CallID,
			ToolName:         name,
			Input:            string(input),
			ProviderMetadata: itemIDMeta(a.providerName, v.ID),
		}

	case responses.ResponseOutputItemMcpCall:
		toolCallID := v.ID
		if v.ApprovalRequestID != "" {
			if mapped := a.streamApprovalToolCallIDs[v.ApprovalRequestID]; mapped != "" {
				toolCallID = mapped
			} else if mapped := a.br.approvalRequestToolCallIDs[v.ApprovalRequestID]; mapped != "" {
				toolCallID = mapped
			}
		}
		toolName := "mcp." + v.Name
		result, err := mcpCallResult(v)
		if err != nil {
			ch <- provider.StreamPart{Type: provider.PartError, APICallError: wrapAsAPICallError(err)}
			return
		}
		dyn := true
		ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: toolCallID, ToolName: toolName, Input: orEmptyObject(v.Arguments), ProviderExecuted: true, Dynamic: &dyn}
		ch <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: toolCallID, ToolName: toolName, Result: result, ProviderMetadata: itemIDMeta(a.providerName, v.ID)}

	case responses.ResponseOutputItemMcpApprovalRequest:
		toolName := "mcp." + v.Name
		dummyID := a.generateID()
		approvalID := v.ID
		a.streamApprovalToolCallIDs[approvalID] = dummyID
		dyn := true
		ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: dummyID, ToolName: toolName, Input: orEmptyObject(v.Arguments), ProviderExecuted: true, Dynamic: &dyn}
		ch <- provider.StreamPart{Type: provider.PartToolApprovalRequest, ApprovalID: approvalID, ToolCallID: dummyID}
	}
}

func (a *streamAdapter) emitCodeInterpreterToolCall(tc *ongoingToolCall, code string, ch chan<- provider.StreamPart) {
	if !tc.codeInputDone {
		ch <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: tc.toolCallID, Delta: `"}`}
		ch <- provider.StreamPart{Type: provider.PartToolInputEnd, ID: tc.toolCallID}
		tc.codeInputDone = true
	}
	if !tc.toolCallEmitted {
		input, _ := json.Marshal(map[string]any{"code": code, "containerId": tc.containerID})
		ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: tc.toolCallID, ToolName: tc.toolName, Input: string(input), ProviderExecuted: true}
		tc.toolCallEmitted = true
	}
}

func (a *streamAdapter) resolveOutputItemID(itemID string, outputIndex int64) string {
	if activeID := a.activeOutputItemIDs[outputIndex]; activeID != "" {
		return activeID
	}
	return itemID
}

func (a *streamAdapter) reasoningState(itemID string) *activeReasoningState {
	state := a.activeReasoning[itemID]
	if state == nil {
		state = &activeReasoningState{summaryParts: map[int64]reasoningSummaryState{}}
		a.activeReasoning[itemID] = state
	}
	return state
}

func (a *streamAdapter) emitAnnotationSource(raw string, annotation any, ch chan<- provider.StreamPart) {
	var union responses.ResponseOutputTextAnnotationUnion
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &union); err != nil {
			return
		}
	} else if annotation != nil {
		b, err := json.Marshal(annotation)
		if err != nil {
			return
		}
		if err := json.Unmarshal(b, &union); err != nil {
			return
		}
	} else {
		return
	}
	switch ann := union.AsAny().(type) {
	case responses.ResponseOutputTextAnnotationURLCitation:
		ch <- provider.StreamPart{
			Type: provider.PartSource,
			Source: &provider.SourceInfo{
				SourceType: provider.SourceTypeURL,
				ID:         a.generateID(),
				URL:        ann.URL,
				Title:      ann.Title,
			},
		}
	case responses.ResponseOutputTextAnnotationFileCitation:
		ch <- provider.StreamPart{
			Type: provider.PartSource,
			Source: &provider.SourceInfo{
				SourceType:       provider.SourceTypeDocument,
				ID:               a.generateID(),
				MediaType:        "text/plain",
				Title:            ann.Filename,
				Filename:         ann.Filename,
				ProviderMetadata: sourceMeta(a.providerName, "file_citation", ann.FileID, ann.Index),
			},
		}
	case responses.ResponseOutputTextAnnotationContainerFileCitation:
		ch <- provider.StreamPart{
			Type: provider.PartSource,
			Source: &provider.SourceInfo{
				SourceType:       provider.SourceTypeDocument,
				ID:               a.generateID(),
				MediaType:        "text/plain",
				Title:            ann.Filename,
				Filename:         ann.Filename,
				ProviderMetadata: containerSourceMeta(a.providerName, ann.FileID, ann.ContainerID),
			},
		}
	case responses.ResponseOutputTextAnnotationFilePath:
		ch <- provider.StreamPart{
			Type: provider.PartSource,
			Source: &provider.SourceInfo{
				SourceType:       provider.SourceTypeDocument,
				ID:               a.generateID(),
				MediaType:        "application/octet-stream",
				Title:            ann.FileID,
				Filename:         ann.FileID,
				ProviderMetadata: sourceMeta(a.providerName, "file_path", ann.FileID, ann.Index),
			},
		}
	}
}

func (a *streamAdapter) handleRawEvent(raw string, ch chan<- provider.StreamPart) {
	var event struct {
		Type        string `json:"type"`
		OutputIndex int64  `json:"output_index"`
		Delta       string `json:"delta"`
		Diff        string `json:"diff"`
	}
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		return
	}
	tc := a.ongoingToolCalls[event.OutputIndex]
	if tc == nil {
		return
	}
	switch event.Type {
	case "response.apply_patch_call_operation_diff.delta":
		ch <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: tc.toolCallID, Delta: jsonEscape(event.Delta)}
		tc.applyPatchHasDiff = true
	case "response.apply_patch_call_operation_diff.done":
		if !tc.applyPatchHasDiff {
			ch <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: tc.toolCallID, Delta: jsonEscape(event.Diff)}
			tc.applyPatchHasDiff = true
		}
		if !tc.applyPatchDone {
			ch <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: tc.toolCallID, Delta: `"}}`}
			ch <- provider.StreamPart{Type: provider.PartToolInputEnd, ID: tc.toolCallID}
			tc.applyPatchDone = true
		}
	}
}

// jsonEscape returns the JSON-escaped form of s without the surrounding quotes,
// so it can be appended into an open JSON string literal in a tool input delta.
func jsonEscape(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return s
	}
	escaped := strings.TrimSpace(b.String())
	if len(escaped) >= 2 {
		return escaped[1 : len(escaped)-1]
	}
	return s
}

// emitFinish emits the finish part with usage and finish reason.
func (a *streamAdapter) emitFinish(resp responses.Response, ch chan<- provider.StreamPart) {
	if a.finishEmitted {
		return
	}
	a.finishEmitted = true
	fr := mapFinishReason(resp.IncompleteDetails.Reason, a.hasFunctionCall)
	usage := convertResponseUsage(resp.Usage)
	ch <- provider.StreamPart{
		Type:             provider.PartFinish,
		FinishReason:     &fr,
		Usage:            &usage,
		ProviderMetadata: responseMeta(a.providerName, &resp),
	}
}

func (a *streamAdapter) emitFailedFinish(resp responses.Response, ch chan<- provider.StreamPart) {
	if a.finishEmitted {
		return
	}
	a.finishEmitted = true
	fr := provider.FinishReason{Unified: provider.FinishReasonError, Raw: "error"}
	if resp.IncompleteDetails.Reason != "" {
		fr = mapFinishReason(resp.IncompleteDetails.Reason, a.hasFunctionCall)
	}
	usage := convertResponseUsage(resp.Usage)
	ch <- provider.StreamPart{
		Type:             provider.PartFinish,
		FinishReason:     &fr,
		Usage:            &usage,
		ProviderMetadata: responseMeta(a.providerName, &resp),
	}
}

func (a *streamAdapter) emitPendingErrorFinish(ch chan<- provider.StreamPart) {
	if !a.encounteredStreamError || a.finishEmitted {
		return
	}
	a.finishEmitted = true
	fr := provider.FinishReason{Unified: provider.FinishReasonError, Raw: "error"}
	usage := provider.Usage{}
	resp := responses.Response{ID: a.responseID}
	ch <- provider.StreamPart{
		Type:             provider.PartFinish,
		FinishReason:     &fr,
		Usage:            &usage,
		ProviderMetadata: responseMeta(a.providerName, &resp),
	}
}
