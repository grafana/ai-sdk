package aisdk

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizePromptForProvider_ConsecutiveToolMessages(t *testing.T) {
	options := func(raw string) provider.ProviderOptions {
		return provider.ProviderOptions{"test": provider.RawProviderOption{Key: "test", Raw: json.RawMessage(raw)}}
	}
	result := func(id string) provider.ContentPart {
		return provider.ToolResultPart(id, "lookup", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: id})
	}
	t.Run("metadata precedence and isolation", func(t *testing.T) {
		first := provider.NewToolMessage(result("a"), result("b"))
		first.ProviderOptions = options(`{"nested":{"base":true,"replace":[1]},"count":9007199254740993}`)
		first.Content[1].ProviderOptions = options(`{"nested":{"part":true,"replace":[]}}`)
		second := provider.NewToolMessage(result("c"))
		second.ProviderOptions = options(`{"nested":{"second":true}}`)
		third := provider.NewToolMessage(result("d"))
		third.ProviderOptions = options(`{"last":true}`)
		messages := []provider.Message{first, second, third}
		before, err := json.Marshal(messages)
		require.NoError(t, err)
		got, err := sanitizePromptForProvider(messages)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Len(t, got[0].Content, 4)
		for i, id := range []string{"a", "b", "c", "d"} {
			assert.Equal(t, id, got[0].Content[i].ToolCallID)
		}
		assert.Nil(t, got[0].Content[0].ProviderOptions)
		encoded, err := json.Marshal(got[0].Content[1].ProviderOptions)
		require.NoError(t, err)
		assert.JSONEq(t, `{"test":{"nested":{"base":true,"part":true,"replace":[]},"count":9007199254740993}}`, string(encoded))
		assert.Equal(t, second.ProviderOptions, got[0].Content[2].ProviderOptions)
		assert.Equal(t, third.ProviderOptions, got[0].ProviderOptions)
		after, err := json.Marshal(messages)
		require.NoError(t, err)
		assert.JSONEq(t, string(before), string(after))
	})
	t.Run("empty messages participate before removal", func(t *testing.T) {
		first := provider.NewToolMessage(result("a"))
		first.ProviderOptions = options(`{"first":true}`)
		empty := provider.NewToolMessage()
		empty.ProviderOptions = options(`{"empty":true}`)
		got, err := sanitizePromptForProvider([]provider.Message{first, empty, provider.NewToolMessage(result("b"))})
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Len(t, got[0].Content, 2)
		encoded, err := json.Marshal(got[0].Content[0].ProviderOptions)
		require.NoError(t, err)
		assert.JSONEq(t, `{"test":{"first":true,"empty":true}}`, string(encoded))
		assert.Nil(t, got[0].ProviderOptions)
	})
	t.Run("role boundaries remain", func(t *testing.T) {
		messages := []provider.Message{provider.NewToolMessage(result("a")), provider.NewAssistantMessage(provider.TextPart("between")), provider.NewToolMessage(result("b"))}
		got, err := sanitizePromptForProvider(messages)
		require.NoError(t, err)
		assert.Equal(t, messages, got)
	})
	t.Run("merge errors propagate", func(t *testing.T) {
		first := provider.NewToolMessage(result("a"))
		first.ProviderOptions = options(`invalid`)
		first.Content[0].ProviderOptions = options(`{}`)
		got, err := sanitizePromptForProvider([]provider.Message{first, provider.NewToolMessage(result("b"))})
		assert.Nil(t, got)
		assert.ErrorContains(t, err, "merging tool message provider options")
	})
}
