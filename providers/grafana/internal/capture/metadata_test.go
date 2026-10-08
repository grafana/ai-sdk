package main

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnaryResponseMessages_ActualOutputOnly(t *testing.T) {
	for _, metadata := range []provider.ProviderMetadata{nil, {}, {"future": json.RawMessage(`{"caller":{"type":"opaque"}}`)}} {
		content := []provider.GenerateContentPart{
			{Type: provider.ContentText, Text: "answer", ProviderMetadata: metadata},
			{Type: provider.ContentReasoning, Text: "thought", ProviderMetadata: metadata},
			{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "weather", Input: json.RawMessage(`{"city":"Rio"}`), ProviderMetadata: metadata},
			{Type: provider.ContentSource, ID: "response-only", ProviderMetadata: metadata},
		}
		messages, err := unaryResponseMessages(content)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		require.Len(t, messages[0].Content, 3)
		for i, part := range messages[0].Content {
			if metadata == nil {
				assert.Nil(t, part.ProviderOptions)
			} else {
				require.NotNil(t, part.ProviderOptions)
				assert.Len(t, part.ProviderOptions, len(metadata))
				for key, raw := range metadata {
					assert.Equal(t, provider.RawProviderOption{Key: key, Raw: raw}, part.ProviderOptions[key])
				}
			}
			assert.Equal(t, content[i].Text, part.Text)
			assert.Equal(t, content[i].ToolCallID, part.ToolCallID)
			assert.Equal(t, content[i].Input, part.Input)
		}
	}
	_, err := unaryResponseMessages([]provider.GenerateContentPart{{Type: provider.ContentToolResult}})
	require.Error(t, err)
}
