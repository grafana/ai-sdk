package openaicompatible

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These native payloads are synthetic codec cases, not provider recordings.
const chatMetadataPayload = `{"choices":[{"message":{"content":null,"refusal":"Cannot help"},"delta":{"refusal":"Cannot help"},"finish_reason":"stop","logprobs":{"content":null,"refusal":[{"token":"Cannot","logprob":-0.1,"bytes":[67,97],"top_logprobs":[{"token":"No","logprob":-2,"bytes":null}]}]}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"prompt_tokens_details":{"cached_tokens":0,"audio_tokens":2},"completion_tokens_details":{"reasoning_tokens":1,"audio_tokens":2,"accepted_prediction_tokens":0,"rejected_prediction_tokens":3},"secret":"discard"},"secret":"discard"}`

func TestChatCompletionMetadataUnary(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		result, err := parseGenerateResponse([]byte(chatMetadataPayload), nil, "custom.chat", "custom", func() string { return "id" }, enabled)
		require.NoError(t, err)
		raw := result.ProviderMetadata["openaiCompatible"]
		if !enabled {
			require.Empty(t, raw)
			continue
		}
		var namespace struct {
			Chat ChatCompletionMetadata `json:"chatCompletion"`
		}
		require.NoError(t, json.Unmarshal(raw, &namespace))
		require.NotNil(t, namespace.Chat.Refusal)
		assert.Equal(t, "Cannot help", *namespace.Chat.Refusal)
		require.Len(t, namespace.Chat.Logprobs.Refusal, 1)
		assert.Equal(t, 2, *namespace.Chat.Usage.PromptTokensDetails.AudioTokens)
		assert.Equal(t, 0, *namespace.Chat.Usage.CompletionTokensDetails.AcceptedPredictionTokens)
		assert.NotContains(t, string(raw), "secret")
		assert.JSONEq(t, `{"acceptedPredictionTokens":0,"rejectedPredictionTokens":3}`, string(result.ProviderMetadata["custom"]))
	}
}

func TestChatCompletionMetadataStream(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		stream := "data: " + chatMetadataPayload + "\n\ndata: [DONE]\n\n"
		parts := make(chan provider.StreamPart, 16)
		m := &model{providerName: "custom.chat"}
		m.runStream(t.Context(), "https://example.test", nil, strings.NewReader(stream), nil, nil, false, "custom", parts, enabled)
		close(parts)
		customs := 0
		for part := range parts {
			require.NotEqual(t, provider.PartError, part.Type)
			if part.Type == provider.PartCustom {
				customs++
				assert.Equal(t, "openai-compatible.chat-completion", part.Kind)
				assert.Contains(t, string(part.ProviderMetadata["openaiCompatible"]), `"refusal":"Cannot help"`)
				assert.NotContains(t, string(part.ProviderMetadata["openaiCompatible"]), `"usage"`)
			}
			if part.Type == provider.PartFinish && enabled {
				assert.Contains(t, string(part.ProviderMetadata["openaiCompatible"]), `"audio_tokens":2`)
				assert.NotContains(t, string(part.ProviderMetadata["openaiCompatible"]), `"refusal"`)
			}
		}
		if enabled {
			assert.Equal(t, 1, customs)
		} else {
			assert.Zero(t, customs)
		}
	}
}

func TestChatCompletionMetadataRejectsInvalidFields(t *testing.T) {
	for _, bad := range []string{
		`{"choices":[{"message":{"refusal":7}}]}`,
		`{"choices":[{"logprobs":{"content":[{"token":"a","logprob":1}]}}]}`,
		`{"choices":[{"logprobs":{"content":[{"token":"a"}]}}]}`,
		`{"choices":[{"logprobs":{"content":[{"token":"a","logprob":0,"bytes":[256]}]}}]}`,
		`{"choices":[],"usage":{"completion_tokens_details":{"audio_tokens":-1}}}`,
	} {
		_, err := decodeChatCompletionMetadata([]byte(bad), false)
		require.Error(t, err, bad)
	}
}

func TestChatCompletionMetadataOptionIsLocal(t *testing.T) {
	enabled := true
	for _, option := range []provider.ProviderOption{
		OpenAIOptions{PreserveChatCompletionMetadata: &enabled},
		provider.RawProviderOption{Key: "openaiCompatible", Raw: json.RawMessage(`{"preserveChatCompletionMetadata":true}`)},
	} {
		options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, ProviderOptions: provider.ProviderOptions{"openaiCompatible": option}}
		m := &model{providerName: "openai-compatible", modelID: "test"}
		body, _, err := m.buildRequest(options, false)
		require.NoError(t, err)
		assert.NotContains(t, body, "preserveChatCompletionMetadata")
		native, err := readOpenAIOptions(options.ProviderOptions, m.providerName)
		require.NoError(t, err)
		require.NotNil(t, native.PreserveChatCompletionMetadata)
		assert.True(t, *native.PreserveChatCompletionMetadata)
	}
}

func TestInvalidChatMetadataStopsStream(t *testing.T) {
	stream := "data: {\"choices\":[{\"delta\":{\"refusal\":7},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
	parts := make(chan provider.StreamPart, 16)
	m := &model{providerName: "custom.chat"}
	m.runStream(t.Context(), "https://example.test", nil, strings.NewReader(stream), nil, nil, false, "custom", parts, true)
	close(parts)
	sawError := false
	for part := range parts {
		if part.Type == provider.PartError {
			sawError = true
		}
		assert.NotEqual(t, provider.PartCustom, part.Type)
		if part.Type == provider.PartFinish {
			require.NotNil(t, part.FinishReason)
			assert.Equal(t, provider.FinishReasonError, part.FinishReason.Unified)
		}
	}
	assert.True(t, sawError)
}
