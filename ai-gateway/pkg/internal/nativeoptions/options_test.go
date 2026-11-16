package nativeoptions

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAnthropicMCPServers_Bounds(t *testing.T) {
	server := anthropicprovider.MCPServer{Name: "echo", URL: "https://mcp.example.test"}
	for _, tc := range []struct {
		name   string
		change func(*anthropicprovider.MCPServer)
	}{
		{"name", func(server *anthropicprovider.MCPServer) { server.Name = strings.Repeat("x", maxMCPNameBytes+1) }},
		{"empty name", func(server *anthropicprovider.MCPServer) { server.Name = "" }},
		{"invalid UTF-8 name", func(server *anthropicprovider.MCPServer) { server.Name = "\xff" }},
		{"url", func(server *anthropicprovider.MCPServer) { server.URL += strings.Repeat("x", maxMCPURLBytes) }},
		{"token", func(server *anthropicprovider.MCPServer) {
			token := strings.Repeat("x", maxMCPTokenBytes+1)
			server.AuthorizationToken = &token
		}},
		{"invalid UTF-8 token", func(server *anthropicprovider.MCPServer) {
			token := "\xff"
			server.AuthorizationToken = &token
		}},
		{"tool count", func(server *anthropicprovider.MCPServer) {
			server.ToolConfiguration = &anthropicprovider.MCPToolConfiguration{AllowedTools: make([]string, maxMCPAllowedTools+1)}
		}},
		{"tool name", func(server *anthropicprovider.MCPServer) {
			server.ToolConfiguration = &anthropicprovider.MCPToolConfiguration{AllowedTools: []string{strings.Repeat("x", maxMCPNameBytes+1)}}
		}},
		{"empty tool name", func(server *anthropicprovider.MCPServer) {
			server.ToolConfiguration = &anthropicprovider.MCPToolConfiguration{AllowedTools: []string{""}}
		}},
		{"invalid UTF-8 tool name", func(server *anthropicprovider.MCPServer) {
			server.ToolConfiguration = &anthropicprovider.MCPToolConfiguration{AllowedTools: []string{"\xff"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := server
			tc.change(&value)
			names, err := validateAnthropicMCPServers([]anthropicprovider.MCPServer{value})
			assert.Nil(t, names)
			require.ErrorIs(t, err, catalog.ErrUnsupportedRequest)
		})
	}
	t.Run("server count", func(t *testing.T) {
		_, err := validateAnthropicMCPServers(make([]anthropicprovider.MCPServer, maxMCPServers+1))
		require.ErrorIs(t, err, catalog.ErrUnsupportedRequest)
	})
	t.Run("server count boundary", func(t *testing.T) {
		servers := make([]anthropicprovider.MCPServer, maxMCPServers)
		for i := range servers {
			servers[i] = anthropicprovider.MCPServer{Name: fmt.Sprintf("server-%d", i), URL: server.URL}
		}
		names, err := validateAnthropicMCPServers(servers)
		require.NoError(t, err)
		assert.Len(t, names, maxMCPServers)
	})
	t.Run("inclusive boundaries", func(t *testing.T) {
		server.Name = strings.Repeat("x", maxMCPNameBytes)
		token := strings.Repeat("x", maxMCPTokenBytes)
		server.AuthorizationToken = &token
		prefix := "https://mcp.example.test/"
		server.URL = prefix + strings.Repeat("x", maxMCPURLBytes-len(prefix))
		tools := make([]string, maxMCPAllowedTools)
		for i := range tools {
			tools[i] = strings.Repeat("x", maxMCPNameBytes)
		}
		server.ToolConfiguration = &anthropicprovider.MCPToolConfiguration{AllowedTools: tools}
		names, err := validateAnthropicMCPServers([]anthropicprovider.MCPServer{server})
		require.NoError(t, err)
		assert.Contains(t, names, server.Name)
	})
}

func TestAnthropic_ServerFallbackPolicy(t *testing.T) {
	const unconfiguredMCPHistory = `[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"lookup","input":{},"providerExecuted":true,"providerOptions":{"anthropic":{"type":"mcp-tool-use","serverName":"external"}}}]}]`
	call := func(anthropic, prompt string) provider.CallOptions {
		options := provider.CallOptions{}
		if anthropic != "" {
			options.ProviderOptions = provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(anthropic)}}
		}
		if prompt != "" {
			require.NoError(t, json.Unmarshal([]byte(prompt), &options.Prompt))
		}
		return options
	}
	configured := NewAnthropicFallbackPolicy([]string{"claude-b", "claude-c"}, false)
	withDefault := NewAnthropicFallbackPolicy([]string{"claude-b"}, true)
	for _, tc := range []struct {
		name    string
		policy  AnthropicFallbackPolicy
		options string
		prompt  string
		allowed bool
	}{
		{"allowed chain", configured, `{"fallbacks":[{"model":"claude-b","max_tokens":2048},{"model":"claude-c"}]}`, "", true},
		{"allowed default", withDefault, `{"fallbacks":"default"}`, "", true},
		{"disallowed model", configured, `{"fallbacks":[{"model":"claude-b"},{"model":"claude-z"}]}`, "", false},
		{"repeated entry", configured, `{"fallbacks":[{"model":"claude-b"},{"model":"claude-b","thinking":{"type":"disabled"}}]}`, "", false},
		{"default without allowDefault", configured, `{"fallbacks":"default"}`, "", false},
		{"zero policy chain", AnthropicFallbackPolicy{}, `{"fallbacks":[{"model":"claude-b"}]}`, "", false},
		{"zero policy default", AnthropicFallbackPolicy{}, `{"fallbacks":"default"}`, "", false},
		{"case variant", configured, `{"fallbacks":[{"model":"Claude-B"}]}`, "", false},
		{"whitespace variant", configured, `{"fallbacks":[{"model":" claude-b"}]}`, "", false},
		{"absent options", AnthropicFallbackPolicy{}, ``, "", true},
		{"null fallbacks", AnthropicFallbackPolicy{}, `{"fallbacks":null}`, "", true},
		{"empty chain", AnthropicFallbackPolicy{}, `{"fallbacks":[]}`, "", true},
		{"inactive fallbacks still check MCP history", AnthropicFallbackPolicy{}, `{"fallbacks":[]}`, unconfiguredMCPHistory, false},
		{"allowed chain still checks MCP configuration", configured, `{"fallbacks":[{"model":"claude-b"}],"mcpServers":[{"type":"url","name":"n","url":"http://insecure.example.test"}]}`, "", false},
		{"allowed chain with valid MCP", configured, `{"fallbacks":[{"model":"claude-b"}],"mcpServers":[{"type":"url","name":"n","url":"https://mcp.example.test"}]}`, "", true},
		{"allowed chain still refuses skills", configured, `{"fallbacks":[{"model":"claude-b"}],"container":{"skills":[{"type":"custom","skillId":"s"}]}}`, "", false},
		{"unrestricted chain", UnrestrictedAnthropicFallbacks(), `{"fallbacks":[{"model":"claude-any"},{"model":"claude-any","speed":"fast"}]}`, "", true},
		{"unrestricted default", UnrestrictedAnthropicFallbacks(), `{"fallbacks":"default"}`, "", true},
		{"unrestricted still refuses skills", UnrestrictedAnthropicFallbacks(), `{"fallbacks":"default","container":{"skills":[{"type":"custom","skillId":"s"}]}}`, "", false},
		{"unrestricted still checks MCP configuration", UnrestrictedAnthropicFallbacks(), `{"fallbacks":"default","mcpServers":[{"type":"url","name":"n","url":"http://insecure.example.test"}]}`, "", false},
		{"unrestricted still checks MCP history", UnrestrictedAnthropicFallbacks(), `{"fallbacks":"default"}`, unconfiguredMCPHistory, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := Anthropic(AnthropicPolicy{Fallbacks: tc.policy})(call(tc.options, tc.prompt))
			if tc.allowed {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, catalog.ErrUnsupportedRequest)
		})
	}
	t.Run("policy copies its models", func(t *testing.T) {
		models := []string{"claude-b"}
		policy := NewAnthropicFallbackPolicy(models, false)
		models[0] = "claude-z"
		assert.NoError(t, Anthropic(AnthropicPolicy{Fallbacks: policy})(call(`{"fallbacks":[{"model":"claude-b"}]}`, "")))
		assert.ErrorIs(t, Anthropic(AnthropicPolicy{Fallbacks: policy})(call(`{"fallbacks":[{"model":"claude-z"}]}`, "")), catalog.ErrUnsupportedRequest)
	})
}
