package anthropic

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnthropicCacheControl_JSONRoundTrip(t *testing.T) {
	for _, ttl := range []string{"", "5m", "1h"} {
		t.Run("ttl="+ttl, func(t *testing.T) {
			marker := CacheControl("ephemeral")
			if ttl != "" {
				marker = CacheControlWithTTL("ephemeral", ttl)
			}
			encoded, err := json.Marshal(provider.BuildProviderOptions(marker))
			require.NoError(t, err)
			expected := `{"anthropic":{"cacheControl":{"type":"ephemeral"}}}`
			if ttl != "" {
				expected = fmt.Sprintf(`{"anthropic":{"cacheControl":{"type":"ephemeral","ttl":%q}}}`, ttl)
			}
			assert.JSONEq(t, expected, string(encoded))

			var decoded provider.ProviderOptions
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			cc := extractCacheControl(decoded)
			require.NotNil(t, cc)
			assert.Equal(t, "ephemeral", cc.Type)
			assert.Equal(t, ttl, cc.TTL)
			resolved, ok, err := provider.ResolveOption[AnthropicCacheControl](decoded, "anthropic")
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, marker, resolved)
		})
	}
}

func TestAnthropicCacheControl_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		data string
		ttl  string
	}{
		{name: "camel case", data: `{"cacheControl":{"type":"ephemeral","ttl":"1h"}}`, ttl: "1h"},
		{name: "snake case", data: `{"cache_control":{"type":"ephemeral","ttl":"5m"}}`, ttl: "5m"},
		{name: "camel case takes precedence", data: `{"cacheControl":{"type":"ephemeral","ttl":"1h"},"cache_control":{"type":"ephemeral","ttl":"5m"}}`, ttl: "1h"},
		{name: "legacy exported fields", data: `{"CacheType":"ephemeral","TTL":"1h"}`, ttl: "1h"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolved, ok, err := provider.ResolveOption[AnthropicCacheControl](makeProviderOpts(tc.data), "anthropic")
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, AnthropicCacheControl{CacheType: "ephemeral", TTL: tc.ttl}, resolved)
		})
	}
	t.Run("legacy partial update", func(t *testing.T) {
		cc := AnthropicCacheControl{CacheType: "ephemeral", TTL: "1h"}
		require.NoError(t, json.Unmarshal([]byte(`{"CacheType":"ephemeral"}`), &cc))
		assert.Equal(t, "1h", cc.TTL)
		require.NoError(t, json.Unmarshal([]byte(`null`), &cc))
		assert.Equal(t, AnthropicCacheControl{CacheType: "ephemeral", TTL: "1h"}, cc)
	})
}

func TestBuildParams_CacheControlJSONRoundTrip(t *testing.T) {
	for _, location := range []string{"system", "message", "part", "tool"} {
		for _, ttl := range []string{"", "5m", "1h"} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/ttl=%s/stream=%t", location, ttl, stream), func(t *testing.T) {
					marker := provider.BuildProviderOptions(CacheControlWithTTL("ephemeral", ttl))
					opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}}
					switch location {
					case "system":
						msg := provider.NewSystemMessage("cached context")
						msg.ProviderOptions = marker
						opts.Prompt = append([]provider.Message{msg}, opts.Prompt...)
					case "message":
						opts.Prompt[0].ProviderOptions = marker
					case "part":
						opts.Prompt[0].Content[0].ProviderOptions = marker
					case "tool":
						opts.Tools = []provider.Tool{{
							Type: provider.ToolTypeFunction, Name: "search",
							InputSchema:     json.RawMessage(`{"type":"object","properties":{}}`),
							ProviderOptions: marker,
						}}
					}
					encoded, err := json.Marshal(opts)
					require.NoError(t, err)
					var decoded provider.CallOptions
					require.NoError(t, json.Unmarshal(encoded, &decoded))
					params, _, warnings, _, err := buildParams("claude-sonnet-4-6", decoded, stream)
					require.NoError(t, err)
					assert.Empty(t, warnings)
					request, err := json.Marshal(params)
					require.NoError(t, err)
					var body map[string]any
					require.NoError(t, json.Unmarshal(request, &body))
					var block map[string]any
					switch location {
					case "system":
						block = body["system"].([]any)[0].(map[string]any)
					case "tool":
						block = body["tools"].([]any)[0].(map[string]any)
					default:
						msg := body["messages"].([]any)[0].(map[string]any)
						block = msg["content"].([]any)[0].(map[string]any)
					}
					expected := map[string]any{"type": "ephemeral"}
					if ttl != "" {
						expected["ttl"] = ttl
					}
					assert.Equal(t, expected, block["cache_control"])
				})
			}
		}
	}
}
