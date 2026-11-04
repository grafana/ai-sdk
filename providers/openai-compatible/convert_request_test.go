package openaicompatible

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoogleThoughtSignature(t *testing.T) {
	cases := []struct {
		name        string
		opts        provider.ProviderOptions
		metadataKey string
		want        string
	}{
		{
			name: "google key",
			opts: provider.ProviderOptions{
				"google": provider.RawProviderOption{Key: "google", Raw: json.RawMessage(`{"thoughtSignature":"sig-google"}`)},
			},
			metadataKey: "google",
			want:        "sig-google",
		},
		{
			name: "custom key wins over google",
			opts: provider.ProviderOptions{
				"myprovider": provider.RawProviderOption{Key: "myprovider", Raw: json.RawMessage(`{"thoughtSignature":"sig-custom"}`)},
				"google":     provider.RawProviderOption{Key: "google", Raw: json.RawMessage(`{"thoughtSignature":"sig-google"}`)},
			},
			metadataKey: "myprovider",
			want:        "sig-custom",
		},
		{
			name: "google fallback when custom key absent",
			opts: provider.ProviderOptions{
				"google": provider.RawProviderOption{Key: "google", Raw: json.RawMessage(`{"thoughtSignature":"sig-google"}`)},
			},
			metadataKey: "myprovider",
			want:        "sig-google",
		},
		{
			name:        "no signature",
			opts:        provider.ProviderOptions{},
			metadataKey: "myprovider",
			want:        "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := googleThoughtSignature(tc.opts, tc.metadataKey)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestThoughtSignature_RoundTrip(t *testing.T) {
	// Response conversion emits the signature under the resolved metadata key.
	extra := &extraContent{Google: &googleExtraContent{ThoughtSignature: "sig-abc"}}
	meta := toolCallProviderMetadata("myprovider", extra)
	require.NotNil(t, meta)
	raw, ok := meta["myprovider"]
	require.True(t, ok)

	// The orchestrator surfaces that metadata back as provider options on the
	// history part; request conversion must read it for the next turn.
	opts := provider.ProviderOptions{
		"myprovider": provider.RawProviderOption{Key: "myprovider", Raw: raw},
	}
	parts := []provider.ContentPart{{
		Type:            provider.ContentPartTypeToolCall,
		ToolCallID:      "call-1",
		ToolName:        "get_weather",
		Input:           []byte(`{"city":"Oslo"}`),
		ProviderOptions: opts,
	}}
	msg, err := convertAssistantMessage(parts, provider.ProviderOptions{}, "myprovider")
	require.NoError(t, err)
	require.Len(t, msg.ToolCalls, 1)
	require.NotNil(t, msg.ToolCalls[0].ExtraContent)
	assert.Equal(t, "sig-abc", msg.ToolCalls[0].ExtraContent.Google.ThoughtSignature)
}

func TestConvertPrompt_ThreadsMetadataKey(t *testing.T) {
	// "my-provider" resolves to the camelCase key "myProvider" when present.
	opts := provider.ProviderOptions{
		"myProvider": provider.RawProviderOption{Key: "myProvider", Raw: json.RawMessage(`{"thoughtSignature":"sig-camel"}`)},
	}
	key := resolveMetadataKey(opts, "my-provider")
	require.Equal(t, "myProvider", key)

	prompt := []provider.Message{{
		Role: provider.RoleAssistant,
		Content: []provider.ContentPart{{
			Type:            provider.ContentPartTypeToolCall,
			ToolCallID:      "call-1",
			ToolName:        "get_weather",
			Input:           []byte(`{"city":"Oslo"}`),
			ProviderOptions: opts,
		}},
	}}
	msgs, err := convertPrompt(prompt, key)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Len(t, msgs[0].ToolCalls, 1)
	require.NotNil(t, msgs[0].ToolCalls[0].ExtraContent)
	assert.Equal(t, "sig-camel", msgs[0].ToolCalls[0].ExtraContent.Google.ThoughtSignature)
}
