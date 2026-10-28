package chatcompletions

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

type toolDelta struct {
	Index    int           `json:"index"`
	ID       string        `json:"id,omitempty"`
	Type     kind          `json:"type,omitempty"`
	Function functionDelta `json:"function"`
}
type functionDelta struct {
	Name      string  `json:"name,omitempty"`
	Arguments *string `json:"arguments,omitempty"`
}
type delta struct {
	Role      role        `json:"role,omitempty"`
	Content   *string     `json:"content,omitempty"`
	ToolCalls []toolDelta `json:"tool_calls,omitempty"`
}
type chunkChoice struct {
	Index    int     `json:"index"`
	Delta    delta   `json:"delta"`
	Finish   *string `json:"finish_reason"`
	Logprobs any     `json:"logprobs"`
}
type chunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []chunkChoice `json:"choices"`
	Usage   *usage        `json:"usage"`
	Grafana *diagnostics  `json:"grafana,omitempty"`
}
type toolState struct {
	call         toolCall
	arguments    strings.Builder
	index        int
	ended, final bool
}
type streamState struct {
	text     strings.Builder
	textSeen bool
	textIDs  map[string]bool
	tools    map[string]*toolState
}

func (s *streamState) consume(p provider.StreamPart, r mappedRequest) (*delta, error) {
	if !utf8.ValidString(p.Delta) || !utf8.ValidString(p.Input) {
		return nil, errOutput
	}
	switch p.Type {
	case provider.PartStreamStart, provider.PartResponseMeta, provider.PartReasoningStart, provider.PartReasoningDelta, provider.PartReasoningEnd:
		return nil, nil
	case provider.PartCustom:
		if p.Kind == "anthropic.message_start" {
			return nil, nil
		}
		return nil, errOutput
	case provider.PartTextStart:
		if p.ID == "" {
			return nil, errOutput
		}
		if _, ok := s.textIDs[p.ID]; ok {
			return nil, errOutput
		}
		s.textIDs[p.ID] = true
		return nil, nil
	case provider.PartTextDelta:
		if !s.textIDs[p.ID] {
			return nil, errOutput
		}
		if r.jsonOutput {
			s.text.WriteString(p.Delta)
		}
		if p.Delta == "" {
			return nil, nil
		}
		s.textSeen = true
		return &delta{Content: ptr(p.Delta)}, nil
	case provider.PartTextEnd:
		if !s.textIDs[p.ID] {
			return nil, errOutput
		}
		s.textIDs[p.ID] = false
		return nil, nil
	case provider.PartToolInputStart:
		if p.ID == "" || len(p.ID) > 256 || !utf8.ValidString(p.ID) || !namePattern.MatchString(p.ToolName) || s.tools[p.ID] != nil || p.ProviderExecuted || boolValue(p.Dynamic) || (p.InputEncoding != "" && p.InputEncoding != provider.ToolInputEncodingJSON) {
			return nil, errOutput
		}
		t := &toolState{call: toolCall{ID: p.ID, Type: kindFunction, Function: callFunction{Name: p.ToolName}}, index: len(s.tools)}
		s.tools[p.ID] = t
		return &delta{ToolCalls: []toolDelta{{Index: t.index, ID: p.ID, Type: kindFunction, Function: functionDelta{Name: p.ToolName, Arguments: ptr("")}}}}, nil
	case provider.PartToolInputDelta:
		t := s.tools[p.ID]
		if t == nil || t.ended || t.final {
			return nil, errOutput
		}
		t.arguments.WriteString(p.Delta)
		if p.Delta == "" {
			return nil, nil
		}
		return &delta{ToolCalls: []toolDelta{{Index: t.index, Function: functionDelta{Arguments: ptr(p.Delta)}}}}, nil
	case provider.PartToolInputEnd:
		t := s.tools[p.ID]
		if t == nil || t.ended || t.final {
			return nil, errOutput
		}
		t.ended = true
		return nil, nil
	case provider.PartToolCall:
		call := toolCall{ID: p.ToolCallID, Type: kindFunction, Function: callFunction{Name: p.ToolName, Arguments: p.Input}}
		if p.ProviderExecuted || boolValue(p.Dynamic) || boolValue(p.Preliminary) || validateCall(call, r) != nil {
			return nil, errOutput
		}
		if t := s.tools[p.ToolCallID]; t != nil {
			if t.final || !t.ended || t.call.Function.Name != call.Function.Name {
				return nil, errOutput
			}
			if t.arguments.Len() == 0 && call.Function.Arguments == "{}" {
				t.arguments.WriteString("{}")
				t.final = true
				return &delta{ToolCalls: []toolDelta{{Index: t.index, Function: functionDelta{Arguments: ptr("{}")}}}}, nil
			}
			if t.arguments.String() != call.Function.Arguments {
				return nil, errOutput
			}
			t.final = true
			return nil, nil
		}
		t := &toolState{call: call, index: len(s.tools), ended: true, final: true}
		s.tools[call.ID] = t
		return &delta{ToolCalls: []toolDelta{{Index: t.index, ID: call.ID, Type: kindFunction, Function: functionDelta{Name: call.Function.Name, Arguments: ptr(call.Function.Arguments)}}}}, nil
	default:
		return nil, errOutput
	}
}
func (s *streamState) finish(p provider.StreamPart, r mappedRequest) (string, *usage, error) {
	if p.FinishReason == nil {
		return "", nil, errOutput
	}
	f, err := finishReason(*p.FinishReason)
	if err != nil {
		return "", nil, err
	}
	if !s.textSeen && len(s.tools) == 0 && f == "stop" {
		return "", nil, errOutput
	}
	for _, active := range s.textIDs {
		if active {
			return "", nil, errOutput
		}
	}
	for _, tool := range s.tools {
		if !tool.final {
			return "", nil, errOutput
		}
	}
	if r.Parallel != nil && !*r.Parallel && len(s.tools) > 1 {
		return "", nil, errOutput
	}
	if requiresTool(r) && len(s.tools) == 0 && f == "stop" {
		return "", nil, errOutput
	}
	if (len(s.tools) > 0) != (f == "tool_calls") {
		return "", nil, errOutput
	}
	if err = r.validateText(s.text.String(), f); err != nil {
		return "", nil, err
	}
	var u *usage
	if p.Usage != nil {
		u, err = mapUsage(*p.Usage)
	}
	return f, u, err
}

func (h *handler) serveStream(ctx context.Context, w http.ResponseWriter, call *execution.Call, r mappedRequest, id, modelID string, created int64) {
	stream, err := call.Stream()
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			WriteError(w, categoryStatus(execution.ClassifyProvider(err)))
		}
		return
	}
	defer stream.Close()
	controller := http.NewResponseController(w)
	if deadline, ok := ctx.Deadline(); ok {
		if err := controller.SetWriteDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
			WriteError(w, 500)
			return
		}
	}
	watchDone := make(chan struct{})
	watchStopped := make(chan struct{})
	go func() {
		defer close(watchStopped)
		select {
		case <-ctx.Done():
			_ = controller.SetWriteDeadline(time.Now())
		case <-watchDone:
		}
	}()
	defer func() { close(watchDone); <-watchStopped }()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	write := func(data []byte) bool {
		frame := append(append([]byte("data: "), data...), []byte("\n\n")...)
		if ctx.Err() != nil {
			return false
		}
		if _, err := w.Write(frame); err != nil {
			return false
		}
		return controller.Flush() == nil
	}
	var pending *diagnostics
	opened := false
	emit := func(d delta, finish *string, u *usage, empty bool) bool {
		c := chunk{ID: id, Object: "chat.completion.chunk", Created: created, Model: modelID, Choices: []chunkChoice{{Delta: d, Finish: finish}}, Usage: u, Grafana: pending}
		if empty {
			c.Choices = []chunkChoice{}
		}
		b, err := json.Marshal(c)
		if err != nil || !write(b) {
			return false
		}
		pending = nil
		return true
	}
	fail := func(status int) { _ = write(errorBody(status)) }
	open := func() bool {
		if opened {
			return true
		}
		opened = true
		return emit(delta{Role: roleAssistant}, nil, nil, false)
	}
	state := streamState{textIDs: map[string]bool{}, tools: map[string]*toolState{}}
	for {
		if ctx.Err() != nil {
			return
		}
		part, wait := stream.Next()
		if wait != execution.WaitPart {
			if wait == execution.WaitClosed {
				fail(502)
			} else if wait != execution.WaitCanceled {
				fail(categoryStatus(execution.ClassifyProvider(wait.Err())))
			}
			return
		}
		if part.Type == provider.PartStreamStart {
			warnings, err := mapWarnings(part.Warnings)
			if err != nil {
				fail(502)
				return
			}
			if len(warnings) > 0 {
				if pending == nil {
					pending = &diagnostics{}
				}
				pending.Warnings = append(pending.Warnings, warnings...)
			}
			continue
		}
		if part.Type == provider.PartResponseMeta {
			native, err := mapNativeResponse(part.ResponseID, part.ModelID, part.Timestamp)
			if err != nil {
				fail(502)
				return
			}
			if !opened {
				native.apply(&id, &modelID, &created)
			} else {
				if pending == nil {
					pending = &diagnostics{}
				}
				pending.NativeResponse = native
				if !emit(delta{}, nil, nil, false) {
					fail(502)
					return
				}
			}
			continue
		}
		if part.Type == provider.PartError {
			fail(categoryStatus(execution.ClassifyStreamPart(part.APICallError)))
			return
		}
		if part.Type == provider.PartFinish {
			f, u, err := state.finish(part, r)
			if err != nil {
				fail(502)
				return
			}
			_, terminal := stream.Next()
			if terminal != execution.WaitClosed {
				if terminal == execution.WaitPart {
					fail(502)
				} else if terminal != execution.WaitCanceled {
					fail(categoryStatus(execution.ClassifyProvider(terminal.Err())))
				}
				return
			}
			if ctx.Err() != nil {
				fail(504)
				return
			}
			if !open() {
				fail(502)
				return
			}
			if !emit(delta{}, &f, nil, false) {
				fail(502)
				return
			}
			if r.StreamOptions != nil && boolValue(r.StreamOptions.IncludeUsage) {
				if !emit(delta{}, nil, u, true) {
					fail(502)
					return
				}
			}
			if !write([]byte("[DONE]")) {
				fail(502)
			}
			return
		}
		d, err := state.consume(part, r)
		if err != nil {
			fail(502)
			return
		}
		if d != nil {
			if !open() {
				fail(502)
				return
			}
			if !emit(*d, nil, nil, false) {
				fail(502)
				return
			}

		}
	}
}
