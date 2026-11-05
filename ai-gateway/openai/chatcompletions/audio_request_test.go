package chatcompletions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	openaicompatible "github.com/grafana/ai-sdk/providers/openai-compatible"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatAudioRequestNative(t *testing.T) {
	raw := `{"model":"alias","messages":[{"role":"assistant","audio":{"id":"audio-previous"},"name":"narrator","content":[{"type":"text","text":"hello"},{"type":"refusal","refusal":"no"}]}],"modalities":["text","audio"],"audio":{"format":"pcm16","voice":{"id":"voice-custom"}}}`
	r, err := mapRequest([]byte(raw))
	require.NoError(t, err)
	options, err := chatTranslator(r)(execution.Target{Type: execution.TargetOpenAICompatible, ProviderName: "custom.chat"}, r.options)
	require.NoError(t, err)
	native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
		assert.JSONEq(t, `["text","audio"]`, string(body["modalities"]))
		assert.JSONEq(t, `{"format":"pcm16","voice":{"id":"voice-custom"}}`, string(body["audio"]))
		assert.JSONEq(t, `[{"role":"assistant","name":"narrator","content":"hello","refusal":"no","audio":{"id":"audio-previous"}}]`, string(body["messages"]))
		assert.NotContains(t, body, "preserveChatCompletionMetadata")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer native.Close()
	model := openaicompatible.New("native", openaicompatible.WithBaseURL(native.URL), openaicompatible.WithProviderName("custom.chat"))
	_, err = model.DoGenerate(t.Context(), options)
	require.NoError(t, err)
}

func TestChatAudioUnsupportedTargetsDoNotInvokeModel(t *testing.T) {
	for _, target := range []execution.TargetType{execution.TargetOpenAI, execution.TargetAnthropic} {
		for _, stream := range []bool{false, true} {
			f := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				t.Error("unexpected native call")
				return nil, nil
			}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				t.Error("unexpected native stream")
				return nil, nil
			}}
			h := testHandler(t, f)
			runtime, err := execution.New(execution.Config{Limits: execution.DefaultLimits, Selector: func(context.Context, string, provider.CallOptions, json.RawMessage) (execution.Selection, error) {
				return execution.Selection{ID: "canonical", Model: execution.Translate(f, execution.Target{Type: target})}, nil
			}})
			require.NoError(t, err)
			h.runtime = runtime
			payload := `{"model":"alias","messages":[{"role":"assistant","audio":{"id":"audio-previous"},"content":null}]}`
			if stream {
				payload = strings.TrimSuffix(payload, "}") + `,"stream":true}`
			}
			req := httptest.NewRequest("POST", Path, strings.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			assert.Equal(t, 400, w.Code, w.Body.String())
		}
	}
}

func TestChatAudioRequestValidation(t *testing.T) {
	for _, suffix := range []string{
		`,"modalities":["audio"]`,
		`,"modalities":["text","text"]`,
		`,"audio":{"format":"wav","voice":{}}`,
		`,"audio":{"format":"bad","voice":"alloy"}`,
		`,"messages":[{"role":"user","audio":{"id":"a"},"content":"x"}]`,
		`,"messages":[{"role":"assistant","audio":{"id":"a","data":"AAEC"}}]`,
	} {
		_, err := mapRequest([]byte(strings.TrimSuffix(basic, "}") + suffix + "}"))
		require.Error(t, err, suffix)
	}
	r, err := mapRequest([]byte(`{"model":"alias","messages":[{"role":"assistant","audio":{"id":"a"}}]}`))
	require.NoError(t, err)
	assert.True(t, r.needsNativeAudio())
	text := requestWith(t, `,"modalities":["text"]`)
	for _, target := range []execution.TargetType{execution.TargetOpenAI, execution.TargetAnthropic} {
		_, err := chatTranslator(text)(execution.Target{Type: target}, text.options)
		require.NoError(t, err)
	}
}
