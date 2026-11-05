package openai

import (
	"encoding/json"
	"fmt"
	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNativeResponseMetadataMapOptIn(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, streaming := range []bool{false, true} {
			for _, value := range []string{`{}`, `{"tag":"native"}`} {
				body := fmt.Sprintf(`{"id":"r","model":"gpt-4o","status":"completed","output":[],"metadata":%s,"usage":{"input_tokens":1,"output_tokens":0,"total_tokens":1}}`, value)
				client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					request, _ := io.ReadAll(req.Body)
					assert.NotContains(t, string(request), "preserveResponseMetadata")
					data := body
					contentType := "application/json"
					if streaming {
						data = "data: {\"type\":\"response.completed\",\"sequence_number\":0,\"response\":" + body + "}\n\n"
						contentType = "text/event-stream"
					}
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(data)), Request: req}, nil
				})}
				model := NewResponses("test", "gpt-4o", WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
				opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}, ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{PreserveResponseMetadata: enabled})}
				var metadata provider.ProviderMetadata
				if streaming {
					result, err := model.DoStream(t.Context(), opts)
					require.NoError(t, err)
					for p := range result.Stream {
						require.NotEqual(t, provider.PartError, p.Type)
						if p.Type == provider.PartFinish {
							metadata = p.ProviderMetadata
						}
					}
				} else {
					result, err := model.DoGenerate(t.Context(), opts)
					require.NoError(t, err)
					metadata = result.ProviderMetadata
				}
				parsed, err := ParseResponsesMetadata(metadata, "openai")
				require.NoError(t, err)
				if enabled {
					require.NotNil(t, parsed.Metadata)
					raw, err := json.Marshal(parsed.Metadata)
					require.NoError(t, err)
					assert.JSONEq(t, value, string(raw))
				} else {
					assert.Nil(t, parsed.Metadata)
				}
			}
		}
	}
	for _, value := range []string{`{"tag":1}`, `{"tag":"` + strings.Repeat("x", 513) + `"}`} {
		var response responses.Response
		require.NoError(t, json.Unmarshal([]byte(`{"metadata":`+value+`}`), &response))
		_, err := responseUserMetadata(&response)
		require.Error(t, err)
	}
}
