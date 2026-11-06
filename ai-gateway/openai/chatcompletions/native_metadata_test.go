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

func TestNativeChatMetadata_Projection(t *testing.T) {
	native := provider.ProviderMetadata{"custom": json.RawMessage(`{"chatCompletion":{"refusal":"Cannot comply","logprobs":{"content":null,"refusal":[{"token":"Cannot","logprob":-0.1,"bytes":null,"top_logprobs":[]}]}}}`)}
	rawUsage := json.RawMessage(`{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5,"prompt_tokens_details":{"audio_tokens":1,"cache_write_tokens":1},"completion_tokens_details":{"audio_tokens":1,"accepted_prediction_tokens":0,"rejected_prediction_tokens":1}}`)
	for _, streaming := range []bool{false, true} {
		f := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			r := textResult()
			r.Content = nil
			r.ProviderMetadata = native
			r.Usage.Raw = rawUsage
			return r, nil
		}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			parts := textParts()
			parts[4].Usage.Raw = rawUsage
			return partsStream(parts[0], provider.StreamPart{Type: provider.PartFinish, FinishReason: parts[4].FinishReason, Usage: parts[4].Usage, ProviderMetadata: native}), nil
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

func TestNativeUsage_RawDetails(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		invalid   bool
	}{
		{"absent", `{}`, false},
		{"explicit zero", `{"prompt_tokens_details":{"audio_tokens":0,"cache_write_tokens":0},"completion_tokens_details":{"accepted_prediction_tokens":0}}`, false},
		{"negative", `{"prompt_tokens_details":{"audio_tokens":-1}}`, true},
		{"above total", `{"prompt_tokens_details":{"cache_write_tokens":4}}`, true},
		{"mismatched totals", `{"prompt_tokens":5}`, true},
		{"invalid type", `{"completion_tokens_details":{"audio_tokens":"x"}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := &usage{Prompt: 3, Completion: 2, Total: 5}
			err := nativeUsage(u, json.RawMessage(tc.raw))
			if tc.invalid {
				require.ErrorIs(t, err, errOutput)
				return
			}
			require.NoError(t, err)
			if tc.name == "explicit zero" {
				require.NotNil(t, u.PromptDetails)
				require.NotNil(t, u.PromptDetails.Audio)
				assert.Zero(t, *u.PromptDetails.Audio)
				assert.Nil(t, u.PromptDetails.Cached)
			}
			if tc.name == "absent" {
				assert.Nil(t, u.PromptDetails)
				assert.Nil(t, u.CompletionDetails)
			}
		})
	}
}
