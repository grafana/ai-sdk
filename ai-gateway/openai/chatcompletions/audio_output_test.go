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
	compatible "github.com/grafana/ai-sdk/providers/openai-compatible"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Synthetic native transport; the official client reads real gateway HTTP/SSE.
func TestOfficialClientAudioOutput(t *testing.T) {
	native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Stream     bool     `json:"stream"`
			Modalities []string `json:"modalities"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, []string{"audio"}, body.Modalities)
		if body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: " + `{"id":"audio-response","choices":[{"index":0,"delta":{"audio":{"id":"audio-1","data":"AAEC","transcript":"hello","expires_at":2000000000}},"finish_reason":null}]}` + "\n\ndata: " + `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n"))
			return
		}
		_, _ = w.Write([]byte(`{"id":"audio-response","choices":[{"index":0,"message":{"role":"assistant","content":null,"audio":{"id":"audio-1","data":"AAEC","transcript":"hello","expires_at":2000000000}},"finish_reason":"stop"}]}`))
	}))
	defer native.Close()
	model := compatible.New("native", compatible.WithBaseURL(native.URL), compatible.WithProviderName("custom.chat"))
	runtime, err := execution.New(execution.Config{Limits: execution.DefaultLimits, Selector: func(context.Context, string, provider.CallOptions, json.RawMessage) (execution.Selection, error) {
		return execution.Selection{ID: "canonical", Model: execution.Translate(model, execution.Target{Type: execution.TargetOpenAICompatible, ProviderName: "custom.chat"})}, nil
	}})
	require.NoError(t, err)
	h, err := New(runtime)
	require.NoError(t, err)
	server := httptest.NewServer(h)
	defer server.Close()
	client := openai.NewClient(option.WithBaseURL(server.URL+"/v1"), option.WithAPIKey("dummy"), option.WithMaxRetries(0))
	params := openai.ChatCompletionNewParams{Model: "alias", Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("speak")}}
	opts := []option.RequestOption{option.WithJSONSet("modalities", []string{"audio"}), option.WithJSONSet("audio", map[string]string{"format": "pcm16", "voice": "alloy"})}
	result, err := client.Chat.Completions.New(t.Context(), params, opts...)
	require.NoError(t, err)
	assert.Equal(t, "AAEC", result.Choices[0].Message.Audio.Data)
	assert.Equal(t, "audio-1", result.Choices[0].Message.Audio.ID)
	stream := client.Chat.Completions.NewStreaming(t.Context(), params, opts...)
	defer func() { _ = stream.Close() }()
	var raw strings.Builder
	for stream.Next() {
		raw.WriteString(stream.Current().RawJSON())
	}
	require.NoError(t, stream.Err())
	assert.Contains(t, raw.String(), `"audio":{"id":"audio-1"`)
	assert.Contains(t, raw.String(), `"data":"AAEC"`)
	assert.Contains(t, raw.String(), `"finish_reason":"stop"`)
}
