package grafana

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_BYOKProjection(t *testing.T) {
	const gatewayOptions = `{"byok":{"anthropic":[{"apiKey":"dummy-anthropic"}],"openai":[{"apiKey":"dummy-openai-first"},{"apiKey":"dummy-openai-second"}]}}`
	const expected = `{"prompt":[],"providerOptions":{"gateway":` + gatewayOptions + `,"openai":{"store":false}}}`
	for _, streaming := range []bool{false, true} {
		t.Run(strconv.FormatBool(streaming), func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/api/v1/aisdk/language-model", r.URL.Path)
				assert.Equal(t, []string{"Bearer 123:dummy-cap"}, r.Header.Values("Authorization"))
				assert.Equal(t, "openai/model-not-in-catalog", r.Header.Get("Ai-Language-Model-Id"))
				assert.Equal(t, strconv.FormatBool(streaming), r.Header.Get("Ai-Language-Model-Streaming"))
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.JSONEq(t, expected, string(body))
				assert.NotContains(t, string(body), "dummy-cap")
				if streaming {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"type\":\"stream-start\"}\n\ndata: [DONE]\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, unaryFixture)
				}
			}))
			t.Cleanup(srv.Close)
			p, err := NewWithCloudCredentials(CloudCredentialsConfig{StackID: 123, CAPToken: "dummy-cap", BaseURL: srv.URL + "/api/v1/aisdk"})
			require.NoError(t, err)
			model, err := p.LanguageModel("openai/model-not-in-catalog")
			require.NoError(t, err)
			opts := provider.CallOptions{Prompt: []provider.Message{}, ProviderOptions: provider.ProviderOptions{
				"gateway": provider.RawProviderOption{Raw: json.RawMessage(gatewayOptions)},
				"openai":  provider.RawProviderOption{Raw: json.RawMessage(`{"store":false}`)},
			}}
			var requestBody []byte
			if streaming {
				result, err := model.DoStream(context.Background(), opts)
				require.NoError(t, err)
				for range result.Stream {
				}
				require.NotNil(t, result.Request)
				requestBody = result.Request.Body
			} else {
				result, err := model.DoGenerate(context.Background(), opts)
				require.NoError(t, err)
				require.NotNil(t, result.Request)
				requestBody = result.Request.Body
			}
			assert.JSONEq(t, expected, string(requestBody))
			assert.Equal(t, int32(1), calls.Load())
		})
	}
}
