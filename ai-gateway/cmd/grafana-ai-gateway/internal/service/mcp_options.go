package service

import (
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
)

const (
	maxMCPServers      = 16
	maxMCPNameBytes    = 128
	maxMCPURLBytes     = 4096
	maxMCPTokenBytes   = 8192
	maxMCPAllowedTools = 128
)

func validateMCPServers(servers []anthropicprovider.MCPServer) (map[string]bool, error) {
	if len(servers) > maxMCPServers {
		return nil, catalog.ErrUnsupportedRequest
	}
	names := make(map[string]bool, len(servers))
	for _, server := range servers {
		if server.Name == "" || len(server.Name) > maxMCPNameBytes || !utf8.ValidString(server.Name) || names[server.Name] || len(server.URL) > maxMCPURLBytes {
			return nil, catalog.ErrUnsupportedRequest
		}
		destination, err := url.Parse(server.URL)
		if err != nil || destination.Scheme != "https" || destination.Hostname() == "" || destination.User != nil || strings.Contains(server.URL, "#") {
			return nil, catalog.ErrUnsupportedRequest
		}
		if server.AuthorizationToken != nil && (len(*server.AuthorizationToken) > maxMCPTokenBytes || !utf8.ValidString(*server.AuthorizationToken)) {
			return nil, catalog.ErrUnsupportedRequest
		}
		if server.ToolConfiguration != nil {
			if len(server.ToolConfiguration.AllowedTools) > maxMCPAllowedTools {
				return nil, catalog.ErrUnsupportedRequest
			}
			for _, tool := range server.ToolConfiguration.AllowedTools {
				if tool == "" || len(tool) > maxMCPNameBytes || !utf8.ValidString(tool) {
					return nil, catalog.ErrUnsupportedRequest
				}
			}
		}
		names[server.Name] = true
	}
	return names, nil
}
