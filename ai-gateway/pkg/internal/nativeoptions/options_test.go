package nativeoptions

import (
	"fmt"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
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
