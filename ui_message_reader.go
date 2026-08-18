package aisdk

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

type uiMessageReaderConfig struct {
	generateID func() string
}

// UIMessageReaderOption configures UI message stream readers.
type UIMessageReaderOption interface {
	applyUIMessageReader(*uiMessageReaderConfig)
	uiMessageReaderOption()
}

type uiMessageReaderOptionFunc func(*uiMessageReaderConfig)

func (f uiMessageReaderOptionFunc) applyUIMessageReader(cfg *uiMessageReaderConfig) { f(cfg) }
func (uiMessageReaderOptionFunc) uiMessageReaderOption()                            {}

// WithUIMessageReaderGenerateID configures the fallback message ID generator
// used when a chunk stream does not provide an ID before one is needed.
func WithUIMessageReaderGenerateID(fn func() string) UIMessageReaderOption {
	return uiMessageReaderOptionFunc(func(cfg *uiMessageReaderConfig) {
		cfg.generateID = fn
	})
}

func buildUIMessageReaderConfig(opts []UIMessageReaderOption) uiMessageReaderConfig {
	var cfg uiMessageReaderConfig
	for _, opt := range opts {
		if opt != nil {
			opt.applyUIMessageReader(&cfg)
		}
	}
	if cfg.generateID == nil {
		cfg.generateID = GenerateID
	}
	return cfg
}

type partialToolCallState struct {
	text             string
	toolName         string
	dynamic          bool
	providerExecuted bool
	callMetadata     provider.ProviderMetadata
}

type uiMessageReaderState struct {
	message UIMessage
	cfg     uiMessageReaderConfig

	activeTextParts      map[string]int
	activeReasoningParts map[string]int
	partialToolCalls     map[string]*partialToolCallState
	dataPartIndex        map[string]int
	fallbackGenerated    bool
}

func newUIMessageReaderState(cfg uiMessageReaderConfig) *uiMessageReaderState {
	return &uiMessageReaderState{
		message: UIMessage{
			Role:  RoleAssistant,
			Parts: nil,
		},
		cfg:                  cfg,
		activeTextParts:      make(map[string]int),
		activeReasoningParts: make(map[string]int),
		partialToolCalls:     make(map[string]*partialToolCallState),
		dataPartIndex:        make(map[string]int),
	}
}

func (s *uiMessageReaderState) ensureID() {
	if s.message.ID != "" || s.fallbackGenerated {
		return
	}
	s.fallbackGenerated = true
	if s.cfg.generateID != nil {
		s.message.ID = s.cfg.generateID()
		return
	}
	s.message.ID = GenerateID()
}

func (s *uiMessageReaderState) snapshot() UIMessage {
	s.ensureID()
	return cloneUIMessage(s.message)
}

func (s *uiMessageReaderState) finalMessage() UIMessage {
	s.ensureID()
	return cloneUIMessage(s.message)
}

func (s *uiMessageReaderState) apply(chunk UIMessageChunk) (bool, error) {
	if !isKnownChunkType(chunk.Type) && chunk.DataName == "" {
		return false, fmt.Errorf("aisdk: unsupported UI message chunk type %q", chunk.Type)
	}
	switch chunk.Type {
	case ChunkStart:
		if chunk.MessageID != "" {
			s.message.ID = chunk.MessageID
		}
		metadataChanged, err := s.updateMessageMetadata(chunk.MessageMetadata)
		if err != nil {
			return false, err
		}
		return chunk.MessageID != "" || metadataChanged, nil

	case ChunkFinish:
		metadataChanged, err := s.updateMessageMetadata(chunk.MessageMetadata)
		if err != nil {
			return false, err
		}
		return metadataChanged, nil

	case ChunkMessageMetadata:
		return s.updateMessageMetadata(chunk.MessageMetadata)

	case ChunkStartStep:
		s.message.Parts = append(s.message.Parts, StepStartPart{})
		return false, nil

	case ChunkFinishStep:
		s.activeTextParts = make(map[string]int)
		s.activeReasoningParts = make(map[string]int)
		return false, nil

	case ChunkAbort, ChunkError:
		return false, nil

	case ChunkTextStart:
		part := TextPart{State: "streaming", ProviderMetadata: cloneProviderMetadata(chunk.ProviderMetadata)}
		s.message.Parts = append(s.message.Parts, part)
		s.activeTextParts[chunk.ID] = len(s.message.Parts) - 1
		return true, nil

	case ChunkTextDelta:
		idx, ok := s.activeTextParts[chunk.ID]
		if !ok {
			return false, fmt.Errorf("aisdk: received text-delta for missing text part %q", chunk.ID)
		}
		part := s.message.Parts[idx].(TextPart)
		part.Text += chunk.Delta
		if chunk.ProviderMetadata != nil {
			part.ProviderMetadata = cloneProviderMetadata(chunk.ProviderMetadata)
		}
		s.message.Parts[idx] = part
		return true, nil

	case ChunkTextEnd:
		idx, ok := s.activeTextParts[chunk.ID]
		if !ok {
			return false, fmt.Errorf("aisdk: received text-end for missing text part %q", chunk.ID)
		}
		part := s.message.Parts[idx].(TextPart)
		part.State = "done"
		if chunk.ProviderMetadata != nil {
			part.ProviderMetadata = cloneProviderMetadata(chunk.ProviderMetadata)
		}
		s.message.Parts[idx] = part
		delete(s.activeTextParts, chunk.ID)
		return true, nil

	case ChunkReasoningStart:
		part := ReasoningPart{ID: chunk.ID, State: "streaming", ProviderMetadata: cloneProviderMetadata(chunk.ProviderMetadata)}
		s.message.Parts = append(s.message.Parts, part)
		s.activeReasoningParts[chunk.ID] = len(s.message.Parts) - 1
		return true, nil

	case ChunkReasoningDelta:
		idx, ok := s.activeReasoningParts[chunk.ID]
		if !ok {
			return false, fmt.Errorf("aisdk: received reasoning-delta for missing reasoning part %q", chunk.ID)
		}
		part := s.message.Parts[idx].(ReasoningPart)
		part.Text += chunk.Delta
		if chunk.ProviderMetadata != nil {
			part.ProviderMetadata = cloneProviderMetadata(chunk.ProviderMetadata)
		}
		s.message.Parts[idx] = part
		return true, nil

	case ChunkReasoningEnd:
		idx, ok := s.activeReasoningParts[chunk.ID]
		if !ok {
			return false, fmt.Errorf("aisdk: received reasoning-end for missing reasoning part %q", chunk.ID)
		}
		part := s.message.Parts[idx].(ReasoningPart)
		part.State = "done"
		if chunk.ProviderMetadata != nil {
			part.ProviderMetadata = cloneProviderMetadata(chunk.ProviderMetadata)
		}
		s.message.Parts[idx] = part
		delete(s.activeReasoningParts, chunk.ID)
		return true, nil

	case ChunkFile:
		s.message.Parts = append(s.message.Parts, FilePart{
			MediaType:        chunk.MediaType,
			URL:              chunk.URL,
			Filename:         chunk.Filename,
			ProviderMetadata: cloneProviderMetadata(chunk.ProviderMetadata),
		})
		return true, nil

	case ChunkReasoningFile:
		s.message.Parts = append(s.message.Parts, ReasoningFilePart{
			MediaType:        chunk.MediaType,
			URL:              chunk.URL,
			ProviderMetadata: cloneProviderMetadata(chunk.ProviderMetadata),
		})
		return true, nil

	case ChunkSourceURL:
		s.message.Parts = append(s.message.Parts, SourceURLPart{
			SourceID:         chunk.SourceID,
			URL:              chunk.URL,
			Title:            chunk.Title,
			ProviderMetadata: cloneProviderMetadata(chunk.ProviderMetadata),
		})
		return true, nil

	case ChunkSourceDocument:
		s.message.Parts = append(s.message.Parts, SourceDocumentPart{
			SourceID:         chunk.SourceID,
			MediaType:        chunk.MediaType,
			Title:            chunk.Title,
			Filename:         chunk.Filename,
			ProviderMetadata: cloneProviderMetadata(chunk.ProviderMetadata),
		})
		return true, nil

	case ChunkToolInputStart:
		dynamic := chunk.Dynamic != nil && *chunk.Dynamic
		s.partialToolCalls[chunk.ToolCallID] = &partialToolCallState{
			toolName:         chunk.ToolName,
			dynamic:          dynamic,
			providerExecuted: chunk.ProviderExecuted,
			callMetadata:     cloneProviderMetadata(chunk.ProviderMetadata),
		}
		s.upsertToolPart(chunk.ToolCallID, chunk.ToolName, dynamic, ToolStateInputStreaming, nil, "", chunk.ProviderExecuted, chunk.ProviderMetadata, false)
		return true, nil

	case ChunkToolInputDelta:
		partial, ok := s.partialToolCalls[chunk.ToolCallID]
		if !ok {
			return false, fmt.Errorf("aisdk: received tool-input-delta for missing tool call %q", chunk.ToolCallID)
		}
		partial.text += chunk.InputTextDelta
		input := parsePartialJSONRaw(partial.text)
		s.upsertToolPart(chunk.ToolCallID, partial.toolName, partial.dynamic, ToolStateInputStreaming, input, "", partial.providerExecuted, partial.callMetadata, false)
		return true, nil

	case ChunkToolInputAvailable:
		dynamic := chunk.Dynamic != nil && *chunk.Dynamic
		s.upsertToolPart(chunk.ToolCallID, chunk.ToolName, dynamic, ToolStateInputAvailable, chunk.Input, "", chunk.ProviderExecuted, chunk.ProviderMetadata, false)
		return true, nil

	case ChunkToolInputError:
		dynamic := chunk.Dynamic != nil && *chunk.Dynamic
		if idx, ok := s.findCurrentStepToolPart(chunk.ToolCallID); ok {
			dynamic = isDynamicToolPart(s.message.Parts[idx])
		}
		s.upsertToolPart(chunk.ToolCallID, chunk.ToolName, dynamic, ToolStateOutputError, chunk.Input, chunk.ErrorText, chunk.ProviderExecuted, chunk.ProviderMetadata, true)
		return true, nil

	case ChunkToolApprovalRequest:
		idx, ok := s.findToolPart(chunk.ToolCallID)
		if !ok {
			return false, fmt.Errorf("aisdk: received tool-approval-request for missing tool call %q", chunk.ToolCallID)
		}
		s.updateToolAt(idx, func(tp *toolPartFields) {
			tp.State = ToolStateApprovalRequested
			tp.Approval = &ToolApproval{ID: chunk.ApprovalID, IsAutomatic: chunk.IsAutomatic, Signature: chunk.Signature}
		})
		return true, nil

	case ChunkToolApprovalResponse:
		idx, ok := s.findToolPartByApproval(chunk.ApprovalID)
		if !ok {
			return false, fmt.Errorf("aisdk: received tool-approval-response for missing approval %q", chunk.ApprovalID)
		}
		s.updateToolAt(idx, func(tp *toolPartFields) {
			approved := chunk.Approved
			approval := &ToolApproval{
				ID:       chunk.ApprovalID,
				Approved: &approved,
				Reason:   chunk.Reason,
			}
			if tp.Approval != nil && tp.Approval.IsAutomatic {
				approval.IsAutomatic = true
			}
			tp.State = ToolStateApprovalResponded
			tp.Approval = approval
			if chunk.ProviderExecuted {
				tp.ProviderExecuted = true
			}
			if chunk.ProviderMetadata != nil {
				tp.CallProviderMetadata = cloneProviderMetadata(chunk.ProviderMetadata)
			}
		})
		return true, nil

	case ChunkToolOutputDenied:
		idx, ok := s.findToolPart(chunk.ToolCallID)
		if !ok {
			return false, fmt.Errorf("aisdk: received tool-output-denied for missing tool call %q", chunk.ToolCallID)
		}
		s.updateToolAt(idx, func(tp *toolPartFields) { tp.State = ToolStateOutputDenied })
		return true, nil

	case ChunkToolOutputAvailable:
		idx, ok := s.findToolPart(chunk.ToolCallID)
		if !ok {
			return false, fmt.Errorf("aisdk: received tool-output-available for missing tool call %q", chunk.ToolCallID)
		}
		s.updateToolAt(idx, func(tp *toolPartFields) {
			tp.State = ToolStateOutputAvailable
			tp.Output = cloneRawMessage(chunk.Output)
			if chunk.ProviderExecuted {
				tp.ProviderExecuted = true
			}
			if chunk.ProviderMetadata != nil {
				tp.ResultProviderMetadata = cloneProviderMetadata(chunk.ProviderMetadata)
			}
		})
		return true, nil

	case ChunkToolOutputError:
		idx, ok := s.findToolPart(chunk.ToolCallID)
		if !ok {
			return false, fmt.Errorf("aisdk: received tool-output-error for missing tool call %q", chunk.ToolCallID)
		}
		s.updateToolAt(idx, func(tp *toolPartFields) {
			tp.State = ToolStateOutputError
			tp.ErrorText = chunk.ErrorText
			if chunk.ProviderExecuted {
				tp.ProviderExecuted = true
			}
			if chunk.ProviderMetadata != nil {
				tp.ResultProviderMetadata = cloneProviderMetadata(chunk.ProviderMetadata)
			}
		})
		return true, nil

	case ChunkCustom:
		s.message.Parts = append(s.message.Parts, CustomPart{
			Kind:             chunk.Kind,
			ProviderMetadata: cloneProviderMetadata(chunk.ProviderMetadata),
		})
		return true, nil

	case ChunkData:
		return s.applyDataChunk(chunk)
	}

	if chunk.DataName != "" {
		return s.applyDataChunk(chunk)
	}
	return false, nil
}

func (s *uiMessageReaderState) applyDataChunk(chunk UIMessageChunk) (bool, error) {
	if chunk.Transient || chunk.DataName == "" {
		return false, nil
	}
	part := DataPart{DataName: chunk.DataName, ID: chunk.ID, Data: cloneRawMessage(chunk.Data)}
	if chunk.ID != "" {
		key := chunk.DataName + "\x00" + chunk.ID
		if idx, ok := s.dataPartIndex[key]; ok {
			s.message.Parts[idx] = part
			return true, nil
		}
		s.dataPartIndex[key] = len(s.message.Parts)
	}
	s.message.Parts = append(s.message.Parts, part)
	return true, nil
}

func (s *uiMessageReaderState) updateMessageMetadata(metadata json.RawMessage) (bool, error) {
	if len(metadata) == 0 {
		return false, nil
	}
	merged, err := mergeRawJSONObjects(s.message.Metadata, metadata)
	if err != nil {
		return false, err
	}
	s.message.Metadata = merged
	return true, nil
}

func mergeRawJSONObjects(current, next json.RawMessage) (json.RawMessage, error) {
	if len(current) == 0 {
		return cloneRawMessage(next), nil
	}
	var currentValue any
	var nextValue any
	if err := json.Unmarshal(current, &currentValue); err != nil {
		return nil, fmt.Errorf("aisdk: parsing current message metadata: %w", err)
	}
	if err := json.Unmarshal(next, &nextValue); err != nil {
		return nil, fmt.Errorf("aisdk: parsing message metadata: %w", err)
	}
	merged := mergeJSONValues(currentValue, nextValue)
	raw, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("aisdk: marshaling message metadata: %w", err)
	}
	return raw, nil
}

func mergeJSONValues(current, next any) any {
	currentObj, currentOK := current.(map[string]any)
	nextObj, nextOK := next.(map[string]any)
	if !currentOK || !nextOK {
		return next
	}
	for key, value := range nextObj {
		if isUnsafeJSONMergeKey(key) {
			continue
		}
		if existing, ok := currentObj[key]; ok {
			currentObj[key] = mergeJSONValues(existing, value)
			continue
		}
		currentObj[key] = value
	}
	return currentObj
}

func isUnsafeJSONMergeKey(key string) bool {
	return key == "__proto__" || key == "constructor" || key == "prototype"
}

func (s *uiMessageReaderState) upsertToolPart(toolCallID, toolName string, dynamic bool, state ToolInvocationState, input json.RawMessage, errorText string, providerExecuted bool, metadata provider.ProviderMetadata, resultMetadata bool) {
	if idx, ok := s.findCurrentStepToolPart(toolCallID); ok {
		s.updateToolAt(idx, func(tp *toolPartFields) {
			tp.ToolName = toolNameOrExisting(toolName, tp.ToolName)
			tp.State = state
			tp.Input = cloneRawMessage(input)
			tp.ErrorText = errorText
			if providerExecuted {
				tp.ProviderExecuted = true
			}
			if metadata != nil {
				if resultMetadata {
					tp.ResultProviderMetadata = cloneProviderMetadata(metadata)
				} else {
					tp.CallProviderMetadata = cloneProviderMetadata(metadata)
				}
			}
		})
		return
	}

	fields := toolPartFields{
		ToolCallID:       toolCallID,
		ToolName:         toolName,
		State:            state,
		Input:            cloneRawMessage(input),
		ErrorText:        errorText,
		ProviderExecuted: providerExecuted,
	}
	if metadata != nil {
		if resultMetadata {
			fields.ResultProviderMetadata = cloneProviderMetadata(metadata)
		} else {
			fields.CallProviderMetadata = cloneProviderMetadata(metadata)
		}
	}
	if dynamic {
		s.message.Parts = append(s.message.Parts, DynamicToolUIPart(fields))
		return
	}
	s.message.Parts = append(s.message.Parts, ToolInvocationPart(fields))
}

func toolNameOrExisting(next, current string) string {
	if next != "" {
		return next
	}
	return current
}

func (s *uiMessageReaderState) findToolPart(toolCallID string) (int, bool) {
	if idx, ok := s.findCurrentStepToolPart(toolCallID); ok {
		return idx, true
	}
	for i := len(s.message.Parts) - 1; i >= 0; i-- {
		if toolPartHasID(s.message.Parts[i], toolCallID) {
			return i, true
		}
	}
	return -1, false
}

func (s *uiMessageReaderState) findCurrentStepToolPart(toolCallID string) (int, bool) {
	start := 0
	for i := len(s.message.Parts) - 1; i >= 0; i-- {
		if _, ok := s.message.Parts[i].(StepStartPart); ok {
			start = i + 1
			break
		}
	}
	for i := start; i < len(s.message.Parts); i++ {
		if toolPartHasID(s.message.Parts[i], toolCallID) {
			return i, true
		}
	}
	return -1, false
}

func toolPartHasID(part Part, toolCallID string) bool {
	switch p := part.(type) {
	case ToolInvocationPart:
		return p.ToolCallID == toolCallID
	case DynamicToolUIPart:
		return p.ToolCallID == toolCallID
	default:
		return false
	}
}

func (s *uiMessageReaderState) findToolPartByApproval(approvalID string) (int, bool) {
	for i, part := range s.message.Parts {
		switch p := part.(type) {
		case ToolInvocationPart:
			if p.Approval != nil && p.Approval.ID == approvalID {
				return i, true
			}
		case DynamicToolUIPart:
			if p.Approval != nil && p.Approval.ID == approvalID {
				return i, true
			}
		}
	}
	return -1, false
}

func isDynamicToolPart(part Part) bool {
	_, ok := part.(DynamicToolUIPart)
	return ok
}

func (s *uiMessageReaderState) updateToolAt(idx int, update func(*toolPartFields)) {
	switch part := s.message.Parts[idx].(type) {
	case ToolInvocationPart:
		fields := toolPartFields(part)
		update(&fields)
		s.message.Parts[idx] = ToolInvocationPart(fields)
	case DynamicToolUIPart:
		fields := toolPartFields(part)
		update(&fields)
		s.message.Parts[idx] = DynamicToolUIPart(fields)
	}
}

func parsePartialJSONRaw(text string) json.RawMessage {
	if raw := parseJSONRaw(text); raw != nil {
		return raw
	}
	return parseJSONRaw(fixPartialJSON(text))
}

func parseJSONRaw(text string) json.RawMessage {
	var value any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return raw
}

type fixJSONState string

const (
	fixStateRoot                    fixJSONState = "ROOT"
	fixStateFinish                  fixJSONState = "FINISH"
	fixStateInsideString            fixJSONState = "INSIDE_STRING"
	fixStateInsideStringEscape      fixJSONState = "INSIDE_STRING_ESCAPE"
	fixStateInsideStringUnicode     fixJSONState = "INSIDE_STRING_UNICODE_ESCAPE"
	fixStateInsideLiteral           fixJSONState = "INSIDE_LITERAL"
	fixStateInsideNumber            fixJSONState = "INSIDE_NUMBER"
	fixStateInsideObjectStart       fixJSONState = "INSIDE_OBJECT_START"
	fixStateInsideObjectKey         fixJSONState = "INSIDE_OBJECT_KEY"
	fixStateInsideObjectAfterKey    fixJSONState = "INSIDE_OBJECT_AFTER_KEY"
	fixStateInsideObjectBeforeValue fixJSONState = "INSIDE_OBJECT_BEFORE_VALUE"
	fixStateInsideObjectAfterValue  fixJSONState = "INSIDE_OBJECT_AFTER_VALUE"
	fixStateInsideObjectAfterComma  fixJSONState = "INSIDE_OBJECT_AFTER_COMMA"
	fixStateInsideArrayStart        fixJSONState = "INSIDE_ARRAY_START"
	fixStateInsideArrayAfterValue   fixJSONState = "INSIDE_ARRAY_AFTER_VALUE"
	fixStateInsideArrayAfterComma   fixJSONState = "INSIDE_ARRAY_AFTER_COMMA"
)

func fixPartialJSON(input string) string {
	stack := []fixJSONState{fixStateRoot}
	lastValidIndex := -1
	literalStart := -1
	unicodeEscapeDigits := 0

	isHexDigit := func(char byte) bool {
		return (char >= '0' && char <= '9') || (char >= 'A' && char <= 'F') || (char >= 'a' && char <= 'f')
	}
	popPush := func(states ...fixJSONState) {
		stack = stack[:len(stack)-1]
		stack = append(stack, states...)
	}
	processValueStart := func(char byte, i int, swapState fixJSONState) {
		switch char {
		case '"':
			lastValidIndex = i
			popPush(swapState, fixStateInsideString)
		case 'f', 't', 'n':
			lastValidIndex = i
			literalStart = i
			popPush(swapState, fixStateInsideLiteral)
		case '-':
			popPush(swapState, fixStateInsideNumber)
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			lastValidIndex = i
			popPush(swapState, fixStateInsideNumber)
		case '{':
			lastValidIndex = i
			popPush(swapState, fixStateInsideObjectStart)
		case '[':
			lastValidIndex = i
			popPush(swapState, fixStateInsideArrayStart)
		}
	}
	processAfterObjectValue := func(char byte, i int) {
		switch char {
		case ',':
			stack = stack[:len(stack)-1]
			stack = append(stack, fixStateInsideObjectAfterComma)
		case '}':
			lastValidIndex = i
			stack = stack[:len(stack)-1]
		}
	}
	processAfterArrayValue := func(char byte, i int) {
		switch char {
		case ',':
			stack = stack[:len(stack)-1]
			stack = append(stack, fixStateInsideArrayAfterComma)
		case ']':
			lastValidIndex = i
			stack = stack[:len(stack)-1]
		}
	}

	for i := 0; i < len(input); i++ {
		char := input[i]
		currentState := stack[len(stack)-1]
		switch currentState {
		case fixStateRoot:
			processValueStart(char, i, fixStateFinish)
		case fixStateInsideObjectStart:
			switch char {
			case '"':
				popPush(fixStateInsideObjectKey)
			case '}':
				lastValidIndex = i
				stack = stack[:len(stack)-1]
			}
		case fixStateInsideObjectAfterComma:
			if char == '"' {
				popPush(fixStateInsideObjectKey)
			}
		case fixStateInsideObjectKey:
			if char == '"' {
				popPush(fixStateInsideObjectAfterKey)
			}
		case fixStateInsideObjectAfterKey:
			if char == ':' {
				popPush(fixStateInsideObjectBeforeValue)
			}
		case fixStateInsideObjectBeforeValue:
			processValueStart(char, i, fixStateInsideObjectAfterValue)
		case fixStateInsideObjectAfterValue:
			processAfterObjectValue(char, i)
		case fixStateInsideString:
			switch char {
			case '"':
				stack = stack[:len(stack)-1]
				lastValidIndex = i
			case '\\':
				stack = append(stack, fixStateInsideStringEscape)
			default:
				lastValidIndex = i
			}
		case fixStateInsideArrayStart:
			if char == ']' {
				lastValidIndex = i
				stack = stack[:len(stack)-1]
			} else {
				lastValidIndex = i
				processValueStart(char, i, fixStateInsideArrayAfterValue)
			}
		case fixStateInsideArrayAfterValue:
			switch char {
			case ',':
				stack = stack[:len(stack)-1]
				stack = append(stack, fixStateInsideArrayAfterComma)
			case ']':
				lastValidIndex = i
				stack = stack[:len(stack)-1]
			default:
				lastValidIndex = i
			}
		case fixStateInsideArrayAfterComma:
			processValueStart(char, i, fixStateInsideArrayAfterValue)
		case fixStateInsideStringEscape:
			stack = stack[:len(stack)-1]
			if char == 'u' {
				unicodeEscapeDigits = 0
				stack = append(stack, fixStateInsideStringUnicode)
			} else {
				lastValidIndex = i
			}
		case fixStateInsideStringUnicode:
			if isHexDigit(char) {
				unicodeEscapeDigits++
				if unicodeEscapeDigits == 4 {
					stack = stack[:len(stack)-1]
					lastValidIndex = i
				}
			}
		case fixStateInsideNumber:
			switch char {
			case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
				lastValidIndex = i
			case 'e', 'E', '-', '.':
			case ',':
				stack = stack[:len(stack)-1]
				if stack[len(stack)-1] == fixStateInsideArrayAfterValue {
					processAfterArrayValue(char, i)
				}
				if stack[len(stack)-1] == fixStateInsideObjectAfterValue {
					processAfterObjectValue(char, i)
				}
			case '}':
				stack = stack[:len(stack)-1]
				if stack[len(stack)-1] == fixStateInsideObjectAfterValue {
					processAfterObjectValue(char, i)
				}
			case ']':
				stack = stack[:len(stack)-1]
				if stack[len(stack)-1] == fixStateInsideArrayAfterValue {
					processAfterArrayValue(char, i)
				}
			default:
				stack = stack[:len(stack)-1]
			}
		case fixStateInsideLiteral:
			partialLiteral := input[literalStart : i+1]
			if !startsWith("false", partialLiteral) && !startsWith("true", partialLiteral) && !startsWith("null", partialLiteral) {
				stack = stack[:len(stack)-1]
				if stack[len(stack)-1] == fixStateInsideObjectAfterValue {
					processAfterObjectValue(char, i)
				} else if stack[len(stack)-1] == fixStateInsideArrayAfterValue {
					processAfterArrayValue(char, i)
				}
			} else {
				lastValidIndex = i
			}
		}
	}

	result := ""
	if lastValidIndex >= 0 {
		result = input[:lastValidIndex+1]
	}
	for i := len(stack) - 1; i >= 0; i-- {
		switch stack[i] {
		case fixStateInsideString:
			result += "\""
		case fixStateInsideObjectKey, fixStateInsideObjectAfterKey, fixStateInsideObjectAfterComma, fixStateInsideObjectStart, fixStateInsideObjectBeforeValue, fixStateInsideObjectAfterValue:
			result += "}"
		case fixStateInsideArrayStart, fixStateInsideArrayAfterComma, fixStateInsideArrayAfterValue:
			result += "]"
		case fixStateInsideLiteral:
			partialLiteral := input[literalStart:]
			if startsWith("true", partialLiteral) {
				result += "true"[len(partialLiteral):]
			} else if startsWith("false", partialLiteral) {
				result += "false"[len(partialLiteral):]
			} else if startsWith("null", partialLiteral) {
				result += "null"[len(partialLiteral):]
			}
		}
	}
	return result
}

func startsWith(s, prefix string) bool { return len(prefix) <= len(s) && s[:len(prefix)] == prefix }

func cloneUIMessage(message UIMessage) UIMessage {
	clone := UIMessage{
		ID:       message.ID,
		Role:     message.Role,
		Metadata: cloneRawMessage(message.Metadata),
	}
	if message.Parts != nil {
		clone.Parts = make([]Part, len(message.Parts))
		for i, part := range message.Parts {
			clone.Parts[i] = clonePart(part)
		}
	}
	return clone
}

func clonePart(part Part) Part {
	switch p := part.(type) {
	case TextPart:
		p.ProviderMetadata = cloneProviderMetadata(p.ProviderMetadata)
		return p
	case ReasoningPart:
		p.ProviderMetadata = cloneProviderMetadata(p.ProviderMetadata)
		return p
	case ToolInvocationPart:
		p.Input = cloneRawMessage(p.Input)
		p.Output = cloneRawMessage(p.Output)
		p.CallProviderMetadata = cloneProviderMetadata(p.CallProviderMetadata)
		p.ResultProviderMetadata = cloneProviderMetadata(p.ResultProviderMetadata)
		p.Approval = cloneToolApproval(p.Approval)
		return p
	case DynamicToolUIPart:
		p.Input = cloneRawMessage(p.Input)
		p.Output = cloneRawMessage(p.Output)
		p.CallProviderMetadata = cloneProviderMetadata(p.CallProviderMetadata)
		p.ResultProviderMetadata = cloneProviderMetadata(p.ResultProviderMetadata)
		p.Approval = cloneToolApproval(p.Approval)
		return p
	case FilePart:
		p.ProviderMetadata = cloneProviderMetadata(p.ProviderMetadata)
		return p
	case ReasoningFilePart:
		p.ProviderMetadata = cloneProviderMetadata(p.ProviderMetadata)
		return p
	case SourceURLPart:
		p.ProviderMetadata = cloneProviderMetadata(p.ProviderMetadata)
		return p
	case SourceDocumentPart:
		p.ProviderMetadata = cloneProviderMetadata(p.ProviderMetadata)
		return p
	case DataPart:
		p.Data = cloneRawMessage(p.Data)
		return p
	case CustomPart:
		p.ProviderMetadata = cloneProviderMetadata(p.ProviderMetadata)
		return p
	case StepStartPart:
		return p
	default:
		return part
	}
}

func cloneToolApproval(approval *ToolApproval) *ToolApproval {
	if approval == nil {
		return nil
	}
	clone := *approval
	if approval.Approved != nil {
		approved := *approval.Approved
		clone.Approved = &approved
	}
	return &clone
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	clone := make(json.RawMessage, len(raw))
	copy(clone, raw)
	return clone
}

func cloneProviderMetadata(meta provider.ProviderMetadata) provider.ProviderMetadata {
	if meta == nil {
		return nil
	}
	clone := make(provider.ProviderMetadata, len(meta))
	for key, value := range meta {
		clone[key] = cloneRawMessage(value)
	}
	return clone
}
