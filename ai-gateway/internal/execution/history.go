package execution

import (
	"errors"
	"maps"

	"github.com/grafana/ai-sdk/provider"
)

// ErrInvalidToolHistory reports a prompt whose tool calls and results are
// inconsistent. [ClassifySelection] classifies it as an invalid request.
var ErrInvalidToolHistory = errors.New("execution: inconsistent tool history")

// ToolHistory is the provider-executed tool calls a prompt leaves without a
// result, by tool call ID, with their tool names. It seeds stream and unary
// output validation. It is never modified after [RebuildToolHistory] returns
// it: every consumer works on its own copy.
type ToolHistory map[string]string

type historicalToolCall struct {
	reemitted        bool
	name             string
	providerExecuted bool
	completed        bool
}

// RebuildToolHistory rebuilds the pending provider-executed calls from a
// prompt. A call ID may repeat only as one re-emission, with the same tool
// name, of a provider-executed call that has no result yet. A result must name
// its call's tool and complete it once. A result with no earlier call is
// tolerated, but a later call with its ID is not. Any other inconsistency
// fails with [ErrInvalidToolHistory].
func RebuildToolHistory(prompt []provider.Message) (ToolHistory, error) {
	calls := make(map[string]historicalToolCall)
	orphanResults := make(map[string]struct{})
	for _, message := range prompt {
		for _, part := range message.Content {
			switch part.Type {
			case provider.ContentPartTypeToolCall:
				if previous, exists := calls[part.ToolCallID]; exists {
					if previous.completed || previous.reemitted || !previous.providerExecuted || !part.ProviderExecuted || previous.name != part.ToolName {
						return nil, ErrInvalidToolHistory
					}
					previous.reemitted = true
					calls[part.ToolCallID] = previous
					continue
				}
				if _, exists := orphanResults[part.ToolCallID]; exists {
					return nil, ErrInvalidToolHistory
				}
				calls[part.ToolCallID] = historicalToolCall{name: part.ToolName, providerExecuted: part.ProviderExecuted}
			case provider.ContentPartTypeToolResult:
				if call, exists := calls[part.ToolCallID]; exists {
					if call.name != part.ToolName || call.completed {
						return nil, ErrInvalidToolHistory
					}
					call.completed = true
					calls[part.ToolCallID] = call
				} else {
					orphanResults[part.ToolCallID] = struct{}{}
				}
			}
		}
	}
	pending := make(ToolHistory)
	for id, call := range calls {
		if call.providerExecuted && !call.completed {
			pending[id] = call.name
		}
	}
	return pending, nil
}

func (h ToolHistory) clone() ToolHistory {
	if h == nil {
		return make(ToolHistory)
	}
	return maps.Clone(h)
}
