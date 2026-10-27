package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStructuredOutput_NativeSchemaNumbers(t *testing.T) {
	for _, mode := range []StructuredOutputMode{StructuredOutputFormat, StructuredOutputJSONTool} {
		for _, streaming := range []bool{false, true} {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				for _, token := range []string{`"const":9007199254740993`, `"enum":[9007199254740993,-9007199254740993,1.234567890123456789,1e100]`, `"default":9007199254740993`} {
					assert.Contains(t, string(body), token)
				}
				if mode == StructuredOutputFormat {
					assert.Contains(t, string(body), `"description":"minimum: 9007199254740993."`)
				} else {
					assert.Contains(t, string(body), `"minimum":9007199254740993`)
				}
				assert.NotContains(t, string(body), "9007199254740992")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"captured","type":"invalid_request_error"}}`))
			}))
			model := New("test", "claude-sonnet-4-5", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))
			opts := provider.CallOptions{MaxOutputTokens: new(64), ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(`{"type":"object","properties":{"value":{"type":"integer","const":9007199254740993,"enum":[9007199254740993,-9007199254740993,1.234567890123456789,1e100],"minimum":9007199254740993,"default":9007199254740993}}}`)}, ProviderOptions: provider.BuildProviderOptions(AnthropicOptions{StructuredOutputMode: mode})}
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
}

func TestStructuredOutput_RejectsTrailingSchemaJSON(t *testing.T) {
	for _, raw := range []string{`{} {}`, `{} true`, `{} garbage`, `null`} {
		params, _, warnings, _, err := buildParams("claude-sonnet-4-5", provider.CallOptions{MaxOutputTokens: new(64), ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(raw)}}, false)
		require.NoError(t, err)
		assert.Nil(t, params.OutputConfig.Format.Schema)
		require.NotEmpty(t, warnings)
		assert.Contains(t, warnings[0].Details, "failed to parse response format schema")
	}
}
