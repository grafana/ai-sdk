package openai

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const parallelInput = `{"tool_uses":[{"recipient_name":"functions.weather","parameters":{"location":"SF"}},{"recipient_name":"functions.cityAttractions","parameters":{"city":"Rome"}}]}`

func TestParallelToolCall_Expansion(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		declared bool
		want     int
	}{
		{name: "valid", input: parallelInput, want: 2},
		{name: "declared parallel", input: parallelInput, declared: true},
		{name: "invalid JSON", input: "invalid"},
		{name: "empty", input: `{"tool_uses":[]}`},
		{name: "unknown tool", input: `{"tool_uses":[{"recipient_name":"functions.missing","parameters":{}}]}`},
		{name: "no prefix", input: `{"tool_uses":[{"recipient_name":"weather","parameters":{}}]}`},
		{name: "null parameters", input: `{"tool_uses":[{"recipient_name":"functions.weather","parameters":null}]}`},
		{name: "array parameters", input: `{"tool_uses":[{"recipient_name":"functions.weather","parameters":[]}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			br := buildResult{functionTools: map[string]struct{}{"weather": {}, "cityAttractions": {}}}
			if tc.declared {
				br.functionTools[parallelToolName] = struct{}{}
			}
			parts := br.expandParallelToolCall(responses.ResponseFunctionToolCall{ID: "fc_parallel", CallID: "call_parallel", Name: parallelToolName, Arguments: tc.input}, "azure")
			require.Len(t, parts, tc.want)
			if tc.want == 0 {
				return
			}
			assert.Equal(t, "call_parallel_0", parts[0].ToolCallID)
			assert.Equal(t, "weather", parts[0].ToolName)
			assert.JSONEq(t, `{"location":"SF"}`, string(parts[0].Input))
			assert.Equal(t, "call_parallel_1", parts[1].ToolCallID)
			assert.Equal(t, "cityAttractions", parts[1].ToolName)
			for index, part := range parts {
				var metadata struct {
					ParallelToolCall parallelToolCallMetadata `json:"parallelToolCall"`
				}
				require.NoError(t, json.Unmarshal(part.ProviderMetadata["azure"], &metadata))
				assert.Equal(t, parallelToolCallMetadata{ItemID: "fc_parallel", ToolCallID: "call_parallel", ToolName: parallelToolName, Input: tc.input, Index: index, Count: 2}, metadata.ParallelToolCall)
			}
		})
	}
}

func parallelHistory(t *testing.T) []provider.Message {
	t.Helper()
	br := buildResult{functionTools: map[string]struct{}{"weather": {}, "cityAttractions": {}}}
	parts := br.expandParallelToolCall(responses.ResponseFunctionToolCall{ID: "fc_parallel", CallID: "call_parallel", Name: parallelToolName, Arguments: parallelInput}, "openai")
	require.Len(t, parts, 2)
	var calls, results []provider.ContentPart
	for _, part := range parts {
		opts := provider.ProviderOptions{"openai": provider.RawProviderOption{Key: "openai", Raw: part.ProviderMetadata["openai"]}}
		call := provider.ToolCallPart(part.ToolCallID, part.ToolName, part.Input)
		call.ProviderOptions = opts
		calls = append(calls, call)
		result := provider.ToolResultPart(part.ToolCallID, part.ToolName, &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: part.ToolName})
		result.ProviderOptions = opts
		results = append(results, result)
	}
	return []provider.Message{provider.NewAssistantMessage(calls...), provider.NewToolMessage(results[1], results[0])}
}

func TestParallelToolCall_Continuation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options OpenAIResponsesOptions
		count   int
	}{
		{name: "conversation", options: OpenAIResponsesOptions{Conversation: "conv_1"}, count: 1},
		{name: "previous response", options: OpenAIResponsesOptions{PreviousResponseID: "resp_1"}, count: 2},
		{name: "ordinary replay", count: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := buildBody(t, "gpt-5.4", provider.CallOptions{Prompt: parallelHistory(t), ProviderOptions: withOpenAIOptions(tc.options)})
			input := body["input"].([]any)
			require.Len(t, input, tc.count)
			if tc.count == 4 {
				assert.Equal(t, "call_parallel_0", input[0].(map[string]any)["call_id"])
				return
			}
			assert.Equal(t, map[string]any{"type": "function_call_output", "call_id": "call_parallel", "output": "weather\ncityAttractions"}, input[len(input)-1])
			if tc.count == 2 {
				assert.Equal(t, map[string]any{"type": "function_call", "call_id": "call_parallel", "name": "parallel", "arguments": parallelInput}, input[0])
			}
		})
	}
	t.Run("incomplete and duplicate groups stay ungrouped", func(t *testing.T) {
		for _, duplicate := range []bool{false, true} {
			prompt := parallelHistory(t)
			prompt[1].Content = prompt[1].Content[:1]
			if duplicate {
				prompt[1].Content = append(prompt[1].Content, prompt[1].Content[0])
			}
			body, _ := buildBody(t, "gpt-5.4", provider.CallOptions{Prompt: prompt, ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Conversation: "conv_1"})})
			for _, item := range body["input"].([]any) {
				assert.NotEqual(t, "call_parallel", item.(map[string]any)["call_id"])
			}
		}
	})
	t.Run("scalar cache breakpoints survive grouping", func(t *testing.T) {
		prompt := parallelHistory(t)
		for i := range prompt[1].Content {
			part := &prompt[1].Content[i]
			options, ok, err := provider.ResolveOption[map[string]json.RawMessage](part.ProviderOptions, "openai")
			require.NoError(t, err)
			require.True(t, ok)
			options["promptCacheBreakpoint"] = json.RawMessage(`{"mode":"explicit"}`)
			raw, err := json.Marshal(options)
			require.NoError(t, err)
			part.ProviderOptions = provider.ProviderOptions{"openai": provider.RawProviderOption{Key: "openai", Raw: raw}}
		}
		body, _ := buildBody(t, "gpt-5.4", provider.CallOptions{Prompt: prompt, ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Conversation: "conv_1"})})
		encoded, err := json.Marshal(body["input"])
		require.NoError(t, err)
		assert.JSONEq(t, `[{"type":"function_call_output","call_id":"call_parallel","output":[{"type":"input_text","text":"weather","prompt_cache_breakpoint":{"mode":"explicit"}},{"type":"input_text","text":"\ncityAttractions","prompt_cache_breakpoint":{"mode":"explicit"}}]}]`, string(encoded))
	})
}

func TestParallelToolCall_UnaryAndFallbackStream(t *testing.T) {
	br := buildResult{functionTools: map[string]struct{}{"weather": {}, "cityAttractions": {}}}
	encoded, err := json.Marshal(parallelInput)
	require.NoError(t, err)
	response := decodeResponse(t, fmt.Sprintf(`{"id":"resp_1","model":"gpt-5.4","status":"completed","output":[{"type":"function_call","id":"fc_parallel","call_id":"call_parallel","name":"parallel","arguments":%s}]}`, encoded))
	result := mustConvertResponse(t, response, br)
	require.Len(t, result.Content, 2)
	assert.Equal(t, provider.FinishReasonToolCalls, result.FinishReason.Unified)

	parts := collectPartsWithBuildResult(t, br,
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_parallel","call_id":"call_parallel","name":"parallel","arguments":""}}`,
		`{"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_parallel","delta":"not "}`,
		`{"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_parallel","delta":"JSON"}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_parallel","call_id":"call_parallel","name":"parallel","arguments":"not JSON"}}`,
	)
	require.Len(t, parts, 6)
	assert.Equal(t, []provider.StreamPartType{provider.PartStreamStart, provider.PartToolInputStart, provider.PartToolInputDelta, provider.PartToolInputDelta, provider.PartToolInputEnd, provider.PartToolCall}, partTypes(parts))
	assert.Equal(t, "not ", parts[2].Delta)
	assert.Equal(t, "JSON", parts[3].Delta)
	assert.Equal(t, "parallel", parts[5].ToolName)
	assert.Equal(t, "not JSON", parts[5].Input)
}
