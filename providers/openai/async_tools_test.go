package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildParams_AsyncToolCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name, model string
		async       *bool
		wantWarning bool
	}{
		{name: "supported", model: "gpt-6-astra", async: boolPointer(true)},
		{name: "unsupported", model: "gpt-5.6", async: boolPointer(true), wantWarning: true},
		{name: "mantle requires verification", model: "openai.gpt-6-astra", async: boolPointer(true), wantWarning: true},
		{name: "explicit false", model: "gpt-5.6", async: boolPointer(false)},
		{name: "absent", model: "gpt-6-astra"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			customArgs := map[string]json.RawMessage{}
			if tc.async != nil {
				value, err := json.Marshal(tc.async)
				require.NoError(t, err)
				customArgs["async"] = value
			}
			body, warnings := buildBody(t, tc.model, provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")},
				Tools: []provider.Tool{
					{Type: provider.ToolTypeFunction, Name: "lookup", ProviderOptions: provider.BuildProviderOptions(OpenAIToolOptions{Async: tc.async, AllowedCallers: []OpenAIAllowedCaller{OpenAIAllowedCallerDirect}})},
					{Type: provider.ToolTypeFunction, Name: "search", ProviderOptions: provider.BuildProviderOptions(OpenAIToolOptions{Async: tc.async, Namespace: &OpenAIToolNamespaceOptions{Name: "utilities"}})},
					{Type: provider.ToolTypeProvider, ID: toolIDCustom, Name: "write_sql", Args: customArgs},
				},
			})
			tools := toolsArray(t, body)
			require.Len(t, tools, 3)
			fn := tools[0]
			namespace := tools[1]["tools"].([]any)[0].(map[string]any)
			custom := tools[2]
			assert.Equal(t, []any{"direct"}, fn["allowed_callers"])
			for _, tool := range []map[string]any{fn, namespace, custom} {
				if tc.async != nil && (!*tc.async || !tc.wantWarning) {
					assert.Equal(t, *tc.async, tool["async"])
				} else {
					assert.NotContains(t, tool, "async")
				}
			}
			if tc.wantWarning {
				for _, name := range []string{"lookup", "search", "write_sql"} {
					assert.Contains(t, warningFeatures(warnings), `async tool calling for "`+name+`"`)
				}
			} else {
				assert.Empty(t, warnings)
			}
		})
	}
}

func boolPointer(value bool) *bool { return &value }

func TestConvertResponse_AsyncToolCallMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, toolType, fields, want string
	}{
		{name: "function true with caller", toolType: "function_call", fields: `,"async":true,"namespace":"ns","caller":{"type":"program","caller_id":"prog_1"}`, want: `{"itemId":"item_1","async":true,"namespace":"ns","caller":{"type":"program","callerId":"prog_1"}}`},
		{name: "function false", toolType: "function_call", fields: `,"async":false`, want: `{"itemId":"item_1","async":false}`},
		{name: "function absent", toolType: "function_call", want: `{"itemId":"item_1"}`},
		{name: "custom true", toolType: "custom_tool_call", fields: `,"async":true`, want: `{"itemId":"item_1","async":true}`},
		{name: "custom false", toolType: "custom_tool_call", fields: `,"async":false`, want: `{"itemId":"item_1","async":false}`},
		{name: "custom absent", toolType: "custom_tool_call", want: `{"itemId":"item_1"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			field := `"arguments":"{}"`
			if tc.toolType == "custom_tool_call" {
				field = `"input":"select 1"`
			}
			resp := decodeResponse(t, `{"id":"resp_1","created_at":1,"model":"gpt-6","status":"completed","output":[{"type":"`+tc.toolType+`","id":"item_1","call_id":"call_1","name":"lookup",`+field+tc.fields+`}]}`)
			result := mustConvertResponse(t, resp, buildResult{})
			require.Len(t, result.Content, 1)
			assert.Equal(t, provider.ContentToolCall, result.Content[0].Type)
			assert.JSONEq(t, tc.want, string(result.Content[0].ProviderMetadata["openai"]))
		})
	}
}

func TestStream_AsyncToolCallMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, toolType, added, done, want string
	}{
		{name: "function added fallback", toolType: "function_call", added: `,"async":true`, want: `{"itemId":"item_1","async":true}`},
		{name: "function done overrides added with namespace and caller", toolType: "function_call", added: `,"async":true`, done: `,"async":false,"namespace":"ns","caller":{"type":"program","caller_id":"prog_1"}`, want: `{"itemId":"item_1","async":false,"namespace":"ns","caller":{"type":"program","callerId":"prog_1"}}`},
		{name: "function done only", toolType: "function_call", done: `,"async":false`, want: `{"itemId":"item_1","async":false}`},
		{name: "function absent", toolType: "function_call", want: `{"itemId":"item_1"}`},
		{name: "custom added fallback", toolType: "custom_tool_call", added: `,"async":false`, want: `{"itemId":"item_1","async":false}`},
		{name: "custom done overrides added", toolType: "custom_tool_call", added: `,"async":false`, done: `,"async":true`, want: `{"itemId":"item_1","async":true}`},
		{name: "custom absent", toolType: "custom_tool_call", want: `{"itemId":"item_1"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			field := `"arguments":"{}"`
			if tc.toolType == "custom_tool_call" {
				field = `"input":"select 1"`
			}
			item := func(extra string) string {
				return `{"type":"` + tc.toolType + `","id":"item_1","call_id":"call_1","name":"lookup",` + field + extra + `}`
			}
			parts := collectParts(t,
				`{"type":"response.output_item.added","sequence_number":0,"output_index":0,"item":`+item(tc.added)+`}`,
				`{"type":"response.output_item.done","sequence_number":1,"output_index":0,"item":`+item(tc.done)+`}`,
			)
			var calls []provider.StreamPart
			for _, part := range parts {
				if part.Type == provider.PartToolCall {
					calls = append(calls, part)
				}
			}
			require.Len(t, calls, 1)
			assert.JSONEq(t, tc.want, string(calls[0].ProviderMetadata["openai"]))
		})
	}
}

func TestToolCallMeta_AzureNamespaceAndAsyncPresence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		async *bool
	}{
		{name: "absent"},
		{name: "false", async: boolPointer(false)},
		{name: "true", async: boolPointer(true)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			meta := toolCallMeta("azure", "id", "", "", "", tc.async)
			assert.NotContains(t, meta, "openai")
			var fields map[string]any
			require.NoError(t, json.Unmarshal(meta["azure"], &fields))
			if tc.async == nil {
				assert.NotContains(t, fields, "async")
			} else {
				assert.Equal(t, *tc.async, fields["async"])
			}
		})
	}
}

func TestBuildParams_AsyncToolCallContinuation(t *testing.T) {
	for _, tc := range []struct {
		name, toolID, toolName string
		store                  bool
		async                  bool
		wantType               string
	}{
		{name: "function nonstored true", toolName: "lookup", async: true, wantType: "function_call"},
		{name: "function nonstored false", toolName: "lookup", wantType: "function_call"},
		{name: "function stored false remains inline", toolName: "lookup", store: true, wantType: "function_call"},
		{name: "custom nonstored true", toolID: toolIDCustom, toolName: "write_sql", async: true, wantType: "custom_tool_call"},
		{name: "custom stored remains reference", toolID: toolIDCustom, toolName: "write_sql", store: true, async: true, wantType: "item_reference"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			part := provider.ToolCallPart("call_1", tc.toolName, json.RawMessage(`{}`))
			part.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{
				ItemID: "item_1", Async: &tc.async, Namespace: "ns", Caller: &OpenAIToolCaller{Type: OpenAIToolCallerProgram, CallerID: "prog_1"},
			})
			result := provider.ToolResultPart("call_1", tc.toolName, &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "done"})
			tools := []provider.Tool{{Type: provider.ToolTypeFunction, Name: tc.toolName}}
			if tc.toolID != "" {
				tools[0] = provider.Tool{Type: provider.ToolTypeProvider, ID: tc.toolID, Name: tc.toolName}
			}
			body, _ := buildBody(t, "gpt-6-astra", provider.CallOptions{
				Prompt: []provider.Message{provider.NewAssistantMessage(part), provider.NewToolMessage(result)},
				Tools:  tools, ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Store: &tc.store}),
			})
			input := body["input"].([]any)
			require.NotEmpty(t, input)
			call := input[0].(map[string]any)
			assert.Equal(t, tc.wantType, call["type"])
			if tc.wantType == "item_reference" {
				assert.Equal(t, "item_1", call["id"])
			} else {
				assert.Equal(t, tc.async, call["async"])
				assert.Equal(t, "call_1", call["call_id"])
				if tc.toolID == "" {
					assert.Equal(t, "ns", call["namespace"])
					assert.Equal(t, "program", call["caller"].(map[string]any)["type"])
				}
			}
			assert.Equal(t, "call_1", input[1].(map[string]any)["call_id"])
		})
	}
}

func TestProgrammaticDenial_RequestConversion(t *testing.T) {
	for _, tc := range []struct {
		name, caller string
		assistant    bool
		custom       bool
		wantError    bool
	}{
		{name: "preceding program call", caller: "program", assistant: true, wantError: true},
		{name: "result caller without assistant", caller: "program", wantError: true},
		{name: "direct caller", caller: "direct", assistant: true},
		{name: "plain denied result", assistant: true},
		{name: "custom tool denial remains supported", caller: "program", custom: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messages := []provider.Message{provider.UserText("hi")}
			if tc.assistant {
				call := provider.ToolCallPart("call_1", "lookup", json.RawMessage(`{}`))
				if tc.caller != "" {
					call.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{Caller: &OpenAIToolCaller{Type: OpenAIToolCallerType(tc.caller), CallerID: "prog_1"}})
				}
				messages = append(messages, provider.NewAssistantMessage(call))
			}
			result := provider.ToolResultPart("call_1", "lookup", &provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied})
			if !tc.assistant && tc.caller != "" {
				result.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{Caller: &OpenAIToolCaller{Type: OpenAIToolCallerType(tc.caller), CallerID: "prog_1"}})
			}
			messages = append(messages, provider.NewToolMessage(result))
			tools := []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup"}}
			if tc.custom {
				tools[0] = provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDCustom, Name: "lookup"}
			}
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				contentType, payload := "application/json", `{"id":"resp_1","status":"completed","output":[]}`
				if req.Header.Get("Accept") == "text/event-stream" {
					contentType = "text/event-stream"
					payload = "event: response.completed\n" + `data: {"type":"response.completed","sequence_number":0,"response":{"id":"resp_1","status":"completed","output":[]}}` + "\n\n"
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(payload)), Request: req}, nil
			})}
			model := NewResponses("test-key", "gpt-6", WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
			opts := provider.CallOptions{Prompt: messages, Tools: tools}
			_, generateErr := model.DoGenerate(t.Context(), opts)
			stream, streamErr := model.DoStream(t.Context(), opts)
			if tc.wantError {
				require.ErrorContains(t, generateErr, "execution-denied results for programmatic tool calls")
				require.ErrorContains(t, streamErr, "execution-denied results for programmatic tool calls")
				assert.Equal(t, 0, calls)
			} else {
				require.NoError(t, generateErr)
				require.NoError(t, streamErr)
				for range stream.Stream {
				}
				assert.Equal(t, 2, calls)
			}
		})
	}
}

func TestBuildParams_ProviderExecutedDenialIsNotHistory(t *testing.T) {
	part := provider.ToolResultPart("call_1", "lookup", &provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied})
	part.ProviderExecuted = true
	body, warnings := buildBody(t, "gpt-6", provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(part)}})
	assert.Empty(t, warnings)
	assert.Empty(t, body["input"])
}
