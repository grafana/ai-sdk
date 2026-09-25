package v4

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

const (
	maxMCPServers      = 16
	maxMCPNameBytes    = 128
	maxMCPURLBytes     = 2048
	maxMCPTokenBytes   = 4096
	maxMCPAllowedTools = 128
)

type mcpServerType string

const mcpServerURL mcpServerType = "url"

type wireMCPServer struct {
	Type               mcpServerType `json:"type"`
	Name               string        `json:"name"`
	URL                string        `json:"url"`
	AuthorizationToken *string       `json:"authorizationToken"`
	ToolConfiguration  *struct {
		Enabled      *bool    `json:"enabled"`
		AllowedTools []string `json:"allowedTools"`
	} `json:"toolConfiguration"`
}

func validateMCPOptions(values map[string]json.RawMessage) *requestFailure {
	raw, exists := values["anthropic"]
	if !exists {
		return nil
	}
	members, valid := jsonObject(raw)
	if !valid {
		return invalidMappingFailure()
	}
	servers, exists := members["mcpServers"]
	if !exists {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(servers), []byte("null")) {
		return invalidMappingFailure()
	}
	var entries []json.RawMessage
	if json.Unmarshal(servers, &entries) != nil || len(entries) > maxMCPServers {
		return invalidMappingFailure()
	}
	if len(entries) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(entries))
	for _, raw := range entries {
		var members map[string]json.RawMessage
		if json.Unmarshal(raw, &members) != nil || members == nil || !allowedMCPMembers(members, "type", "name", "url", "authorizationToken", "toolConfiguration") {
			return invalidMappingFailure()
		}
		var entry wireMCPServer
		if json.Unmarshal(raw, &entry) != nil || entry.Type != mcpServerURL || entry.Name == "" || len(entry.Name) > maxMCPNameBytes || seen[entry.Name] || len(entry.URL) > maxMCPURLBytes {
			return invalidMappingFailure()
		}
		seen[entry.Name] = true
		endpoint, err := url.Parse(entry.URL)
		if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.User != nil || strings.Contains(entry.URL, "#") {
			return invalidMappingFailure()
		}
		if entry.AuthorizationToken != nil && len(*entry.AuthorizationToken) > maxMCPTokenBytes {
			return invalidMappingFailure()
		}
		if config, ok := members["toolConfiguration"]; ok && !bytes.Equal(bytes.TrimSpace(config), []byte("null")) {
			var fields map[string]json.RawMessage
			if json.Unmarshal(config, &fields) != nil || fields == nil || !allowedMCPMembers(fields, "enabled", "allowedTools") {
				return invalidMappingFailure()
			}
			if entry.ToolConfiguration != nil {
				if len(entry.ToolConfiguration.AllowedTools) > maxMCPAllowedTools {
					return invalidMappingFailure()
				}
				if allowed, exists := fields["allowedTools"]; exists && !bytes.Equal(bytes.TrimSpace(allowed), []byte("null")) {
					var names []json.RawMessage
					if json.Unmarshal(allowed, &names) != nil {
						return invalidMappingFailure()
					}
					for _, name := range names {
						var text string
						if len(name) == 0 || name[0] != '"' || json.Unmarshal(name, &text) != nil || len(text) > maxMCPNameBytes {
							return invalidMappingFailure()
						}
					}
				}
			}
		}
	}
	return nil
}

func configuredMCPNames(options provider.ProviderOptions) map[string]bool {
	value, ok := options["anthropic"].(provider.RawProviderOption)
	if !ok {
		return nil
	}
	var root struct {
		MCPServers []wireMCPServer `json:"mcpServers"`
	}
	if json.Unmarshal(value.Raw, &root) != nil {
		return nil
	}
	names := make(map[string]bool, len(root.MCPServers))
	for _, server := range root.MCPServers {
		names[server.Name] = true
	}
	return names
}

func allowedMCPMembers(values map[string]json.RawMessage, names ...string) bool {
	allowed := make(map[string]bool, len(names))
	for _, name := range names {
		allowed[name] = true
	}
	for name := range values {
		if !allowed[name] {
			return false
		}
	}
	return true
}
