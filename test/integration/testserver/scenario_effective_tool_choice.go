package main

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

var effectiveToolChoiceScenarios = []struct {
	name     string
	choice   provider.ToolChoice
	callName string
}{
	{"effective-tool-choice-required-miss", provider.ToolChoice{Type: provider.ToolChoiceRequired}, ""},
	{"effective-tool-choice-named-miss", provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "lookup"}, "other"},
	{"effective-tool-choice-none-rejects", provider.ToolChoice{Type: provider.ToolChoiceNone}, "other"},
	{"effective-tool-choice-required-valid", provider.ToolChoice{Type: provider.ToolChoiceRequired}, "lookup"},
	{"effective-tool-choice-named-valid", provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "lookup"}, "lookup"},
}

func init() {
	for _, scenario := range effectiveToolChoiceScenarios {
		registerScenario(scenario.name, func(w http.ResponseWriter, r *http.Request) {
			result := streamEffectiveToolChoice(r.Context(), scenario.choice, scenario.callName, func() {})
			stream := result.ToUIMessageStream(
				aisdk.WithUIMessageStreamGenerateID(func() string { return "message-1" }),
				aisdk.OnUIMessageStreamError(func(error) string { return "tool choice violated" }),
			)
			if err := aisdk.PipeUIMessageStreamToResponse(w, stream); err != nil && r.Context().Err() == nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		})
	}
}

type effectiveToolChoiceModel struct {
	callName          string
	withToolLifecycle bool
}

func (effectiveToolChoiceModel) SpecificationVersion() string               { return "v4" }
func (effectiveToolChoiceModel) Provider() string                           { return "test" }
func (effectiveToolChoiceModel) ModelID() string                            { return "test-effective-tool-choice" }
func (effectiveToolChoiceModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (effectiveToolChoiceModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (m effectiveToolChoiceModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	var parts []provider.StreamPart
	if m.callName == "" {
		parts = append(parts,
			provider.StreamPart{Type: provider.PartReasoningStart, ID: "r1"},
			provider.StreamPart{Type: provider.PartReasoningDelta, ID: "r1", Delta: "thinking"},
			provider.StreamPart{Type: provider.PartReasoningEnd, ID: "r1"},
		)
	}
	if m.callName == "" || m.withToolLifecycle {
		parts = append(parts,
			provider.StreamPart{Type: provider.PartTextStart, ID: "t1"},
			provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "response"},
			provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"},
		)
	}
	if m.callName != "" {
		if m.withToolLifecycle {
			parts = append(parts,
				provider.StreamPart{Type: provider.PartToolInputStart, ID: "call-1", ToolName: m.callName},
				provider.StreamPart{Type: provider.PartToolInputDelta, ID: "call-1", Delta: `{}`},
				provider.StreamPart{Type: provider.PartToolInputEnd, ID: "call-1"},
			)
		}
		parts = append(parts, provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call-1", ToolName: m.callName, Input: `{}`})
		if m.withToolLifecycle {
			parts = append(parts,
				provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "hosted", ToolName: "remote", Input: `{}`, ProviderExecuted: true},
				provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "hosted", ToolName: "remote", Result: json.RawMessage(`"hosted output"`)},
			)
		}
	}
	parts = append(parts, provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls, Raw: "tool_calls"}})
	stream := make(chan provider.StreamPart, len(parts))
	for _, part := range parts {
		stream <- part
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func streamEffectiveToolChoice(ctx context.Context, choice provider.ToolChoice, callName string, onLocalEffect func()) *aisdk.StreamTextResult {
	return aisdk.StreamText(ctx, effectiveToolChoiceModel{callName: callName, withToolLifecycle: choice.Type == provider.ToolChoiceNone},
		aisdk.WithModelMessages(provider.UserText("test")), aisdk.WithToolChoice(choice),
		aisdk.WithTools(aisdk.ToolSet{
			"lookup": {Execute: func(context.Context, json.RawMessage, aisdk.ToolExecutionOptions) (json.RawMessage, error) {
				onLocalEffect()
				return json.RawMessage(`"found"`), nil
			}},
			"other": {
				NeedsApproval: aisdk.ApprovalIf(func(json.RawMessage, aisdk.ToolExecutionOptions) (bool, error) {
					onLocalEffect()
					return true, nil
				}),
				Execute: func(context.Context, json.RawMessage, aisdk.ToolExecutionOptions) (json.RawMessage, error) {
					onLocalEffect()
					return json.RawMessage(`"unexpected"`), nil
				},
			},
		}),
	)
}
