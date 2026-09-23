package v4

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeProviderToolHistory(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		pending    map[string]string
		status     int
	}{
		{name: "unresolved hosted call", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"deferred","toolName":"code_execution","input":{"code":"print(1)"},"providerExecuted":true}]}]}`, pending: map[string]string{"deferred": "code_execution"}, status: http.StatusOK},
		{name: "completed hosted call", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"code_execution","input":{},"providerExecuted":true},{"type":"tool-result","toolCallId":"call","toolName":"code_execution","output":{"type":"json","value":{"ok":true}}}]}]}`, pending: map[string]string{}, status: http.StatusOK},
		{name: "client call never eligible", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"local","toolName":"f","input":{}}]}]}`, pending: map[string]string{}, status: http.StatusOK},
		{name: "MCP continuation", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"mcp","toolName":"echo","input":{},"providerExecuted":true,"providerOptions":{"anthropic":{"type":"mcp-tool-use","serverName":"echo"}}},{"type":"tool-result","toolCallId":"mcp","toolName":"echo","output":{"type":"json","value":{"ok":true}},"providerOptions":{"anthropic":{"type":"mcp-tool-use","serverName":"echo"}}}]}],"providerOptions":{"anthropic":{"mcpServers":[{"type":"url","name":"echo","url":"https://mcp.example.test"}]}}}`, pending: map[string]string{}, status: http.StatusOK},
		{name: "unconfigured MCP name", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"mcp","toolName":"echo","input":{},"providerExecuted":true,"providerOptions":{"anthropic":{"type":"mcp-tool-use","serverName":"other"}}}]}],"providerOptions":{"anthropic":{"mcpServers":[{"type":"url","name":"echo","url":"https://mcp.example.test"}]}}}`, status: http.StatusBadRequest},
		{name: "MCP marker on local call", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"mcp","toolName":"echo","input":{},"providerOptions":{"anthropic":{"type":"mcp-tool-use","serverName":"echo"}}}]}],"providerOptions":{"anthropic":{"mcpServers":[{"type":"url","name":"echo","url":"https://mcp.example.test"}]}}}`, status: http.StatusBadRequest},
		{name: "duplicate historical call", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"f","input":{}},{"type":"tool-call","toolCallId":"call","toolName":"f","input":{}}]}]}`, status: http.StatusBadRequest},
		{name: "mismatched history result", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"f","input":{},"providerExecuted":true},{"type":"tool-result","toolCallId":"call","toolName":"other","output":{"type":"text","value":"result"}}]}]}`, status: http.StatusBadRequest},
		{name: "reserved part options", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"f","input":{},"providerOptions":{"gateway":{}}}]}]}`, status: http.StatusBadRequest},
		{name: "nested output options deferred", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-result","toolCallId":"call","toolName":"f","output":{"type":"json","value":false,"providerOptions":{"anthropic":{"private":true}}}}]}]}`, status: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			response := harness.serve(validRequest(tc.body))
			assert.Equal(t, tc.status, response.Code, response.Body.String())
			if tc.status != http.StatusOK {
				assert.Zero(t, harness.model.callCount())
				return
			}
			opts := harness.model.receivedOptions()
			pending, failure := unresolvedProviderCalls(opts.Prompt)
			require.Nil(t, failure)
			assert.Equal(t, tc.pending, pending)
			if tc.name == "MCP continuation" {
				parts := opts.Prompt[0].Content
				assert.True(t, parts[0].ProviderExecuted)
				assert.JSONEq(t, `{"type":"mcp-tool-use","serverName":"echo"}`, string(parts[0].ProviderOptions["anthropic"].(provider.RawProviderOption).Raw))
				assert.JSONEq(t, `{"ok":true}`, string(parts[1].Output.JSON))
				assert.Equal(t, parts[0].ProviderOptions, parts[1].ProviderOptions)
			}
		})
	}
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
