package openai

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const parallelToolCallInput = `{"tool_uses":[{"recipient_name":"functions.weather","parameters":{"location":"San Francisco"}},{"recipient_name":"functions.cityAttractions","parameters":{"city":"Rome"}}]}`

func parallelToolCallMetadata(t *testing.T, index int) provider.ProviderOptions {
	t.Helper()
	raw, err := json.Marshal(OpenAIPartOptions{ParallelToolCall: &OpenAIParallelToolCall{
		ItemID: "fc_parallel", ToolCallID: "call_parallel", ToolName: "parallel",
		Input: parallelToolCallInput, Index: index, Count: 2,
	}})
	require.NoError(t, err)
	return provider.ProviderOptions{"openai": provider.RawProviderOption{Key: "openai", Raw: raw}}
}

func TestBuildParams_RegroupsParallelToolCallContinuation(t *testing.T) {
	weatherCall := provider.ToolCallPart("call_parallel_0", "weather", json.RawMessage(`{"location":"San Francisco"}`))
	weatherCall.ProviderOptions = parallelToolCallMetadata(t, 0)
	attractionsCall := provider.ToolCallPart("call_parallel_1", "cityAttractions", json.RawMessage(`{"city":"Rome"}`))
	attractionsCall.ProviderOptions = parallelToolCallMetadata(t, 1)
	weatherResult := provider.ToolResultPart("call_parallel_0", "weather", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"temperature":72}`)})
	weatherResult.ProviderOptions = parallelToolCallMetadata(t, 0)
	attractionsResult := provider.ToolResultPart("call_parallel_1", "cityAttractions", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "Colosseum"})
	attractionsResult.ProviderOptions = parallelToolCallMetadata(t, 1)

	body, _ := buildBody(t, "gpt-5.4", provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewAssistantMessage(weatherCall, attractionsCall),
			provider.NewToolMessage(weatherResult, attractionsResult),
		},
		Tools: []provider.Tool{
			{Type: provider.ToolTypeFunction, Name: "weather"},
			{Type: provider.ToolTypeFunction, Name: "cityAttractions"},
		},
		ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{PreviousResponseID: "resp_previous"}),
	})
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
	breakpoint := &PromptCacheBreakpoint{Mode: "explicit"}
	weatherResult := provider.ToolResultPart("call_parallel_0", "weather", &provider.ToolResultOutput{
		Type:            provider.ToolOutputJSON,
		JSON:            json.RawMessage(`{"temperature":72}`),
		ProviderOptions: provider.BuildProviderOptions(OpenAIPartOptions{PromptCacheBreakpoint: breakpoint}),
	})
	weatherResult.ProviderOptions = parallelToolCallMetadata(t, 0)
	attractionsResult := provider.ToolResultPart("call_parallel_1", "cityAttractions", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "Colosseum"})
	attractionsResult.ProviderOptions = parallelToolCallMetadata(t, 1)

	body, _ := buildBody(t, "gpt-5.4", provider.CallOptions{
		Prompt: []provider.Message{provider.NewToolMessage(weatherResult, attractionsResult)},
		Tools: []provider.Tool{
			{Type: provider.ToolTypeFunction, Name: "weather"},
			{Type: provider.ToolTypeFunction, Name: "cityAttractions"},
		},
		ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{PreviousResponseID: "resp_previous"}),
	})
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
