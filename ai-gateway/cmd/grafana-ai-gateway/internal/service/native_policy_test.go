package service

import (
	"encoding/json"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/openai/chatcompletions"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestChatCompletionsPoliciesFailClosed(t *testing.T) {
	file := config.File{Providers: map[string]config.Provider{"responses": {Type: "openai"}, "anthropic": {Type: "anthropic"}, "compatible": {Type: "openai-compatible"}}, Models: map[string]config.Model{
		"text":                {Primary: config.Primary{Provider: "responses", Model: "gpt-4.1"}},
		"reasoning":           {Primary: config.Primary{Provider: "responses", Model: "o3-mini"}},
		"anthropic":           {Primary: config.Primary{Provider: "anthropic", Model: "claude-sonnet-4-20250514"}},
		"compatible":          {Primary: config.Primary{Provider: "compatible", Model: "gpt-4o"}},
		"unknown":             {Primary: config.Primary{Provider: "responses", Model: "future-model"}},
		"response-fallback":   {Primary: config.Primary{Provider: "responses", Model: "gpt-4.1"}, Fallback: []config.Primary{{Provider: "anthropic", Model: "claude-sonnet-4-20250514"}}},
		"compatible-fallback": {Primary: config.Primary{Provider: "anthropic", Model: "claude-sonnet-4-20250514"}, Fallback: []config.Primary{{Provider: "compatible", Model: "gpt-4o-mini"}}},
		"text-fallback":       {Primary: config.Primary{Provider: "anthropic", Model: "claude-sonnet-4-20250514"}, Fallback: []config.Primary{{Provider: "anthropic", Model: "claude-3-5-haiku-20241022"}}},
	}}
	policies := ChatCompletionsPolicies(file)
	assert.ElementsMatch(t, []string{"text", "reasoning", "anthropic", "compatible", "text-fallback"}, mapKeys(policies))

	seed := 1
	require.Error(t, policies["text"](&provider.CallOptions{Seed: &seed}, chatcompletions.Requirements{}))
	text := provider.CallOptions{}
	require.NoError(t, policies["text"](&text, chatcompletions.Requirements{StrictJSONOutput: true}))
	encoded, err := json.Marshal(text.ProviderOptions)
	require.NoError(t, err)
	assert.JSONEq(t, `{"openai":{"store":false,"strictJsonSchema":true}}`, string(encoded))

	reasoning := provider.CallOptions{Reasoning: provider.ReasoningHigh}
	require.NoError(t, policies["reasoning"](&reasoning, chatcompletions.Requirements{}))
	reasoning.Temperature = ptr(0.5)
	require.Error(t, policies["reasoning"](&reasoning, chatcompletions.Requirements{}))

	compatible := provider.CallOptions{}
	require.NoError(t, policies["compatible"](&compatible, chatcompletions.Requirements{}))
	encoded, err = json.Marshal(compatible.ProviderOptions)
	require.NoError(t, err)
	assert.JSONEq(t, `{"openaiCompatible":{"store":false}}`, string(encoded))

	strictFalse := false
	anthropic := provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "tool", Strict: &strictFalse}}}
	require.NoError(t, policies["anthropic"](&anthropic, chatcompletions.Requirements{}))
	assert.Equal(t, 4096, *anthropic.MaxOutputTokens)
	assert.Nil(t, anthropic.Tools[0].Strict)
	strictTrue := true
	require.Error(t, policies["anthropic"](&provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "tool", Strict: &strictTrue}}}, chatcompletions.Requirements{}))
	require.Error(t, policies["anthropic"](&provider.CallOptions{Temperature: ptr(2.0)}, chatcompletions.Requirements{}))
	require.Error(t, policies["anthropic"](&provider.CallOptions{MaxOutputTokens: ptr(4097)}, chatcompletions.Requirements{}))

	fallback := provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "tool"}}}
	require.Error(t, policies["text-fallback"](&fallback, chatcompletions.Requirements{}))
}

func ptr[T any](value T) *T { return &value }

func mapKeys(values map[string]chatcompletions.RequestPolicy) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	return result
}
