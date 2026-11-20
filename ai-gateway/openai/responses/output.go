package responses

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

var errOutput = errors.New("responses: unsupported or invalid output")

type status string

const (
	statusInProgress status = "in_progress"
	statusCompleted  status = "completed"
	statusIncomplete status = "incomplete"
	statusFailed     status = "failed"
)

type outputText struct {
	Type        partType `json:"type"`
	Text        string   `json:"text"`
	Annotations []any    `json:"annotations"`
}

type messageOutput struct {
	ID      string       `json:"id"`
	Type    itemType     `json:"type"`
	Status  status       `json:"status"`
	Role    string       `json:"role"`
	Content []outputText `json:"content"`
}

type functionCallOutput struct {
	ID        string   `json:"id"`
	Type      itemType `json:"type"`
	Status    status   `json:"status"`
	CallID    string   `json:"call_id"`
	Name      string   `json:"name"`
	Arguments string   `json:"arguments"`
}

type reasoningOutput struct {
	ID      string        `json:"id"`
	Type    itemType      `json:"type"`
	Summary []summaryPart `json:"summary"`
}

type usage struct {
	InputTokens   int            `json:"input_tokens"`
	InputDetails  *inputDetails  `json:"input_tokens_details,omitempty"`
	OutputTokens  int            `json:"output_tokens"`
	OutputDetails *outputDetails `json:"output_tokens_details,omitempty"`
	TotalTokens   int            `json:"total_tokens"`
}

type inputDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type outputDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type incompleteDetails struct {
	Reason string `json:"reason"`
}

type responseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type response struct {
	ID                string             `json:"id"`
	Object            string             `json:"object"`
	CreatedAt         int64              `json:"created_at"`
	Status            status             `json:"status"`
	Model             string             `json:"model"`
	Output            []any              `json:"output"`
	Usage             *usage             `json:"usage"`
	Error             *responseError     `json:"error"`
	IncompleteDetails *incompleteDetails `json:"incomplete_details"`
}

func newResponse(id, model string, created int64) *response {
	return &response{ID: id, Object: "response", CreatedAt: created, Status: statusInProgress, Model: model, Output: []any{}}
}

func itemID(prefix, responseID string, index int) string {
	return fmt.Sprintf("%s_%s%d", prefix, strings.TrimPrefix(responseID, "resp_"), index)
}

func mapUsage(u provider.Usage) (*usage, error) {
	for _, v := range []*int{u.InputTokens.Total, u.InputTokens.NoCache, u.InputTokens.CacheRead, u.InputTokens.CacheWrite, u.OutputTokens.Total, u.OutputTokens.Text, u.OutputTokens.Reasoning} {
		if v != nil && *v < 0 {
			return nil, errOutput
		}
	}
	if u.InputTokens.Total == nil || u.OutputTokens.Total == nil {
		return nil, nil
	}
	in, out := *u.InputTokens.Total, *u.OutputTokens.Total
	if out > math.MaxInt-in {
		return nil, errOutput
	}
	if c := u.InputTokens.CacheRead; c != nil && *c > in {
		return nil, errOutput
	}
	if r := u.OutputTokens.Reasoning; r != nil && *r > out {
		return nil, errOutput
	}
	result := &usage{InputTokens: in, OutputTokens: out, TotalTokens: in + out}
	if c := u.InputTokens.CacheRead; c != nil {
		result.InputDetails = &inputDetails{*c}
	}
	if r := u.OutputTokens.Reasoning; r != nil {
		result.OutputDetails = &outputDetails{*r}
	}
	return result, nil
}

// finish maps a provider finish reason to the terminal status and, for an
// incomplete response, its reason.
func finish(f provider.FinishReason) (status, string, error) {
	switch f.Unified {
	case provider.FinishReasonStop, provider.FinishReasonToolCalls:
		return statusCompleted, "", nil
	case provider.FinishReasonLength:
		return statusIncomplete, "max_output_tokens", nil
	case provider.FinishReasonContentFilter:
		return statusIncomplete, "content_filter", nil
	}
	return "", "", errOutput
}

func requiresTool(r mappedRequest) bool {
	return r.options.ToolChoice != nil && (r.options.ToolChoice.Type == provider.ToolChoiceRequired || r.options.ToolChoice.Type == provider.ToolChoiceTool)
}

func validateCall(id, name, arguments string, r mappedRequest) error {
	if id == "" || len(id) > 256 || !utf8.ValidString(id) || !namePattern.MatchString(name) || !utf8.ValidString(arguments) {
		return errOutput
	}
	if c := r.options.ToolChoice; c != nil && (c.Type == provider.ToolChoiceNone || c.Type == provider.ToolChoiceTool && c.ToolName != name) {
		return errOutput
	}
	if r.toolValidators[name] == nil {
		return errOutput
	}
	return nil
}

func validateArguments(name, arguments string, r mappedRequest) error {
	if !jsonObject([]byte(arguments)) {
		return errOutput
	}
	for _, t := range r.options.Tools {
		if t.Name == name {
			if boolValue(t.Strict) && r.toolValidators[name].Validate(json.RawMessage(arguments)) != nil {
				return errOutput
			}
			return nil
		}
	}
	return errOutput
}

// validateFinish checks the complete output against the request before a
// terminal event: tool calls against the finish reason and tool choice, and
// structured text against its schema.
func validateFinish(r mappedRequest, f provider.FinishReason, calls []functionCallOutput, text string) error {
	partial := f.Unified == provider.FinishReasonLength || f.Unified == provider.FinishReasonContentFilter
	if !partial {
		for _, call := range calls {
			if validateArguments(call.Name, call.Arguments, r) != nil {
				return errOutput
			}
		}
		if (len(calls) > 0) != (f.Unified == provider.FinishReasonToolCalls) {
			return errOutput
		}
	}
	if r.Parallel != nil && !*r.Parallel && len(calls) > 1 {
		return errOutput
	}
	if requiresTool(r) && len(calls) == 0 && f.Unified == provider.FinishReasonStop {
		return errOutput
	}
	if !utf8.ValidString(text) {
		return errOutput
	}
	if r.jsonOutput && f.Unified == provider.FinishReasonStop {
		if !json.Valid([]byte(text)) || (r.strictOutput && r.validator.Validate(json.RawMessage(text)) != nil) {
			return errOutput
		}
	}
	return nil
}

// mapGenerate builds a unary response. Consecutive text parts form one
// message item and consecutive reasoning parts one reasoning item, as in a
// stream.
func mapGenerate(result *provider.GenerateResult, r mappedRequest, out *response) error {
	if result == nil {
		return errOutput
	}
	terminal, reason, err := finish(result.FinishReason)
	if err != nil {
		return err
	}
	var calls []functionCallOutput
	var text strings.Builder
	used := map[string]bool{}
	for _, p := range result.Content {
		last := len(out.Output) - 1
		switch p.Type {
		case provider.ContentText:
			if !utf8.ValidString(p.Text) {
				return errOutput
			}
			text.WriteString(p.Text)
			if last >= 0 {
				if m, ok := out.Output[last].(*messageOutput); ok {
					m.Content[0].Text += p.Text
					continue
				}
			}
			out.Output = append(out.Output, &messageOutput{ID: itemID("msg", out.ID, last+1), Type: itemMessage, Status: statusCompleted, Role: "assistant", Content: []outputText{{Type: partOutputText, Text: p.Text, Annotations: []any{}}}})
		case provider.ContentReasoning:
			if !utf8.ValidString(p.Text) {
				return errOutput
			}
			item, ok := (*reasoningOutput)(nil), false
			if last >= 0 {
				item, ok = out.Output[last].(*reasoningOutput)
			}
			if !ok {
				item = &reasoningOutput{ID: itemID("rs", out.ID, last+1), Type: itemReasoning, Summary: []summaryPart{}}
				out.Output = append(out.Output, item)
			}
			if p.Text != "" {
				item.Summary = append(item.Summary, summaryPart{Type: partSummaryText, Text: p.Text})
			}
		case provider.ContentToolCall:
			arguments := string(p.Input)
			if p.ProviderExecuted || boolValue(p.Dynamic) || boolValue(p.Preliminary) || used[p.ToolCallID] || validateCall(p.ToolCallID, p.ToolName, arguments, r) != nil {
				return errOutput
			}
			used[p.ToolCallID] = true
			call := functionCallOutput{ID: itemID("fc", out.ID, last+1), Type: itemFunctionCall, Status: statusCompleted, CallID: p.ToolCallID, Name: p.ToolName, Arguments: arguments}
			calls = append(calls, call)
			out.Output = append(out.Output, &call)
		default:
			return errOutput
		}
	}
	if err := validateFinish(r, result.FinishReason, calls, text.String()); err != nil {
		return err
	}
	out.Usage, err = mapUsage(result.Usage)
	if err != nil {
		return err
	}
	out.Status = terminal
	if reason != "" {
		out.IncompleteDetails = &incompleteDetails{reason}
	}
	return nil
}
