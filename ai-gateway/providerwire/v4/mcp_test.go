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

const mcpRequestOptions = `"providerOptions":{"anthropic":{"mcpServers":[{"type":"url","name":"echo","url":"https://mcp.example.test","authorizationToken":"private-token"}]}}`

func TestRuntimeProviderToolDefinitionsAndMCPOptions(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("streaming=%t", streaming), func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			body := `{"prompt":[],"tools":[{"type":"function","name":"f","inputSchema":{}},{"type":"provider","id":"anthropic.code_execution_20260120","name":"code","args":{}},{"type":"provider","id":"provider.search","name":"search","args":{"limit":0,"nested":{"value":null}}}],"providerOptions":{"anthropic":{"mcpServers":[{"type":"url","name":"echo","url":"https://mcp.example.test/tools","authorizationToken":"private-token","toolConfiguration":{"enabled":false,"allowedTools":[]}}]}}}`
			request := validRequest(body)
			if streaming {
				request.Header.Set(HeaderStreaming, "true")
			}
			response := harness.serve(request)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.Equal(t, 1, harness.model.callCount())
			opts := harness.model.receivedOptions()
			require.Len(t, opts.Tools, 3)
			assert.Equal(t, provider.ToolTypeFunction, opts.Tools[0].Type)
			assert.Equal(t, provider.ToolTypeProvider, opts.Tools[1].Type)
			assert.Equal(t, "anthropic.code_execution_20260120", opts.Tools[1].ID)
			assert.NotNil(t, opts.Tools[1].Args)
			assert.JSONEq(t, `0`, string(opts.Tools[2].Args["limit"]))
			assert.JSONEq(t, `{"value":null}`, string(opts.Tools[2].Args["nested"]))
			mcp, ok := opts.ProviderOptions["anthropic"].(provider.RawProviderOption)
			require.True(t, ok)
			assert.JSONEq(t, `{"mcpServers":[{"type":"url","name":"echo","url":"https://mcp.example.test/tools","authorizationToken":"private-token","toolConfiguration":{"enabled":false,"allowedTools":[]}}]}`, string(mcp.Raw))
			assert.NotContains(t, response.Body.String(), "private-token")
		})
	}
	for _, tc := range []struct{ name, body string }{
		{"missing args", `{"prompt":[],"tools":[{"type":"provider","id":"provider.search","name":"search"}]}`},
		{"null args", `{"prompt":[],"tools":[{"type":"provider","id":"provider.search","name":"search","args":null}]}`},
		{"function-only field", `{"prompt":[],"tools":[{"type":"provider","id":"provider.search","name":"search","args":{},"strict":false}]}`},
		{"missing server type", `{"prompt":[],"providerOptions":{"anthropic":{"mcpServers":[{"name":"echo","url":"https://mcp.example.test"}]}}}`},
		{"untrusted scheme", `{"prompt":[],"providerOptions":{"anthropic":{"mcpServers":[{"type":"url","name":"echo","url":"http://mcp.example.test"}]}}}`},
		{"URL credentials", `{"prompt":[],"providerOptions":{"anthropic":{"mcpServers":[{"type":"url","name":"echo","url":"https://secret:password@mcp.example.test"}]}}}`},
		{"empty URL fragment", `{"prompt":[],"providerOptions":{"anthropic":{"mcpServers":[{"type":"url","name":"echo","url":"https://mcp.example.test/#"}]}}}`},
		{"duplicate server", `{"prompt":[],"providerOptions":{"anthropic":{"mcpServers":[{"type":"url","name":"echo","url":"https://mcp.example.test"},{"type":"url","name":"echo","url":"https://mcp2.example.test"}]}}}`},
		{"extra root options", `{"prompt":[],"providerOptions":{"anthropic":{"mcpServers":[],"thinking":{"type":"enabled"}}}}`},
		{"nested typed null", `{"prompt":[],"providerOptions":{"anthropic":{"mcpServers":[{"type":"url","name":"echo","url":"https://mcp.example.test","toolConfiguration":{"allowedTools":[null]}}]}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, streaming := range []bool{false, true} {
				harness := newRuntimeHarness(t, testLimits())
				request := validRequest(tc.body)
				if streaming {
					request.Header.Set(HeaderStreaming, "true")
				}
				response := harness.serve(request)
				assert.Equal(t, http.StatusBadRequest, response.Code)
				assert.Zero(t, harness.model.callCount())
				assert.NotContains(t, response.Body.String(), "secret")
			}
		})
	}
}

func TestRuntimeUnaryMCP_MetadataAndHistory(t *testing.T) {
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

func TestStreamingProviderTools_DeferredMCPResultOnly(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(
			provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`"failed"`), IsError: true, ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"type":"mcp-tool-use","serverName":"echo","private":"discard"}`)}},
			finishPart(),
		)}, nil
	}
	body := `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"echo","input":{},"providerExecuted":true,"providerOptions":{"anthropic":{"type":"mcp-tool-use","serverName":"echo"}}}]}],` + mcpRequestOptions + `}`
	response := harness.serve(streamRequest(body))
	output := response.Body.String()
	assert.Equal(t, 1, strings.Count(output, `"type":"tool-result"`))
	assert.NotContains(t, output, `"type":"tool-call"`)
	assert.Contains(t, output, `"isError":true`)
	assert.Contains(t, output, `"serverName":"echo"`)
	assert.NotContains(t, output, "private-token")
	assert.NotContains(t, output, "discard")
	assert.Equal(t, 1, strings.Count(output, `"type":"finish"`))
	requireStreamBodyMatchesSchema(t, output)
}

func TestMapToolMetadata_MCPProjection(t *testing.T) {
	metadata := provider.ProviderMetadata{
		"anthropic": json.RawMessage(`{"type":"mcp-tool-use","serverName":"echo","caller":{"type":"direct"},"secret":"private"}`),
		"openai":    json.RawMessage(`{"itemId":"item-1","namespace":"tools","caller":{"type":"program","callerId":"parent","private":"hidden"},"backendModel":"private"}`),
		"private":   json.RawMessage(`{"credential":"private"}`),
	}
	mapped, err := mapToolMetadata(metadata, map[string]bool{"echo": true}, 1024)
	require.NoError(t, err)
	require.Len(t, mapped, 2)
	assert.JSONEq(t, `{"type":"mcp-tool-use","serverName":"echo"}`, string(mapped["anthropic"]))
	assert.JSONEq(t, `{"itemId":"item-1","namespace":"tools","caller":{"type":"program","callerId":"parent"}}`, string(mapped["openai"]))
	assert.NotContains(t, string(mapped["openai"]), "private")
}
