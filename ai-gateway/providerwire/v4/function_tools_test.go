package v4

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeUnaryFunctionTools(t *testing.T) {
	body := `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"weather","input":{"city":"Rio"}}]},{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"weather","output":{"type":"json","value":null}}]}],"tools":[{"type":"function","name":"weather","description":"","inputSchema":{"type":"object"},"strict":false,"inputExamples":[{"input":{"city":"Rio"}}],"providerOptions":{"anthropic":{"cacheControl":{"type":"ephemeral"}}}}],"toolChoice":{"type":"tool","toolName":"weather"}}`
	harness := newRuntimeHarness(t, testLimits())
	response := harness.serve(validRequest(body))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	options := harness.model.receivedOptions()
	require.Len(t, options.Tools, 1)
	require.NotNil(t, options.Tools[0].Strict)
	assert.False(t, *options.Tools[0].Strict)
	assert.JSONEq(t, `{"type":"object"}`, string(options.Tools[0].InputSchema))
	require.Len(t, options.Tools[0].InputExamples, 1)
	assert.JSONEq(t, `{"city":"Rio"}`, string(options.Tools[0].InputExamples[0].Input))
	assert.Equal(t, provider.ToolChoiceTool, options.ToolChoice.Type)
	assert.JSONEq(t, `{"city":"Rio"}`, string(options.Prompt[0].Content[0].Input))
	assert.Equal(t, "null", string(options.Prompt[1].Content[0].Output.JSON))
}

func TestRuntimeUnaryProviderTools_MetadataAndHistory(t *testing.T) {
	metadata := provider.ProviderMetadata{"anthropic": json.RawMessage(`{"caller":{"type":"direct"},"backend":"private"}`), "private": json.RawMessage(`{"token":"private-token"}`)}
	call := provider.GenerateContentPart{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "echo", Input: json.RawMessage(`{"message":"hi"}`), ProviderExecuted: true, Dynamic: new(true), ProviderMetadata: metadata}
	resultPart := provider.GenerateContentPart{Type: provider.ContentToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`{"result":0}`), ProviderExecuted: true, ProviderMetadata: metadata}
	for _, tc := range []struct {
		name, body string
		content    []provider.GenerateContentPart
	}{
		{name: "call and result", body: `{"prompt":[]}`, content: []provider.GenerateContentPart{call, resultPart}},
		{name: "result only after history", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"echo","input":{"message":"hi"},"providerExecuted":true,"providerOptions":{"anthropic":{"caller":{"type":"direct"}}}}]}]}`, content: []provider.GenerateContentPart{resultPart}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				response := validGenerateResult()
				response.Content = tc.content
				return response, nil
			}
			response := harness.serve(validRequest(tc.body))
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			compiled, err := schema.CompileSchema(unarySuccessSchemaJSON)
			require.NoError(t, err)
			require.NoError(t, compiled.Validate(json.RawMessage(response.Body.Bytes())))
			assert.Equal(t, 1, harness.model.callCount())
			assert.Contains(t, response.Body.String(), `"type":"tool-result"`)
			assert.Contains(t, response.Body.String(), `"result":{"result":0}`)
			if tc.name == "call and result" {
				assert.Contains(t, response.Body.String(), `"providerExecuted":true`)
				assert.Contains(t, response.Body.String(), `"dynamic":true`)
			}
			assert.Contains(t, response.Body.String(), `"caller":{"type":"direct"}`)
			for _, marker := range []string{"private-token", "backend", `"private"`} {
				assert.Contains(t, response.Body.String(), marker)
			}
		})
	}
}

func TestRuntimeUnaryProviderTools_InvalidResultsFailBeforeCommit(t *testing.T) {
	for _, tc := range []struct{ name, body, id, toolName, result string }{
		{name: "unknown", body: `{"prompt":[]}`, id: "unknown", toolName: "echo", result: `{}`},
		{name: "name mismatch", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"echo","input":{},"providerExecuted":true}]}]}`, id: "call", toolName: "other", result: `{}`},
		{name: "completed in history", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"echo","input":{},"providerExecuted":true},{"type":"tool-result","toolCallId":"call","toolName":"echo","output":{"type":"json","value":{}}}]}]}`, id: "call", toolName: "echo", result: `{}`},
		{name: "null result", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"echo","input":{},"providerExecuted":true}]}]}`, id: "call", toolName: "echo", result: `null`},
		{name: "empty historical name", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"","input":{},"providerExecuted":true}]}]}`, id: "call", toolName: "", result: `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				response := validGenerateResult()
				response.Content = []provider.GenerateContentPart{{Type: provider.ContentToolResult, ToolCallID: tc.id, ToolName: tc.toolName, Result: json.RawMessage(tc.result)}}
				return response, nil
			}
			response := harness.serve(validRequest(tc.body))
			assert.Equal(t, http.StatusInternalServerError, response.Code)
			assert.Equal(t, string(canonicalInternalError), response.Body.String())
		})
	}
}

func TestRuntimeProviderToolHistory_OptionsRemainOpaque(t *testing.T) {
	body := `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"echo","input":{},"providerExecuted":true,"providerOptions":{"future":{"function":{"name":"extension","arguments":"{}"},"ID":"extension","nested":[null,false,{}]}}}]}]}`
	for _, streaming := range []bool{false, true} {
		harness := newRuntimeHarness(t, testLimits())
		request := validRequest(body)
		if streaming {
			request.Header.Set(HeaderStreaming, "true")
		}
		response := harness.serve(request)
		require.Equal(t, http.StatusOK, response.Code)
		options := harness.model.receivedOptions()
		value, ok := options.Prompt[0].Content[0].ProviderOptions["future"].(provider.RawProviderOption)
		require.True(t, ok)
		assert.JSONEq(t, `{"function":{"name":"extension","arguments":"{}"},"ID":"extension","nested":[null,false,{}]}`, string(value.Raw))
		assert.Equal(t, 1, harness.resolver.callCount())
		assert.Equal(t, 1, harness.model.callCount())
	}
}

func TestRuntimeUnaryProviderTools_DeferredCompoundOutputFailsBeforeCommit(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		result := validGenerateResult()
		result.Content = []provider.GenerateContentPart{
			{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "search", Input: json.RawMessage(`{}`), ProviderExecuted: true},
			{Type: provider.ContentSource, SourceType: provider.SourceTypeURL, ID: "\xff", URL: "https://private-source.example"},
		}
		return result, nil
	}
	response := harness.serve(validRequest(`{"prompt":[],"tools":[{"type":"provider","id":"anthropic.web_search_20250305","name":"search","args":{}}]}`))
	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.Equal(t, string(canonicalInternalError), response.Body.String())
	assert.NotContains(t, response.Body.String(), "private-source")
}

func TestMapFunctionTools_PreservesExplicitEmptyInputExamples(t *testing.T) {
	tools, _, failure := mapFunctionTools([]json.RawMessage{json.RawMessage(`{"type":"function","name":"weather","inputSchema":{"type":"object"},"inputExamples":[]}`)}, nil)
	require.Nil(t, failure)
	require.Len(t, tools, 1)
	assert.Empty(t, tools[0].InputExamples)
	assert.NotNil(t, tools[0].InputExamples)

	tools, _, failure = mapFunctionTools([]json.RawMessage{json.RawMessage(`{"type":"function","name":"weather","inputSchema":{"type":"object"}}`)}, nil)
	require.Nil(t, failure)
	require.Len(t, tools, 1)
	assert.Nil(t, tools[0].InputExamples)
}

func TestUnaryFunctionOutput(t *testing.T) {
	result := validGenerateResult()
	result.Content = append(result.Content, provider.GenerateContentPart{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "weather", Input: json.RawMessage(`{"city":"Rio"}`), ProviderMetadata: provider.ProviderMetadata{"private": json.RawMessage(`{"secret":"hidden"}`)}})
	mapped, err := mapUnarySuccess(result, 1<<20)
	require.NoError(t, err)
	body, ok := encodeUnarySuccess(mapped, 1<<20)
	require.True(t, ok)
	assert.Contains(t, string(body), `"input":"{\"city\":\"Rio\"}"`)
	assert.Contains(t, string(body), `"providerMetadata":{"private":{"secret":"hidden"}}`)
	for _, marker := range []string{"providerExecuted", "dynamic"} {
		t.Run(marker, func(t *testing.T) {
			copyResult := *result
			copyResult.Content = append([]provider.GenerateContentPart(nil), result.Content...)
			if marker == "providerExecuted" {
				copyResult.Content[1].ProviderExecuted = true
			} else {
				yes := true
				copyResult.Content[1].Dynamic = &yes
			}
			mapped, err := mapUnarySuccess(&copyResult, 1<<20)
			require.NoError(t, err)
			body, ok := encodeUnarySuccess(mapped, 1<<20)
			require.True(t, ok)
			assert.Contains(t, string(body), `"`+marker+`":true`)
		})
	}
}

func TestRuntimeProviderToolHistory(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		pending    map[string]string
		status     int
	}{
		{name: "unresolved hosted call", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"deferred","toolName":"code_execution","input":{"code":"print(1)"},"providerExecuted":true}]}]}`, pending: map[string]string{"deferred": "code_execution"}, status: http.StatusOK},
		{name: "completed hosted call", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"code_execution","input":{},"providerExecuted":true},{"type":"tool-result","toolCallId":"call","toolName":"code_execution","output":{"type":"json","value":{"ok":true}}}]}]}`, pending: map[string]string{}, status: http.StatusOK},
		{name: "truncated history result", body: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"prior","toolName":"other","output":{"type":"text","value":"done"}}]}]}`, pending: map[string]string{}, status: http.StatusOK},
		{name: "result before provider call with same ID", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-result","toolCallId":"call","toolName":"echo","output":{"type":"json","value":{}}},{"type":"tool-call","toolCallId":"call","toolName":"echo","input":{},"providerExecuted":true}]}]}`, status: http.StatusBadRequest},
		{name: "client call never eligible", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"local","toolName":"f","input":{}}]}]}`, pending: map[string]string{}, status: http.StatusOK},
		{name: "provider continuation", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"echo","input":{},"providerExecuted":true,"providerOptions":{"anthropic":{"caller":{"type":"direct"}}}},{"type":"tool-result","toolCallId":"call","toolName":"echo","output":{"type":"json","value":{"ok":true}},"providerOptions":{"anthropic":{"caller":{"type":"direct"}}}}]}]}`, pending: map[string]string{}, status: http.StatusOK},
		{name: "opaque MCP-like namespace", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"mcp","toolName":"echo","input":{},"providerExecuted":true,"providerOptions":{"anthropic":{"type":"mcp-tool-use","serverName":"other"}}}]}],"providerOptions":{"anthropic":{"mcpServers":[{"type":"url","name":"echo","url":"https://mcp.example.test"}]}}}`, pending: map[string]string{"mcp": "echo"}, status: http.StatusOK},
		{name: "opaque metadata does not grant ownership", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"mcp","toolName":"echo","input":{},"providerOptions":{"anthropic":{"type":"mcp-tool-use","serverName":"echo"}}}]}],"providerOptions":{"anthropic":{"mcpServers":[{"type":"url","name":"echo","url":"https://mcp.example.test"}]}}}`, pending: map[string]string{}, status: http.StatusOK},
		{name: "duplicate historical call", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"f","input":{}},{"type":"tool-call","toolCallId":"call","toolName":"f","input":{}}]}]}`, status: http.StatusBadRequest},
		{name: "mismatched history result", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"f","input":{},"providerExecuted":true},{"type":"tool-result","toolCallId":"call","toolName":"other","output":{"type":"text","value":"result"}}]}]}`, status: http.StatusBadRequest},
		{name: "reserved part options", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"f","input":{},"providerOptions":{"gateway":{}}}]}]}`, status: http.StatusBadRequest},
		{name: "nested output options deferred", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-result","toolCallId":"call","toolName":"f","output":{"type":"json","value":false,"providerOptions":{"anthropic":{"private":true}}}}]}]}`, status: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			response := harness.serve(validRequest(tc.body))
			require.Equal(t, tc.status, response.Code, response.Body.String())
			if tc.status != http.StatusOK {
				assert.Zero(t, harness.model.callCount())
				return
			}
			opts := harness.model.receivedOptions()
			pending, failure := unresolvedProviderCalls(opts.Prompt)
			require.Nil(t, failure)
			assert.Equal(t, tc.pending, pending)
			if tc.name == "provider continuation" {
				parts := opts.Prompt[0].Content
				assert.True(t, parts[0].ProviderExecuted)
				assert.JSONEq(t, `{"caller":{"type":"direct"}}`, string(parts[0].ProviderOptions["anthropic"].(provider.RawProviderOption).Raw))
				assert.JSONEq(t, `{"ok":true}`, string(parts[1].Output.JSON))
				assert.Equal(t, parts[0].ProviderOptions, parts[1].ProviderOptions)
			}
		})
	}
	t.Run("reserved namespace precedes MCP metadata", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		body := `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"echo","input":{},"providerExecuted":true,"providerOptions":{"gateway":{},"anthropic":{"type":"mcp-tool-use","serverName":"echo"}}}]}]}`
		response := harness.serve(validRequest(body))
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.JSONEq(t, string(reservedProviderOptionsError), response.Body.String())
		assert.Zero(t, harness.resolver.callCount())
		assert.Zero(t, harness.model.callCount())
	})
}

func TestUnresolvedProviderCalls_RejectsAmbiguousHistory(t *testing.T) {
	for _, prompt := range [][]provider.Message{
		{provider.NewAssistantMessage(provider.ToolCallPart("call", "echo", json.RawMessage(`{}`)), provider.ToolCallPart("call", "echo", json.RawMessage(`{}`)))},
		{provider.NewAssistantMessage(provider.ToolCallPart("call", "echo", json.RawMessage(`{}`)), provider.ToolResultPart("call", "other", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "bad"}))},
	} {
		_, failure := unresolvedProviderCalls(prompt)
		require.NotNil(t, failure)
	}
}

func TestRuntimeFunctionTools_Boundaries(t *testing.T) {
	for _, body := range []string{
		`{"prompt":[],"tools":[{"type":"function","name":"f","inputSchema":{},"args":{}}]}`,
		`{"prompt":[],"tools":[{"type":"function","name":"f","inputSchema":{},"providerOptions":{"gateway":{}}}]}`,
		`{"prompt":[],"tools":[{"type":"function","name":"f","inputSchema":{},"providerOptions":{"grafana":{"secret":"private"}}}]}`,
		`{"prompt":[{"role":"user","content":[{"type":"tool-call","toolCallId":"a","toolName":"f","input":{}}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"a","toolName":"f","output":{"type":"text"}}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"a","toolName":"f","output":{"type":"text","value":"","providerOptions":{"anthropic":{"private":true}}}}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"a","toolName":"f","output":{"type":"execution-denied","reason":""}}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"a","toolName":"f","output":{"type":"content","value":[{"type":"text","text":"","providerOptions":{"p":{"x":1}}}]}}]}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			response := harness.serve(validRequest(body))
			require.Equal(t, http.StatusBadRequest, response.Code)
			assert.Zero(t, harness.model.callCount())
			assert.NotContains(t, response.Body.String(), "private")
		})
	}
	for _, body := range []string{
		`{"prompt":[],"tools":[{"type":"function","name":"f","inputSchema":{}}]}`,
		`{"prompt":[],"toolChoice":{"type":"none"}}`,
		`{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"a","toolName":"f","input":{}}]}]}`,
		`{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"a","toolName":"f","input":{},"providerExecuted":true}]}]}`,
	} {
		harness := newRuntimeHarness(t, testLimits())
		request := validRequest(body)
		request.Header.Set(HeaderStreaming, "true")
		response := harness.serve(request)
		require.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, 1, harness.model.callCount())
	}
}

func TestRuntimeFunctionTools_SelectedEmptyArms(t *testing.T) {
	for _, output := range []string{
		`{"type":"text","value":""}`, `{"type":"error-text","value":""}`,
		`{"type":"json","value":null}`, `{"type":"error-json","value":null}`,
		`{"type":"content","value":[]}`, `{"type":"content","value":[{"type":"text","text":""}]}`,
	} {
		harness := newRuntimeHarness(t, testLimits())
		body := `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"a","toolName":"f","output":` + output + `}]}]}`
		response := harness.serve(validRequest(body))
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		mapped, err := json.Marshal(harness.model.receivedOptions().Prompt[0].Content[0].Output)
		require.NoError(t, err)
		assert.JSONEq(t, output, string(mapped))
	}
}

func TestRuntimeUnaryFunctionOutput_SupportsExecutionMarkers(t *testing.T) {
	for _, marker := range []string{"providerExecuted", "dynamic", "preliminary"} {
		harness := newRuntimeHarness(t, testLimits())
		harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			result := validGenerateResult()
			part := provider.GenerateContentPart{Type: provider.ContentToolCall, ToolCallID: "private-call", ToolName: "private-tool", Input: json.RawMessage("{}")}
			yes := true
			switch marker {
			case "providerExecuted":
				part.ProviderExecuted = true
			case "dynamic":
				part.Dynamic = &yes
			case "preliminary":
				part.Preliminary = &yes
			}
			result.Content = append(result.Content, part)
			return result, nil
		}
		response := harness.serve(validRequest(`{"prompt":[]}`))
		if marker == "preliminary" {
			assert.Equal(t, http.StatusInternalServerError, response.Code)
			assert.Equal(t, string(canonicalInternalError), response.Body.String())
		} else {
			assert.Equal(t, http.StatusOK, response.Code)
			assert.Contains(t, response.Body.String(), `"`+marker+`":true`)
		}
	}
}

func TestUnaryFunctionOutput_CompleteBounds(t *testing.T) {
	result := validGenerateResult()
	result.Content = []provider.GenerateContentPart{{Type: provider.ContentToolCall, ToolCallID: "a", ToolName: "f", Input: json.RawMessage(strings.Repeat("\x00", 32))}}
	mapped, err := mapUnarySuccess(result, 1<<20)
	require.NoError(t, err)
	body, ok := encodeUnarySuccess(mapped, 1<<20)
	require.True(t, ok)
	_, ok = encodeUnarySuccess(mapped, int64(len(body)))
	assert.True(t, ok)
	_, ok = encodeUnarySuccess(mapped, int64(len(body)-1))
	assert.False(t, ok)
	for _, field := range []string{"id", "name", "input"} {
		copyResult := *result
		copyResult.Content = append([]provider.GenerateContentPart(nil), result.Content...)
		switch field {
		case "id":
			copyResult.Content[0].ToolCallID = strings.Repeat("x", 2048)
		case "name":
			copyResult.Content[0].ToolName = strings.Repeat("x", 2048)
		case "input":
			copyResult.Content[0].Input = json.RawMessage(strings.Repeat("x", 2048))
		}
		_, err := mapUnarySuccess(&copyResult, 1024)
		assert.Error(t, err)
	}
}

func TestRuntimeUnaryFunctionOutput_InvalidIdentifiersAndUTF8(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*provider.GenerateContentPart)
	}{
		{name: "missing id", mutate: func(part *provider.GenerateContentPart) { part.ToolCallID = "" }},
		{name: "missing name", mutate: func(part *provider.GenerateContentPart) { part.ToolName = "" }},
		{name: "invalid id UTF-8", mutate: func(part *provider.GenerateContentPart) { part.ToolCallID = string([]byte{0xff}) }},
		{name: "invalid name UTF-8", mutate: func(part *provider.GenerateContentPart) { part.ToolName = string([]byte{0xff}) }},
		{name: "invalid input UTF-8", mutate: func(part *provider.GenerateContentPart) { part.Input = json.RawMessage{0xff} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				result := validGenerateResult()
				part := provider.GenerateContentPart{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "weather", Input: json.RawMessage(`{}`)}
				tc.mutate(&part)
				result.Content = append(result.Content, part)
				return result, nil
			}
			response := harness.serve(validRequest(`{"prompt":[]}`))
			assert.Equal(t, http.StatusInternalServerError, response.Code)
			assert.Equal(t, string(canonicalInternalError), response.Body.String())
		})
	}
}

func TestRuntimeUnaryFunctionOutput_OpaqueArguments(t *testing.T) {
	for _, input := range []string{"", `{`, `{"city":"Rio"}`, "\"\\\n☃<>&"} {
		t.Run(input, func(t *testing.T) {
			for _, disabled := range []*bool{nil, new(false)} {
				harness := newRuntimeHarness(t, testLimits())
				harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					result := validGenerateResult()
					result.Content = []provider.GenerateContentPart{{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "weather", Input: json.RawMessage(input), Dynamic: disabled, ProviderMetadata: provider.ProviderMetadata{"private": json.RawMessage(`{"secret":"private-sentinel"}`)}}}
					return result, nil
				}
				response := harness.serve(validRequest(`{"prompt":[]}`))
				require.Equal(t, http.StatusOK, response.Code)
				var decoded struct {
					Content []struct {
						Input   string `json:"input"`
						Dynamic *bool  `json:"dynamic"`
					} `json:"content"`
				}
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &decoded))
				require.Len(t, decoded.Content, 1)
				assert.Equal(t, input, decoded.Content[0].Input)
				assert.Contains(t, response.Body.String(), `"providerMetadata":{"private":{"secret":"private-sentinel"}}`)
				assert.Equal(t, disabled, decoded.Content[0].Dynamic)
			}
		})
	}
}
