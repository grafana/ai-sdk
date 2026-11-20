// Package messages adapts an Anthropic Messages API subset to the shared
// Gateway execution runtime.
package messages

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"unicode/utf8"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

// Path is the Anthropic Messages adapter endpoint; the host supplies authentication.
const Path = "/v1/messages"

type handler struct{ runtime *execution.Runtime }

// New constructs a Messages adapter over the host runtime.
func New(runtime *execution.Runtime) (http.Handler, error) {
	if runtime == nil {
		return nil, errors.New("anthropic messages: runtime is nil")
	}
	return &handler{runtime: runtime}, nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != Path || req.URL.RawPath != "" {
		WriteError(w, 404)
		return
	}
	if req.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		WriteError(w, 405)
		return
	}
	if media, _, err := mime.ParseMediaType(req.Header.Get("Content-Type")); err != nil || media != "application/json" {
		WriteError(w, 415)
		return
	}
	if req.Header.Get("Content-Encoding") != "" {
		WriteError(w, 400)
		return
	}
	body, err := io.ReadAll(req.Body)
	if err != nil || !utf8.Valid(body) {
		WriteError(w, 400)
		return
	}
	in, options, err := mapRequest(body)
	if err != nil {
		WriteError(w, 400)
		return
	}
	call, err := h.runtime.Call(req.Context(), in.Model, options, execution.WithTranslator(messagesTranslator(in)))
	if err != nil {
		if errors.Is(req.Context().Err(), context.Canceled) {
			return
		}
		category, _ := execution.ClassifySelection(err)
		WriteError(w, categoryStatus(category))
		return
	}
	defer call.Close()
	if in.Stream {
		h.serveStream(req.Context(), w, call)
		return
	}
	result, err := call.Generate()
	if err != nil {
		if errors.Is(req.Context().Err(), context.Canceled) {
			return
		}
		WriteError(w, categoryStatus(execution.ClassifyProvider(err)))
		return
	}
	out, ok := mapGenerate(result, call.ModelID())
	if !ok {
		WriteError(w, 502)
		return
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		WriteError(w, 502)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(encoded)
}

type usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}

func (u *usage) update(value provider.Usage) {
	if value.InputTokens.NoCache != nil {
		u.InputTokens = *value.InputTokens.NoCache
	} else if value.InputTokens.Total != nil {
		u.InputTokens = *value.InputTokens.Total
		if value.InputTokens.CacheRead != nil {
			u.InputTokens -= *value.InputTokens.CacheRead
		}
		if value.InputTokens.CacheWrite != nil {
			u.InputTokens -= *value.InputTokens.CacheWrite
		}
	}
	if value.OutputTokens.Total != nil {
		u.OutputTokens = *value.OutputTokens.Total
	} else if value.OutputTokens.Text != nil && value.OutputTokens.Reasoning != nil {
		u.OutputTokens = *value.OutputTokens.Text + *value.OutputTokens.Reasoning
	}
	if value.InputTokens.CacheRead != nil {
		u.CacheReadInputTokens = *value.InputTokens.CacheRead
	}
	if value.InputTokens.CacheWrite != nil {
		u.CacheCreationInputTokens = *value.InputTokens.CacheWrite
	}
}

type messageResponse struct {
	ID           string           `json:"id"`
	Type         envelopeType     `json:"type"`
	Role         provider.Role    `json:"role"`
	Model        string           `json:"model"`
	Content      []map[string]any `json:"content"`
	StopReason   *stopReasonType  `json:"stop_reason"`
	StopSequence *string          `json:"stop_sequence"`
	Usage        usage            `json:"usage"`
}

func mapGenerate(result *provider.GenerateResult, model string) (messageResponse, bool) {
	content := make([]map[string]any, 0, len(result.Content))
	for _, part := range result.Content {
		switch part.Type {
		case provider.ContentText:
			content = append(content, map[string]any{"type": contentBlockText, "text": part.Text})
		case provider.ContentToolCall:
			if part.ToolCallID == "" || part.ToolName == "" || !isObject(json.RawMessage(part.Input)) {
				return messageResponse{}, false
			}
			content = append(content, map[string]any{"type": contentBlockToolUse, "id": part.ToolCallID, "name": part.ToolName, "input": json.RawMessage(part.Input)})
		case provider.ContentReasoning:
		default:
			return messageResponse{}, false
		}
	}
	reason, ok := stopReason(result.FinishReason)
	if !ok {
		return messageResponse{}, false
	}
	id := "msg_" + aisdk.GenerateID()
	if result.Response != nil && result.Response.ID != "" {
		id = result.Response.ID
	}
	out := messageResponse{ID: id, Type: envelopeMessage, Role: provider.RoleAssistant, Model: model, Content: content, StopReason: &reason}
	out.Usage.update(result.Usage)
	return out, true
}

func stopReason(reason provider.FinishReason) (stopReasonType, bool) {
	switch reason.Unified {
	case provider.FinishReasonToolCalls:
		return stopReasonToolUse, true
	case provider.FinishReasonContentFilter:
		return stopReasonRefusal, true
	case provider.FinishReasonLength:
		return stopReasonMaxTokens, true
	case provider.FinishReasonStop:
		switch stopReasonType(reason.Raw) {
		case stopReasonPauseTurn, stopReasonStopSequence:
			return stopReasonType(reason.Raw), true
		}
		return stopReasonEndTurn, true
	default:
		return "", false
	}
}
