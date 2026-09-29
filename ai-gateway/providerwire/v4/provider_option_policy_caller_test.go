package v4

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderOptionPolicy_DirectCallerOnlyOnBasicAssistantCall(t *testing.T) {
	option := func(raw string) provider.ProviderOptions {
		return provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(raw)}}
	}
	direct := option(`{"caller":{"type":"direct"}}`)
	policy := catalog.ProviderOptionPolicy{Namespaces: []string{"anthropic"}, Fields: map[string][]string{"anthropic": {"caller", "cacheControl"}}}
	input := provider.CallOptions{
		ProviderOptions: direct,
		Tools:           []provider.Tool{{ProviderOptions: direct}},
		Prompt: []provider.Message{
			{Role: provider.RoleUser, ProviderOptions: direct, Content: []provider.ContentPart{{Type: provider.ContentPartTypeText, ProviderOptions: direct}}},
			{Role: provider.RoleAssistant, ProviderOptions: direct, Content: []provider.ContentPart{
				{Type: provider.ContentPartTypeToolCall, ProviderOptions: option(`{"caller":{"type":"direct"},"cacheControl":{"type":"ephemeral"}}`)},
				{Type: provider.ContentPartTypeToolCall, ProviderOptions: option(`{"caller":{"type":"code_execution_20260120","toolId":"private-key"}}`)},
				{Type: provider.ContentPartTypeToolCall, ProviderOptions: option(`{"caller":{"type":"direct","toolId":"private-key"}}`)},
				{Type: provider.ContentPartTypeToolCall, ProviderOptions: option(`{"Caller":{"type":"code_execution_20260120","toolId":"private-key"}}`)},
				{Type: provider.ContentPartTypeToolCall, ProviderOptions: option(`{"caller":{"type":"direct"},"CALLER":{"type":"code_execution_20260120","toolId":"private-key"}}`)},
				{Type: provider.ContentPartTypeToolCall, ProviderExecuted: true, ProviderOptions: direct},
				{Type: provider.ContentPartTypeText, ProviderOptions: direct},
			}},
			{Role: provider.RoleTool, Content: []provider.ContentPart{{Type: provider.ContentPartTypeToolResult, ProviderOptions: direct}}},
		},
	}
	result := applyProviderOptionPolicy(input, policy)
	assert.JSONEq(t, `{"caller":{"type":"direct"},"cacheControl":{"type":"ephemeral"}}`, string(result.Prompt[1].Content[0].ProviderOptions["anthropic"].(provider.RawProviderOption).Raw))
	assert.JSONEq(t, `{"caller":{"type":"direct"}}`, string(result.Prompt[1].Content[4].ProviderOptions["anthropic"].(provider.RawProviderOption).Raw))
	for _, options := range []provider.ProviderOptions{
		result.ProviderOptions, result.Tools[0].ProviderOptions, result.Prompt[0].ProviderOptions,
		result.Prompt[0].Content[0].ProviderOptions, result.Prompt[1].ProviderOptions,
		result.Prompt[1].Content[1].ProviderOptions, result.Prompt[1].Content[2].ProviderOptions,
		result.Prompt[1].Content[3].ProviderOptions, result.Prompt[1].Content[5].ProviderOptions,
		result.Prompt[1].Content[6].ProviderOptions, result.Prompt[2].Content[0].ProviderOptions,
	} {
		if value, ok := options["anthropic"].(provider.RawProviderOption); ok {
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(value.Raw, &fields))
			assert.NotContains(t, fields, "caller")
			assert.NotContains(t, string(value.Raw), "private-key")
			assert.NotContains(t, string(value.Raw), "Caller")
		}
	}
	assert.JSONEq(t, `{"caller":{"type":"direct"}}`, string(input.Prompt[0].ProviderOptions["anthropic"].(provider.RawProviderOption).Raw))
}
