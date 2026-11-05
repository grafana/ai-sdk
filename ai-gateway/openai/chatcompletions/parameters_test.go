package chatcompletions

import (
	"encoding/json"
	openaiprovider "github.com/grafana/ai-sdk/providers/openai"
	"github.com/openai/openai-go/v3/option"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	compatible "github.com/grafana/ai-sdk/providers/openai-compatible"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatScalarNativeMapping(t *testing.T) {
	r := requestWith(t, `,"user":"u","safety_identifier":"safe","service_tier":"priority","prompt_cache_key":"cache","prompt_cache_retention":"24h","metadata":{"tag":"test"},"logprobs":true,"top_logprobs":2`)
	for _, tc := range []struct {
		target    execution.Target
		key, want string
	}{
		{execution.Target{Type: execution.TargetOpenAICompatible, ProviderName: "custom.chat"}, "custom", `{"user":"u","safety_identifier":"safe","service_tier":"priority","prompt_cache_key":"cache","prompt_cache_retention":"24h","metadata":{"tag":"test"},"logprobs":true,"top_logprobs":2,"store":false,"strictJsonSchema":false,"preserveChatCompletionMetadata":true}`},
		{execution.Target{Type: execution.TargetOpenAI}, "openai", `{"user":"u","safetyIdentifier":"safe","serviceTier":"priority","promptCacheKey":"cache","promptCacheRetention":"24h","metadata":{"tag":"test"},"logprobs":2,"store":false,"strictJsonSchema":false}`},
	} {
		options, err := chatTranslator(r)(tc.target, r.options)
		require.NoError(t, err)
		raw := options.ProviderOptions[tc.key].(provider.RawProviderOption).Raw
		assert.JSONEq(t, tc.want, string(raw))
	}
	bias := requestWith(t, `,"logit_bias":{"123":-100,"456":100}`)
	_, err := chatTranslator(bias)(execution.Target{Type: execution.TargetOpenAI}, bias.options)
	require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality)
	options, err := chatTranslator(bias)(execution.Target{Type: execution.TargetOpenAICompatible, ProviderName: "custom.chat"}, bias.options)
	require.NoError(t, err)
	raw, err := json.Marshal(options.ProviderOptions["custom"])
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"logit_bias":{"123":-100,"456":100}`)
	for _, suffix := range []string{`,"user":"u"`, `,"metadata":{"tag":"test"}`, `,"logprobs":true`, `,"logit_bias":{"1":1}`} {
		r := requestWith(t, suffix)
		_, err := chatTranslator(r)(execution.Target{Type: execution.TargetAnthropic}, r.options)
		require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality)
	}
}

func TestChatScalarBoundary(t *testing.T) {
	for _, suffix := range []string{`,"top_logprobs":0`, `,"logprobs":false,"top_logprobs":1`, `,"logprobs":true,"top_logprobs":21`, `,"metadata":{"x":1}`, `,"logit_bias":{"word":1}`, `,"logit_bias":{"1":101}`, `,"service_tier":"invented"`} {
		_, err := mapRequest([]byte(basic[:len(basic)-1] + suffix + "}"))
		require.Error(t, err, suffix)
	}
	for _, suffix := range []string{`,"logprobs":true`, `,"logprobs":true,"top_logprobs":0`} {
		r := requestWith(t, suffix)
		options, err := chatTranslator(r)(execution.Target{Type: execution.TargetOpenAI}, r.options)
		require.NoError(t, err)
		raw, err := json.Marshal(options.ProviderOptions["openai"])
		require.NoError(t, err)
		assert.Contains(t, string(raw), `"logprobs":1`)
	}
}

func TestChatLogprobProjection(t *testing.T) {
	raw := json.RawMessage(`{"logprobs":[[{"token":"a","logprob":-0.1,"top_logprobs":[{"token":"b","logprob":-2}]}],[{"token":"c","logprob":-0.2,"top_logprobs":[]}]]}`)
	r := requestWith(t, `,"logprobs":true`)
	projected, err := r.projectLogprobs(provider.ProviderMetadata{"openai": raw}, nil)
	require.NoError(t, err)
	require.Len(t, projected.Content, 2)
	assert.Empty(t, projected.Content[0].TopLogprobs)
	assert.Nil(t, projected.Content[0].Bytes)
	native := &compatible.ChatCompletionLogprobs{Content: []compatible.ChatCompletionTokenLogprob{{ChatCompletionTokenProbability: compatible.ChatCompletionTokenProbability{Token: ptr("a"), Logprob: ptr(-0.1)}, TopLogprobs: []compatible.ChatCompletionTokenProbability{{Token: ptr("b"), Logprob: ptr(-2.0)}}}}}
	projected, err = r.projectLogprobs(nil, native)
	require.NoError(t, err)
	assert.Empty(t, projected.Content[0].TopLogprobs)
	assert.Len(t, native.Content[0].TopLogprobs, 1)
	r.TopLogprobs = ptr(1)
	projected, err = r.projectLogprobs(nil, native)
	require.NoError(t, err)
	assert.Len(t, projected.Content[0].TopLogprobs, 1)
	_, err = r.projectLogprobs(provider.ProviderMetadata{"openai": json.RawMessage(`{"logprobs":[[{"token":"a","logprob":1}]]}`)}, nil)
	require.Error(t, err)
	r.Logprobs = ptr(false)
	projected, err = r.projectLogprobs(nil, native)
	require.NoError(t, err)
	assert.Nil(t, projected)
}

func TestChatParametersReachNativeRequest(t *testing.T) {
	for _, target := range []execution.TargetType{execution.TargetOpenAI, execution.TargetOpenAICompatible} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			var body map[string]json.RawMessage
			require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
			for key, want := range map[string]string{"user": `"u"`, "safety_identifier": `"safe"`, "service_tier": `"auto"`, "prompt_cache_key": `"cache"`, "metadata": `{"tag":"test"}`, "top_logprobs": `2`} {
				assert.JSONEq(t, want, string(body[key]), key)
			}
			assert.NotContains(t, body, "preserveChatCompletionMetadata")
			w.Header().Set("Content-Type", "application/json")
			if target == execution.TargetOpenAI {
				_, _ = w.Write([]byte(`{"id":"r","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))
			} else {
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]} `))
			}
		}))
		r := requestWith(t, `,"user":"u","safety_identifier":"safe","service_tier":"auto","prompt_cache_key":"cache","metadata":{"tag":"test"},"logprobs":true,"top_logprobs":2`)
		options, err := chatTranslator(r)(execution.Target{Type: target, ProviderName: "custom.chat"}, r.options)
		require.NoError(t, err)
		var model provider.LanguageModel
		if target == execution.TargetOpenAI {
			model = openaiprovider.NewResponses("test", "gpt-4o", openaiprovider.WithRequestOptions(option.WithBaseURL(server.URL)))
		} else {
			model = compatible.New("test", compatible.WithBaseURL(server.URL), compatible.WithProviderName("custom.chat"))
		}
		_, err = model.DoGenerate(t.Context(), options)
		require.NoError(t, err)
		server.Close()
	}
}
