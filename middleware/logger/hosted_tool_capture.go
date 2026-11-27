package logger

import (
	"encoding/json"
	"maps"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

func toolArgsForLog(tool provider.Tool) map[string]json.RawMessage {
	switch tool.ID {
	case "openai.mcp":
		if _, tunneled := tool.Args["tunnelId"]; tunneled {
			args := maps.Clone(tool.Args)
			delete(args, "authorization")
			delete(args, "headers")
			return args
		}
	case "openai.code_interpreter":
		var container map[string]json.RawMessage
		if json.Unmarshal(tool.Args["container"], &container) == nil && omitDomainSecrets(container) {
			args := maps.Clone(tool.Args)
			args["container"], _ = json.Marshal(container)
			return args
		}
	}
	return tool.Args
}

func omitDomainSecrets(container map[string]json.RawMessage) bool {
	var changed bool
	for key, raw := range container {
		if !strings.EqualFold(key, "networkPolicy") {
			continue
		}
		var policy map[string]json.RawMessage
		if json.Unmarshal(raw, &policy) != nil {
			continue
		}
		for field := range policy {
			if strings.EqualFold(field, "domainSecrets") {
				delete(policy, field)
				container[key], _ = json.Marshal(policy)
				changed = true
			}
		}
	}
	return changed
}
