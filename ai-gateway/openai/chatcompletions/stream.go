package chatcompletions

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

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
	bytes    int64
}

// consume validates provider lifecycle and returns only representable native progress.
func (s *streamState) consume(p provider.StreamPart, r mappedRequest, max int64) (*delta, error) {
	s.bytes += int64(len(p.Delta)) + int64(len(p.Input)) + int64(len(p.ID)) + int64(len(p.ToolCallID)) + int64(len(p.ToolName))
	if s.bytes > max || !utf8.ValidString(p.Delta) || !utf8.ValidString(p.Input) {
		return nil, errOutput
	}
	switch p.Type {
	case provider.PartStreamStart:
		for _, warning := range p.Warnings {
			if warning.Type == provider.WarnUnsupported {
				return nil, errOutput
			}
		}
		return nil, nil
	case provider.PartResponseMeta, provider.PartReasoningStart, provider.PartReasoningDelta, provider.PartReasoningEnd:
		return nil, nil
	case provider.PartTextStart:
		if p.ID == "" || len(s.textIDs) >= 1024 {
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
		if p.ID == "" || len(p.ID) > 256 || !utf8.ValidString(p.ID) || !namePattern.MatchString(p.ToolName) || len(s.tools) >= 128 || s.tools[p.ID] != nil || p.ProviderExecuted || boolValue(p.Dynamic) {
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
			if t.final || !t.ended || t.call.Function.Name != call.Function.Name || t.arguments.String() != call.Function.Arguments {
				return nil, errOutput
			}
			t.final = true
			return nil, nil
		}
		if len(s.tools) >= 128 {
			return nil, errOutput
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

func (h *handler) serveStream(ctx context.Context, cancel context.CancelFunc, w http.ResponseWriter, model provider.LanguageModel, r mappedRequest, id, modelID string, created int64) {
	type setupResult struct {
		stream *provider.StreamResult
		err    error
	}
	setup := make(chan setupResult)
	go func() {
		stream, err := model.DoStream(ctx, r.options)
		select {
		case setup <- setupResult{stream, err}:
		case <-ctx.Done():
			if stream != nil {
				drain(stream.Stream, h.limits.DrainDuration)
			}
		}
	}()
	var started setupResult
	select {
	case <-ctx.Done():
		WriteError(w, 504)
		return
	case started = <-setup:
	}
	if started.stream != nil {
		// Transfer the claimed channel to one bounded cleanup owner without
		// holding the response open for an uncooperative producer.
		defer func() { cancel(); go drain(started.stream.Stream, h.limits.DrainDuration) }()
	}
	if ctx.Err() != nil {
		WriteError(w, 504)
		return
	}
	if started.err != nil {
		WriteError(w, errorStatus(started.err))
		return
	}
	if started.stream == nil || started.stream.Stream == nil {
		WriteError(w, 502)
		return
	}
	controller := http.NewResponseController(w)
	deadline, _ := ctx.Deadline()
	// A response-controller deadline bounds socket backpressure; custom writers must support it.
	if err := controller.SetWriteDeadline(deadline); err != nil {
		WriteError(w, 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	bytesWritten := int64(0)
	write := func(data []byte) bool {
		frame := append(append([]byte("data: "), data...), []byte("\n\n")...)
		budget := h.limits.ResponseBytes
		if !bytes.HasPrefix(data, []byte(`{"error":`)) {
			budget -= 256
		}
		if int64(len(frame)) > h.limits.FrameBytes || bytesWritten+int64(len(frame)) > budget {
			return false
		}
		bytesWritten += int64(len(frame))
		if _, err := w.Write(frame); err != nil {
			return false
		}
		return controller.Flush() == nil
	}
	emit := func(d delta, finish *string, u *usage, empty bool) bool {
		c := chunk{ID: id, Object: "chat.completion.chunk", Created: created, Model: modelID, Choices: []chunkChoice{{Delta: d, Finish: finish}}, Usage: u}
		if empty {
			c.Choices = []chunkChoice{}
		}
		b, err := json.Marshal(c)
		return err == nil && write(b)
	}
	fail := func(status int) { _ = write(errorBody(status)) }
	if !emit(delta{Role: roleAssistant}, nil, nil, false) {
		fail(502)
		return
	}
	state := streamState{textIDs: map[string]bool{}, tools: map[string]*toolState{}}
	idle := time.NewTimer(h.limits.IdleDuration)
	defer idle.Stop()
	lastProgress := time.Now()
	parts := 0
	for {
		if ctx.Err() != nil || !time.Now().Before(deadline) {
			fail(504)
			return
		}
		if time.Since(lastProgress) >= h.limits.IdleDuration {
			fail(504)
			return
		}
		select {
		case <-ctx.Done():
			fail(504)
			return
		case <-idle.C:
			fail(504)
			return
		case part, ok := <-started.stream.Stream:
			if !ok {
				fail(502)
				return
			}
			parts++
			if parts > h.limits.StreamParts {
				fail(502)
				return
			}
			if part.Type == provider.PartError {
				status := 502
				if part.APICallError != nil {
					status = errorStatus(part.APICallError)
				}
				fail(status)
				return
			}
			if part.Type == provider.PartFinish {
				f, u, err := state.finish(part, r)
				if err != nil {
					fail(502)
					return
				}
				// Finish is terminal in the provider contract. Require closure before
				// committing success so duplicate finishes/late content cannot hide in
				// cleanup after an apparently successful native completion.
				closeTimer := time.NewTimer(h.limits.IdleDuration)
				select {
				case <-ctx.Done():
					closeTimer.Stop()
					fail(504)
					return
				case <-closeTimer.C:
					fail(504)
					return
				case _, open := <-started.stream.Stream:
					closeTimer.Stop()
					if open {
						fail(502)
						return
					}
				}
				if ctx.Err() != nil {
					fail(504)
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
			d, err := state.consume(part, r, h.limits.ResponseBytes)
			if err != nil {
				fail(502)
				return
			}
			if d != nil {
				if !emit(*d, nil, nil, false) {
					fail(502)
					return
				}
				lastProgress = time.Now()
				if !idle.Stop() {
					select {
					case <-idle.C:
					default:
					}
				}
				idle.Reset(h.limits.IdleDuration)
			}
		}
	}
}
