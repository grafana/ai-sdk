package v4

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const functionToolRequest = `{"prompt":[{"role":"user","content":[{"type":"text","text":"Read the evidence"}]}],"tools":[{"type":"function","name":"read_evidence","description":"Read service evidence","inputSchema":{"type":"object","properties":{"service":{"type":"string"}}},"inputExamples":[{"input":{"service":"checkout"}}],"strict":false,"providerOptions":{"example":{}}}],"toolChoice":{"type":"required"}}`

func TestFunctionTools_RequestMapping(t *testing.T) {
	t.Run("definition", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		response := harness.serve(validRequest(functionToolRequest))
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		options := harness.model.receivedOptions()
		require.Len(t, options.Tools, 1)
		tool := options.Tools[0]
		assert.Equal(t, provider.ToolTypeFunction, tool.Type)
		assert.Equal(t, "read_evidence", tool.Name)
		assert.Equal(t, "Read service evidence", tool.Description)
		assert.JSONEq(t, `{"type":"object","properties":{"service":{"type":"string"}}}`, string(tool.InputSchema))
		require.Len(t, tool.InputExamples, 1)
		assert.JSONEq(t, `{"service":"checkout"}`, string(tool.InputExamples[0].Input))
		require.NotNil(t, tool.Strict)
		assert.False(t, *tool.Strict)
		assert.Nil(t, tool.ProviderOptions)
		assert.Equal(t, &provider.ToolChoice{Type: provider.ToolChoiceRequired}, options.ToolChoice)
	})

	t.Run("strict presence", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			fields string
			strict *bool
		}{
			{name: "omitted"},
			{name: "false", fields: `,"strict":false`, strict: new(false)},
			{name: "true", fields: `,"strict":true`, strict: new(true)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				response := harness.serve(validRequest(`{"prompt":[],"tools":[{"type":"function","name":"read_evidence","inputSchema":{}` + tc.fields + `}]}`))
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				require.Len(t, harness.model.receivedOptions().Tools, 1)
				assert.Equal(t, tc.strict, harness.model.receivedOptions().Tools[0].Strict)
			})
		}
	})

	t.Run("choice without definitions", func(t *testing.T) {
		for _, tc := range []struct {
			body   string
			choice *provider.ToolChoice
		}{
			{body: `{"prompt":[]}`},
			{body: `{"prompt":[],"toolChoice":{"type":"auto"}}`, choice: &provider.ToolChoice{Type: provider.ToolChoiceAuto}},
			{body: `{"prompt":[],"toolChoice":{"type":"none"}}`, choice: &provider.ToolChoice{Type: provider.ToolChoiceNone}},
			{body: `{"prompt":[],"toolChoice":{"type":"required"}}`, choice: &provider.ToolChoice{Type: provider.ToolChoiceRequired}},
			{body: `{"prompt":[],"toolChoice":{"type":"tool","toolName":"read_evidence"}}`, choice: &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "read_evidence"}},
		} {
			t.Run(tc.body, func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				response := harness.serve(validRequest(tc.body))
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				assert.Equal(t, tc.choice, harness.model.receivedOptions().ToolChoice)
			})
		}
	})
}

func TestFunctionTools_HistoryMapping(t *testing.T) {
	for _, input := range []string{`{"service":"checkout"}`, `null`, `"value"`, `42`, `false`, `[1,"two"]`} {
		t.Run(input, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			body := fmt.Sprintf(`{"prompt":[{"role":"assistant","content":[{"type":"text","text":"Checking"},{"type":"tool-call","toolCallId":"call-1","toolName":"read_evidence","input":%s,"providerExecuted":false,"providerOptions":{"example":{}}}],"providerOptions":{"example":{}}}]}`, input)
			response := harness.serve(validRequest(body))
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.Equal(t, []provider.Message{provider.NewAssistantMessage(provider.TextPart("Checking"), provider.ToolCallPart("call-1", "read_evidence", json.RawMessage(input)))}, harness.model.receivedOptions().Prompt)
		})
	}

	for _, tc := range []struct {
		name   string
		output string
		want   provider.ToolResultOutput
	}{
		{name: "text", output: `{"type":"text","value":"","providerOptions":{"example":{}}}`, want: provider.ToolResultOutput{Type: provider.ToolOutputText}},
		{name: "json", output: `{"type":"json","value":{"errorRate":4.2}}`, want: provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"errorRate":4.2}`)}},
		{name: "json null", output: `{"type":"json","value":null}`, want: provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`null`)}},
		{name: "error text", output: `{"type":"error-text","value":"failed"}`, want: provider.ToolResultOutput{Type: provider.ToolOutputErrorText, Text: "failed"}},
		{name: "error json", output: `{"type":"error-json","value":{"error":"failed"}}`, want: provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`{"error":"failed"}`)}},
		{name: "denied", output: `{"type":"execution-denied","reason":"not approved"}`, want: provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied, Reason: "not approved"}},
		{name: "denied without reason", output: `{"type":"execution-denied"}`, want: provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			body := fmt.Sprintf(`{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call-1","toolName":"read_evidence","output":%s,"providerOptions":{"example":{}}}],"providerOptions":{"example":{}}}]}`, tc.output)
			response := harness.serve(validRequest(body))
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.Equal(t, []provider.Message{provider.NewToolMessage(provider.ToolResultPart("call-1", "read_evidence", &tc.want))}, harness.model.receivedOptions().Prompt)
		})
	}
}

func TestFunctionTools_UnsupportedRequests(t *testing.T) {
	for _, tc := range []struct {
		name       string
		body       string
		capability unsupportedCapability
	}{
		{name: "provider tools", body: `{"prompt":[],"tools":[{"type":"provider","name":"search","id":"example.search","args":{}}]}`, capability: capabilityProviderDefinedTools},
		{name: "provider executed", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call-1","toolName":"read_evidence","input":{},"providerExecuted":true}]}]}`, capability: capabilityProviderExecutedTools},
		{name: "provider executed result", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call-1","toolName":"read_evidence","input":{}},{"type":"tool-result","toolCallId":"call-1","toolName":"read_evidence","output":{"type":"text","value":"private-sentinel"}}]}]}`, capability: capabilityProviderExecutedTools},
		{name: "multipart result", body: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call-1","toolName":"read_evidence","output":{"type":"content","value":[{"type":"text","text":"result"}]}}]}]}`, capability: capabilityMultipartToolResults},
		{name: "tool options", body: `{"prompt":[],"tools":[{"type":"function","name":"read_evidence","inputSchema":{},"providerOptions":{"example":{"secret":"private-sentinel"}}}]}`, capability: capabilityProviderOptions},
		{name: "call options", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call-1","toolName":"read_evidence","input":{},"providerOptions":{"example":{"secret":"private-sentinel"}}}]}]}`, capability: capabilityProviderOptions},
		{name: "result options", body: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call-1","toolName":"read_evidence","output":{"type":"text","value":"result"},"providerOptions":{"example":{"secret":"private-sentinel"}}}]}]}`, capability: capabilityProviderOptions},
		{name: "output options", body: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call-1","toolName":"read_evidence","output":{"type":"text","value":"result","providerOptions":{"example":{"secret":"private-sentinel"}}}}]}]}`, capability: capabilityProviderOptions},
		{name: "message options", body: `{"prompt":[{"role":"tool","content":[],"providerOptions":{"example":{"secret":"private-sentinel"}}}]}`, capability: capabilityProviderOptions},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			response := harness.serve(validRequest(tc.body))
			assert.Equal(t, http.StatusBadRequest, response.Code)
			assert.Equal(t, string(unsupportedCapabilityDocument(tc.capability)), response.Body.String())
			assert.NotContains(t, response.Body.String(), "private-sentinel")
			assert.Zero(t, harness.resolver.callCount())
			assert.Zero(t, harness.model.callCount())
		})
	}
}

func TestFunctionTools_SchemaBeforeMapping(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "invalid choice before unsupported definition", body: `{"prompt":[],"tools":[{"type":"provider","name":"read_evidence","id":"example.read_evidence","args":{}}],"toolChoice":{"type":"tool"}}`},
		{name: "invalid output before unsupported assistant result", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-result","toolCallId":"call-1","toolName":"read_evidence","output":{"type":"text","value":null}}]}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			response := harness.serve(validRequest(tc.body))
			assert.Equal(t, http.StatusBadRequest, response.Code)
			assert.Equal(t, string(canonicalInvalidRequestError), response.Body.String())
			assert.Zero(t, harness.resolver.callCount())
			assert.Zero(t, harness.model.callCount())
		})
	}
}

func TestFunctionTools_UnaryResponse(t *testing.T) {
	compiled, err := schema.CompileSchema(unarySuccessSchemaJSON)
	require.NoError(t, err)
	for _, input := range []string{"", `{`, `{"service":"checkout"}`, "\"\\\n☃<>&"} {
		for _, tc := range []struct {
			name    string
			dynamic *bool
		}{
			{name: "dynamic omitted"},
			{name: "dynamic false", dynamic: new(false)},
			{name: "dynamic true", dynamic: new(true)},
		} {
			t.Run(fmt.Sprintf("%q/%s", input, tc.name), func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					return &provider.GenerateResult{
						Content: []provider.GenerateContentPart{
							{Type: provider.ContentText, Text: ""},
							{Type: provider.ContentToolCall, ToolCallID: "call-1", ToolName: "read_evidence", Input: json.RawMessage(input), Dynamic: tc.dynamic, ProviderMetadata: map[string]json.RawMessage{"private": json.RawMessage(`{"secret":"private-sentinel"}`)}},
						},
						FinishReason: provider.FinishReason{Unified: provider.FinishReasonToolCalls},
					}, nil
				}
				response := harness.serve(validRequest(`{"prompt":[]}`))
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				require.NoError(t, compiled.Validate(json.RawMessage(response.Body.Bytes())))
				var result struct {
					Content []map[string]json.RawMessage `json:"content"`
				}
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
				require.Len(t, result.Content, 2)
				assert.JSONEq(t, `""`, string(result.Content[0]["text"]))
				assert.JSONEq(t, `"tool-call"`, string(result.Content[1]["type"]))
				assert.JSONEq(t, `"call-1"`, string(result.Content[1]["toolCallId"]))
				assert.JSONEq(t, `"read_evidence"`, string(result.Content[1]["toolName"]))
				var returnedInput string
				require.NoError(t, json.Unmarshal(result.Content[1]["input"], &returnedInput))
				assert.Equal(t, input, returnedInput)
				if tc.dynamic == nil {
					assert.NotContains(t, result.Content[1], "dynamic")
				} else {
					assert.Equal(t, fmt.Sprint(*tc.dynamic), string(result.Content[1]["dynamic"]))
				}
				assert.NotContains(t, response.Body.String(), "private-sentinel")
			})
		}
	}
}

func TestFunctionTools_InvalidUnaryResponse(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*provider.GenerateContentPart)
	}{
		{name: "provider execution", mutate: func(part *provider.GenerateContentPart) { part.ProviderExecuted = true }},
		{name: "missing ID", mutate: func(part *provider.GenerateContentPart) { part.ToolCallID = "" }},
		{name: "missing name", mutate: func(part *provider.GenerateContentPart) { part.ToolName = "" }},
		{name: "invalid ID UTF-8", mutate: func(part *provider.GenerateContentPart) { part.ToolCallID = string([]byte{0xff}) }},
		{name: "invalid name UTF-8", mutate: func(part *provider.GenerateContentPart) { part.ToolName = string([]byte{0xff}) }},
		{name: "invalid input UTF-8", mutate: func(part *provider.GenerateContentPart) { part.Input = json.RawMessage{0xff} }},
		{name: "oversized ID", mutate: func(part *provider.GenerateContentPart) {
			part.ToolCallID = strings.Repeat("x", int(testLimits().UnaryResponseBytes))
		}},
		{name: "oversized name", mutate: func(part *provider.GenerateContentPart) {
			part.ToolName = strings.Repeat("x", int(testLimits().UnaryResponseBytes))
		}},
		{name: "oversized input", mutate: func(part *provider.GenerateContentPart) {
			part.Input = json.RawMessage(strings.Repeat("x", int(testLimits().UnaryResponseBytes)))
		}},
		{name: "escaped input exceeds bound", mutate: func(part *provider.GenerateContentPart) {
			part.Input = json.RawMessage(strings.Repeat("\n", int(testLimits().UnaryResponseBytes)/2))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				part := provider.GenerateContentPart{Type: provider.ContentToolCall, ToolCallID: "call-1", ToolName: "read_evidence", Input: json.RawMessage(`{}`)}
				tc.mutate(&part)
				return &provider.GenerateResult{Content: []provider.GenerateContentPart{part}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonToolCalls}}, nil
			}
			response := harness.serve(validRequest(`{"prompt":[]}`))
			assert.Equal(t, http.StatusInternalServerError, response.Code)
			assert.Equal(t, string(canonicalInternalError), response.Body.String())
		})
	}
}
