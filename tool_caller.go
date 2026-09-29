package aisdk

import (
	"fmt"
	"maps"
	"slices"

	"github.com/grafana/ai-sdk/provider"
)

func validateToolRoutes(tools ToolSet, routes ToolRoutes) error {
	if tools == nil || routes == nil {
		return nil
	}
	for name, tool := range tools {
		if tool.Caller == nil {
			continue
		}
		switch tool.Caller.Type {
		case ToolCallerLocal:
			if tool.Caller.Bind == nil {
				return fmt.Errorf("aisdk: tool callers: invalid local caller %q", name)
			}
		case ToolCallerProvider:
			if tool.Caller.PrepareProviderOptions == nil {
				return fmt.Errorf("aisdk: tool callers: invalid provider caller %q", name)
			}
		default:
			return fmt.Errorf("aisdk: tool callers: invalid caller %q", name)
		}
	}
	for name, route := range routes {
		if _, ok := tools[name]; !ok {
			return fmt.Errorf("aisdk: tool callers: unknown tool %q", name)
		}
		for _, callerName := range route.Callers {
			caller, ok := tools[callerName]
			if !ok || caller.Caller == nil {
				return fmt.Errorf("aisdk: tool callers: invalid caller %q for tool %q", callerName, name)
			}
		}
	}
	return nil
}

func validateToolExecutors(tools ToolSet) error {
	for name, tool := range tools {
		if tool.Execute != nil && tool.ExecuteStream != nil {
			return fmt.Errorf("aisdk: tool %q has both Execute and ExecuteStream", name)
		}
	}
	return nil
}

func prepareToolsForCallers(tools ToolSet, routes ToolRoutes, active []string, activeSet bool) (ToolSet, ToolSet, []provider.Message) {
	if tools == nil || routes == nil {
		return tools, tools, nil
	}
	activeNames := make(map[string]bool, len(active))
	for _, name := range active {
		activeNames[name] = true
	}
	execution := make(ToolSet, len(tools))
	for name, tool := range tools {
		if !activeSet || activeNames[name] {
			execution[name] = tool
		}
	}
	model := maps.Clone(execution)
	local := make(map[string]ToolSet)
	for _, name := range slices.Sorted(maps.Keys(routes)) {
		tool, ok := execution[name]
		if !ok {
			continue
		}
		route := routes[name]
		providerVisible := false
		for _, callerName := range route.Callers {
			caller := execution[callerName].Caller
			if caller == nil {
				continue
			}
			if caller.Type == ToolCallerProvider {
				providerVisible = true
				tool.ProviderOptions = caller.PrepareProviderOptions(maps.Clone(tool.ProviderOptions))
			} else {
				if local[callerName] == nil {
					local[callerName] = make(ToolSet)
				}
				local[callerName][name] = tool
			}
		}
		execution[name] = tool
		if route.Direct || providerVisible {
			model[name] = tool
		} else {
			delete(model, name)
		}
	}
	var messages []provider.Message
	for _, name := range slices.Sorted(maps.Keys(execution)) {
		tool := execution[name]
		if tool.Caller == nil || tool.Caller.Type != ToolCallerLocal {
			continue
		}
		callerTools := local[name]
		if callerTools == nil {
			callerTools = make(ToolSet)
		}
		bound := tool.Caller.Bind(callerTools)
		execution[name] = bound
		if _, ok := model[name]; !ok {
			continue
		}
		if tool.Caller.PrepareModelMessage == nil {
			model[name] = bound
		} else if text := tool.Caller.PrepareModelMessage(callerTools); text != nil {
			messages = append(messages, provider.UserText(*text))
		}
	}
	return execution, model, messages
}

func appendToolCallerMessages(messages []provider.Message, announcements []provider.Message) []provider.Message {
	if len(announcements) == 0 {
		return messages
	}
	seen := make(map[string]bool)
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == provider.RoleUser && len(msg.Content) == 1 && msg.Content[0].Type == provider.ContentPartTypeText {
			seen[msg.Content[0].Text] = true
			break
		}
	}
	for _, msg := range announcements {
		text := msg.Content[0].Text
		if seen[text] {
			continue
		}
		seen[text] = true
		messages = append(messages, msg)
	}
	return messages
}
