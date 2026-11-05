package chatcompletionsadapter_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOfficialSDK_NativeChoices(t *testing.T) {
	var calls atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, float64(2), body["n"])
		base := map[string]any{"id": "multi-native", "model": "native-model", "created": 123}
		usage := map[string]int{"prompt_tokens": 3, "completion_tokens": 4, "total_tokens": 7}
		if body["stream"] != true {
			base["choices"] = []any{map[string]any{"index": 1, "message": map[string]any{"role": "assistant", "content": "second"}, "finish_reason": "length"}, map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "first"}, "finish_reason": "stop"}}
			base["usage"] = usage
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(base))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		frame := func(choices []any) {
			base["choices"] = choices
			data, err := json.Marshal(base)
			require.NoError(t, err)
			_, err = fmt.Fprintf(w, "data: %s\n\n", data)
			require.NoError(t, err)
		}
		frame([]any{map[string]any{"index": 1, "delta": map[string]any{"content": "second"}}, map[string]any{"index": 0, "delta": map[string]any{"content": "first"}}})
		frame([]any{map[string]any{"index": 1, "delta": map[string]any{}, "finish_reason": "length"}, map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}})
		base["usage"] = usage
		frame([]any{})
		_, err := fmt.Fprint(w, "data: [DONE]\n\n")
		require.NoError(t, err)
	}))
	defer backend.Close()
	config := fmt.Sprintf("providers:\n  native:\n    type: openai-compatible\n    apiKeyEnv: CHAT_COMPLETIONS_ADAPTER_TEST_KEY\n    baseURL: %s/v1\nmodels:\n  public/chat:\n    name: Chat\n    primary:\n      provider: native\n      model: native-model\n    aliases: [chat]\n", backend.URL)
	address := startCommand(t, backend.URL, "self-hosted", config)
	client := openai.NewClient(option.WithBaseURL(address+"/v1/"), option.WithAPIKey(testToken), option.WithMaxRetries(0))
	params := sdkParams(t, `,"n":2`)
	result, err := client.Chat.Completions.New(t.Context(), params)
	require.NoError(t, err)
	require.Len(t, result.Choices, 2)
	assert.Equal(t, int64(1), result.Choices[0].Index)
	assert.Equal(t, "second", result.Choices[0].Message.Content)
	assert.Equal(t, int64(7), result.Usage.TotalTokens)
	assert.EqualValues(t, 1, calls.Load())
	params.StreamOptions = openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)}
	stream := client.Chat.Completions.NewStreaming(t.Context(), params)
	texts := map[int64]string{}
	finishes := map[int64]string{}
	usageChunks := 0
	for stream.Next() {
		chunk := stream.Current()
		for _, choice := range chunk.Choices {
			texts[choice.Index] += choice.Delta.Content
			if choice.FinishReason != "" {
				finishes[choice.Index] = choice.FinishReason
			}
		}
		if len(chunk.Choices) == 0 {
			usageChunks++
			assert.Equal(t, int64(7), chunk.Usage.TotalTokens)
		}
	}
	require.NoError(t, stream.Err())
	require.NoError(t, stream.Close())
	assert.Equal(t, map[int64]string{0: "first", 1: "second"}, texts)
	assert.Equal(t, map[int64]string{0: "stop", 1: "length"}, finishes)
	assert.Equal(t, 1, usageChunks)
	assert.EqualValues(t, 2, calls.Load())
}
