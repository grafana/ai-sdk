package openaicompatible

import (
	"encoding/json"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

const moderationSample = `{"input":{"type":"moderation_results","model":"omni","results":[{"type":"moderation_result","model":"omni","flagged":false,"categories":{"violence":false},"category_scores":{"violence":0.01},"category_applied_input_types":{"violence":["text"]}}]},"output":{"type":"error","code":"unavailable","message":"retry later"}}`

func TestNativeModerationMetadata(t *testing.T) {
	payload := `{"choices":[{"message":{"content":"hello"},"finish_reason":"stop"}],"moderation":` + moderationSample + `}`
	result, err := parseGenerateResponse([]byte(payload), nil, "custom.chat", "custom", func() string { return "id" }, true)
	require.NoError(t, err)
	var ns struct {
		Chat ChatCompletionMetadata `json:"chatCompletion"`
	}
	require.NoError(t, json.Unmarshal(result.ProviderMetadata["openaiCompatible"], &ns))
	require.NoError(t, ns.Chat.Moderation.Validate())
	encoded, err := json.Marshal(ns.Chat.Moderation)
	require.NoError(t, err)
	assert.JSONEq(t, moderationSample, string(encoded))
	normal, err := parseGenerateResponse([]byte(payload), nil, "custom.chat", "custom", func() string { return "id" }, false)
	require.NoError(t, err)
	assert.NotContains(t, normal.ProviderMetadata, "openaiCompatible")
	parts := make(chan provider.StreamPart, 16)
	m := &model{providerName: "custom.chat"}
	m.runStream(t.Context(), "https://example.test", nil, strings.NewReader("data: "+payload+"\n\ndata: [DONE]\n\n"), nil, nil, false, "custom", parts, true)
	close(parts)
	found := false
	for p := range parts {
		require.NotEqual(t, provider.PartError, p.Type)
		if p.Type == provider.PartCustom {
			found = true
			require.NoError(t, json.Unmarshal(p.ProviderMetadata["openaiCompatible"], &ns))
			require.NoError(t, ns.Chat.Moderation.Validate())
		}
	}
	require.True(t, found)
	for _, invalid := range []string{strings.Replace(moderationSample, `0.01`, `1.01`, 1), strings.Replace(moderationSample, `"text"`, `"video"`, 1), strings.Replace(moderationSample, `"flagged":false,`, "", 1), `{"input":{"type":"moderation_results","model":"omni"},"output":{"type":"error","code":"x","message":"y"}}`} {
		var m ChatModeration
		require.NoError(t, json.Unmarshal([]byte(invalid), &m))
		require.Error(t, m.Validate())
	}
}
func TestNativeModerationConfiguration(t *testing.T) {
	m := &model{modelID: "native", providerName: "custom.chat"}
	raw := json.RawMessage(`{"moderation":{"model":"omni","policy":{"input":{"mode":"block"},"output":{"mode":"score"}}}}`)
	body, _, err := m.buildRequest(provider.CallOptions{ProviderOptions: provider.ProviderOptions{"custom": provider.RawProviderOption{Key: "custom", Raw: raw}}}, false)
	require.NoError(t, err)
	encoded, err := json.Marshal(body["moderation"])
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"omni","policy":{"input":{"mode":"block"},"output":{"mode":"score"}}}`, string(encoded))
	require.Error(t, (&ChatModerationOptions{Model: "omni", Policy: &ChatModerationPolicy{Input: &ChatModerationMode{Mode: "ignore"}}}).Validate())
}
