package openai

import (
	"fmt"
	"strings"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/responses"
)

type allowedToolType string

const (
	allowedToolFunction allowedToolType = "function"
	allowedToolCustom   allowedToolType = "custom"
	allowedToolMCP      allowedToolType = "mcp"
)

type allowedToolResolution struct {
	kind        allowedToolType
	name        string
	serverLabel string
	reason      string
	ambiguous   bool
}

type allowedToolRegistry struct {
	direct  map[string]allowedToolResolution
	aliases map[string]allowedToolResolution
}

func (r *allowedToolRegistry) record(name, canonical string, resolution allowedToolResolution) {
	r.direct[name] = resolution
	if canonical == "" || canonical == name {
		return
	}
	if existing, ok := r.aliases[canonical]; ok && existing != resolution {
		r.aliases[canonical] = allowedToolResolution{ambiguous: true}
	} else {
		r.aliases[canonical] = resolution
	}
}

func allowedProviderTool(t provider.Tool, prepared responses.ToolUnionParam) allowedToolResolution {
	switch {
	case prepared.OfCustom != nil:
		return allowedToolResolution{kind: allowedToolCustom, name: prepared.OfCustom.Name}
	case prepared.OfMcp != nil:
		return allowedToolResolution{kind: allowedToolMCP, serverLabel: prepared.OfMcp.ServerLabel}
	case prepared.OfToolSearch != nil:
		return allowedToolResolution{reason: "OpenAI does not support tool_search tools in tool_choice.allowed_tools"}
	default:
		return allowedToolResolution{kind: allowedToolType(providerToolNames[t.ID])}
	}
}

func (r allowedToolRegistry) resolve(option AllowedToolsOption, mapping toolNameMapping) ([]map[string]any, []provider.Warning, error) {
	var entries []map[string]any
	var warnings []provider.Warning
	var dropped []string
	for _, name := range option.ToolNames {
		direct, hasDirect := r.direct[name]
		alias, hasAlias := r.aliases[name]
		warn := func(details string) {
			warnings = append(warnings, provider.Warning{Type: provider.WarnUnsupported, Feature: fmt.Sprintf("allowedTools entry %q", name), Details: details})
		}
		resolution := alias
		if hasDirect {
			resolution = direct
			if hasAlias {
				warn("this name is both a tool name and the provider tool name of another tool in this request; the tool with this name is allowed")
			}
		}
		switch {
		case resolution.ambiguous:
			warn("several tools in this request share this provider tool name; use the tool name from the tools for this request instead")
			dropped = append(dropped, name)
			continue
		case !hasDirect && !hasAlias:
			warn("the tool is not part of the tools for this request and is sent as a function tool")
			resolution = allowedToolResolution{kind: allowedToolFunction, name: mapping.toProviderToolName(name)}
		case resolution.reason != "":
			warn(resolution.reason + "; the tool is removed from the allowed tools")
			dropped = append(dropped, name)
			continue
		}
		entry := map[string]any{"type": resolution.kind}
		switch resolution.kind {
		case allowedToolFunction, allowedToolCustom:
			entry["name"] = resolution.name
		case allowedToolMCP:
			entry["server_label"] = resolution.serverLabel
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil, warnings, fmt.Errorf("openai: allowedTools with only tools that cannot be allow-listed (%s)", strings.Join(dropped, ", "))
	}
	return entries, warnings, nil
}
