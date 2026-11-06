package chatcompletions

import (
	"encoding/json"
	"fmt"
)

func (r *mappedRequest) normalizeLegacy() error {
	r.legacyFunctions = r.Functions != nil || present(r.FunctionChoice)
	if r.legacyFunctions {
		if r.Tools != nil || present(r.ToolChoice) || boolValue(r.Parallel) {
			return errRequest
		}
		r.Parallel = ptr(false)
		for i := range r.Functions {
			r.Tools = append(r.Tools, tool{Type: kindFunction, Function: &r.Functions[i]})
		}
		if present(r.FunctionChoice) {
			var choice string
			if json.Unmarshal(r.FunctionChoice, &choice) == nil {
				r.ToolChoice = r.FunctionChoice
			} else {
				var named struct {
					Name string `json:"name"`
				}
				if decodeStrict(r.FunctionChoice, &named) != nil || !namePattern.MatchString(named.Name) {
					return errRequest
				}
				r.ToolChoice, _ = json.Marshal(map[string]any{"type": "function", "function": map[string]string{"name": named.Name}})
			}
		}
	}
	used := map[string]bool{}
	for _, m := range r.Messages {
		for _, call := range m.ToolCalls {
			used[call.ID] = true
		}
	}
	pendingName, pendingID := "", ""
	for i := range r.Messages {
		m := &r.Messages[i]
		if pendingID != "" && m.Role != roleFunction {
			return errRequest
		}
		if m.Role == roleFunction {
			if pendingID == "" || m.Name == nil || *m.Name != pendingName || len(m.ToolCalls) > 0 || m.FunctionCall != nil || m.ToolCallID != nil || m.Refusal != nil {
				return errRequest
			}
			var text *string
			if len(m.Content) == 0 || json.Unmarshal(m.Content, &text) != nil {
				return errRequest
			}
			if text == nil {
				m.Content = json.RawMessage(`""`)
			}
			m.Role, m.Name, m.ToolCallID = roleTool, nil, ptr(pendingID)
			pendingName, pendingID = "", ""
		}
		if m.FunctionCall != nil {
			if m.Role != roleAssistant || len(m.ToolCalls) > 0 || m.ToolCallID != nil {
				return errRequest
			}
			id := fmt.Sprintf("chat_legacy_%d", i)
			for used[id] {
				id += "_"
			}
			used[id] = true
			pendingName, pendingID = m.FunctionCall.Name, id
			m.ToolCalls = []toolCall{{ID: id, Type: kindFunction, Function: *m.FunctionCall}}
			m.FunctionCall = nil
		}
	}
	if pendingID != "" {
		return errRequest
	}
	return nil
}

func legacyFinish(finish string) string {
	if finish == "tool_calls" {
		return "function_call"
	}
	return finish
}

func legacyDelta(d delta) (delta, error) {
	if len(d.ToolCalls) > 0 {
		if len(d.ToolCalls) != 1 || d.ToolCalls[0].Index != 0 {
			return delta{}, errOutput
		}
		d.FunctionCall = &d.ToolCalls[0].Function
		d.ToolCalls = nil
	}
	return d, nil
}
