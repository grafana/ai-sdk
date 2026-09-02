package openai

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildParams_RegroupsParallelToolCallContinuation(t *testing.T) {
	input := `{"tool_uses":[{"recipient_name":"functions.weather","parameters":{"location":"San Francisco"}},{"recipient_name":"functions.cityAttractions","parameters":{"city":"Rome"}}]}`
	metadata := func(index int) provider.ProviderOptions {
		raw, err := json.Marshal(OpenAIPartOptions{ParallelToolCall: &OpenAIParallelToolCall{
			ItemID: "fc_parallel", ToolCallID: "call_parallel", ToolName: "parallel",
			Input: input, Index: index, Count: 2,
		}})
		require.NoError(t, err)
		return provider.ProviderOptions{"openai": provider.RawProviderOption{Key: "openai", Raw: raw}}
	}

	weatherCall := provider.ToolCallPart("call_parallel_0", "weather", json.RawMessage(`{"location":"San Francisco"}`))
	weatherCall.ProviderOptions = metadata(0)
	attractionsCall := provider.ToolCallPart("call_parallel_1", "cityAttractions", json.RawMessage(`{"city":"Rome"}`))
	attractionsCall.ProviderOptions = metadata(1)
	weatherResult := provider.ToolResultPart("call_parallel_0", "weather", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"temperature":72}`)})
	weatherResult.ProviderOptions = metadata(0)
	attractionsResult := provider.ToolResultPart("call_parallel_1", "cityAttractions", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "Colosseum"})
	attractionsResult.ProviderOptions = metadata(1)

	previousResponseID := "resp_previous"
	params, _, _, err := buildParams("gpt-5.4", provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewAssistantMessage(weatherCall, attractionsCall),
			provider.NewToolMessage(weatherResult, attractionsResult),
		},
		Tools: []provider.Tool{
			{Type: provider.ToolTypeFunction, Name: "weather"},
			{Type: provider.ToolTypeFunction, Name: "cityAttractions"},
		},
		ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{PreviousResponseID: previousResponseID}),
	})
	require.NoError(t, err)

	raw, err := json.Marshal(params)
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))
	items := body["input"].([]any)
	require.Len(t, items, 2)
	call := items[0].(map[string]any)
	assert.Equal(t, "function_call", call["type"])
	assert.Equal(t, "call_parallel", call["call_id"])
	assert.Equal(t, "parallel", call["name"])
	output := items[1].(map[string]any)
	assert.Equal(t, "function_call_output", output["type"])
	assert.Equal(t, "call_parallel", output["call_id"])
	assert.Equal(t, `{"temperature":72}`+"\n"+"Colosseum", output["output"])
}

func TestBuildParams_RegroupsParallelToolCallPromptCacheBreakpoints(t *testing.T) {
	input := `{"tool_uses":[{"recipient_name":"functions.weather","parameters":{"location":"San Francisco"}},{"recipient_name":"functions.cityAttractions","parameters":{"city":"Rome"}}]}`
	breakpoint := &PromptCacheBreakpoint{Mode: "explicit"}
	metadata := func(index int) provider.ProviderOptions {
		raw, err := json.Marshal(OpenAIPartOptions{ParallelToolCall: &OpenAIParallelToolCall{
			ItemID: "fc_parallel", ToolCallID: "call_parallel", ToolName: "parallel",
			Input: input, Index: index, Count: 2,
		}})
		require.NoError(t, err)
		return provider.ProviderOptions{"openai": provider.RawProviderOption{Key: "openai", Raw: raw}}
	}

	weatherResult := provider.ToolResultPart("call_parallel_0", "weather", &provider.ToolResultOutput{
		Type:            provider.ToolOutputJSON,
		JSON:            json.RawMessage(`{"temperature":72}`),
		ProviderOptions: provider.BuildProviderOptions(OpenAIPartOptions{PromptCacheBreakpoint: breakpoint}),
	})
	weatherResult.ProviderOptions = metadata(0)
	attractionsResult := provider.ToolResultPart("call_parallel_1", "cityAttractions", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "Colosseum"})
	attractionsResult.ProviderOptions = metadata(1)

	previousResponseID := "resp_previous"
	params, _, _, err := buildParams("gpt-5.4", provider.CallOptions{
		Prompt: []provider.Message{provider.NewToolMessage(weatherResult, attractionsResult)},
		Tools: []provider.Tool{
			{Type: provider.ToolTypeFunction, Name: "weather"},
			{Type: provider.ToolTypeFunction, Name: "cityAttractions"},
		},
		ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{PreviousResponseID: previousResponseID}),
	})
	require.NoError(t, err)

	raw, err := json.Marshal(params)
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))
	items := body["input"].([]any)
	require.Len(t, items, 1)
	output := items[0].(map[string]any)
	content := output["output"].([]any)
	require.Len(t, content, 2)
	assert.Equal(t, `{"temperature":72}`, content[0].(map[string]any)["text"])
	assert.Equal(t, map[string]any{"mode": "explicit"}, content[0].(map[string]any)["prompt_cache_breakpoint"])
	assert.Equal(t, "\nColosseum", content[1].(map[string]any)["text"])
	assert.NotContains(t, content[1].(map[string]any), "prompt_cache_breakpoint")
}
