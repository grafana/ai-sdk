package execution

import (
	"encoding/json"
	"net/http"

	"github.com/grafana/ai-sdk/provider"
)

// RequestSources extracts known credential values from mapped call options.
func RequestSources(options provider.CallOptions) []string {
	headers := make(http.Header, len(options.Headers))
	for name, value := range options.Headers {
		headers.Add(name, value)
	}
	sources := HeaderSources(headers)
	sources = append(sources, anthropicMCPSources(options.ProviderOptions)...)
	for _, tool := range options.Tools {
		if tool.Type == provider.ToolTypeProvider && tool.ID == "openai.mcp" {
			sources = append(sources, openaiMCPSources(tool.Args)...)
		}
	}
	return sources
}

func anthropicMCPSources(options provider.ProviderOptions) []string {
	raw, ok := options["anthropic"].(provider.RawProviderOption)
	if !ok {
		return nil
	}
	var fields struct {
		MCPServers []struct {
			URL                string `json:"url"`
			AuthorizationToken string `json:"authorizationToken"`
		} `json:"mcpServers"`
	}
	if json.Unmarshal(raw.Raw, &fields) != nil {
		return nil
	}
	var sources []string
	for _, server := range fields.MCPServers {
		sources = append(sources, URLSources(server.URL)...)
		if server.AuthorizationToken != "" {
			sources = append(sources, server.AuthorizationToken)
		}
	}
	return sources
}

func openaiMCPSources(args map[string]json.RawMessage) []string {
	var sources []string
	var destination string
	if json.Unmarshal(args["serverUrl"], &destination) == nil {
		sources = append(sources, URLSources(destination)...)
	}
	var authorization string
	if json.Unmarshal(args["authorization"], &authorization) == nil && authorization != "" {
		sources = append(sources, authorization)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(args["headers"], &fields) == nil {
		headers := make(http.Header, len(fields))
		for name, raw := range fields {
			var value string
			if json.Unmarshal(raw, &value) == nil {
				headers.Add(name, value)
			}
		}
		sources = append(sources, HeaderSources(headers)...)
	}
	return sources
}
