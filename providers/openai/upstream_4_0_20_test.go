package openai

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildParams_ForwardCompatibleModelDefaults(t *testing.T) {
	temperature := 0.2
	topP := 0.8
	body, _ := buildBody(t, "gpt-99", provider.CallOptions{
		Prompt:      []provider.Message{provider.NewSystemMessage("Follow instructions"), provider.UserText("Say ok")},
		Temperature: &temperature,
		TopP:        &topP,
	})
	input := body["input"].([]any)
	assert.Equal(t, "developer", input[0].(map[string]any)["role"])
	assert.NotContains(t, body, "temperature")
	assert.NotContains(t, body, "top_p")
}

func TestBuildParams_ToolSearchOutputItemReference(t *testing.T) {
	result := provider.ToolResultPart("tsc_hosted_123", "search", &provider.ToolResultOutput{
		Type: provider.ToolOutputJSON,
		JSON: json.RawMessage(`{"tools":[{"type":"function","name":"get_weather","parameters":{"type":"object"},"strict":false}]}`),
	})
	result.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{ItemID: "tso_hosted_456"})
	body, _ := buildBody(t, "gpt-5.6", provider.CallOptions{
		Prompt: []provider.Message{provider.NewAssistantMessage(result)},
		Tools:  []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDToolSearch, Name: "search"}},
	})
	input := body["input"].([]any)
	require.Len(t, input, 1)
	assert.Equal(t, map[string]any{"type": "item_reference", "id": "tso_hosted_456"}, input[0])
}

func TestBuildParams_ClientToolSearchOutput(t *testing.T) {
	result := provider.ToolResultPart("tsc_client_123", "search", &provider.ToolResultOutput{
		Type: provider.ToolOutputJSON,
		JSON: json.RawMessage(`{"tools":[{"type":"function","name":"get_weather","parameters":{"type":"object"},"strict":false}]}`),
	})
	body, _ := buildBody(t, "gpt-5.6", provider.CallOptions{
		Prompt: []provider.Message{provider.NewToolMessage(result)},
		Tools:  []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDToolSearch, Name: "search"}},
	})
	input := body["input"].([]any)
	require.Len(t, input, 1)
	assert.Equal(t, "tool_search_output", input[0].(map[string]any)["type"])
	assert.Equal(t, "client", input[0].(map[string]any)["execution"])
	assert.Equal(t, "tsc_client_123", input[0].(map[string]any)["call_id"])
	assert.NotContains(t, input[0].(map[string]any), "id")
}

func TestBuildParams_NonJSONClientToolSearchOutputUsesFunctionFallback(t *testing.T) {
	result := provider.ToolResultPart("tsc_client_123", "search", &provider.ToolResultOutput{
		Type: provider.ToolOutputText,
		Text: "not JSON",
	})
	body, _ := buildBody(t, "gpt-5.6", provider.CallOptions{
		Prompt: []provider.Message{provider.NewToolMessage(result)},
		Tools:  []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDToolSearch, Name: "search"}},
	})
	input := body["input"].([]any)
	require.Len(t, input, 1)
	assert.Equal(t, map[string]any{
		"type": "function_call_output", "call_id": "tsc_client_123", "output": "not JSON",
	}, input[0])
}

func TestBuildParams_ProgrammaticToolCalling(t *testing.T) {
	store := false
	caller := &OpenAIToolCaller{Type: OpenAIToolCallerProgram, CallerID: "call_program"}
	programCall := provider.ToolCallPart("call_program", "program", json.RawMessage(`{"code":"await tools.lookup()","fingerprint":"fp"}`))
	programCall.ProviderExecuted = true
	programCall.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{ItemID: "program_item"})
	functionCall := provider.ToolCallPart("call_lookup", "lookup", json.RawMessage(`{}`))
	functionCall.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{ItemID: "function_item", Caller: caller})
	functionResult := provider.ToolResultPart("call_lookup", "lookup", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"ok":true}`)})
	functionResult.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{Caller: caller})
	programResult := provider.ToolResultPart("call_program", "program", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"result":"done","status":"completed"}`)})
	programResult.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{ItemID: "program_output_item"})

	body, warnings := buildBody(t, "gpt-5.6", provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewAssistantMessage(programCall, functionCall, programResult),
			provider.NewToolMessage(functionResult),
		},
		Tools: []provider.Tool{
			{Type: provider.ToolTypeProvider, ID: toolIDProgrammatic, Name: "program"},
			{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(`{"type":"object"}`), ProviderOptions: provider.BuildProviderOptions(OpenAIToolOptions{
				AllowedCallers: []OpenAIAllowedCaller{OpenAIAllowedCallerProgrammatic},
				OutputSchema:   json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}}}`),
			})},
		},
		ProviderOptions: provider.BuildProviderOptions(OpenAIResponsesOptions{Store: &store}),
	})
	assert.Empty(t, warnings)

	encoded, err := json.Marshal(body["tools"])
	require.NoError(t, err)
	assert.JSONEq(t, `[
		{"type":"programmatic_tool_calling"},
		{"type":"function","name":"lookup","parameters":{"type":"object"},"allowed_callers":["programmatic"],"output_schema":{"type":"object","properties":{"ok":{"type":"boolean"}}}}
	]`, string(encoded))

	input, err := json.Marshal(body["input"])
	require.NoError(t, err)
	assert.JSONEq(t, `[
		{"type":"program","id":"program_item","call_id":"call_program","code":"await tools.lookup()","fingerprint":"fp"},
		{"type":"function_call","call_id":"call_lookup","name":"lookup","arguments":"{}","caller":{"type":"program","caller_id":"call_program"}},
		{"type":"program_output","id":"program_output_item","call_id":"call_program","result":"done","status":"completed"},
		{"type":"function_call_output","call_id":"call_lookup","output":"{\"ok\":true}","caller":{"type":"program","caller_id":"call_program"}}
	]`, string(input))
}

func TestConvertResponse_ProgrammaticToolCalling(t *testing.T) {
	var response responses.Response
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"resp_1","created_at":1,"model":"gpt-5.6","status":"completed",
		"output":[
			{"type":"program","id":"program_item","call_id":"call_program","code":"code","fingerprint":"fp"},
			{"type":"function_call","id":"function_item","call_id":"call_lookup","name":"lookup","arguments":"{}","caller":{"type":"program","caller_id":"call_program"}},
			{"type":"program_output","id":"program_output_item","call_id":"call_program","result":"done","status":"completed"}
		],
		"usage":{"input_tokens":1,"input_tokens_details":{"cached_tokens":0},"output_tokens":1,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":2}
	}`), &response))
	br := buildResult{toolNameMapping: newToolNameMapping([]provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDProgrammatic, Name: "program"}})}
	result, err := convertResponse(&response, br, seqIDGen(), "openai")
	require.NoError(t, err)
	require.Len(t, result.Content, 3)
	assert.True(t, result.Content[0].ProviderExecuted)
	assert.JSONEq(t, `{"itemId":"program_item"}`, string(result.Content[0].ProviderMetadata["openai"]))
	assert.JSONEq(t, `{"itemId":"function_item","caller":{"type":"program","callerId":"call_program"}}`, string(result.Content[1].ProviderMetadata["openai"]))
	assert.JSONEq(t, `{"result":"done","status":"completed"}`, string(result.Content[2].Result))
}

func TestStream_ProgrammaticToolCalling(t *testing.T) {
	br := buildResult{toolNameMapping: newToolNameMapping([]provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDProgrammatic, Name: "program"}})}
	parts := collectPartsWithBuildResult(t, br,
		`{"type":"response.output_item.done","output_index":0,"sequence_number":1,"item":{"type":"program","id":"program_item","call_id":"call_program","code":"code","fingerprint":"fp"}}`,
		`{"type":"response.output_item.done","output_index":1,"sequence_number":2,"item":{"type":"function_call","id":"function_item","call_id":"call_lookup","name":"lookup","arguments":"{}","caller":{"type":"program","caller_id":"call_program"}}}`,
		`{"type":"response.output_item.done","output_index":2,"sequence_number":3,"item":{"type":"program_output","id":"program_output_item","call_id":"call_program","result":"done","status":"completed"}}`,
	)
	var calls []provider.StreamPart
	for _, part := range parts {
		if part.Type == provider.PartToolCall || part.Type == provider.PartToolResult {
			calls = append(calls, part)
		}
	}
	require.Len(t, calls, 3)
	assert.True(t, calls[0].ProviderExecuted)
	assert.JSONEq(t, `{"itemId":"function_item","caller":{"type":"program","callerId":"call_program"}}`, string(calls[1].ProviderMetadata["openai"]))
	assert.JSONEq(t, `{"result":"done","status":"completed"}`, string(calls[2].Result))
}
