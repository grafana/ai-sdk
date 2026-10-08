package v4

import (
	"encoding/json"
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

func requestProtectedSources(options provider.CallOptions) []string {
	var sources []string
	if raw, ok := options.ProviderOptions["anthropic"].(provider.RawProviderOption); ok {
		var fields struct {
			MCPServers []struct {
				AuthorizationToken string `json:"authorizationToken"`
			} `json:"mcpServers"`
		}
		if json.Unmarshal(raw.Raw, &fields) == nil {
			for _, server := range fields.MCPServers {
				if server.AuthorizationToken != "" {
					sources = append(sources, server.AuthorizationToken)
				}
			}
		}
	}
	for _, tool := range options.Tools {
		if tool.Type != provider.ToolTypeProvider || tool.ID != "openai.mcp" {
			continue
		}
		var authorization string
		if json.Unmarshal(tool.Args["authorization"], &authorization) == nil && authorization != "" {
			sources = append(sources, authorization)
		}
		var fields map[string]string
		if json.Unmarshal(tool.Args["headers"], &fields) == nil {
			headers := make(http.Header, len(fields))
			for name, value := range fields {
				headers.Set(name, value)
			}
			sources = append(sources, execution.HeaderSources(headers)...)
		}
	}
	return sources
}
