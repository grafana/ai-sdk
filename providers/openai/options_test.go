package openai

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time checks: the typed option structs satisfy provider.ProviderOption.
var (
	_ provider.ProviderOption = OpenAIResponsesOptions{}
	_ provider.ProviderOption = OpenAIToolOptions{}
	_ provider.ProviderOption = OpenAIPartOptions{}
)

func TestOpenAIResponsesOptions_ProviderKey(t *testing.T) {
	assert.Equal(t, "openai", OpenAIResponsesOptions{}.ProviderKey())
	assert.Equal(t, "openai", OpenAIToolOptions{}.ProviderKey())
	assert.Equal(t, "openai", OpenAIPartOptions{}.ProviderKey())
}

func TestOpenAIResponsesOptions_MarshalRoundTrip(t *testing.T) {
	store := false
	parallel := true
	maxCalls := int64(5)
	opts := OpenAIResponsesOptions{
		PreviousResponseID: "resp_prev",
		Instructions:       "be terse",
		PromptCacheOptions: &PromptCacheOptions{Mode: "explicit", TTL: "30m"},
		ReasoningEffort:    "high",
		ReasoningMode:      "pro",
		ReasoningContext:   "all_turns",
		ReasoningSummary:   "auto",
		Truncation:         "auto",
		Store:              &store,
		ParallelToolCalls:  &parallel,
		MaxToolCalls:       &maxCalls,
		ServiceTier:        "flex",
		TextVerbosity:      "low",
		Include:            []string{"reasoning.encrypted_content"},
		Metadata:           map[string]string{"k": "v"},
		AllowedTools:       &AllowedToolsOption{ToolNames: []string{"a", "b"}, Mode: "required"},
		ContextManagement:  []ContextManagementEntry{{Type: "compaction"}},
	}

	b, err := json.Marshal(opts)
	require.NoError(t, err)

	var got OpenAIResponsesOptions
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, opts, got)
}

func TestOpenAIPartOptions_MarshalRoundTrip(t *testing.T) {
	opts := OpenAIPartOptions{
		ItemID:                "item_1",
		PromptCacheBreakpoint: &PromptCacheBreakpoint{Mode: "explicit"},
		ImageDetail:           "high",
	}

	b, err := json.Marshal(opts)
	require.NoError(t, err)

	var got OpenAIPartOptions
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, opts, got)
}

func TestLogprobsOption_Unmarshal(t *testing.T) {
	t.Run("bool", func(t *testing.T) {
		var l LogprobsOption
		require.NoError(t, json.Unmarshal([]byte("true"), &l))
		require.NotNil(t, l.Bool)
		assert.True(t, *l.Bool)
		assert.Nil(t, l.Int)
	})
	t.Run("number", func(t *testing.T) {
		var l LogprobsOption
		require.NoError(t, json.Unmarshal([]byte("5"), &l))
		require.NotNil(t, l.Int)
		assert.EqualValues(t, 5, *l.Int)
		assert.Nil(t, l.Bool)
	})
	t.Run("marshal bool", func(t *testing.T) {
		b := true
		out, err := json.Marshal(LogprobsOption{Bool: &b})
		require.NoError(t, err)
		assert.Equal(t, "true", string(out))
	})
	t.Run("marshal number", func(t *testing.T) {
		n := int64(7)
		out, err := json.Marshal(LogprobsOption{Int: &n})
		require.NoError(t, err)
		assert.Equal(t, "7", string(out))
	})
}

func TestResolveProviderOptions_RawRoundTrip(t *testing.T) {
	// Simulate a wire boundary: options arrive as RawProviderOption.
	raw := provider.ProviderOptions{
		"openai": provider.RawProviderOption{
			Key: "openai",
			Raw: json.RawMessage(`{"previousResponseId":"resp_x","store":false}`),
		},
	}
	got, name, err := resolveProviderOptions(provider.CallOptions{ProviderOptions: raw})
	require.NoError(t, err)
	assert.Equal(t, "openai", name)
	assert.Equal(t, "resp_x", got.PreviousResponseID)
	require.NotNil(t, got.Store)
	assert.False(t, *got.Store)
}
