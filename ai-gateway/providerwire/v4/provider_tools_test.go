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

const mcpRequestOptions = `"providerOptions":{"anthropic":{"mcpServers":[{"type":"url","name":"echo","url":"https://mcp.example.test","authorizationToken":"private-token"}]}}`

func TestRuntimeUnaryProviderTools_MetadataAndHistory(t *testing.T) {
	metadata := provider.ProviderMetadata{"anthropic": json.RawMessage(`{"type":"mcp-tool-use","serverName":"echo","backend":"private"}`), "private": json.RawMessage(`{"token":"private-token"}`)}
	call := provider.GenerateContentPart{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "echo", Input: json.RawMessage(`{"message":"hi"}`), ProviderExecuted: true, Dynamic: true, ProviderMetadata: metadata}
	resultPart := provider.GenerateContentPart{Type: provider.ContentToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`{"result":0}`), ProviderExecuted: true, ProviderMetadata: metadata}
	for _, tc := range []struct {
		name, body string
		content    []provider.GenerateContentPart
	}{
		{name: "call and result", body: `{"prompt":[],` + mcpRequestOptions + `}`, content: []provider.GenerateContentPart{call, resultPart}},
		{name: "result only after history", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"echo","input":{"message":"hi"},"providerExecuted":true,"providerOptions":{"anthropic":{"type":"mcp-tool-use","serverName":"echo"}}}]}],` + mcpRequestOptions + `}`, content: []provider.GenerateContentPart{resultPart}},
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
			assert.Contains(t, response.Body.String(), `"serverName":"echo"`)
			for _, marker := range []string{"private-token", "backend", `"private"`} {
				assert.NotContains(t, response.Body.String(), marker)
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

func TestRuntimeUnaryProviderTools_DeferredCompoundOutputFailsBeforeCommit(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		result := validGenerateResult()
		result.Content = []provider.GenerateContentPart{
			{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "search", Input: json.RawMessage(`{}`), ProviderExecuted: true},
			{Type: provider.ContentSource, SourceType: provider.SourceTypeURL, URL: "https://private-source.example"},
		}
		return result, nil
	}
	response := harness.serve(validRequest(`{"prompt":[],"tools":[{"type":"provider","id":"anthropic.web_search_20250305","name":"search","args":{}}]}`))
	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.Equal(t, string(canonicalInternalError), response.Body.String())
	assert.NotContains(t, response.Body.String(), "private-source")
}

func TestUnaryProviderToolResult_BoundsAndPreliminary(t *testing.T) {
	call := provider.GenerateContentPart{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "echo", Input: json.RawMessage(`{}`)}
	result := provider.GenerateContentPart{Type: provider.ContentToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`{"data":"` + strings.Repeat("a", 100) + `"}`)}
	value := validGenerateResult()
	value.Content = []provider.GenerateContentPart{call, result}
	mapped, err := mapUnarySuccess(value, 1<<20)
	require.NoError(t, err)
	body, ok := encodeUnarySuccess(mapped, 1<<20)
	require.True(t, ok)
	_, err = mapUnarySuccess(value, int64(len(result.Result))-1)
	require.Error(t, err)
	_, ok = encodeUnarySuccess(mapped, int64(len(body))-1)
	assert.False(t, ok)
	result.ProviderMetadata = provider.ProviderMetadata{"private": json.RawMessage(`{"secret":"` + strings.Repeat("x", 1000) + `"}`)}
	value.Content = []provider.GenerateContentPart{call, result}
	_, err = mapUnarySuccess(value, 512)
	require.Error(t, err)
	result.ProviderMetadata = nil
	result.Preliminary = true
	value.Content = []provider.GenerateContentPart{call, result}
	_, err = mapUnarySuccess(value, 1<<20)
	require.Error(t, err)
}
