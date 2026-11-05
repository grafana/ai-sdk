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

func TestExtendedNativeParameters(t *testing.T) {
	for _, suffix := range []string{`,"prediction":{"type":"content","content":"hello"}`, `,"prediction":{"type":"content","content":[{"type":"text","text":"hello","prompt_cache_breakpoint":{"mode":"explicit","ttl":"12h"}}]}`, `,"web_search_options":{"search_context_size":"high","user_location":{"type":"approximate","approximate":{"city":"Recife"}}}`} {
		r := requestWith(t, suffix)
		opts, err := chatTranslator(r)(execution.Target{Type: execution.TargetOpenAICompatible, ProviderName: "custom.chat"}, r.options)
		require.NoError(t, err)
		data := opts.ProviderOptions["custom"].(provider.RawProviderOption).Raw
		var got map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(data, &got))
		var expected map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte("{"+suffix[1:]+"}"), &expected))
		for key, value := range expected {
			assert.JSONEq(t, string(value), string(got[key]))
		}
		for _, backend := range []execution.TargetType{execution.TargetOpenAI, execution.TargetAnthropic} {
			_, err = chatTranslator(r)(execution.Target{Type: backend}, r.options)
			require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality)
		}
	}
	for _, backend := range []execution.TargetType{execution.TargetOpenAICompatible, execution.TargetOpenAI} {
		r := requestWith(t, `,"verbosity":"low"`)
		opts, err := chatTranslator(r)(execution.Target{Type: backend, ProviderName: "openai"}, r.options)
		require.NoError(t, err)
		data, err := json.Marshal(opts.ProviderOptions)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"textVerbosity":"low"`)
	}
	for _, suffix := range []string{`,"verbosity":"verbose"`, `,"prediction":{"type":"content","content":[]}`, `,"prediction":{"type":"content","content":[{"type":"text","text":"a","id":"unsafe"}]}`, `,"web_search_options":{"user_location":{"type":"exact","approximate":{}}}`} {
		_, err := mapRequest([]byte(basic[:len(basic)-1] + suffix + "}"))
		require.Error(t, err)
	}
}

func TestOfficialGoExtendedParametersNativeTransport(t *testing.T) {
	native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
		assert.JSONEq(t, `{"type":"content","content":"hello"}`, string(body["prediction"]))
		assert.JSONEq(t, `{"search_context_size":"high"}`, string(body["web_search_options"]))
		assert.JSONEq(t, `"low"`, string(body["verbosity"]))
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello"},"finish_reason":"stop"}]}`))
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
	require.NoError(t, json.Unmarshal([]byte(basic[:len(basic)-1]+`,"prediction":{"type":"content","content":"hello"},"web_search_options":{"search_context_size":"high"},"verbosity":"low"}`), &params))
	result, err := client.Chat.Completions.New(t.Context(), params)
	require.NoError(t, err)
	assert.Equal(t, "hello", result.Choices[0].Message.Content)
}
