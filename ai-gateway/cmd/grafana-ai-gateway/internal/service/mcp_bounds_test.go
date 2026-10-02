package service

import (
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateMCPServers_Bounds(t *testing.T) {
	server := anthropicprovider.MCPServer{Name: "echo", URL: "https://mcp.example.test"}
	for _, tc := range []struct {
		name   string
		change func(*anthropicprovider.MCPServer)
	}{
		{"name", func(server *anthropicprovider.MCPServer) { server.Name = strings.Repeat("x", maxMCPNameBytes+1) }},
		{"url", func(server *anthropicprovider.MCPServer) { server.URL += strings.Repeat("x", maxMCPURLBytes) }},
		{"token", func(server *anthropicprovider.MCPServer) {
			token := strings.Repeat("x", maxMCPTokenBytes+1)
			server.AuthorizationToken = &token
		}},
		{"tool count", func(server *anthropicprovider.MCPServer) {
			server.ToolConfiguration = &anthropicprovider.MCPToolConfiguration{AllowedTools: make([]string, maxMCPAllowedTools+1)}
		}},
		{"tool name", func(server *anthropicprovider.MCPServer) {
			server.ToolConfiguration = &anthropicprovider.MCPToolConfiguration{AllowedTools: []string{strings.Repeat("x", maxMCPNameBytes+1)}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := server
			tc.change(&value)
			names, err := validateMCPServers([]anthropicprovider.MCPServer{value})
			assert.Nil(t, names)
			require.ErrorIs(t, err, catalog.ErrUnsupportedRequest)
		})
	}
	t.Run("server count", func(t *testing.T) {
		_, err := validateMCPServers(make([]anthropicprovider.MCPServer, maxMCPServers+1))
		require.ErrorIs(t, err, catalog.ErrUnsupportedRequest)
	})
	t.Run("inclusive boundaries", func(t *testing.T) {
		server.Name = strings.Repeat("x", maxMCPNameBytes)
		token := strings.Repeat("x", maxMCPTokenBytes)
		server.AuthorizationToken = &token
		names, err := validateMCPServers([]anthropicprovider.MCPServer{server})
		require.NoError(t, err)
		assert.True(t, names[server.Name])
	})
}
