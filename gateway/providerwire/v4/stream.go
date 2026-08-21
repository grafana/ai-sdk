package v4

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/gateway/failure"
	"github.com/grafana/ai-sdk/provider"
)

type streamPartType string

const (
	streamPartStart            streamPartType = "stream-start"
	streamPartResponseMetadata streamPartType = "response-metadata"
	streamPartTextStart        streamPartType = "text-start"
	streamPartTextDelta        streamPartType = "text-delta"
	streamPartTextEnd          streamPartType = "text-end"
	streamPartFinish           streamPartType = "finish"
	streamPartError            streamPartType = "error"
)

type streamStartDTO struct {
	Type     streamPartType `json:"type"`
	Warnings []struct{}     `json:"warnings"`
}

type responseMetadataDTO struct {
	Type      streamPartType `json:"type"`
	ID        string         `json:"id,omitempty"`
	ModelID   string         `json:"modelId"`
	Timestamp string         `json:"timestamp,omitempty"`
}

type textStartDTO struct {
	Type streamPartType `json:"type"`
	ID   string         `json:"id"`
}

type textDeltaDTO struct {
	Type  streamPartType `json:"type"`
	ID    string         `json:"id"`
	Delta string         `json:"delta"`
}

type textEndDTO struct {
	Type streamPartType `json:"type"`
	ID   string         `json:"id"`
}

type finishDTO struct {
	Type         streamPartType  `json:"type"`
	FinishReason finishReasonDTO `json:"finishReason"`
	Usage        usageDTO        `json:"usage"`
}

type streamState struct {
	providerStartSeen bool
	seenProviderPart  bool
	metadataSeen      bool
	textSeen          bool
	activeTextID      string
	finishSeen        bool
}

func appendFrame(body []byte) []byte {
	frame := make([]byte, 0, len(body)+8)
	frame = append(frame, "data: "...)
	frame = append(frame, body...)
	frame = append(frame, '\n', '\n')
	return frame
}

func (h *Handler) encodeStreamDTO(dto any) ([]byte, error) {
	body, err := json.Marshal(dto)
	if err != nil {
		return nil, err
	}
	if err := h.schemas.stream.Validate(body); err != nil {
		return nil, err
	}
	frame := appendFrame(body)
	if int64(len(frame)) > h.maxEventBytes {
		return nil, errors.New("providerwire v4: stream event exceeds configured limit")
	}
	return frame, nil
}

func writeFrame(w http.ResponseWriter, frame []byte) error {
	written, err := w.Write(frame)
	if err != nil {
		return err
	}
	if written != len(frame) {
		return io.ErrShortWrite
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func (h *Handler) serveStream(w http.ResponseWriter, model provider.LanguageModel, options provider.CallOptions, canonicalModelID string, callContext context.Context, cancel context.CancelFunc) {
	callResult, ok := awaitModelCall(callContext, func() (*provider.StreamResult, error) {
		return model.DoStream(callContext, options)
	})
	if !ok {
		h.writeError(w, contextFailureValue(callContext))
		return
	}
	if callResult.err != nil {
		h.writeError(w, reduceProviderError(callContext, callResult.err))
		return
	}
	result := callResult.result
	if result == nil || result.Stream == nil {
		if value, ok := contextFailure(callContext); ok {
			h.writeError(w, value)
		} else {
			h.writeError(w, canonicalInternal)
		}
		return
	}

	w.Header().Set("Content-Type", MIMESSE)
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.WriteHeader(http.StatusOK)
	startFrame, err := h.encodeStreamDTO(streamStartDTO{Type: streamPartStart, Warnings: []struct{}{}})
	if err != nil || writeFrame(w, startFrame) != nil {
		cancel()
		return
	}

	state := streamState{}
	idleDeadline := time.Now().Add(h.idleTimeout)
	idle := time.NewTimer(h.idleTimeout)
	defer idle.Stop()
	for {
		select {
		case <-callContext.Done():
			cancel()
			h.emitTerminalError(w, contextFailureValue(callContext))
			return
		case <-idle.C:
			cancel()
			h.emitTerminalError(w, makeFailure(failure.CategoryTimeout, messageTimeout))
			return
		case part, open := <-result.Stream:
			if idleExpired(time.Now(), idleDeadline) {
				cancel()
				h.emitTerminalError(w, makeFailure(failure.CategoryTimeout, messageTimeout))
				return
			}
			dto, emit, terminal, closed := mapReceivedStreamPart(callContext, &state, part, open, canonicalModelID)
			if terminal != nil {
				cancel()
				h.emitTerminalError(w, *terminal)
				return
			}
			if closed {
				return
			}
			if !emit {
				continue
			}
			frame, err := h.encodeStreamDTO(dto)
			if err != nil {
				cancel()
				h.emitTerminalError(w, canonicalInternal)
				return
			}
			if err := writeFrame(w, frame); err != nil {
				cancel()
				return
			}
			idleDeadline = time.Now().Add(h.idleTimeout)
			resetTimer(idle, h.idleTimeout)
		}
	}
}

func idleExpired(now, deadline time.Time) bool { return !now.Before(deadline) }

func contextFailureValue(ctx context.Context) failure.Failure {
	value, ok := contextFailure(ctx)
	if !ok {
		return canonicalInternal
	}
	return value
}

func (h *Handler) emitTerminalError(w http.ResponseWriter, value failure.Failure) {
	_ = writeFrame(w, h.errorFrame(value))
}

func resetTimer(timer *time.Timer, timeout time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(timeout)
}

func mapReceivedStreamPart(ctx context.Context, state *streamState, part provider.StreamPart, open bool, canonicalModelID string) (any, bool, *failure.Failure, bool) {
	if value, ok := contextFailure(ctx); ok {
		return nil, false, &value, false
	}
	if !open {
		if state.finishSeen {
			return nil, false, nil, true
		}
		value := canonicalInternal
		return nil, false, &value, false
	}
	dto, emit, terminal := mapStreamPart(ctx, state, part, canonicalModelID)
	return dto, emit, terminal, false
}

func mapStreamPart(ctx context.Context, state *streamState, part provider.StreamPart, canonicalModelID string) (any, bool, *failure.Failure) {
	fail := func(value failure.Failure) (any, bool, *failure.Failure) { return nil, false, &value }
	if state.finishSeen || !utf8.ValidString(canonicalModelID) {
		return fail(canonicalInternal)
	}
	if len(part.Warnings) != 0 && part.Type != provider.PartStreamStart && part.Type != provider.PartFinish {
		return fail(canonicalInternal)
	}
	switch part.Type {
	case provider.PartStreamStart:
		if state.providerStartSeen || state.seenProviderPart || len(part.Warnings) != 0 {
			return fail(canonicalInternal)
		}
		state.providerStartSeen = true
		state.seenProviderPart = true
		return nil, false, nil
	case provider.PartResponseMeta:
		if state.metadataSeen || state.textSeen || state.activeTextID != "" || !utf8.ValidString(part.ResponseID) || !utf8.ValidString(part.ModelID) || !utf8.ValidString(part.Provider) {
			return fail(canonicalInternal)
		}
		state.metadataSeen = true
		state.seenProviderPart = true
		timestamp := ""
		if !part.Timestamp.IsZero() {
			timestamp = part.Timestamp.Format(time.RFC3339Nano)
		}
		return responseMetadataDTO{Type: streamPartResponseMetadata, ID: part.ResponseID, ModelID: canonicalModelID, Timestamp: timestamp}, true, nil
	case provider.PartTextStart:
		if state.activeTextID != "" || part.ID == "" || !utf8.ValidString(part.ID) {
			return fail(canonicalInternal)
		}
		state.activeTextID = part.ID
		state.textSeen = true
		state.seenProviderPart = true
		return textStartDTO{Type: streamPartTextStart, ID: part.ID}, true, nil
	case provider.PartTextDelta:
		if state.activeTextID == "" || part.ID != state.activeTextID || !utf8.ValidString(part.ID) || !utf8.ValidString(part.Delta) {
			return fail(canonicalInternal)
		}
		state.seenProviderPart = true
		return textDeltaDTO{Type: streamPartTextDelta, ID: part.ID, Delta: part.Delta}, true, nil
	case provider.PartTextEnd:
		if state.activeTextID == "" || part.ID != state.activeTextID || !utf8.ValidString(part.ID) {
			return fail(canonicalInternal)
		}
		state.activeTextID = ""
		state.seenProviderPart = true
		return textEndDTO{Type: streamPartTextEnd, ID: part.ID}, true, nil
	case provider.PartFinish:
		if state.activeTextID != "" || len(part.Warnings) != 0 || part.FinishReason == nil || part.Usage == nil {
			return fail(canonicalInternal)
		}
		finishReason, err := mapFinishReason(*part.FinishReason)
		if err != nil {
			return fail(canonicalInternal)
		}
		usage, err := mapUsage(*part.Usage)
		if err != nil {
			return fail(canonicalInternal)
		}
		state.finishSeen = true
		state.seenProviderPart = true
		return finishDTO{Type: streamPartFinish, FinishReason: finishReason, Usage: usage}, true, nil
	case provider.PartError:
		value := reduceProviderError(ctx, part.APICallError)
		return fail(value)
	default:
		return fail(canonicalInternal)
	}
}
