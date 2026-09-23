package v4

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapToolMetadata_ReviewedProjection(t *testing.T) {
	metadata := provider.ProviderMetadata{
		"anthropic": json.RawMessage(`{"type":"mcp-tool-use","serverName":"echo","caller":{"type":"direct"},"secret":"private"}`),
		"openai":    json.RawMessage(`{"itemId":"item-1","namespace":"tools","caller":{"type":"program","callerId":"parent","private":"hidden"},"backendModel":"private"}`),
		"private":   json.RawMessage(`{"credential":"private"}`),
	}
	mapped, err := mapToolMetadata(metadata, map[string]bool{"echo": true}, 1024)
	require.NoError(t, err)
	require.Len(t, mapped, 2)
	assert.JSONEq(t, `{"type":"mcp-tool-use","serverName":"echo"}`, string(mapped["anthropic"]))
	assert.JSONEq(t, `{"itemId":"item-1","namespace":"tools","caller":{"type":"program","callerId":"parent"}}`, string(mapped["openai"]))
	assert.NotContains(t, string(mapped["openai"]), "private")
}

func TestMapToolMetadata_RejectsUnconfiguredOrOversizedValues(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata provider.ProviderMetadata
		mcpNames map[string]bool
		limit    int64
	}{
		{name: "unconfigured server", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"type":"mcp-tool-use","serverName":"other"}`)}, mcpNames: map[string]bool{"echo": true}, limit: 1024},
		{name: "missing server", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"type":"mcp-tool-use"}`)}, mcpNames: map[string]bool{"echo": true}, limit: 1024},
		{name: "wrong item id type", metadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":42}`)}, limit: 1024},
		{name: "malformed metadata", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{`)}, limit: 1024},
		{name: "oversized known field", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"type":"mcp-tool-use","serverName":"` + strings.Repeat("x", 200) + `"}`)}, limit: 128},
		{name: "oversized unknown namespace", metadata: provider.ProviderMetadata{"private": json.RawMessage(`{"secret":"` + strings.Repeat("x", 200) + `"}`)}, limit: 128},
		{name: "excess cardinality", metadata: provider.ProviderMetadata{"one": nil, "two": nil}, limit: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := mapToolMetadata(tc.metadata, tc.mcpNames, tc.limit)
			require.Error(t, err)
		})
	}
}
