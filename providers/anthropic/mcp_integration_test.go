package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_ProviderToolsMCPOptionsReachNativeTransport(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var body map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.JSONEq(t, `[{"type":"code_execution_20260120","name":"code_execution"}]`, string(body["tools"]))
		assert.JSONEq(t, `[{"type":"url","name":"echo","url":"https://mcp.example.test/tools","authorization_token":"mcp-private-token","tool_configuration":{"enabled":false,"allowed_tools":[]}}]`, string(body["mcp_servers"]))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"claude-sonnet-4-6","stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()
	model := New("key", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))
	maxOutputTokens := 64
	_, err := model.DoGenerate(context.Background(), provider.CallOptions{
		MaxOutputTokens: &maxOutputTokens,
		Prompt:          []provider.Message{provider.UserText("Use tools")},
		Tools:           []provider.Tool{{Type: provider.ToolTypeProvider, ID: "anthropic.code_execution_20260120", Name: "python", Args: map[string]json.RawMessage{}}},
		ProviderOptions: provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"mcpServers":[{"type":"url","name":"echo","url":"https://mcp.example.test/tools","authorizationToken":"mcp-private-token","toolConfiguration":{"enabled":false,"allowedTools":[]}}]}`)}},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, requests)
}
