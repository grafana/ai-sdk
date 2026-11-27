package messages

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	anthropicoption "github.com/anthropics/anthropic-sdk-go/v2/option"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/grafana/ai-sdk/providers/openai"
	openaicompatible "github.com/grafana/ai-sdk/providers/openai-compatible"
	openaioption "github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_SettingsNativeRequests(t *testing.T) {
	for _, tc := range []struct {
		name     string
		target   execution.Target
		model    func(string) provider.LanguageModel
		response string
		want     string
	}{
		{
			name: "anthropic", target: execution.Target{Type: execution.TargetAnthropic},
			model: func(url string) provider.LanguageModel {
				return anthropic.New("test", "claude-sonnet-4-6", anthropic.WithRequestOptions(anthropicoption.WithBaseURL(url), anthropicoption.WithMaxRetries(0)))
			},
			response: `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`,
			want:     `{"output_config":{"effort":"high"},"tool_choice":{"type":"auto","disable_parallel_tool_use":true},"max_tokens":8192}`,
		},
		{
			name: "openai", target: execution.Target{Type: execution.TargetOpenAI},
			model: func(url string) provider.LanguageModel {
				return openai.NewResponses("test", "gpt-5", openai.WithRequestOptions(openaioption.WithBaseURL(url), openaioption.WithMaxRetries(0)))
			},
			response: `{"id":"resp_1","model":"gpt-5","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`,
			want:     `{"parallel_tool_calls":false,"max_output_tokens":8192}`,
		},
		{
			name: "compatible", target: execution.Target{Type: execution.TargetOpenAICompatible, ProviderName: "vendor.chat"},
			model: func(url string) provider.LanguageModel {
				return openaicompatible.New("model", openaicompatible.WithAPIKey("test"), openaicompatible.WithBaseURL(url), openaicompatible.WithProviderName("vendor.chat"))
			},
			response: `{"id":"chat_1","model":"model","choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
			want:     `{"parallel_tool_calls":false,"max_tokens":8192}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan map[string]json.RawMessage, 1)
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]json.RawMessage
				if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				requests <- body
				if tc.target.Type == execution.TargetOpenAICompatible && body["reasoning_effort"] != nil {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"error":{"message":"reasoning_effort is not supported","type":"invalid_request_error"}}`))
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer backend.Close()
			h := settingsHandler(t, execution.Translate(tc.model(backend.URL), tc.target))
			settingsRequest(t, h, `,"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{}}}],"output_config":{"effort":"high"},"tool_choice":{"type":"auto","disable_parallel_tool_use":true}`, false)
			require.Len(t, requests, 1)
			got := <-requests
			var want map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(tc.want), &want))
			for key, value := range want {
				require.Contains(t, got, key)
				assert.JSONEq(t, string(value), string(got[key]), key)
			}
			assert.NotContains(t, got, "thinking")
			if tc.target.Type != execution.TargetAnthropic {
				assert.NotContains(t, got, "output_config")
				assert.NotContains(t, got, "reasoning_effort")
				if reasoning := got["reasoning"]; reasoning != nil {
					var values map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(reasoning, &values))
					assert.NotContains(t, values, "effort")
				}
			}
		})
	}
}

func TestMessagesTranslator_PreservesOptions(t *testing.T) {
	for _, existing := range []provider.ProviderOption{
		anthropic.AnthropicOptions{Effort: "low", Thinking: &anthropic.ThinkingConfig{Type: anthropic.ThinkingAdaptive}},
		provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"effort":"low","thinking":{"type":"adaptive"},"opaque":9007199254740993}`)},
	} {
		original := provider.BuildProviderOptions(existing)
		before, err := json.Marshal(original)
		require.NoError(t, err)
		in, options, err := mapRequest([]byte(`{"model":"alias","max_tokens":8,"messages":[{"role":"user","content":"hi"}],"tool_choice":{"type":"auto","disable_parallel_tool_use":false}}`))
		require.NoError(t, err)
		options.ProviderOptions = original
		translated, err := messagesTranslator(in)(execution.Target{Type: execution.TargetAnthropic}, options)
		require.NoError(t, err)
		after, err := json.Marshal(original)
		require.NoError(t, err)
		assert.Equal(t, string(before), string(after))
		namespace, ok, err := provider.ResolveOption[map[string]json.RawMessage](translated.ProviderOptions, "anthropic")
		require.NoError(t, err)
		require.True(t, ok)
		assert.JSONEq(t, `"low"`, string(namespace["effort"]))
		assert.JSONEq(t, `{"type":"adaptive"}`, string(namespace["thinking"]))
		assert.Equal(t, "false", string(namespace["disableParallelToolUse"]))
		if opaque, ok := namespace["opaque"]; ok {
			assert.Equal(t, "9007199254740993", string(opaque))
		}
	}
}
