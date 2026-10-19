package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStructuredOutput_NativeSchemaNumbers(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			for _, token := range []string{`"const":9007199254740993`, `"enum":[9007199254740993,-9007199254740993,1.234567890123456789,1e100]`, `"minimum":9007199254740993`, `"default":9007199254740993`} {
				assert.Contains(t, string(body), token)
			}
			assert.NotContains(t, string(body), "9007199254740992")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":{"message":"captured","type":"invalid_request_error"}}`))
		}))
		model := NewResponses("test", "gpt-4o", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))
		opts := provider.CallOptions{ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(`{"type":"object","properties":{"value":{"type":"integer","const":9007199254740993,"enum":[9007199254740993,-9007199254740993,1.234567890123456789,1e100],"minimum":9007199254740993,"default":9007199254740993}}}`)}}
		if streaming {
			_, err := model.DoStream(context.Background(), opts)
			var apiError *provider.APICallError
			require.ErrorAs(t, err, &apiError)
			assert.Equal(t, http.StatusBadRequest, apiError.StatusCode)
		} else {
			_, err := model.DoGenerate(context.Background(), opts)
			var apiError *provider.APICallError
			require.ErrorAs(t, err, &apiError)
			assert.Equal(t, http.StatusBadRequest, apiError.StatusCode)
		}
		server.Close()
		assert.EqualValues(t, 1, calls.Load())
	}
}
func TestNormalizeOpenAIJSONSchema_RejectsTrailingJSON(t *testing.T) {
	for _, raw := range []string{`{} {}`, `{} true`, `{} garbage`} {
		_, _, err := normalizeOpenAIJSONSchema(json.RawMessage(raw))
		require.Error(t, err)
	}
}
