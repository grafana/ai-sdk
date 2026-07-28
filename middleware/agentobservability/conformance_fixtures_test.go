package agentobservability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/require"
)

// TestSetupConformanceInputs writes the canonical input fixtures used by
// TestConformance_Generation / TestConformance_Stream / TestConformance_Hooks.
// It runs only when AGENTO11Y_REGEN=1 is set; otherwise it is a no-op so normal
// CI runs don't touch the testdata tree.
//
// To regenerate fixtures end-to-end:
//
//	cd middleware/agentobservability && AGENTO11Y_REGEN=1 go test ./...
//
// That sequence writes the input fixtures here and then the conformance
// tests below write the expected_generation.json / expected_prompt.json
// snapshots from the live mappers.
func TestSetupConformanceInputs(t *testing.T) {
	if !regenerateConformanceFixtures {
		t.Skip("AGENTO11Y_REGEN not set; skipping fixture write")
	}

	writeFixtureInputs(t)
}

func writeFixtureInputs(t *testing.T) {
	t.Helper()
	for name, ftext := range generationInputs() {
		dir := filepath.Join("testdata", "generation", name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		writeJSONFile(t, filepath.Join(dir, "params.json"), ftext.params)
		writeJSONFile(t, filepath.Join(dir, "result.json"), ftext.result)
	}
	for name, ftext := range streamInputs() {
		dir := filepath.Join("testdata", "stream", name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		writeJSONFile(t, filepath.Join(dir, "params.json"), ftext.params)
		writeJSONFile(t, filepath.Join(dir, "stream.json"), ftext.parts)
	}
	for name, ftext := range hookInputs() {
		dir := filepath.Join("testdata", "hooks", name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		writeJSONFile(t, filepath.Join(dir, "original_prompt.json"), ftext.original)
		writeJSONFile(t, filepath.Join(dir, "hook_response.json"), ftext.response)
	}
}

type genFixtureInput struct {
	params provider.CallOptions
	result provider.GenerateResult
}

func generationInputs() map[string]genFixtureInput {
	intP := func(v int) *int { return &v }
	floatP := func(v float64) *float64 { return &v }

	plainParams := provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("what is the weather?")},
		MaxOutputTokens: intP(1024),
		Temperature:     floatP(0.7),
	}
	plainResult := provider.GenerateResult{
		Content: []provider.GenerateContentPart{
			{Type: provider.ContentText, Text: "It is sunny."},
		},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "end_turn"},
		Usage: provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: intP(10)},
			OutputTokens: provider.OutputTokenUsage{Total: intP(20)},
		},
		Response: &provider.GenerateResponse{
			ResponseMetadata: provider.ResponseMetadata{ID: "msg_plain", ModelID: "claude-sonnet-4-5"},
		},
	}

	toolParams := provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("weather in SF?")},
		MaxOutputTokens: intP(1024),
		Tools: []provider.Tool{{
			Type:        provider.ToolTypeFunction,
			Name:        "get_weather",
			Description: "Get weather",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
		}},
	}
	toolResult := provider.GenerateResult{
		Content: []provider.GenerateContentPart{
			{Type: provider.ContentText, Text: "Looking up SF…"},
			{
				Type:       provider.ContentToolCall,
				ToolCallID: "tu_1",
				ToolName:   "get_weather",
				Input:      json.RawMessage(`{"city":"SF"}`),
			},
		},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonToolCalls, Raw: "tool_use"},
		Usage: provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: intP(15)},
			OutputTokens: provider.OutputTokenUsage{Total: intP(8)},
		},
		Response: &provider.GenerateResponse{
			ResponseMetadata: provider.ResponseMetadata{ID: "msg_tool", ModelID: "claude-sonnet-4-5"},
		},
	}

	reasoningParams := provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("think before answering: 2+2?")},
		MaxOutputTokens: intP(1024),
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{
				Key: "anthropic",
				Raw: json.RawMessage(`{"thinking":{"type":"enabled","budgetTokens":2048}}`),
			},
		},
	}
	reasoningResult := provider.GenerateResult{
		Content: []provider.GenerateContentPart{
			{Type: provider.ContentReasoning, Text: "two plus two…"},
			{Type: provider.ContentText, Text: "4"},
		},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "end_turn"},
		Usage: provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: intP(20)},
			OutputTokens: provider.OutputTokenUsage{Total: intP(12), Reasoning: intP(8)},
		},
		Response: &provider.GenerateResponse{
			ResponseMetadata: provider.ResponseMetadata{ID: "msg_reasoning", ModelID: "claude-sonnet-4-5"},
		},
	}

	maxTokensParams := provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("write a long essay")},
		MaxOutputTokens: intP(50),
	}
	maxTokensResult := provider.GenerateResult{
		Content: []provider.GenerateContentPart{
			{Type: provider.ContentText, Text: "An essay about life is, in many ways"},
		},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonLength, Raw: "max_tokens"},
		Usage: provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: intP(5)},
			OutputTokens: provider.OutputTokenUsage{Total: intP(50)},
		},
		Response: &provider.GenerateResponse{
			ResponseMetadata: provider.ResponseMetadata{ID: "msg_max", ModelID: "claude-sonnet-4-5"},
		},
	}

	toolUseStopParams := provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("look up the temperature")},
		MaxOutputTokens: intP(1024),
		Tools: []provider.Tool{{
			Type:        provider.ToolTypeFunction,
			Name:        "get_temperature",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
		}},
		ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceRequired},
	}
	toolUseStopResult := provider.GenerateResult{
		Content: []provider.GenerateContentPart{
			{
				Type:       provider.ContentToolCall,
				ToolCallID: "tu_x",
				ToolName:   "get_temperature",
				Input:      json.RawMessage(`{"city":"NYC"}`),
			},
		},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonToolCalls, Raw: "tool_use"},
		Usage: provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: intP(12)},
			OutputTokens: provider.OutputTokenUsage{Total: intP(7)},
		},
		Response: &provider.GenerateResponse{
			ResponseMetadata: provider.ResponseMetadata{ID: "msg_tooluse", ModelID: "claude-sonnet-4-5"},
		},
	}

	return map[string]genFixtureInput{
		"plain_text":               {plainParams, plainResult},
		"tool_call":                {toolParams, toolResult},
		"reasoning_with_signature": {reasoningParams, reasoningResult},
		"max_tokens_stop":          {maxTokensParams, maxTokensResult},
		"tool_use_stop":            {toolUseStopParams, toolUseStopResult},
	}
}

type streamFixtureInput struct {
	params provider.CallOptions
	parts  []provider.StreamPart
}

func streamInputs() map[string]streamFixtureInput {
	intP := func(v int) *int { return &v }
	finishStop := provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "end_turn"}

	textOnly := streamFixtureInput{
		params: provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}},
		parts: []provider.StreamPart{
			{Type: provider.PartTextStart, ID: "t0"},
			{Type: provider.PartTextDelta, ID: "t0", Delta: "Hello, "},
			{Type: provider.PartTextDelta, ID: "t0", Delta: "world!"},
			{Type: provider.PartTextEnd, ID: "t0"},
			{Type: provider.PartFinish, FinishReason: &finishStop, Usage: &provider.Usage{
				InputTokens:  provider.InputTokenUsage{Total: intP(2)},
				OutputTokens: provider.OutputTokenUsage{Total: intP(3)},
			}},
		},
	}

	reasoning := streamFixtureInput{
		params: provider.CallOptions{Prompt: []provider.Message{provider.UserText("2+2?")}},
		parts: []provider.StreamPart{
			{Type: provider.PartReasoningStart, ID: "r0"},
			{Type: provider.PartReasoningDelta, ID: "r0", Delta: "let me think"},
			{Type: provider.PartReasoningDelta, ID: "r0",
				ProviderMetadata: provider.ProviderMetadata{
					"anthropic": json.RawMessage(`{"signature":"sig-abc"}`),
				},
			},
			{Type: provider.PartReasoningEnd, ID: "r0"},
			{Type: provider.PartTextStart, ID: "t0"},
			{Type: provider.PartTextDelta, ID: "t0", Delta: "4"},
			{Type: provider.PartTextEnd, ID: "t0"},
			{Type: provider.PartFinish, FinishReason: &finishStop, Usage: &provider.Usage{
				InputTokens:  provider.InputTokenUsage{Total: intP(4)},
				OutputTokens: provider.OutputTokenUsage{Total: intP(2), Reasoning: intP(1)},
			}},
		},
	}

	finishTool := provider.FinishReason{Unified: provider.FinishReasonToolCalls, Raw: "tool_use"}
	toolCall := streamFixtureInput{
		params: provider.CallOptions{
			Prompt: []provider.Message{provider.UserText("weather?")},
			Tools: []provider.Tool{{
				Type:        provider.ToolTypeFunction,
				Name:        "get_weather",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
			}},
		},
		parts: []provider.StreamPart{
			{Type: provider.PartTextStart, ID: "t0"},
			{Type: provider.PartTextDelta, ID: "t0", Delta: "Looking it up…"},
			{Type: provider.PartTextEnd, ID: "t0"},
			{Type: provider.PartToolInputStart, ID: "tu_s", ToolName: "get_weather"},
			{Type: provider.PartToolInputDelta, ID: "tu_s", Delta: `{"city":`},
			{Type: provider.PartToolInputDelta, ID: "tu_s", Delta: `"SF"}`},
			{Type: provider.PartToolInputEnd, ID: "tu_s"},
			{Type: provider.PartFinish, FinishReason: &finishTool, Usage: &provider.Usage{
				InputTokens:  provider.InputTokenUsage{Total: intP(5)},
				OutputTokens: provider.OutputTokenUsage{Total: intP(8)},
			}},
		},
	}

	return map[string]streamFixtureInput{
		"text_only":                textOnly,
		"text_reasoning_signature": reasoning,
		"text_tool_call":           toolCall,
	}
}

type hookFixtureInput struct {
	original []provider.Message
	response agento11y.HookEvaluateResponse
}

func hookInputs() map[string]hookFixtureInput {
	signedReasoning := provider.ContentPart{
		Type: provider.ContentPartTypeReasoning,
		Text: "thinking…",
		ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{
				Key: "anthropic",
				Raw: json.RawMessage(`{"signature":"sig-xyz"}`),
			},
		},
	}

	allow := hookFixtureInput{
		original: []provider.Message{provider.UserText("hello")},
		response: agento11y.HookEvaluateResponse{Action: agento11y.HookActionAllow},
	}

	deny := hookFixtureInput{
		original: []provider.Message{provider.UserText("hello")},
		response: agento11y.HookEvaluateResponse{
			Action: agento11y.HookActionDeny,
			RuleID: "rule-42",
			Reason: "policy violation",
		},
	}

	transform := hookFixtureInput{
		original: []provider.Message{
			provider.UserText("question"),
			provider.NewAssistantMessage(signedReasoning, provider.TextPart("Here is your answer")),
		},
		response: agento11y.HookEvaluateResponse{
			Action: agento11y.HookActionAllow,
			TransformedInput: &agento11y.HookInput{
				Messages: []agento11y.Message{
					{Role: agento11y.RoleUser, Parts: []agento11y.Part{agento11y.TextPart("modified question")}},
					{Role: agento11y.RoleAssistant, Parts: []agento11y.Part{agento11y.TextPart("Here is your answer")}},
				},
			},
		},
	}

	return map[string]hookFixtureInput{
		"allow":                         allow,
		"deny":                          deny,
		"transform_preserves_signature": transform,
	}
}
