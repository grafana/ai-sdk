package anthropicmessages

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
)

type Handler struct{ models catalog.ModelResolver }

func New(models catalog.ModelResolver) http.Handler { return &Handler{models: models} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" {
		writeError(w, http.StatusNotFound, "not_found_error", "Not found")
		return
	}
	if hasUnsupportedHeaders(r.Header) {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "Request contains unsupported content")
		return
	}
	body, err := readBody(r)
	if err != nil {
		writeRequestError(w)
		return
	}
	input, options, err := mapRequest(body)
	if err != nil {
		writeMappingError(w, err)
		return
	}
	resolved, err := h.models.ResolveModel(r.Context(), input.Model)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found_error", "Model not found")
		return
	}
	responseModel := resolved.ID
	if responseModel == "" {
		responseModel = input.Model
	}
	if input.Stream {
		h.serveStream(w, r.Context(), resolved.Model, responseModel, options)
		return
	}
	result, err := resolved.Model.DoGenerate(r.Context(), options)
	if err != nil {
		writeError(w, http.StatusBadGateway, "api_error", "Provider request failed")
		return
	}
	if result == nil {
		writeError(w, http.StatusBadGateway, "api_error", "Provider request failed")
		return
	}
	message, ok := responseMessage(responseModel, result)
	if !ok {
		writeError(w, http.StatusBadGateway, "api_error", "Provider response is unsupported")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(message)
}

func hasUnsupportedHeaders(headers http.Header) bool {
	for name, values := range headers {
		if strings.EqualFold(name, "anthropic-beta") || strings.EqualFold(name, "anthropic-user-profile-id") || strings.EqualFold(name, "anthropic-workspace-id") {
			for _, value := range values {
				if strings.TrimSpace(value) != "" {
					return true
				}
			}
		}
	}
	return false
}

func readBody(r *http.Request) ([]byte, error) {
	defer func() { _ = r.Body.Close() }()
	return io.ReadAll(r.Body)
}

func writeRequestError(w http.ResponseWriter) {
	writeError(w, http.StatusBadRequest, "invalid_request_error", "Invalid request")
}

func writeMappingError(w http.ResponseWriter, err error) {
	switch err {
	case errUnsupportedRequest:
		writeError(w, http.StatusBadRequest, "invalid_request_error", "Request contains unsupported content")
	default:
		writeRequestError(w)
	}
}

func writeError(w http.ResponseWriter, status int, kind, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "error", "error": map[string]string{"type": kind, "message": message}})
}

type usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}

type messageResponse struct {
	ID           string           `json:"id"`
	Type         string           `json:"type"`
	Role         string           `json:"role"`
	Model        string           `json:"model"`
	Content      []map[string]any `json:"content"`
	StopReason   *string          `json:"stop_reason"`
	StopSequence *string          `json:"stop_sequence"`
	Usage        usage            `json:"usage"`
}

func responseMessage(model string, result *provider.GenerateResult) (messageResponse, bool) {
	content := make([]map[string]any, 0, len(result.Content))
	for _, part := range result.Content {
		switch part.Type {
		case provider.ContentText:
			content = append(content, map[string]any{"type": "text", "text": part.Text})
		case provider.ContentToolCall:
			if part.ToolCallID == "" || part.ToolName == "" || !isJSONObject(part.Input) {
				return messageResponse{}, false
			}
			content = append(content, map[string]any{"type": "tool_use", "id": part.ToolCallID, "name": part.ToolName, "input": part.Input})
		default:
			return messageResponse{}, false
		}
	}
	stop, ok := stopReason(result.FinishReason)
	if !ok {
		return messageResponse{}, false
	}
	return messageResponse{ID: responseID(result.Response), Type: "message", Role: "assistant", Model: model, Content: content, StopReason: &stop, Usage: usageFromProvider(result.Usage)}, true
}

func responseID(response *provider.GenerateResponse) string {
	if response != nil && response.ID != "" {
		return response.ID
	}
	return "msg_" + aisdk.GenerateID()
}

func usageFromProvider(value provider.Usage) usage {
	out := usage{}
	out.update(value)
	return out
}

func (out *usage) update(value provider.Usage) {
	if value.InputTokens.NoCache != nil {
		out.InputTokens = *value.InputTokens.NoCache
	} else if value.InputTokens.Total != nil {
		out.InputTokens = *value.InputTokens.Total
		if value.InputTokens.CacheRead != nil {
			out.InputTokens -= *value.InputTokens.CacheRead
		}
		if value.InputTokens.CacheWrite != nil {
			out.InputTokens -= *value.InputTokens.CacheWrite
		}
	}
	if value.OutputTokens.Total != nil {
		out.OutputTokens = *value.OutputTokens.Total
	} else if value.OutputTokens.Text != nil && value.OutputTokens.Reasoning != nil {
		out.OutputTokens = *value.OutputTokens.Text + *value.OutputTokens.Reasoning
	}
	if value.InputTokens.CacheRead != nil {
		out.CacheReadInputTokens = *value.InputTokens.CacheRead
	}
	if value.InputTokens.CacheWrite != nil {
		out.CacheCreationInputTokens = *value.InputTokens.CacheWrite
	}
}

func stopReason(reason provider.FinishReason) (string, bool) {
	switch reason.Unified {
	case provider.FinishReasonToolCalls:
		return "tool_use", true
	case provider.FinishReasonLength:
		return "max_tokens", true
	case provider.FinishReasonStop:
		switch reason.Raw {
		case "pause_turn", "stop_sequence":
			return reason.Raw, true
		default:
			return "end_turn", true
		}
	default:
		return "", false
	}
}

func (h *Handler) serveStream(w http.ResponseWriter, ctx context.Context, model provider.LanguageModel, responseModel string, options provider.CallOptions) {
	result, err := model.DoStream(ctx, options)
	if err != nil || result == nil || result.Stream == nil {
		writeError(w, http.StatusBadGateway, "api_error", "Provider request failed")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	controller := http.NewResponseController(w)
	s := streamWriter{w: w, flush: func() { _ = controller.Flush() }}
	msgID := "msg_" + aisdk.GenerateID()
	currentIndex := -1
	nextIndex := 0
	currentToolID := ""
	var totalUsage usage
	var finish provider.FinishReason
	messageStarted := false
	toolIDs := make(map[string]struct{})
	closeCurrent := func() {
		if currentIndex >= 0 {
			s.event("content_block_stop", map[string]any{"type": "content_block_stop", "index": currentIndex})
			currentIndex = -1
			currentToolID = ""
		}
	}
	startText := func() {
		currentIndex = nextIndex
		nextIndex++
		s.event("content_block_start", map[string]any{"type": "content_block_start", "index": currentIndex, "content_block": map[string]any{"type": "text", "text": ""}})
	}
	startTool := func(id, name string) {
		currentIndex = nextIndex
		nextIndex++
		currentToolID = id
		toolIDs[id] = struct{}{}
		s.event("content_block_start", map[string]any{"type": "content_block_start", "index": currentIndex, "content_block": map[string]any{"type": "tool_use", "id": id, "name": name, "input": map[string]any{}}})
	}
	for {
		var part provider.StreamPart
		var open bool
		select {
		case <-ctx.Done():
			return
		case part, open = <-result.Stream:
			if !open {
				goto streamComplete
			}
		}
		if part.Type == provider.PartStreamStart || part.Type == provider.PartRaw {
			continue
		}
		if part.Type == provider.PartError {
			s.event("error", map[string]any{"type": "error", "error": map[string]string{"type": "api_error", "message": "Provider request failed"}})
			return
		}
		if part.Type == provider.PartResponseMeta {
			if part.ResponseID != "" {
				msgID = part.ResponseID
			}
			if part.Usage != nil {
				totalUsage.update(*part.Usage)
			}
			if !messageStarted {
				s.event("message_start", map[string]any{"type": "message_start", "message": messageResponse{ID: msgID, Type: "message", Role: "assistant", Model: responseModel, Content: []map[string]any{}, Usage: totalUsage}})
				messageStarted = true
			}
			continue
		}
		if !messageStarted {
			s.event("message_start", map[string]any{"type": "message_start", "message": messageResponse{ID: msgID, Type: "message", Role: "assistant", Model: responseModel, Content: []map[string]any{}, Usage: totalUsage}})
			messageStarted = true
		}
		switch part.Type {
		case provider.PartTextStart:
			closeCurrent()
			startText()
		case provider.PartTextDelta:
			if currentIndex < 0 || currentToolID != "" {
				closeCurrent()
				startText()
			}
			s.event("content_block_delta", map[string]any{"type": "content_block_delta", "index": currentIndex, "delta": map[string]string{"type": "text_delta", "text": part.Delta}})
		case provider.PartTextEnd:
			if currentToolID == "" {
				closeCurrent()
			}
		case provider.PartToolInputStart:
			if part.ID == "" || part.ToolName == "" {
				s.event("error", map[string]any{"type": "error", "error": map[string]string{"type": "api_error", "message": "Provider response is unsupported"}})
				return
			}
			closeCurrent()
			startTool(part.ID, part.ToolName)
		case provider.PartToolInputDelta:
			if currentIndex < 0 || currentToolID == "" || (part.ID != "" && part.ID != currentToolID) {
				if part.ID == "" || part.ToolName == "" {
					s.event("error", map[string]any{"type": "error", "error": map[string]string{"type": "api_error", "message": "Provider response is unsupported"}})
					return
				}
				closeCurrent()
				startTool(part.ID, part.ToolName)
			}
			s.event("content_block_delta", map[string]any{"type": "content_block_delta", "index": currentIndex, "delta": map[string]string{"type": "input_json_delta", "partial_json": part.Delta}})
		case provider.PartToolInputEnd:
			if currentToolID == "" || (part.ID != "" && part.ID != currentToolID) {
				s.event("error", map[string]any{"type": "error", "error": map[string]string{"type": "api_error", "message": "Provider response is unsupported"}})
				return
			}
			closeCurrent()
		case provider.PartToolCall:
			if _, seen := toolIDs[part.ToolCallID]; !seen {
				if part.ToolCallID == "" || part.ToolName == "" || !isJSONObject(json.RawMessage(part.Input)) {
					s.event("error", map[string]any{"type": "error", "error": map[string]string{"type": "api_error", "message": "Provider response is unsupported"}})
					return
				}
				closeCurrent()
				startTool(part.ToolCallID, part.ToolName)
				s.event("content_block_delta", map[string]any{"type": "content_block_delta", "index": currentIndex, "delta": map[string]string{"type": "input_json_delta", "partial_json": part.Input}})
				closeCurrent()
			}
		case provider.PartFinish:
			if part.FinishReason != nil {
				finish = *part.FinishReason
			}
			if part.Usage != nil {
				totalUsage.update(*part.Usage)
			}
		default:
			s.event("error", map[string]any{"type": "error", "error": map[string]string{"type": "api_error", "message": "Provider response is unsupported"}})
			return
		}
	}

streamComplete:
	if _, ok := stopReason(finish); !ok {
		s.event("error", map[string]any{"type": "error", "error": map[string]string{"type": "api_error", "message": "Provider response is unsupported"}})
		return
	}
	closeCurrent()
	if !messageStarted {
		s.event("message_start", map[string]any{"type": "message_start", "message": messageResponse{ID: msgID, Type: "message", Role: "assistant", Model: responseModel, Content: []map[string]any{}, Usage: totalUsage}})
	}
	reason, _ := stopReason(finish)
	s.event("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": reason, "stop_sequence": nil}, "usage": totalUsage})
	s.event("message_stop", map[string]string{"type": "message_stop"})
}

type streamWriter struct {
	w     http.ResponseWriter
	flush func()
}

func (s streamWriter) event(name string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, strings.TrimSpace(string(data)))
	s.flush()
}
