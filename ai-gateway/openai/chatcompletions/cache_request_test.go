package chatcompletions

import (
	"context"
	"encoding/json"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	compatible "github.com/grafana/ai-sdk/providers/openai-compatible"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

const cacheRequest = `{"model":"alias","prompt_cache_options":{"mode":"explicit","ttl":"30m"},"messages":[{"role":"system","content":[{"type":"text","text":"rules","prompt_cache_breakpoint":{"mode":"explicit"}},{"type":"text","text":"tail"}]},{"role":"developer","content":[{"type":"text","text":"more","prompt_cache_breakpoint":{"mode":"explicit"}}]},{"role":"user","content":[{"type":"text","text":"hello","prompt_cache_breakpoint":{"mode":"explicit","ttl":"12h"}}]},{"role":"assistant","content":[{"type":"text","text":"calling","prompt_cache_breakpoint":{"mode":"explicit"}}],"tool_calls":[{"type":"function","id":"c","function":{"name":"f","arguments":"{}"}}]},{"role":"tool","tool_call_id":"c","content":[{"type":"text","text":"result","prompt_cache_breakpoint":{"mode":"explicit"}}]}]}`

func TestOfficialGoCacheBoundaries(t *testing.T) {
	native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Messages []struct {
				Role       string             `json:"role"`
				Content    []cacheContentPart `json:"content"`
				ToolCallID string             `json:"tool_call_id"`
			}
			Stream bool
			Cache  *compatible.ChatPromptCacheOptions `json:"prompt_cache_options"`
		}
		require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
		require.Len(t, body.Messages, 5)
		assert.Equal(t, "explicit", body.Cache.Mode)
		assert.Equal(t, "30m", body.Cache.TTL)
		for i, m := range body.Messages {
			require.NotEmpty(t, m.Content)
			assert.NotNil(t, m.Content[0].Breakpoint, i)
		}
		require.Len(t, body.Messages[0].Content, 2)
		assert.Equal(t, "tail", *body.Messages[0].Content[1].Text)
		assert.Equal(t, "developer", body.Messages[1].Role)
		assert.Equal(t, "c", body.Messages[4].ToolCallID)
		if body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"id\":\"c\",\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
		} else {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
		}
	}))
	defer native.Close()
	model := compatible.New("native", compatible.WithBaseURL(native.URL), compatible.WithProviderName("custom.chat"))
	h := testHandler(t, &fakeModel{})
	runtime, err := execution.New(execution.Config{Limits: execution.DefaultLimits, Selector: func(context.Context, string, provider.CallOptions, json.RawMessage) (execution.Selection, error) {
		return execution.Selection{ID: "canonical", Model: execution.Translate(model, execution.Target{Type: execution.TargetOpenAICompatible, ProviderName: "custom.chat"})}, nil
	}})
	require.NoError(t, err)
	h.runtime = runtime
	server := httptest.NewServer(h)
	defer server.Close()
	client := openaisdk.NewClient(option.WithBaseURL(server.URL+"/v1"), option.WithAPIKey("test"), option.WithMaxRetries(0))
	var params openaisdk.ChatCompletionNewParams
	require.NoError(t, json.Unmarshal([]byte(cacheRequest), &params))
	result, err := client.Chat.Completions.New(t.Context(), params)
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Choices[0].Message.Content)
	stream := client.Chat.Completions.NewStreaming(t.Context(), params)
	text := ""
	for stream.Next() {
		for _, choice := range stream.Current().Choices {
			text += choice.Delta.Content
		}
	}
	require.NoError(t, stream.Err())
	require.NoError(t, stream.Close())
	assert.Equal(t, "ok", text)
}

func TestCacheTranslationIsImmutableAndCapabilityChecked(t *testing.T) {
	r, err := mapRequest([]byte(cacheRequest))
	require.NoError(t, err)
	before, err := json.Marshal(r.options)
	require.NoError(t, err)
	_, err = chatTranslator(r)(execution.Target{Type: execution.TargetOpenAICompatible}, r.options)
	require.NoError(t, err)
	after, err := json.Marshal(r.options)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
	for _, target := range []execution.TargetType{execution.TargetOpenAI, execution.TargetAnthropic} {
		_, err = chatTranslator(r)(execution.Target{Type: target}, r.options)
		require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality)
	}
}
