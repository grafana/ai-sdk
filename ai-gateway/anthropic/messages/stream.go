package messages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

type eventWriter struct {
	w     http.ResponseWriter
	flush func()
}

func (e eventWriter) event(name eventType, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(e.w, "event: %s\ndata: %s\n\n", name, data)
	e.flush()
}

func (e eventWriter) errorEvent(status int) {
	typ, message := errorType(status)
	e.event(eventError, errorEnvelope{Type: envelopeError, Error: apiError{Type: typ, Message: message}})
}

type toolBlock struct {
	index  int
	closed bool
}

type streamState struct {
	events    eventWriter
	model     string
	id        string
	usage     usage
	started   bool
	next      int
	textIndex int
	textOpen  bool
	tools     map[string]*toolBlock
}

func (s *streamState) start() {
	if s.started {
		return
	}
	s.started = true
	s.events.event(eventMessageStart, map[string]any{"type": eventMessageStart, "message": messageResponse{
		ID: s.id, Type: envelopeMessage, Role: provider.RoleAssistant, Model: s.model, Content: []map[string]any{}, Usage: s.usage,
	}})
}

func (s *streamState) closeText() {
	if !s.textOpen {
		return
	}
	s.events.event(eventContentBlockStop, map[string]any{"type": eventContentBlockStop, "index": s.textIndex})
	s.textOpen = false
}

func (s *streamState) open(content map[string]any) int {
	s.start()
	s.closeText()
	index := s.next
	s.next++
	s.events.event(eventContentBlockStart, map[string]any{"type": eventContentBlockStart, "index": index, "content_block": content})
	return index
}

func (s *streamState) openText() {
	s.textIndex = s.open(map[string]any{"type": contentBlockText, "text": ""})
	s.textOpen = true
}

func (s *streamState) openTool(id, name string) *toolBlock {
	tool := &toolBlock{index: s.open(map[string]any{"type": contentBlockToolUse, "id": id, "name": name, "input": map[string]any{}})}
	s.tools[id] = tool
	return tool
}

func (s *streamState) closeTool(tool *toolBlock) {
	if tool.closed {
		return
	}
	s.events.event(eventContentBlockStop, map[string]any{"type": eventContentBlockStop, "index": tool.index})
	tool.closed = true
}

func (s *streamState) delta(index int, kind deltaType, key, value string) {
	s.events.event(eventContentBlockDelta, map[string]any{"type": eventContentBlockDelta, "index": index, "delta": map[string]any{"type": kind, key: value}})
}

// consume applies one provider part and reports whether it broke the
// lifecycle the adapter can represent.
func (s *streamState) consume(part provider.StreamPart, finish *provider.FinishReason) bool {
	switch part.Type {
	case provider.PartStreamStart, provider.PartRaw, provider.PartCustom,
		provider.PartReasoningStart, provider.PartReasoningDelta, provider.PartReasoningEnd:
	case provider.PartResponseMeta:
		if part.ResponseID != "" && !s.started {
			s.id = part.ResponseID
		}
		if part.Usage != nil {
			s.usage.update(*part.Usage)
		}
		s.start()
	case provider.PartTextStart:
		s.openText()
	case provider.PartTextDelta:
		if !s.textOpen {
			s.openText()
		}
		s.delta(s.textIndex, deltaText, "text", part.Delta)
	case provider.PartTextEnd:
		if s.textOpen {
			s.closeText()
		}
	case provider.PartToolInputStart:
		if part.ID == "" || part.ToolName == "" || s.tools[part.ID] != nil {
			return false
		}
		s.openTool(part.ID, part.ToolName)
	case provider.PartToolInputDelta:
		tool := s.tools[part.ID]
		if tool == nil || tool.closed {
			return false
		}
		s.delta(tool.index, deltaInputJSON, "partial_json", part.Delta)
	case provider.PartToolInputEnd:
		tool := s.tools[part.ID]
		if tool == nil {
			return false
		}
		s.closeTool(tool)
	case provider.PartToolCall:
		if s.tools[part.ToolCallID] == nil {
			if part.ToolCallID == "" || part.ToolName == "" || !isObject(json.RawMessage(part.Input)) {
				return false
			}
			tool := s.openTool(part.ToolCallID, part.ToolName)
			s.delta(tool.index, deltaInputJSON, "partial_json", part.Input)
			s.closeTool(tool)
		}
	case provider.PartFinish:
		if part.FinishReason != nil {
			*finish = *part.FinishReason
		}
		if part.Usage != nil {
			s.usage.update(*part.Usage)
		}
	default:
		return false
	}
	return true
}

func (h *handler) serveStream(ctx context.Context, w http.ResponseWriter, call *execution.Call) {
	stream, err := call.Stream()
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return
		}
		WriteError(w, categoryStatus(execution.ClassifyProvider(err)))
		return
	}
	defer stream.Close()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	controller := http.NewResponseController(w)
	state := &streamState{
		events: eventWriter{w: w, flush: func() { _ = controller.Flush() }},
		model:  call.ModelID(),
		id:     "msg_" + aisdk.GenerateID(),
		tools:  make(map[string]*toolBlock),
	}
	var finish provider.FinishReason
	for {
		part, wait := stream.Next()
		if wait != execution.WaitPart {
			if wait == execution.WaitFinished {
				break
			}
			if wait == execution.WaitCanceled && errors.Is(ctx.Err(), context.Canceled) {
				return
			}
			if wait != execution.WaitClosedAfterError {
				state.events.errorEvent(categoryStatus(execution.ClassifyProvider(wait.Err())))
			}
			return
		}
		if part.Type == provider.PartError {
			state.events.errorEvent(categoryStatus(execution.ClassifyStreamPart(part.APICallError)))
			return
		}
		if !state.consume(part, &finish) {
			state.events.errorEvent(502)
			return
		}
	}
	reason, ok := stopReason(finish)
	if !ok {
		state.events.errorEvent(502)
		return
	}
	state.start()
	state.closeText()
	state.events.event(eventMessageDelta, map[string]any{"type": eventMessageDelta, "delta": map[string]any{"stop_reason": reason, "stop_sequence": nil}, "usage": state.usage})
	state.events.event(eventMessageStop, map[string]any{"type": eventMessageStop})
}
