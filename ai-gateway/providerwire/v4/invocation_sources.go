package v4

import (
	"encoding/json"
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

func requestProtectedSources(options provider.CallOptions) []string {
	headers := make(http.Header, len(options.Headers))
	for name, value := range options.Headers {
		headers.Add(name, value)
	}
	sources := execution.HeaderSources(headers)
	if raw, ok := options.ProviderOptions["anthropic"].(provider.RawProviderOption); ok {
		var fields struct {
			MCPServers []struct {
				URL                string `json:"url"`
				AuthorizationToken string `json:"authorizationToken"`
			} `json:"mcpServers"`
		}
		if json.Unmarshal(raw.Raw, &fields) == nil {
			for _, server := range fields.MCPServers {
				sources = append(sources, execution.URLSources(server.URL)...)
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
		var destination string
		if json.Unmarshal(tool.Args["serverUrl"], &destination) == nil {
			sources = append(sources, execution.URLSources(destination)...)
		}
		var authorization string
		if json.Unmarshal(tool.Args["authorization"], &authorization) == nil && authorization != "" {
			sources = append(sources, authorization)
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(tool.Args["headers"], &fields) == nil {
			headers := make(http.Header, len(fields))
			for name, raw := range fields {
				var value string
				if json.Unmarshal(raw, &value) == nil {
					headers.Add(name, value)
				}
			}
			sources = append(sources, execution.HeaderSources(headers)...)
		}
	}
	return sources
}
