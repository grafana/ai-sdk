package chatcompletions

import (
	"encoding/json"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	compatible "github.com/grafana/ai-sdk/providers/openai-compatible"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMultimodalCachePreservesImageOptionsAndOtherMessages(t *testing.T) {
	raw := `{"model":"alias","messages":[{"role":"user","name":"speaker","content":"unmarked"},{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.test/image.png","detail":"original"},"prompt_cache_breakpoint":{"mode":"explicit","ttl":"12h"}},{"type":"input_audio","input_audio":{"data":"AAEC","format":"wav"},"prompt_cache_breakpoint":{"mode":"explicit"}},{"type":"file","file":{"file_id":"file-original"},"prompt_cache_breakpoint":{"mode":"explicit"}}]}]}`
	r, err := mapRequest([]byte(raw))
	require.NoError(t, err)
	before, err := json.Marshal(r.options)
	require.NoError(t, err)
	opts, err := chatTranslator(r)(execution.Target{Type: execution.TargetOpenAICompatible, ProviderName: "custom.chat"}, r.options)
	require.NoError(t, err)
	after, err := json.Marshal(r.options)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
	native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
		assert.JSONEq(t, `[{"role":"user","name":"speaker","content":"unmarked"},{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.test/image.png","detail":"original"},"prompt_cache_breakpoint":{"mode":"explicit","ttl":"12h"}},{"type":"input_audio","input_audio":{"data":"AAEC","format":"wav"},"prompt_cache_breakpoint":{"mode":"explicit"}},{"type":"file","file":{"file_id":"file-original"},"prompt_cache_breakpoint":{"mode":"explicit"}}]}]`, string(body["messages"]))
		if string(body["stream"]) == "true" {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
		} else {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
		}
	}))
	defer native.Close()
	model := compatible.New("native", compatible.WithBaseURL(native.URL), compatible.WithProviderName("custom.chat"))
	_, err = model.DoGenerate(t.Context(), opts)
	require.NoError(t, err)
	stream, err := model.DoStream(t.Context(), opts)
	require.NoError(t, err)
	for part := range stream.Stream {
		require.NotEqual(t, provider.PartError, part.Type)
	}
	for _, target := range []execution.TargetType{execution.TargetOpenAI, execution.TargetAnthropic} {
		_, err = chatTranslator(r)(execution.Target{Type: target}, r.options)
		require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality)
	}
}
