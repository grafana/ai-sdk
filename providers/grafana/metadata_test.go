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

func TestProviderMetadata_DecodeScopes(t *testing.T) {
	for _, metadata := range []string{"", `{}`, `{"future":{"nested":[null,false,0,"",[],{}],"token":"sk-application-data"}}`} {
		suffix := ""
		if metadata != "" {
			suffix = `,"providerMetadata":` + metadata
		}
		for _, content := range []string{
			`"type":"text","text":"hello"`,
			`"type":"reasoning","text":""`,
			`"type":"reasoning-file","mediaType":"image/png","data":{"type":"data","data":""}`,
			`"type":"source","sourceType":"url","id":"native","url":"https://example.test"`,
			`"type":"tool-call","toolCallId":"call","toolName":"weather","input":"{}"`,
		} {
			result, err := decodeGenerate([]byte(`{"content":[{` + content + suffix + `}],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}` + suffix + `}`))
			require.NoError(t, err)
			require.Len(t, result.Content, 1)
			assertDecodedMetadata(t, metadata, result.ProviderMetadata)
			assertDecodedMetadata(t, metadata, result.Content[0].ProviderMetadata)
		}
		for _, event := range metadataStreamEvents() {
			part, err := decodeStreamPart([]byte(`{` + event + suffix + `}`))
			require.NoError(t, err)
			if part.Type == provider.PartSource {
				assertDecodedMetadata(t, metadata, part.Source.ProviderMetadata)
			} else {
				assertDecodedMetadata(t, metadata, part.ProviderMetadata)
			}
		}
	}
}

func metadataStreamEvents() []string {
	return []string{
		`"type":"text-start","id":"text"`, `"type":"text-delta","id":"text","delta":""`, `"type":"text-end","id":"text"`,
		`"type":"reasoning-start","id":"r"`, `"type":"reasoning-delta","id":"r","delta":""`, `"type":"reasoning-end","id":"r"`,
		`"type":"reasoning-file","mediaType":"image/png","data":{"type":"data","data":""}`,
		`"type":"source","sourceType":"url","id":"native","url":"https://example.test"`,
		`"type":"tool-input-start","id":"call","toolName":"weather"`, `"type":"tool-input-delta","id":"call","delta":""`, `"type":"tool-input-end","id":"call"`,
		`"type":"tool-call","toolCallId":"call","toolName":"weather","input":"{}"`,
		`"type":"tool-result","toolCallId":"call","toolName":"weather","result":{}`,
		`"type":"finish","finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}`,
	}
}

func assertDecodedMetadata(t *testing.T, want string, metadata provider.ProviderMetadata) {
	t.Helper()
	if want == "" {
		assert.Nil(t, metadata)
		return
	}
	require.NotNil(t, metadata)
	raw, err := json.Marshal(metadata)
	require.NoError(t, err)
	assert.JSONEq(t, want, string(raw))
}

func TestProviderMetadata_MalformedScopes(t *testing.T) {
	for _, metadata := range []string{
		`null`, `[]`, `false`, `{"future":null}`, `{"future":[]}`, `{"future":0}`,
		"{\"" + string([]byte{255}) + "\":{}}",
		"{\"future\":{\"value\":\"" + string([]byte{255}) + "\"}}",
	} {
		t.Run(metadata, func(t *testing.T) {
			_, err := decodeGenerate([]byte(`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"providerMetadata":` + metadata + `}`))
			require.Error(t, err)
			_, err = decodeGenerate([]byte(`{"content":[{"type":"text","text":"hello","providerMetadata":` + metadata + `}],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}}`))
			require.Error(t, err)
			for i, event := range metadataStreamEvents() {
				t.Run(fmt.Sprint(i), func(t *testing.T) {
					_, err := decodeStreamPart([]byte(`{` + event + `,"providerMetadata":` + metadata + `}`))
					require.Error(t, err)
				})
			}
		})
	}
}

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
