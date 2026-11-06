package openaicompatible

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Synthetic native audio samples verify the codec, not audio synthesis quality.
func TestChatAudioOutput(t *testing.T) {
	payload := `{"choices":[{"message":{"content":null,"audio":{"id":"audio-1","expires_at":123,"data":"AAEC","transcript":"hello","discard":"private"}},"finish_reason":"stop"}]}`
	result, err := parseGenerateResponse([]byte(payload), nil, "custom.chat", "custom", func() string { return "id" }, true)
	require.NoError(t, err)
	var namespace struct {
		Chat ChatCompletionMetadata `json:"chatCompletion"`
	}
	require.NoError(t, json.Unmarshal(result.ProviderMetadata["openaiCompatible"], &namespace))
	require.NotNil(t, namespace.Chat.Audio)
	require.NoError(t, namespace.Chat.Audio.ValidateComplete())
	assert.Equal(t, "AAEC", *namespace.Chat.Audio.Data)
	assert.Equal(t, "audio-1", *namespace.Chat.Audio.ID)
	assert.NotContains(t, string(result.ProviderMetadata["openaiCompatible"]), "discard")
	normal, err := parseGenerateResponse([]byte(payload), nil, "custom.chat", "custom", func() string { return "id" }, false)
	require.NoError(t, err)
	assert.NotContains(t, normal.ProviderMetadata, "openaiCompatible")
}

func TestChatAudioDeltas(t *testing.T) {
	stream := `data: {"choices":[{"delta":{"audio":{"id":"audio-1"}}}]}

data: {"choices":[{"delta":{"audio":{"data":"AAEC","transcript":"hello"}}}]}

data: {"choices":[{"delta":{"audio":{"expires_at":123}},"finish_reason":"stop"}]}

data: [DONE]

`
	parts := make(chan provider.StreamPart, 16)
	m := &model{providerName: "custom.chat"}
	m.runStream(t.Context(), "https://example.test", nil, strings.NewReader(stream), nil, nil, false, "custom", parts, true)
	close(parts)
	var audio []*ChatCompletionAudio
	for part := range parts {
		require.NotEqual(t, provider.PartError, part.Type)
		if part.Type == provider.PartCustom {
			var namespace struct {
				Chat ChatCompletionMetadata `json:"chatCompletion"`
			}
			require.NoError(t, json.Unmarshal(part.ProviderMetadata["openaiCompatible"], &namespace))
			audio = append(audio, namespace.Chat.Audio)
		}
	}
	require.Len(t, audio, 3)
	assert.Equal(t, "audio-1", *audio[0].ID)
	assert.Nil(t, audio[0].Data)
	assert.Equal(t, "AAEC", *audio[1].Data)
	assert.Nil(t, audio[1].ID)
	assert.Equal(t, int64(123), *audio[2].ExpiresAt)
	assert.Nil(t, audio[2].Transcript)
}

func TestChatAudioInvalid(t *testing.T) {
	for _, raw := range []string{
		`{"id":"a","expires_at":1,"data":"not-base64","transcript":"t"}`,
		`{"id":"","expires_at":1,"data":"AAEC","transcript":"t"}`,
		`{"id":"a","expires_at":-1,"data":"AAEC","transcript":"t"}`,
		`{"id":"a","data":"AAEC","transcript":"t"}`,
		`{"id":"a","expires_at":1,"data":1,"transcript":"t"}`,
	} {
		_, err := decodeChatCompletionMetadata([]byte(`{"choices":[{"message":{"audio":`+raw+`}}]}`), false)
		require.Error(t, err, raw)
	}
}

func TestChatAudioNativeRequestAndHistory(t *testing.T) {
	for _, voice := range []ChatCompletionVoice{{Name: "alloy"}, {ID: "voice-1"}} {
		options := provider.CallOptions{
			Prompt:          []provider.Message{{Role: provider.RoleAssistant, ProviderOptions: provider.BuildProviderOptions(ChatCompletionMessageOptions{Audio: &ChatCompletionAudioReference{ID: "audio-previous"}})}},
			ProviderOptions: provider.BuildProviderOptions(OpenAIOptions{Modalities: []string{"text", "audio"}, Audio: &ChatCompletionAudioOptions{Format: "pcm16", Voice: voice}}),
		}
		m := &model{providerName: "openai-compatible", modelID: "audio-model"}
		body, _, err := m.buildRequest(options, false)
		require.NoError(t, err)
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		assert.Contains(t, string(encoded), `"modalities":["text","audio"]`)
		assert.Contains(t, string(encoded), `"format":"pcm16"`)
		assert.Contains(t, string(encoded), `"audio":{"id":"audio-previous"},"content":null,"role":"assistant"`)
		if voice.Name != "" {
			assert.Contains(t, string(encoded), `"voice":"alloy"`)
		} else {
			assert.Contains(t, string(encoded), `"voice":{"id":"voice-1"}`)
		}
	}
	for _, raw := range []string{`{"audio":{"id":""}}`, `{"audio":{"id":"x","data":"AAEC"}}`, `{"audio":"x"}`} {
		_, err := convertAssistantMessage(nil, provider.ProviderOptions{"openaiCompatible": provider.RawProviderOption{Key: "openaiCompatible", Raw: json.RawMessage(raw)}})
		require.Error(t, err, raw)
	}
}

func TestChatAudioOptionsRawAndInvalid(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		valid bool
	}{
		{`{"modalities":["audio"],"audio":{"format":"wav","voice":"alloy"}}`, true},
		{`{"modalities":["audio"],"audio":{"format":"aac","voice":{"id":"voice-1"}}}`, true},
		{`{"modalities":["audio"]}`, false},
		{`{"modalities":["video"]}`, false},
		{`{"modalities":["audio","audio"]}`, false},
		{`{"audio":{"format":"bad","voice":"alloy"}}`, false},
		{`{"audio":{"format":"wav","voice":3}}`, false},
	} {
		m := &model{providerName: "custom.chat", modelID: "audio-model"}
		_, _, err := m.buildRequest(provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, ProviderOptions: provider.ProviderOptions{"custom": provider.RawProviderOption{Key: "custom", Raw: json.RawMessage(tc.raw)}}}, false)
		if tc.valid {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}
