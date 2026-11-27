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
	chatMetadata    ChatCompletionMetadata

	textActive      bool
	reasoningActive bool
	hasRefusal      bool
	// rawFinishReason is mapped when the stream ends, because the last delta,
	// which can carry a refusal, often shares a chunk with the finish reason.
	rawFinishReason *string
	finishReason    provider.FinishReason
	// finishReasonSet records that the stream reported a finish reason or that
	// an error already decided it. A stream that ends with neither was cut short.
	finishReasonSet  bool
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

func (m *model) runStream(ctx context.Context, endpoint string, requestBody []byte, body io.Reader, headers http.Header, warnings []provider.Warning, includeRaw bool, metadataKey string, out chan<- provider.StreamPart, preserveChat bool) {
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

		if firstChunk && (chunk.ID != "" || chunk.Model != "" || (chunk.Created != nil && *chunk.Created != 0)) {
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
		if preserveChat {
			chat, err := decodeChatCompletionMetadata(data, true)
			if err != nil {
				if !sendStreamPart(ctx, out, provider.StreamPart{Type: provider.PartError, APICallError: chatMetadataError(endpoint, err)}) {
					return
				}
			} else {
				state.chatMetadata.append(chat)
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
		s.rawFinishReason = choice.FinishReason
		s.finishReasonSet = true
	}

	delta := choice.Delta
	isArray := delta.Content.array != nil
	reasoning := delta.ReasoningContent.selectText(delta.Reasoning, isArray)
	if reasoning != "" && !s.sendReasoningDelta(reasoning) {
		return false
	}

	if text := delta.Content.text; text != "" && !s.sendTextDelta(text) {
		return false
	}
	for _, part := range delta.Content.array {
		switch part.Type {
		case provider.ContentReasoning:
			if !s.sendReasoningDelta(part.Text) {
				return false
			}
		case provider.ContentText:
			if !s.sendTextDelta(part.Text) {
				return false
			}
		}
	}
	if delta.Refusal != "" {
		s.hasRefusal = true
		if !s.sendTextDelta(string(delta.Refusal)) {
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

func (s *streamState) sendReasoningDelta(delta string) bool {
	if s.textActive {
		if !sendStreamPart(s.ctx, s.out, provider.StreamPart{Type: provider.PartTextEnd, ID: "txt-0"}) {
			return false
		}
		s.textActive = false
	}
	if !s.reasoningActive {
		if !sendStreamPart(s.ctx, s.out, provider.StreamPart{Type: provider.PartReasoningStart, ID: "reasoning-0"}) {
			return false
		}
		s.reasoningActive = true
	}
	return sendStreamPart(s.ctx, s.out, provider.StreamPart{Type: provider.PartReasoningDelta, ID: "reasoning-0", Delta: delta})
}

// sendTextDelta appends to the active text part, ending active reasoning first.
// Adjacent content and refusal deltas share the part.
func (s *streamState) sendTextDelta(delta string) bool {
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
	return sendStreamPart(s.ctx, s.out, provider.StreamPart{Type: provider.PartTextDelta, ID: "txt-0", Delta: delta})
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
				Type:  provider.PartToolInputDelta,
				ID:    state.id,
				Delta: argument,
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
			Type:     provider.PartToolInputStart,
			ID:       state.id,
			ToolName: state.name,
		}) {
			return false
		}
		state.started = true
		if state.args != "" {
			if !sendStreamPart(s.ctx, s.out, provider.StreamPart{
				Type:  provider.PartToolInputDelta,
				ID:    state.id,
				Delta: state.args,
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
				Type:     provider.PartToolInputStart,
				ID:       tc.id,
				ToolName: tc.name,
			}) {
				return
			}
			if tc.args != "" {
				if !sendStreamPart(s.ctx, s.out, provider.StreamPart{
					Type:  provider.PartToolInputDelta,
					ID:    tc.id,
					Delta: tc.args,
				}) {
					return
				}
			}
		}
		if !s.finishToolCall(tc) {
			return
		}
	}

	// A stream that closes without a finish reason, through EOF, [DONE] or a
	// body that was never SSE, ended early. Reporting it as a normal finish
	// would hand the caller truncated output as success, so it ends as an
	// error, as the registered @ai-sdk/openai-compatible flush does.
	if !s.finishReasonSet {
		s.emitRecoverableError(provider.NewAPICallError(provider.APICallErrorOptions{
			Message: "openai: response stream ended without a finish reason",
			URL:     s.endpoint,
		}))
	}
	if s.rawFinishReason != nil {
		s.finishReason = mapFinishReason(s.rawFinishReason, s.hasRefusal)
	}
	metadata, err := withChatCompletionMetadata(responseProviderMetadata(s.metadataKey, s.rawUsage), s.metadataKey, s.chatMetadata)
	if err != nil {
		if !sendStreamPart(s.ctx, s.out, provider.StreamPart{Type: provider.PartError, APICallError: chatMetadataError(s.endpoint, err)}) {
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
		ProviderMetadata: metadata,
	})
}

func (s *streamState) finishToolCall(tc *streamToolCall) bool {
	if !sendStreamPart(s.ctx, s.out, provider.StreamPart{
		Type: provider.PartToolInputEnd,
		ID:   tc.id,
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
	s.rawFinishReason = nil
	s.finishReasonSet = true
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
	s.rawFinishReason = nil
	s.finishReasonSet = true
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
