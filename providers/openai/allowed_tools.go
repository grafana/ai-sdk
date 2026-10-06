package openai

import (
	"fmt"
	"strings"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/responses"
)

type allowedToolKind string

const (
	allowedToolFunction allowedToolKind = "function"
	allowedToolCustom   allowedToolKind = "custom"
	allowedToolMCP      allowedToolKind = "mcp"
)

type allowedToolResolution struct {
	kind   allowedToolKind
	name   string
	reason string
}

func (r allowedToolResolution) entry() map[string]any {
	entry := map[string]any{"type": r.kind}
	switch r.kind {
	case allowedToolFunction, allowedToolCustom:
		entry["name"] = r.name
	case allowedToolMCP:
		entry["server_label"] = r.name
	}
	return entry
}

type allowedToolAlias struct {
	resolution allowedToolResolution
	ambiguous  bool
}

type allowedToolIndex struct {
	direct  map[string]allowedToolResolution
	aliases map[string]allowedToolAlias
}

func newAllowedToolIndex() allowedToolIndex {
	return allowedToolIndex{
		direct:  make(map[string]allowedToolResolution),
		aliases: make(map[string]allowedToolAlias),
	}
}

func (idx allowedToolIndex) record(name string, resolution allowedToolResolution, canonical string) {
	idx.direct[name] = resolution
	if canonical == "" || canonical == name {
		return
	}
	alias, ok := idx.aliases[canonical]
	if !ok {
		idx.aliases[canonical] = allowedToolAlias{resolution: resolution}
	} else if alias.resolution != resolution {
		idx.aliases[canonical] = allowedToolAlias{resolution: alias.resolution, ambiguous: true}
	}
}

func providerAllowedToolResolution(t provider.Tool, emitted responses.ToolUnionParam) allowedToolResolution {
	switch t.ID {
	case toolIDCustom:
		return allowedToolResolution{kind: allowedToolCustom, name: emitted.OfCustom.Name}
	case toolIDMCP:
		return allowedToolResolution{kind: allowedToolMCP, name: emitted.OfMcp.ServerLabel}
	case toolIDToolSearch:
		return allowedToolResolution{reason: "OpenAI does not support tool_search tools in tool_choice.allowed_tools"}
	default:
		return allowedToolResolution{kind: allowedToolKind(providerToolNames[t.ID])}
	}
}

func applyAllowedTools(body *responses.ResponseNewParams, selection *AllowedToolsOption, idx allowedToolIndex, mapping toolNameMapping) ([]provider.Warning, error) {
	var warnings []provider.Warning
	var entries []map[string]any
	var dropped []string
	for _, name := range selection.ToolNames {
		feature := "allowedTools entry \"" + name + "\""
		resolution, direct := idx.direct[name]
		alias, hasAlias := idx.aliases[name]
		if direct && hasAlias {
			warnings = append(warnings, provider.Warning{
				Type: provider.WarnUnsupported, Feature: feature,
				Details: "this name is both a tool name and the provider tool name of another tool in this request; the tool with this name is allowed",
			})
		}
		if !direct && hasAlias {
			if alias.ambiguous {
				warnings = append(warnings, provider.Warning{
					Type: provider.WarnUnsupported, Feature: feature,
					Details: "several tools in this request share this provider tool name; use the tool name from the tools for this request instead",
				})
				dropped = append(dropped, name)
				continue
			}
			resolution = alias.resolution
		}
		if !direct && !hasAlias {
			warnings = append(warnings, provider.Warning{
				Type: provider.WarnUnsupported, Feature: feature,
				Details: "the tool is not part of the tools for this request and is sent as a function tool",
			})
			entries = append(entries, map[string]any{"type": allowedToolFunction, "name": mapping.toProviderToolName(name)})
			continue
		}
		if resolution.reason != "" {
			warnings = append(warnings, provider.Warning{
				Type: provider.WarnUnsupported, Feature: feature,
				Details: resolution.reason + "; the tool is removed from the allowed tools",
			})
			dropped = append(dropped, name)
			continue
		}
		entries = append(entries, resolution.entry())
	}
	if len(entries) == 0 {
		return warnings, fmt.Errorf("openai: allowedTools with only tools that cannot be allow-listed (%s)", strings.Join(dropped, ", "))
	}
	mode := responses.ToolChoiceAllowedMode("auto")
	if selection.Mode != "" {
		mode = responses.ToolChoiceAllowedMode(selection.Mode)
	}
	body.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
		OfAllowedTools: &responses.ToolChoiceAllowedParam{Mode: mode, Tools: entries},
	}
	return warnings, nil
}
