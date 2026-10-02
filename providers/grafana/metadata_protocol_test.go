package grafana

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderMetadata_ProtocolFailures(t *testing.T) {
	for i, metadata := range []string{
		`null`, `{"future":null}`, `{"future":[]}`, `{"future":false}`, `{"future":`,
		"{\"future\":{\"offending-value\":\"" + string([]byte{255}) + "\"}}",
		"{\"" + string([]byte{255}) + "\":{\"value\":0}}",
		strings.Repeat(" ", 1025) + `{}`,
	} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/stream=%v", i, streaming), func(t *testing.T) {
				limits := DefaultLimits()
				limits.UnaryBytes, limits.StreamEventBytes = 1024, 1024
				p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
					if streaming {
						w.Header().Set("Content-Type", "text/event-stream")
						_, err := fmt.Fprint(w, sseFrame(`{"type":"stream-start","warnings":[]}`)+sseFrame(`{"type":"finish","finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"providerMetadata":`+metadata+`}`))
						assert.NoError(t, err)
					} else {
						w.Header().Set("Content-Type", "application/json")
						_, err := fmt.Fprint(w, `{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"providerMetadata":`+metadata+`}`)
						assert.NoError(t, err)
					}
				}, &limits)
				model, err := p.LanguageModel("assistant")
				require.NoError(t, err)
				if streaming {
					result, err := model.DoStream(context.Background(), provider.CallOptions{})
					require.NoError(t, err)
					errors := 0
					for part := range result.Stream {
						assert.NotEqual(t, provider.PartFinish, part.Type)
						if part.Type == provider.PartError {
							errors++
							require.NotNil(t, part.APICallError)
							assert.False(t, part.APICallError.IsRetryable)
							assert.NotContains(t, part.APICallError.Error(), "offending-value")
						}
					}
					assert.Equal(t, 1, errors)
				} else {
					result, err := model.DoGenerate(context.Background(), provider.CallOptions{})
					require.Error(t, err)
					assert.Nil(t, result)
					var api *provider.APICallError
					require.ErrorAs(t, err, &api)
					assert.False(t, api.IsRetryable)
					assert.NotContains(t, err.Error(), "offending-value")
				}
			})
		}
	}
}

func TestProviderMetadata_SurrogateEscapes(t *testing.T) {
	result, err := decodeGenerate([]byte(`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"providerMetadata":{"future":{"lone":"\ud800","paired":"\ud83d\ude00"}}}`))
	require.NoError(t, err)
	assert.Contains(t, string(result.ProviderMetadata["future"]), `\ud800`)
	var object map[string]any
	require.NoError(t, json.Unmarshal(result.ProviderMetadata["future"], &object))
	assert.Equal(t, "\U0001f600", object["paired"])
}
