package chatcompletions

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeChatMetadata(t *testing.T) {
	native := provider.ProviderMetadata{"openaiCompatible": json.RawMessage(`{"chatCompletion":{"refusal":"Cannot comply","logprobs":{"content":null,"refusal":[{"token":"Cannot","logprob":-0.1,"bytes":null,"top_logprobs":[]}]},"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5,"prompt_tokens_details":{"audio_tokens":1},"completion_tokens_details":{"audio_tokens":1,"accepted_prediction_tokens":0,"rejected_prediction_tokens":1}}}}`)}
	for _, streaming := range []bool{false, true} {
		f := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			r := textResult()
			r.Content = nil
			r.ProviderMetadata = native
			return r, nil
		}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			parts := textParts()
			return partsStream(parts[0], provider.StreamPart{Type: provider.PartCustom, Kind: "openai-compatible.chat-completion", ProviderMetadata: native}, provider.StreamPart{Type: provider.PartFinish, FinishReason: parts[4].FinishReason, Usage: parts[4].Usage, ProviderMetadata: native}), nil
		}}
		body := strings.TrimSuffix(basic, "}") + `,"response_format":{"type":"json_object"}`
		if streaming {
			body += `,"stream":true,"stream_options":{"include_usage":true}`
		}
		body += "}"
		req := httptest.NewRequest("POST", Path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		testHandler(t, f).ServeHTTP(w, req)
		require.Equal(t, 200, w.Code)
		assert.NotContains(t, w.Body.String(), `"error"`)
		assert.Contains(t, w.Body.String(), `"refusal":"Cannot comply"`)
		assert.Contains(t, w.Body.String(), `"token":"Cannot"`)
		assert.Contains(t, w.Body.String(), `"accepted_prediction_tokens":0`)
		assert.Contains(t, w.Body.String(), `"audio_tokens":1`)
	}
}
