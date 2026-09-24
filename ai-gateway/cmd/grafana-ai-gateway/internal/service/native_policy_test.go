package service

import (
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/openai/chatcompletions"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNativePoliciesFailClosed(t *testing.T) {
	file := config.File{Providers: map[string]config.Provider{"responses": {Type: "openai"}, "anthropic": {Type: "anthropic"}, "compatible": {Type: "openai-compatible"}}, Models: map[string]config.Model{
		"text":                {Primary: config.Primary{Provider: "responses", Model: "gpt-4.1"}},
		"reasoning":           {Primary: config.Primary{Provider: "responses", Model: "o3-mini"}},
		"unknown":             {Primary: config.Primary{Provider: "responses", Model: "future-model"}},
		"response-fallback":   {Primary: config.Primary{Provider: "responses", Model: "gpt-4.1"}, Fallback: []config.Primary{{Provider: "anthropic", Model: "claude-sonnet-4-20250514"}}},
		"compatible-fallback": {Primary: config.Primary{Provider: "anthropic", Model: "claude-sonnet-4-20250514"}, Fallback: []config.Primary{{Provider: "compatible", Model: "gpt-4o-mini"}}},
		"text-fallback":       {Primary: config.Primary{Provider: "anthropic", Model: "claude-sonnet-4-20250514"}, Fallback: []config.Primary{{Provider: "anthropic", Model: "claude-3-5-haiku-20241022"}}},
	}}
	assert.Equal(t, map[string]chatcompletions.Backend{"text": chatcompletions.BackendResponses, "reasoning": chatcompletions.BackendReasoning, "text-fallback": chatcompletions.BackendFallback}, NativePolicies(file))
}
