package openaicompatible

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

type streamState struct {
	ctx             context.Context
	out             chan<- provider.StreamPart
	endpoint        string
	responseHeaders http.Header
	providerName    string
	metadataKey     string
	includeRaw      bool

	textActive       bool
	reasoningActive  bool
	finishReason     provider.FinishReason
	usage            *provider.Usage
	rawUsage         *openAIUsage
	toolCalls        []*streamToolCall
	toolCallsByID    map[string]*streamToolCall
	toolCallsByIndex map[int]*streamToolCall
	latestToolCall   *streamToolCall
	errorEmitted     bool
}

type streamToolCall struct {
	id               string
	idSet            bool
	providerID       string
	name             string
	nameSet          bool
	args             string
	started          bool
	finished         bool
	providerMetadata provider.ProviderMetadata
}

func (m *model) runStream(ctx context.Context, endpoint string, requestBody []byte, body io.Reader, headers http.Header, warnings []provider.Warning, includeRaw bool, metadataKey string, out chan<- provider.StreamPart) {
	state := &streamState{
		ctx:              ctx,
		out:              out,
		endpoint:         endpoint,
		responseHeaders:  headers,
		providerName:     m.providerName,
		metadataKey:      metadataKey,
		includeRaw:       includeRaw,
		finishReason:     provider.FinishReason{Unified: provider.FinishReasonOther},
		toolCallsByID:    map[string]*streamToolCall{},
		toolCallsByIndex: map[int]*streamToolCall{},
	}

	if !sendStreamPart(ctx, out, provider.StreamPart{Type: provider.PartStreamStart, Warnings: warnings}) {
		return
	}

	reader := newOpenAISSEReader(body)
	firstChunk := true
	for {
		data, err := reader.Next()
		if errors.Is(err, io.EOF) {
			state.flush()
			return
		}
		if err != nil {
			state.emitError(streamDecodeError(endpoint, err))
			state.flush()
			return
		}
		if string(data) == "[DONE]" {
			state.flush()
			return
		}
		if includeRaw {
			var rawValue json.RawMessage
			if json.Valid(data) {
				rawValue = json.RawMessage(append([]byte(nil), data...))
			}
			if !sendStreamPart(ctx, out, provider.StreamPart{
				Type:     provider.PartRaw,
				RawValue: rawValue,
			}) {
				return
			}
		}

		errorData, errorMessage, hasError, err := streamError(data)
		if err != nil {
			state.emitRecoverableError(streamDecodeError(endpoint, err))
			continue
		}
		if hasError {
			retryable := false
			state.emitProviderError(provider.NewAPICallError(provider.APICallErrorOptions{
				Message:           errorMessage,
				URL:               endpoint,
				RequestBodyValues: json.RawMessage(append([]byte(nil), requestBody...)),
				ResponseHeaders:   cloneHeaders(headers),
				ResponseBody:      string(data),
				IsRetryable:       &retryable,
				Data:              errorData,
			}))
			continue
		}

		var chunk chatCompletionResponse
		if err := json.Unmarshal(data, &chunk); err != nil {
			state.emitRecoverableError(streamDecodeError(endpoint, err))
			continue
		}
		if err := validateStreamChunk(chunk); err != nil {
			state.emitRecoverableError(streamDecodeError(endpoint, err))
			continue
		}

		if firstChunk {
			firstChunk = false
			metadata := responseMetadata(chunk.ID, chunk.Model, m.providerName, chunk.Created)
			if !sendStreamPart(ctx, out, provider.StreamPart{
				Type:            provider.PartResponseMeta,
				ResponseID:      metadata.ID,
				ModelID:         metadata.ModelID,
				Provider:        metadata.Provider,
				Timestamp:       metadata.Timestamp,
				ResponseHeaders: flattenHeaders(headers),
			}) {
				return
			}
		}
		if chunk.Usage != nil {
			usage := convertUsage(chunk.Usage)
			state.usage = &usage
			state.rawUsage = chunk.Usage
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		if !state.handleChoice(*chunk.Choices[0]) {
			return
		}
	}
}

func validateStreamChunk(chunk chatCompletionResponse) error {
	if chunk.Choices == nil {
		return errors.New("response chunk contained no choices")
	}
	for _, choice := range chunk.Choices {
		if choice == nil {
			return errors.New("response chunk contained invalid choice")
		}
		if choice.Delta.Role != "" && choice.Delta.Role != "assistant" {
			return errors.New("stream choice contained invalid role")
		}
		for _, toolCall := range choice.Delta.ToolCalls {
			if toolCall.Function == nil {
				return errors.New("stream tool call missing function")
			}
		}
	}
	return nil
}

func streamError(data []byte) (json.RawMessage, string, bool, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, "", false, err
	}
	raw, ok := envelope["error"]
	if !ok {
		return nil, "", false, nil
	}

	copied := json.RawMessage(append([]byte(nil), raw...))
	var payload openAIErrorPayload
	if err := json.Unmarshal(raw, &payload); err == nil && payload.Message != "" {
		return copied, "openai: " + payload.Message, true, nil
	}
	return copied, "openai: stream error: " + string(raw), true, nil
}

func (s *streamState) handleChoice(choice chatChoice) bool {
	if choice.FinishReason != nil {
		s.finishReason = mapFinishReason(choice.FinishReason)
	}

	delta := choice.Delta
	reasoning := delta.ReasoningContent
	if reasoning == "" {
		reasoning = delta.Reasoning
	}
	if reasoning != "" {
		if !s.reasoningActive {
			if !sendStreamPart(s.ctx, s.out, provider.StreamPart{Type: provider.PartReasoningStart, ID: "reasoning-0"}) {
				return false
			}
			s.reasoningActive = true
		}
		if !sendStreamPart(s.ctx, s.out, provider.StreamPart{Type: provider.PartReasoningDelta, ID: "reasoning-0", Delta: reasoning}) {
			return false
		}
	}

	if delta.Content != "" {
		if s.reasoningActive {
			if !sendStreamPart(s.ctx, s.out, provider.StreamPart{Type: provider.PartReasoningEnd, ID: "reasoning-0"}) {
				return false
			}
			s.reasoningActive = false
		}
		if !s.textActive {
			if !sendStreamPart(s.ctx, s.out, provider.StreamPart{Type: provider.PartTextStart, ID: "txt-0"}) {
				return false
			}
			s.textActive = true
		}
		if !sendStreamPart(s.ctx, s.out, provider.StreamPart{Type: provider.PartTextDelta, ID: "txt-0", Delta: delta.Content}) {
			return false
		}
	}

	if len(delta.ToolCalls) > 0 {
		if s.reasoningActive {
			if !sendStreamPart(s.ctx, s.out, provider.StreamPart{Type: provider.PartReasoningEnd, ID: "reasoning-0"}) {
				return false
			}
			s.reasoningActive = false
		}
		for _, toolDelta := range delta.ToolCalls {
			if !s.handleToolCallDelta(toolDelta) {
				return false
			}
		}
	}

	return true
}

func (s *streamState) handleToolCallDelta(delta chatToolCallDelta) bool {
	state := s.resolveToolCall(delta)
	if state == nil {
		state = &streamToolCall{}
		s.toolCalls = append(s.toolCalls, state)
		if delta.Index == nil && delta.Function.Name == nil {
			if delta.ID != nil {
				state.id = *delta.ID
				state.idSet = true
				state.providerID = *delta.ID
				if *delta.ID != "" {
					s.toolCallsByID[*delta.ID] = state
				}
			}
			state.finished = true
			s.latestToolCall = state
			message := "openai: stream tool call missing function name"
			if delta.ID == nil {
				message = "openai: stream tool call missing id"
			}
			s.emitError(provider.NewAPICallError(provider.APICallErrorOptions{
				Message: message,
				URL:     s.endpoint,
			}))
			return true
		}
	}
	if delta.Index != nil {
		s.toolCallsByIndex[*delta.Index] = state
	}
	s.latestToolCall = state

	if state.finished {
		return true
	}
	if delta.ID != nil && !state.started && !state.idSet {
		state.id = *delta.ID
		state.idSet = true
		state.providerID = *delta.ID
		if *delta.ID != "" {
			s.toolCallsByID[*delta.ID] = state
		}
	}
	if delta.Function.Name != nil {
		state.name = *delta.Function.Name
		state.nameSet = true
	}
	argumentPresent := delta.Function.Arguments != nil
	argument := ""
	if argumentPresent {
		argument = *delta.Function.Arguments
		if state.started {
			state.args += argument
			return sendStreamPart(s.ctx, s.out, provider.StreamPart{
				Type:       provider.PartToolInputDelta,
				ID:         state.id,
				ToolCallID: state.id,
				ToolName:   state.name,
				Delta:      argument,
			})
		}
		state.args += argument
	}
	if !state.started && state.providerMetadata == nil {
		state.providerMetadata = toolCallProviderMetadata(s.metadataKey, delta.ExtraContent)
	}
	if !state.started && state.nameSet {
		if !state.idSet {
			s.emitError(provider.NewAPICallError(provider.APICallErrorOptions{
				Message: "openai: stream tool call missing id",
				URL:     s.endpoint,
			}))
			state.finished = true
			return true
		}
		if !sendStreamPart(s.ctx, s.out, provider.StreamPart{
			Type:       provider.PartToolInputStart,
			ID:         state.id,
			ToolCallID: state.id,
			ToolName:   state.name,
		}) {
			return false
		}
		state.started = true
		if state.args != "" {
			if !sendStreamPart(s.ctx, s.out, provider.StreamPart{
				Type:       provider.PartToolInputDelta,
				ID:         state.id,
				ToolCallID: state.id,
				ToolName:   state.name,
				Delta:      state.args,
			}) {
				return false
			}
		}
	}
	return true
}

func (s *streamState) resolveToolCall(delta chatToolCallDelta) *streamToolCall {
	if delta.ID != nil && *delta.ID != "" {
		if state := s.toolCallsByID[*delta.ID]; state != nil {
			return state
		}
		if delta.Index != nil {
			if state := s.toolCallsByIndex[*delta.Index]; state != nil && !state.started {
				return state
			}
		}
		return nil
	}
	if delta.Index != nil {
		return s.toolCallsByIndex[*delta.Index]
	}
	return s.latestToolCall
}

func (s *streamState) flush() {
	if s.reasoningActive {
		_ = sendStreamPart(s.ctx, s.out, provider.StreamPart{Type: provider.PartReasoningEnd, ID: "reasoning-0"})
		s.reasoningActive = false
	}
	if s.textActive {
		_ = sendStreamPart(s.ctx, s.out, provider.StreamPart{Type: provider.PartTextEnd, ID: "txt-0"})
		s.textActive = false
	}

	for _, tc := range s.toolCalls {
		if tc.finished {
			continue
		}
		if !tc.nameSet {
			s.emitError(provider.NewAPICallError(provider.APICallErrorOptions{
				Message: "openai: stream tool call missing function name",
				URL:     s.endpoint,
			}))
			continue
		}
		if !tc.idSet {
			s.emitError(provider.NewAPICallError(provider.APICallErrorOptions{
				Message: "openai: stream tool call missing id",
				URL:     s.endpoint,
			}))
			continue
		}
		if !tc.started {
			if !sendStreamPart(s.ctx, s.out, provider.StreamPart{
				Type:       provider.PartToolInputStart,
				ID:         tc.id,
				ToolCallID: tc.id,
				ToolName:   tc.name,
			}) {
				return
			}
			if tc.args != "" {
				if !sendStreamPart(s.ctx, s.out, provider.StreamPart{
					Type:       provider.PartToolInputDelta,
					ID:         tc.id,
					ToolCallID: tc.id,
					ToolName:   tc.name,
					Delta:      tc.args,
				}) {
					return
				}
			}
		}
		if !s.finishToolCall(tc) {
			return
		}
	}

	if s.errorEmitted {
		s.finishReason = provider.FinishReason{Unified: provider.FinishReasonError}
	}
	usage := s.usage
	if usage == nil {
		usage = &provider.Usage{}
	}
	_ = sendStreamPart(s.ctx, s.out, provider.StreamPart{
		Type:             provider.PartFinish,
		FinishReason:     &s.finishReason,
		Usage:            usage,
		ProviderMetadata: responseProviderMetadata(s.metadataKey, s.rawUsage),
	})
}

func (s *streamState) finishToolCall(tc *streamToolCall) bool {
	if !sendStreamPart(s.ctx, s.out, provider.StreamPart{
		Type:       provider.PartToolInputEnd,
		ID:         tc.id,
		ToolCallID: tc.id,
		ToolName:   tc.name,
	}) {
		return false
	}
	input := tc.args
	if strings.TrimSpace(input) == "" {
		input = "{}"
	}
	if !sendStreamPart(s.ctx, s.out, provider.StreamPart{
		Type:             provider.PartToolCall,
		ToolCallID:       tc.id,
		ToolName:         tc.name,
		Input:            input,
		ProviderMetadata: tc.providerMetadata,
	}) {
		return false
	}
	tc.finished = true
	return true
}

func (s *streamState) emitProviderError(err *provider.APICallError) {
	s.finishReason = provider.FinishReason{Unified: provider.FinishReasonError}
	_ = sendStreamPart(s.ctx, s.out, provider.StreamPart{
		Type:         provider.PartError,
		APICallError: err,
	})
}

func (s *streamState) emitError(err *provider.APICallError) {
	s.errorEmitted = true
	s.emitRecoverableError(err)
}

func (s *streamState) emitRecoverableError(err *provider.APICallError) {
	s.finishReason = provider.FinishReason{Unified: provider.FinishReasonError}
	_ = sendStreamPart(s.ctx, s.out, provider.StreamPart{
		Type:         provider.PartError,
		APICallError: err,
	})
}

func sendStreamPart(ctx context.Context, ch chan<- provider.StreamPart, part provider.StreamPart) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- part:
		return true
	}
}

type openAISSEReader struct {
	br *bufio.Reader
}

func newOpenAISSEReader(r io.Reader) *openAISSEReader {
	return &openAISSEReader{br: bufio.NewReader(r)}
}

func (r *openAISSEReader) Next() ([]byte, error) {
	var data strings.Builder
	for {
		line, err := r.br.ReadString('\n')
		eof := errors.Is(err, io.EOF)
		if err != nil && !eof {
			return nil, err
		}

		if line != "" {
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				if data.Len() == 0 {
					if eof {
						return nil, io.EOF
					}
					continue
				}
				return []byte(data.String()), nil
			}
			if !strings.HasPrefix(line, ":") {
				field, value, ok := strings.Cut(line, ":")
				if ok && field == "data" {
					value = strings.TrimPrefix(value, " ")
					if data.Len() > 0 {
						data.WriteByte('\n')
					}
					data.WriteString(value)
				}
			}
		}

		if eof {
			if data.Len() == 0 {
				return nil, io.EOF
			}
			return []byte(data.String()), nil
		}
	}
}
