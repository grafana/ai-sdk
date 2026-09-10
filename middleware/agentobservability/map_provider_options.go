package agentobservability

import (
	"encoding/json"

	"github.com/grafana/ai-sdk/provider"
)

// Metadata keys mirror the upstream agento11y Anthropic helper so existing
// dashboards and BigQuery views keep working. Adding new keys is safe;
// changing or removing existing keys breaks observability filters.
const (
	// MetadataThinkingBudgetTokens is the key under which the Anthropic
	// thinking budget is recorded in Generation.Metadata. Mirrors
	// agento11y/go-providers/anthropic's thinkingBudgetMetadataKey constant.
	MetadataThinkingBudgetTokens = "agento11y.gen_ai.request.thinking.budget_tokens"
	// MetadataServerToolUseWebSearchRequests records billed Anthropic web searches.
	MetadataServerToolUseWebSearchRequests = "agento11y.gen_ai.usage.server_tool_use.web_search_requests"
	// MetadataServerToolUseWebFetchRequests records billed Anthropic web fetches.
	MetadataServerToolUseWebFetchRequests = "agento11y.gen_ai.usage.server_tool_use.web_fetch_requests"
	// MetadataServerToolUseTotalRequests records all billed Anthropic server-tool requests.
	MetadataServerToolUseTotalRequests = "agento11y.gen_ai.usage.server_tool_use.total_requests"
)

// metadataFromUsage extracts provider usage fields that Agent Observability
// represents as generation metadata.
func metadataFromUsage(usage provider.Usage) map[string]any {
	if len(usage.Raw) == 0 {
		return nil
	}
	var raw struct {
		ServerToolUse struct {
			WebSearchRequests int64 `json:"web_search_requests"`
			WebFetchRequests  int64 `json:"web_fetch_requests"`
		} `json:"server_tool_use"`
	}
	if err := json.Unmarshal(usage.Raw, &raw); err != nil {
		return nil
	}
	search := raw.ServerToolUse.WebSearchRequests
	fetch := raw.ServerToolUse.WebFetchRequests
	if search < 0 || fetch < 0 || search+fetch == 0 {
		return nil
	}
	out := map[string]any{MetadataServerToolUseTotalRequests: search + fetch}
	if search > 0 {
		out[MetadataServerToolUseWebSearchRequests] = search
	}
	if fetch > 0 {
		out[MetadataServerToolUseWebFetchRequests] = fetch
	}
	return out
}

// metadataFromProviderOptions extracts request fields that Agent Observability
// represents as generation metadata.
func metadataFromProviderOptions(params provider.CallOptions) map[string]any {
	out := map[string]any{}
	if budget := thinkingBudgetFromAnthropic(params.ProviderOptions); budget != nil {
		out[MetadataThinkingBudgetTokens] = *budget
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
