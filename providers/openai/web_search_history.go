package openai

import (
	"encoding/json"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

type webSearchActionType string

const (
	webSearchActionSearch     webSearchActionType = "search"
	webSearchActionOpenPage   webSearchActionType = "openPage"
	webSearchActionFindInPage webSearchActionType = "findInPage"
	webSearchNativeOpenPage   webSearchActionType = "open_page"
	webSearchNativeFindInPage webSearchActionType = "find_in_page"
)

type webSearchSourceType string

const (
	webSearchSourceURL webSearchSourceType = "url"
	webSearchSourceAPI webSearchSourceType = "api"
)

func webSearchHistoryItem(part provider.ContentPart, namespace string) (item *responses.ResponseInputItemUnionParam, denied bool) {
	output := part.Output
	if output == nil {
		return nil, false
	}
	if output.Type == provider.ToolOutputExecutionDenied {
		return nil, true
	}
	if output.Type != provider.ToolOutputJSON {
		return nil, false
	}
	var result map[string]json.RawMessage
	if json.Unmarshal(output.JSON, &result) != nil {
		return nil, false
	}
	var resultType provider.ToolResultOutputType
	if json.Unmarshal(result["type"], &resultType) == nil && resultType == provider.ToolOutputExecutionDenied {
		return nil, true
	}

	action := webSearchHistoryAction(result)
	if action == nil {
		return nil, false
	}
	return &responses.ResponseInputItemUnionParam{OfWebSearchCall: &responses.ResponseFunctionWebSearchParam{
		ID:     webSearchHistoryItemID(part, namespace),
		Status: responses.ResponseFunctionWebSearchStatusCompleted,
		Action: param.Override[responses.ResponseFunctionWebSearchActionUnionParam](action),
	}}, false
}

func webSearchHistoryItemID(part provider.ContentPart, namespace string) string {
	switch options := part.ProviderOptions[namespace].(type) {
	case OpenAIPartOptions:
		if options.ItemID != "" {
			return options.ItemID
		}
	case provider.RawProviderOption:
		var history map[string]json.RawMessage
		if json.Unmarshal(options.Raw, &history) == nil {
			if id, err := requiredString(history["itemId"], "itemId"); err == nil {
				return id
			}
		}
	}
	return part.ToolCallID
}

func webSearchHistoryAction(result map[string]json.RawMessage) map[string]any {
	var action map[string]json.RawMessage
	if json.Unmarshal(result["action"], &action) != nil {
		return nil
	}
	kind, err := requiredString(action["type"], "type")
	if err != nil {
		return nil
	}
	sources, ok := webSearchHistorySources(result["sources"])
	if !ok {
		return nil
	}

	native := make(map[string]any)
	switch webSearchActionType(kind) {
	case webSearchActionSearch:
		native["type"] = webSearchActionSearch
		if raw := action["query"]; len(raw) > 0 {
			query, err := requiredString(raw, "query")
			if err != nil {
				return nil
			}
			native["query"] = query
		}
		if raw := action["queries"]; len(raw) > 0 {
			queries, err := requiredStringSlice(raw, "queries")
			if err != nil {
				return nil
			}
			native["queries"] = queries
		}
		if sources != nil {
			native["sources"] = sources
		}
	case webSearchActionOpenPage:
		native["type"] = webSearchNativeOpenPage
		if !copyNullishWebSearchString(native, action, "url") {
			return nil
		}
	case webSearchActionFindInPage:
		native["type"] = webSearchNativeFindInPage
		if !copyNullishWebSearchString(native, action, "url") || !copyNullishWebSearchString(native, action, "pattern") {
			return nil
		}
	default:
		return nil
	}

	return native
}

func copyNullishWebSearchString(native map[string]any, action map[string]json.RawMessage, key string) bool {
	raw := action[key]
	if len(raw) == 0 {
		return true
	}
	if isJSONNull(raw) {
		native[key] = nil
		return true
	}
	value, err := requiredString(raw, key)
	if err != nil {
		return false
	}
	native[key] = value
	return true
}

func webSearchHistorySources(raw json.RawMessage) ([]map[string]string, bool) {
	if len(raw) == 0 {
		return nil, true
	}
	var values []map[string]json.RawMessage
	if json.Unmarshal(raw, &values) != nil || values == nil {
		return nil, false
	}
	sources := make([]map[string]string, 0, len(values))
	for _, value := range values {
		kind, err := requiredString(value["type"], "type")
		if err != nil {
			return nil, false
		}
		var field string
		switch webSearchSourceType(kind) {
		case webSearchSourceURL:
			field = "url"
		case webSearchSourceAPI:
			field = "name"
		default:
			return nil, false
		}
		text, err := requiredString(value[field], field)
		if err != nil {
			return nil, false
		}
		sources = append(sources, map[string]string{"type": kind, field: text})
	}
	return sources, true
}
