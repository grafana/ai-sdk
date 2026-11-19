package messages

import (
	"context"
	"encoding/json"
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

type blockKind uint8

const (
	blockNone blockKind = iota
	blockText
	blockTool
)

type streamState struct {
	events  eventWriter
	model   string
	id      string
	usage   usage
	started bool
	next    int
	index   int
	kind    blockKind
	toolID  string
	tools   map[string]bool
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

func (s *streamState) closeBlock() {
	if s.kind == blockNone {
		return
	}
	s.events.event(eventContentBlockStop, map[string]any{"type": eventContentBlockStop, "index": s.index})
	s.kind, s.toolID = blockNone, ""
}

func (s *streamState) open(kind blockKind, content map[string]any) {
	s.start()
	s.closeBlock()
	s.index, s.kind = s.next, kind
	s.next++
	s.events.event(eventContentBlockStart, map[string]any{"type": eventContentBlockStart, "index": s.index, "content_block": content})
}

func (s *streamState) openText() {
	s.open(blockText, map[string]any{"type": contentBlockText, "text": ""})
}

func (s *streamState) openTool(id, name string) {
	s.open(blockTool, map[string]any{"type": contentBlockToolUse, "id": id, "name": name, "input": map[string]any{}})
	s.toolID = id
	s.tools[id] = true
}

func (s *streamState) delta(kind deltaType, key, value string) {
	s.events.event(eventContentBlockDelta, map[string]any{"type": eventContentBlockDelta, "index": s.index, "delta": map[string]any{"type": kind, key: value}})
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
		if s.kind != blockText {
			s.openText()
		}
		s.delta(deltaText, "text", part.Delta)
	case provider.PartTextEnd:
		if s.kind == blockText {
			s.closeBlock()
		}
	case provider.PartToolInputStart:
		if part.ID == "" || part.ToolName == "" {
			return false
		}
		s.openTool(part.ID, part.ToolName)
	case provider.PartToolInputDelta:
		if s.kind != blockTool || (part.ID != "" && part.ID != s.toolID) {
			return false
		}
		s.delta(deltaInputJSON, "partial_json", part.Delta)
	case provider.PartToolInputEnd:
		if s.kind != blockTool {
			return false
		}
		s.closeBlock()
	case provider.PartToolCall:
		if !s.tools[part.ToolCallID] {
			if part.ToolCallID == "" || part.ToolName == "" || !isObject(json.RawMessage(part.Input)) {
				return false
			}
			s.openTool(part.ToolCallID, part.ToolName)
			s.delta(deltaInputJSON, "partial_json", part.Input)
			s.closeBlock()
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
		if ctx.Err() != nil {
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
		tools:  make(map[string]bool),
	}
	var finish provider.FinishReason
	for {
		part, wait := stream.Next()
		if wait != execution.WaitPart {
			if wait == execution.WaitFinished {
				break
			}
			if wait == execution.WaitCanceled && ctx.Err() != nil {
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
	state.closeBlock()
	state.events.event(eventMessageDelta, map[string]any{"type": eventMessageDelta, "delta": map[string]any{"stop_reason": reason, "stop_sequence": nil}, "usage": state.usage})
	state.events.event(eventMessageStop, map[string]any{"type": eventMessageStop})
}
