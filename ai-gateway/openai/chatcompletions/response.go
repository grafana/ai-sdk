package chatcompletions

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

var errOutput = errors.New("chat completions: unsupported or invalid output")

type usage struct {
	Prompt            int                `json:"prompt_tokens"`
	Completion        int                `json:"completion_tokens"`
	Total             int                `json:"total_tokens"`
	PromptDetails     *promptDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionDetails *completionDetails `json:"completion_tokens_details,omitempty"`
}
type promptDetails struct {
	Cached int `json:"cached_tokens"`
}
type completionDetails struct {
	Reasoning int `json:"reasoning_tokens"`
}
type outputMessage struct {
	Role      role       `json:"role"`
	Content   *string    `json:"content"`
	ToolCalls []toolCall `json:"tool_calls,omitempty"`
}
type choice struct {
	Index    int           `json:"index"`
	Message  outputMessage `json:"message"`
	Finish   string        `json:"finish_reason"`
	Logprobs any           `json:"logprobs"`
}
type completion struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
	Usage   *usage   `json:"usage,omitempty"`
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
	for _, v := range []*int{u.InputTokens.NoCache, u.InputTokens.CacheRead, u.InputTokens.CacheWrite} {
		if v != nil && *v > in {
			return nil, errOutput
		}
	}
	for _, v := range []*int{u.OutputTokens.Text, u.OutputTokens.Reasoning} {
		if v != nil && *v > out {
			return nil, errOutput
		}
	}
	result := &usage{Prompt: in, Completion: out, Total: in + out}
	if u.InputTokens.CacheRead != nil {
		result.PromptDetails = &promptDetails{*u.InputTokens.CacheRead}
	}
	if u.OutputTokens.Reasoning != nil {
		result.CompletionDetails = &completionDetails{*u.OutputTokens.Reasoning}
	}
	return result, nil
}
func finishReason(f provider.FinishReason) (string, error) {
	switch f.Unified {
	case provider.FinishReasonStop:
		return "stop", nil
	case provider.FinishReasonLength:
		return "length", nil
	case provider.FinishReasonContentFilter:
		return "content_filter", nil
	case provider.FinishReasonToolCalls:
		return "tool_calls", nil
	}
	return "", errOutput
}
func (r mappedRequest) validateText(text, finish string) error {
	if !utf8.ValidString(text) {
		return errOutput
	}
	if r.jsonOutput && finish == "stop" {
		if !json.Valid([]byte(text)) {
			return errOutput
		}
		if r.strictOutput && r.validator != nil && r.validator.Validate(json.RawMessage(text)) != nil {
			return errOutput
		}
	}
	return nil
}
func validateCall(call toolCall, r mappedRequest) error {
	if call.ID == "" || len(call.ID) > 256 || !utf8.ValidString(call.ID) || !namePattern.MatchString(call.Function.Name) || !utf8.ValidString(call.Function.Arguments) || !jsonObject([]byte(call.Function.Arguments)) {
		return errOutput
	}
	if r.options.ToolChoice != nil && (r.options.ToolChoice.Type == provider.ToolChoiceNone || r.options.ToolChoice.Type == provider.ToolChoiceTool && r.options.ToolChoice.ToolName != call.Function.Name) {
		return errOutput
	}
	for _, t := range r.options.Tools {
		if t.Name == call.Function.Name {
			if boolValue(t.Strict) {
				validator := r.toolValidators[t.Name]
				if validator == nil || validator.Validate(json.RawMessage(call.Function.Arguments)) != nil {
					return errOutput
				}
			}
			return nil
		}
	}
	return errOutput
}
func mapGenerate(result *provider.GenerateResult, r mappedRequest, id, model string, created int64, max int64) (completion, error) {
	out := completion{ID: id, Object: "chat.completion", Created: created, Model: model}
	if result == nil {
		return out, errOutput
	}
	if len(result.Content) > 1024 {
		return out, errOutput
	}
	for _, warning := range result.Warnings {
		if warning.Type == provider.WarnUnsupported {
			return out, errOutput
		}
	}
	finish, err := finishReason(result.FinishReason)
	if err != nil {
		return out, err
	}
	msg := outputMessage{Role: roleAssistant}
	var content strings.Builder
	var size int64
	hasText := false
	used := map[string]bool{}
	for _, p := range result.Content {
		size += int64(len(p.Text)) + int64(len(p.Input)) + int64(len(p.ToolName)) + int64(len(p.ToolCallID))
		if size > max {
			return out, errOutput
		}
		switch p.Type {
		case provider.ContentText:
			content.WriteString(p.Text)
			hasText = true
		case provider.ContentReasoning:
		case provider.ContentToolCall:
			call := toolCall{ID: p.ToolCallID, Type: kindFunction, Function: callFunction{Name: p.ToolName, Arguments: string(p.Input)}}
			if p.ProviderExecuted || boolValue(p.Dynamic) || boolValue(p.Preliminary) || used[call.ID] || validateCall(call, r) != nil {
				return out, errOutput
			}
			used[call.ID] = true
			msg.ToolCalls = append(msg.ToolCalls, call)
		default:
			return out, errOutput
		}
	}
	text := content.String()
	if text == "" && len(msg.ToolCalls) == 0 && finish == "stop" {
		return out, errOutput
	}
	if r.Parallel != nil && !*r.Parallel && len(msg.ToolCalls) > 1 {
		return out, errOutput
	}
	if requiresTool(r) && len(msg.ToolCalls) == 0 && finish == "stop" {
		return out, errOutput
	}
	if (len(msg.ToolCalls) > 0) != (finish == "tool_calls") {
		return out, errOutput
	}
	if err = r.validateText(text, finish); err != nil {
		return out, err
	}
	if hasText || len(msg.ToolCalls) == 0 {
		msg.Content = &text
	}
	out.Usage, err = mapUsage(result.Usage)
	if err != nil {
		return out, err
	}
	out.Choices = []choice{{Message: msg, Finish: finish}}
	return out, nil
}

func requiresTool(r mappedRequest) bool {
	return r.options.ToolChoice != nil && (r.options.ToolChoice.Type == provider.ToolChoiceRequired || r.options.ToolChoice.Type == provider.ToolChoiceTool)
}
