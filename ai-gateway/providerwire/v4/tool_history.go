package v4

import "github.com/grafana/ai-sdk/provider"

type historicalToolCall struct {
	name             string
	providerExecuted bool
	completed        bool
}

func unresolvedProviderCalls(prompt []provider.Message) (map[string]string, *requestFailure) {
	calls := make(map[string]historicalToolCall)
	for _, message := range prompt {
		for _, part := range message.Content {
			switch part.Type {
			case provider.ContentPartTypeToolCall:
				if _, exists := calls[part.ToolCallID]; exists {
					return nil, invalidMappingFailure()
				}
				calls[part.ToolCallID] = historicalToolCall{name: part.ToolName, providerExecuted: part.ProviderExecuted}
			case provider.ContentPartTypeToolResult:
				if call, exists := calls[part.ToolCallID]; exists {
					if call.name != part.ToolName || call.completed {
						return nil, invalidMappingFailure()
					}
					call.completed = true
					calls[part.ToolCallID] = call
				}
			}
		}
	}
	pending := make(map[string]string)
	for id, call := range calls {
		if call.providerExecuted && !call.completed {
			pending[id] = call.name
		}
	}
	return pending, nil
}
