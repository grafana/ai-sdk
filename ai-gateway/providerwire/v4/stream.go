package v4

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/internal/streamrecover"
	"github.com/grafana/ai-sdk/provider"
)

type streamEvent struct {
	approvalID       string
	typeName         provider.StreamPartType
	warnings         []responseWarning
	id               string
	kind             string
	modelID          string
	delta            string
	timestamp        time.Time
	finishReason     provider.FinishReason
	inputUsage       unaryInputTokenUsage
	outputUsage      unaryOutputTokenUsage
	rawUsage         json.RawMessage
	toolName         string
	input            string
	result           json.RawMessage
	isError          bool
	metadata         provider.ProviderMetadata
	mediaType        string
	fileData         *provider.StreamFileData
	source           any
	citations        []provider.Citation
	inputEncoding    provider.ToolInputEncoding
	providerExecuted bool
	dynamic          *bool
	preliminary      *bool
	rawValue         json.RawMessage
}

type streamStartEvent struct {
	Type     provider.StreamPartType `json:"type"`
	Warnings []responseWarning       `json:"warnings"`
}

type streamMetadataEvent struct {
	Type provider.StreamPartType `json:"type"`
	responseIdentity
}

type streamTextEvent struct {
	Citations []provider.Citation       `json:"citations,omitzero"`
	Type      provider.StreamPartType   `json:"type"`
	ID        string                    `json:"id"`
	Delta     *string                   `json:"delta,omitempty"`
	Metadata  provider.ProviderMetadata `json:"providerMetadata,omitzero"`
}

type streamCustomEvent struct {
	Type     provider.StreamPartType   `json:"type"`
	Kind     string                    `json:"kind"`
	Metadata provider.ProviderMetadata `json:"providerMetadata,omitzero"`
}

type streamRawEvent struct {
	Type     provider.StreamPartType `json:"type"`
	RawValue json.RawMessage         `json:"rawValue"`
}

type streamFinishEvent struct {
	Type         provider.StreamPartType   `json:"type"`
	Usage        unaryUsage                `json:"usage"`
	FinishReason unaryFinishReason         `json:"finishReason"`
	Metadata     provider.ProviderMetadata `json:"providerMetadata,omitzero"`
}

func encodeStreamFrame(value streamEvent) ([]byte, bool) {
	if !validStreamEvent(value) {
		return nil, false
	}

	for _, text := range []string{value.approvalID, value.id, value.modelID, value.delta, value.toolName, value.input, value.mediaType, value.finishReason.Raw} {
		if !utf8.ValidString(text) {
			return nil, false
		}
	}
	if !utf8.Valid(value.result) || !utf8.Valid(value.rawValue) {
		return nil, false
	}
	for _, warning := range value.warnings {
		if !utf8.ValidString(warning.Details) {
			return nil, false
		}
		for _, text := range []*string{warning.Feature, warning.Setting, warning.Message} {
			if text != nil && !utf8.ValidString(*text) {
				return nil, false
			}
		}
	}
	var (
		payload []byte
		err     error
	)
	switch value.typeName {
	case provider.PartReasoningStart, provider.PartReasoningEnd:
		payload, err = json.Marshal(streamTextEvent{Type: value.typeName, ID: value.id, Metadata: value.metadata})
	case provider.PartReasoningDelta:
		payload, err = json.Marshal(streamTextEvent{Type: value.typeName, ID: value.id, Delta: &value.delta, Metadata: value.metadata})
	case provider.PartReasoningFile, provider.PartFile:
		if !validReasoningFile(value.fileData, value.mediaType) {
			return nil, false
		}
		payload, err = json.Marshal(reasoningFilePart{Type: string(value.typeName), MediaType: value.mediaType, Data: projectReasoningFile(value.fileData), Metadata: value.metadata})
	case provider.PartSource:
		payload, err = json.Marshal(value.source)
	case provider.PartCustom:
		if !validCustomKind(value.kind) {
			return nil, false
		}
		payload, err = json.Marshal(streamCustomEvent{Type: value.typeName, Kind: value.kind, Metadata: value.metadata})
	case provider.PartStreamStart:
		warnings := value.warnings
		if warnings == nil {
			warnings = []responseWarning{}
		}
		payload, err = json.Marshal(streamStartEvent{Type: value.typeName, Warnings: warnings})
	case provider.PartResponseMeta:
		identity, identityErr := mapResponseIdentity(value.id, value.modelID, value.timestamp)
		if identityErr != nil {
			return nil, false
		}
		payload, err = json.Marshal(streamMetadataEvent{Type: value.typeName, responseIdentity: identity})
	case provider.PartTextStart, provider.PartTextEnd:
		payload, err = json.Marshal(streamTextEvent{Type: value.typeName, ID: value.id, Citations: value.citations, Metadata: value.metadata})
	case provider.PartTextDelta:
		payload, err = json.Marshal(streamTextEvent{Type: value.typeName, ID: value.id, Delta: &value.delta, Metadata: value.metadata})
	case provider.PartToolInputStart:
		payload, err = json.Marshal(streamToolStartEvent{Type: value.typeName, ID: value.id, ToolName: value.toolName, InputEncoding: value.inputEncoding, ProviderExecuted: value.providerExecuted, Dynamic: value.dynamic, Metadata: value.metadata})
	case provider.PartToolInputDelta:
		payload, err = json.Marshal(streamToolDeltaEvent{Type: value.typeName, ID: value.id, Delta: value.delta, Metadata: value.metadata})
	case provider.PartToolInputEnd:
		payload, err = json.Marshal(streamToolEndEvent{Type: value.typeName, ID: value.id, Metadata: value.metadata})
	case provider.PartToolApprovalRequest:
		payload, err = json.Marshal(approvalRequestEvent{Type: string(value.typeName), ApprovalID: value.approvalID, ToolCallID: value.id, Metadata: value.metadata})
	case provider.PartToolCall:
		payload, err = json.Marshal(streamToolCallEvent{Type: value.typeName, ToolCallID: value.id, ToolName: value.toolName, Input: value.input, ProviderExecuted: value.providerExecuted, Dynamic: value.dynamic, Metadata: value.metadata})
	case provider.PartToolResult:
		payload, err = json.Marshal(streamToolResultEvent{Type: value.typeName, ToolCallID: value.id, ToolName: value.toolName, Result: value.result, IsError: value.isError, Dynamic: value.dynamic, Preliminary: value.preliminary, Metadata: value.metadata})
	case provider.PartRaw:
		payload, err = json.Marshal(streamRawEvent{Type: value.typeName, RawValue: value.rawValue})
	case provider.PartFinish:
		payload, err = json.Marshal(streamFinishEvent{
			Type:         value.typeName,
			Usage:        unaryUsage{InputTokens: value.inputUsage, OutputTokens: value.outputUsage, Raw: value.rawUsage},
			FinishReason: unaryFinishReason{Unified: value.finishReason.Unified, Raw: value.finishReason.Raw},
			Metadata:     value.metadata,
		})
	default:
		return nil, false
	}
	if err != nil {
		return nil, false
	}

	frame := make([]byte, 0, len("data: ")+len(payload)+len("\n\n"))
	frame = append(frame, "data: "...)
	frame = append(frame, payload...)
	frame = append(frame, '\n', '\n')
	return frame, true
}

func validStreamEvent(value streamEvent) bool {
	if !validMetadata(value.metadata) {
		return false
	}
	switch value.typeName {
	case provider.PartTextStart, provider.PartTextEnd:
		return validCitations(value.citations)
	case provider.PartToolInputStart:
		return value.inputEncoding == "" || value.inputEncoding == provider.ToolInputEncodingJSON || value.inputEncoding == provider.ToolInputEncodingText
	case provider.PartSource:
		return value.source != nil
	case provider.PartToolApprovalRequest:
		return value.id != "" && value.approvalID != ""
	case provider.PartRaw:
		return len(value.rawValue) > 0
	case provider.PartFinish:
		return validRawUsage(value.rawUsage)
	default:
		return true
	}
}

func (h *handler) serveStream(w http.ResponseWriter, call *execution.Call, options provider.CallOptions, history map[string]string) {
	stream, err := call.Stream()
	if err != nil {
		view := call.View(err)
		h.writeSafeError(w, safeErrorFromProvider(err), view.Metadata())
		return
	}
	defer stream.Close()
	if !commitStreamResponse(w) {
		return
	}
	h.runStream(w, stream, call, history, options.IncludeRawChunks)
}

func commitStreamResponse(w http.ResponseWriter) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.WriteHeader(http.StatusOK)
	return flushStreamResponse(w)
}

func flushStreamResponse(w http.ResponseWriter) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	err := http.NewResponseController(w).Flush()
	return err == nil || errors.Is(err, http.ErrNotSupported)
}

func writeCompleteStreamFrame(w http.ResponseWriter, frame []byte) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	n, err := w.Write(frame)
	if err != nil || n != len(frame) {
		return false
	}
	return flushStreamResponse(w)
}

type streamPartResult uint8

const (
	streamPartContinue streamPartResult = iota + 1
	streamPartAdapterFailure
	streamPartWriterFailure
)

type segmentPhase uint8

const (
	segmentOpen segmentPhase = iota
	segmentFinished
)

type streamState struct {
	approvalIDs         map[string]struct{}
	execution           execution.View
	phase               segmentPhase
	activeID            string
	segmentTextIDs      map[string]struct{}
	tools               map[string]toolStreamState
	history             map[string]toolStreamState
	reasoningIDs        map[string]struct{}
	segmentReasoningIDs map[string]struct{}
	// includeRaw records that the caller requested raw stream parts. Raw parts
	// are dropped otherwise, including any an adapter emits unrequested.
	includeRaw bool
	// afterProviderError records that the last written event was a forwarded
	// provider error, so a provider close ends the response cleanly.
	afterProviderError bool
}

func newStreamState(history map[string]string) *streamState {
	const capacity = 64
	state := &streamState{segmentTextIDs: make(map[string]struct{}, capacity), tools: make(map[string]toolStreamState, capacity), history: make(map[string]toolStreamState, len(history)), reasoningIDs: make(map[string]struct{}, capacity), segmentReasoningIDs: make(map[string]struct{}, capacity)}
	for id, name := range history {
		state.history[id] = toolStreamState{name: name, phase: toolCallEmitted, providerExecuted: true}
	}
	return state
}

func (s *streamState) quiescent() bool {
	if s.activeID != "" || len(s.reasoningIDs) != 0 {
		return false
	}
	for _, tool := range s.tools {
		if tool.phase == toolInputOpen || tool.phase == toolResultPreliminary {
			return false
		}
	}
	for _, tool := range s.history {
		if tool.phase == toolResultPreliminary {
			return false
		}
	}
	return true
}

func (s *streamState) startSegment() {
	s.phase = segmentOpen
	clear(s.segmentTextIDs)
	clear(s.segmentReasoningIDs)
}

func allowedAfterFinish(partType provider.StreamPartType) bool {
	switch partType {
	case provider.PartResponseMeta, provider.PartRaw, provider.PartError:
		return true
	default:
		return false
	}
}

func (h *handler) runStream(w http.ResponseWriter, stream *execution.Stream, call *execution.Call, history map[string]string, includeRaw bool) {
	part, wait := stream.Next()
	view := call.View(wait.Err())
	if wait != execution.WaitPart {
		if h.emitStreamEvent(w, streamEvent{typeName: provider.PartStreamStart}) != streamWriteSuccess {
			return
		}
		h.writeStreamTerminalForWait(w, wait, view)
		return
	}

	state := newStreamState(history)
	state.execution = view
	state.includeRaw = includeRaw
	if part.Type == provider.PartStreamStart {
		result := streamWriteEncodingFailure
		if warnings, err := mapWarnings(part.Warnings); err == nil {
			result = h.emitStreamEvent(w, streamEvent{typeName: provider.PartStreamStart, warnings: warnings})
		}
		switch result {
		case streamWriteSuccess:
		case streamWriteEncodingFailure:
			stream.Close()
			if h.emitStreamEvent(w, streamEvent{typeName: provider.PartStreamStart}) == streamWriteSuccess {
				h.writeStreamTerminalError(w, safeError{category: safeInternal}, view.Metadata())
			}
			return
		default:
			return
		}
	} else {
		if h.emitStreamEvent(w, streamEvent{typeName: provider.PartStreamStart}) != streamWriteSuccess {
			return
		}
		result := h.processStreamPart(w, state, part)
		if h.handleStreamPartResult(w, stream, result, state.execution) {
			return
		}
	}
	h.consumeStreamParts(w, stream, state)
}

func (h *handler) consumeStreamParts(w http.ResponseWriter, stream *execution.Stream, state *streamState) {
	for {
		part, wait := stream.Next()
		if wait != execution.WaitPart {
			if wait == execution.WaitClosed && (state.phase == segmentFinished || state.afterProviderError) {
				return
			}
			h.writeStreamTerminalForWait(w, wait, state.execution)
			return
		}
		result := h.processStreamPart(w, state, part)
		if h.handleStreamPartResult(w, stream, result, state.execution) {
			return
		}
	}
}

func (h *handler) handleStreamPartResult(w http.ResponseWriter, stream *execution.Stream, result streamPartResult, view execution.View) bool {
	switch result {
	case streamPartContinue:
		return false
	case streamPartAdapterFailure:
		stream.Close()
		h.writeStreamTerminalError(w, safeError{category: safeInternal}, view.Metadata())
	}
	return true
}

func (h *handler) processStreamPart(w http.ResponseWriter, state *streamState, part provider.StreamPart) streamPartResult {
	if state.phase == segmentFinished && !allowedAfterFinish(part.Type) {
		return streamPartAdapterFailure
	}
	if part.Type != provider.PartError && part.Type != provider.PartRaw {
		state.afterProviderError = false
	}
	switch part.Type {
	case provider.PartToolApprovalRequest:
		return h.processApprovalRequest(w, state, part)
	case provider.PartReasoningStart, provider.PartReasoningDelta, provider.PartReasoningEnd, provider.PartReasoningFile, provider.PartFile:
		return h.processReasoningPart(w, state, part)
	case provider.PartSource:
		if part.Source == nil {
			return streamPartAdapterFailure
		}
		source, err := mapSource(*part.Source)
		if err != nil {
			return streamPartAdapterFailure
		}
		if result := h.emitStreamEvent(w, streamEvent{typeName: provider.PartSource, source: source}); result != streamWriteSuccess {
			if result == streamWriteEncodingFailure {
				return streamPartAdapterFailure
			}
			return streamPartWriterFailure
		}
		return streamPartContinue
	case provider.PartCustom:
		if result := h.emitStreamEvent(w, streamEvent{typeName: provider.PartCustom, kind: part.Kind, metadata: part.ProviderMetadata}); result != streamWriteSuccess {
			if result == streamWriteEncodingFailure {
				return streamPartAdapterFailure
			}
			return streamPartWriterFailure
		}
		return streamPartContinue
	case provider.PartToolInputStart, provider.PartToolInputDelta, provider.PartToolInputEnd, provider.PartToolCall, provider.PartToolResult:
		return h.processToolStreamPart(w, state, part)
	case provider.PartResponseMeta:
		if !state.quiescent() {
			return streamPartAdapterFailure
		}
		event := streamEvent{typeName: provider.PartResponseMeta, id: part.ResponseID, modelID: part.ModelID, timestamp: part.Timestamp}
		if result := h.emitStreamEvent(w, event); result != streamWriteSuccess {
			if result == streamWriteEncodingFailure {
				return streamPartAdapterFailure
			}
			return streamPartWriterFailure
		}
		state.startSegment()
		return streamPartContinue
	case provider.PartTextStart:
		if state.activeID != "" || part.ID == "" || !utf8.ValidString(part.ID) {
			return streamPartAdapterFailure
		}
		if _, exists := state.segmentTextIDs[part.ID]; exists {
			return streamPartAdapterFailure
		}
		if result := h.emitStreamEvent(w, streamEvent{typeName: provider.PartTextStart, id: part.ID, metadata: part.ProviderMetadata}); result != streamWriteSuccess {
			if result == streamWriteEncodingFailure {
				return streamPartAdapterFailure
			}
			return streamPartWriterFailure
		}
		state.segmentTextIDs[part.ID] = struct{}{}
		state.activeID = part.ID
		return streamPartContinue
	case provider.PartTextDelta:
		if state.activeID == "" || part.ID != state.activeID {
			return streamPartAdapterFailure
		}
		if result := h.emitStreamEvent(w, streamEvent{typeName: provider.PartTextDelta, id: part.ID, delta: part.Delta, metadata: part.ProviderMetadata}); result != streamWriteSuccess {
			if result == streamWriteEncodingFailure {
				return streamPartAdapterFailure
			}
			return streamPartWriterFailure
		}
		return streamPartContinue
	case provider.PartTextEnd:
		if state.activeID == "" || part.ID != state.activeID {
			return streamPartAdapterFailure
		}
		if result := h.emitStreamEvent(w, streamEvent{typeName: provider.PartTextEnd, id: part.ID, citations: part.Citations, metadata: part.ProviderMetadata}); result != streamWriteSuccess {
			if result == streamWriteEncodingFailure {
				return streamPartAdapterFailure
			}
			return streamPartWriterFailure
		}
		state.activeID = ""
		return streamPartContinue
	case provider.PartError:
		if part.APICallError != nil && streamrecover.Is(part.APICallError) {
			return streamPartAdapterFailure
		}
		value := safeErrorFromStreamProvider(part.APICallError)
		metadata := state.execution.Metadata()
		current := state.execution.Current(part.APICallError)
		if !h.emitSafeStreamError(w, value, metadata, current) {
			return streamPartWriterFailure
		}
		state.afterProviderError = true
		return streamPartContinue
	case provider.PartFinish:
		if !state.quiescent() || len(part.Warnings) != 0 || part.FinishReason == nil || part.Usage == nil {
			return streamPartAdapterFailure
		}
		if !validStreamFinishReason(*part.FinishReason) {
			return streamPartAdapterFailure
		}
		inputUsage, err := mapInputUsage(part.Usage.InputTokens)
		if err != nil {
			return streamPartAdapterFailure
		}
		outputUsage, err := mapOutputUsage(part.Usage.OutputTokens)
		if err != nil {
			return streamPartAdapterFailure
		}
		event := streamEvent{typeName: provider.PartFinish, finishReason: *part.FinishReason, inputUsage: inputUsage, outputUsage: outputUsage, rawUsage: part.Usage.Raw, metadata: part.ProviderMetadata}
		if !validMetadata(event.metadata) {
			return streamPartAdapterFailure
		}
		if metadata := execution.Metadata(state.execution.Overview, event.metadata); metadata != nil {
			event.metadata = metadata
		}
		frame, ok := encodeStreamFrame(event)
		if !ok {
			return streamPartAdapterFailure
		}
		if !writeCompleteStreamFrame(w, frame) {
			return streamPartWriterFailure
		}
		state.phase = segmentFinished
		return streamPartContinue
	case provider.PartRaw:
		if !state.includeRaw {
			return streamPartContinue
		}
		value, ok := projectRawValue(part.RawValue)
		if !ok {
			return streamPartContinue
		}
		if result := h.emitStreamEvent(w, streamEvent{typeName: provider.PartRaw, rawValue: value}); result != streamWriteSuccess {
			if result == streamWriteEncodingFailure {
				return streamPartAdapterFailure
			}
			return streamPartWriterFailure
		}
		state.afterProviderError = false
		return streamPartContinue
	case provider.PartStreamStart:
		return streamPartAdapterFailure
	default:
		return streamPartAdapterFailure
	}
}

func validStreamFinishReason(reason provider.FinishReason) bool {
	return utf8.ValidString(reason.Raw) && validStreamUnifiedFinishReason(reason.Unified)
}

func validStreamUnifiedFinishReason(reason provider.UnifiedFinishReason) bool {
	switch reason {
	case provider.FinishReasonStop,
		provider.FinishReasonLength,
		provider.FinishReasonContentFilter,
		provider.FinishReasonToolCalls,
		provider.FinishReasonError,
		provider.FinishReasonOther:
		return true
	default:
		return false
	}
}

type streamWriteResult uint8

const (
	streamWriteSuccess streamWriteResult = iota + 1
	streamWriteEncodingFailure
	streamWriteWriterFailure
)

func (h *handler) emitStreamEvent(w http.ResponseWriter, event streamEvent) streamWriteResult {
	frame, ok := encodeStreamFrame(event)
	if !ok {
		return streamWriteEncodingFailure
	}
	if !writeCompleteStreamFrame(w, frame) {
		return streamWriteWriterFailure
	}
	return streamWriteSuccess
}

func (h *handler) emitSafeStreamError(w http.ResponseWriter, value safeError, metadata provider.ProviderMetadata, current *execution.Failure) bool {
	return writeCompleteStreamFrame(w, encodeStreamError(value, metadata, current))
}

func (h *handler) writeStreamTerminalForWait(w http.ResponseWriter, wait execution.Wait, view execution.View) {
	h.writeStreamTerminalError(w, safeErrorFromProvider(wait.Err()), view.Metadata())
}

func (h *handler) writeStreamTerminalError(w http.ResponseWriter, value safeError, metadata provider.ProviderMetadata) {
	h.emitSafeStreamError(w, value, metadata, nil)
}
