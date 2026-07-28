package agentobservability

import "github.com/grafana/ai-sdk/provider"

// Metadata keys mirror the upstream agento11y Anthropic helper so existing
// dashboards and BigQuery views keep working. Adding new keys is safe;
// changing or removing existing keys breaks observability filters.
const (
	// MetadataThinkingBudgetTokens is the key under which the Anthropic
	// thinking budget is recorded in Generation.Metadata. Mirrors
	// agento11y/go-providers/anthropic's thinkingBudgetMetadataKey constant.
	MetadataThinkingBudgetTokens = "agento11y.gen_ai.request.thinking.budget_tokens"
)

// metadataFromProviderOptions decodes the subset of params.ProviderOptions
// the middleware needs to surface as agento11y.Generation.Metadata entries.
//
// The implementation never imports a provider module; every ProviderOptions
// value is reached via the opaque RawProviderOption / encoding/json path. New
// providers' options can be wired in by adding cases here without changing
// the public API surface.
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
