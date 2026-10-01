package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/bedrock/mantle"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mantleTransportFunc func(*http.Request) (*http.Response, error)

func (f mantleTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestMantle_TransportContext(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", streaming), func(t *testing.T) {
			var bodies [][]byte
			client := &http.Client{Transport: mantleTransportFunc(func(r *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					return nil, err
				}
				bodies = append(bodies, body)
				assert.Contains(t, r.Header.Get("Authorization"), "/us-west-2/bedrock-mantle/aws4_request")
				assert.Contains(t, r.Header.Get("Authorization"), "x-sdk-call")
				assert.Equal(t, "test-session", r.Header.Get("X-Amz-Security-Token"))
				hash := sha256.Sum256(body)
				assert.Equal(t, hex.EncodeToString(hash[:]), r.Header.Get("X-Amz-Content-Sha256"))
				status := http.StatusOK
				contentType := "application/json"
				responseBody := `{"id":"resp_1","model":"gpt-4o","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
				if len(bodies) == 1 {
					status = http.StatusInternalServerError
					responseBody = `{"error":{"message":"retry","type":"server_error"}}`
				} else if streaming {
					contentType = "text/event-stream"
					responseBody = fmt.Sprintf("data: {\"type\":\"response.completed\",\"response\":%s}\n\ndata: [DONE]\n\n", responseBody)
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}, "X-Mantle": {"final"}}, Body: io.NopCloser(strings.NewReader(responseBody)), Request: r}, nil
			})}
			m, err := mantle.NewResponses(t.Context(), "openai.gpt-4o", mantle.Config{AWSRegion: "us-west-2", AWSAccessKeyID: "test-key", AWSSecretAccessKey: "test-secret", AWSSessionToken: "test-session"}, option.WithHTTPClient(client), option.WithMaxRetries(1), option.WithJSONSet("metadata.capture", "mantle"))
			require.NoError(t, err)
			var request *provider.RequestMetadata
			var responseHeaders map[string]string
			opts := provider.CallOptions{Headers: map[string]string{"X-SDK-Call": "call"}}
			if streaming {
				result, err := m.DoStream(t.Context(), opts)
				require.NoError(t, err)
				request = result.Request
				responseHeaders = result.Response.Headers
				for range result.Stream {
				}
			} else {
				result, err := m.DoGenerate(t.Context(), opts)
				require.NoError(t, err)
				request = result.Request
				responseHeaders = result.Response.Headers
			}
			require.Len(t, bodies, 2)
			require.NotNil(t, request)
			assert.JSONEq(t, string(bodies[1]), string(request.Body))
			assert.JSONEq(t, string(bodies[0]), string(bodies[1]))
			var wire map[string]any
			require.NoError(t, json.Unmarshal(request.Body, &wire))
			assert.Equal(t, map[string]any{"capture": "mantle"}, wire["metadata"])
			assert.Equal(t, "final", responseHeaders["X-Mantle"])
		})
	}
}
