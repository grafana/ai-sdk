package v4

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
)

func TestRequestProtectedSources_OrdinaryDestinations(t *testing.T) {
	options := provider.CallOptions{
		ProviderOptions: provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"mcpServers":[{"url":"https://ordinary.example/tools?topic=ordinary"}]}`)}},
		Tools:           []provider.Tool{{Type: provider.ToolTypeProvider, ID: "openai.mcp", Args: map[string]json.RawMessage{"serverUrl": json.RawMessage(`"https://ordinary.example/tools?topic=ordinary"`)}}},
	}
	assert.Empty(t, requestProtectedSources(options))
}

func TestRequestProtectedSources_AllHeaderValues(t *testing.T) {
	options := provider.CallOptions{Headers: map[string]string{"x-goog-api-key": "body-secret"}, Tools: []provider.Tool{{Type: provider.ToolTypeProvider, ID: "openai.mcp", Args: map[string]json.RawMessage{"headers": json.RawMessage(`{"Authorization":"Bearer first-secret","authorization":"Bearer second-secret","anthropic-api-key":"anthropic-secret","openai-api-key":"openai-secret","x-note":7}`)}}}}
	sources := requestProtectedSources(options)
	for _, source := range []string{"body-secret", "first-secret", "second-secret", "anthropic-secret", "openai-secret"} {
		assert.Contains(t, sources, source)
	}
}
