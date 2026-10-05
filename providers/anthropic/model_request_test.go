package anthropic

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_MessagesRequestTarget(t *testing.T) {
	for _, tc := range []struct {
		name           string
		requestOptions []option.RequestOption
		wantTarget     string
		betas          []string
		wantHeaderBeta string
		wantMetadata   string
	}{
		{name: "default", wantTarget: "/v1/messages"},
		{name: "provider beta headers", wantTarget: "/v1/messages", betas: []string{"test-beta"}},
		{
			name: "caller repeated query and body options",
			requestOptions: []option.RequestOption{
				option.WithQuery("api-version", "v1"),
				option.WithQueryAdd("feature", "a"),
				option.WithQueryAdd("feature", "b"),
				option.WithJSONSet("metadata", map[string]string{"user_id": "test-user"}),
			},
			wantTarget:   "/v1/messages?api-version=v1&feature=a&feature=b",
			betas:        []string{"test-beta"},
			wantMetadata: `{"user_id":"test-user"}`,
		},
		{
			name:           "caller beta query override",
			requestOptions: []option.RequestOption{option.WithQuery("beta", "false")},
			wantTarget:     "/v1/messages?beta=false",
			betas:          []string{"test-beta"},
		},
		{
			name:           "caller beta header",
			requestOptions: []option.RequestOption{option.WithHeader("anthropic-beta", "caller-beta")},
			wantTarget:     "/v1/messages",
			betas:          []string{"test-beta"},
			wantHeaderBeta: "caller-beta",
		},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				type capturedRequest struct {
					method, target string
					headers        http.Header
					body           []byte
					err            error
				}
				requests := make(chan capturedRequest, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					requests <- capturedRequest{r.Method, r.RequestURI, r.Header.Clone(), body, err}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"captured"}}`)
				}))
				defer server.Close()

				requestOptions := []option.RequestOption{
					option.WithBaseURL(server.URL),
					option.WithHTTPClient(server.Client()),
					option.WithMaxRetries(0),
					option.WithHeader("X-Test", "preserved"),
				}
				requestOptions = append(requestOptions, tc.requestOptions...)
				model := New("test-key", "claude-sonnet-4-6", WithRequestOptions(requestOptions...))
				maxTokens := 64
				opts := provider.CallOptions{
					Prompt:          []provider.Message{provider.UserText("hello")},
					MaxOutputTokens: &maxTokens,
					ProviderOptions: provider.BuildProviderOptions(AnthropicOptions{Betas: tc.betas}),
				}
				var err error
				if stream {
					_, err = model.DoStream(t.Context(), opts)
				} else {
					_, err = model.DoGenerate(t.Context(), opts)
				}
				require.ErrorContains(t, err, "captured")
				captured := <-requests
				require.NoError(t, captured.err)
				assert.Equal(t, http.MethodPost, captured.method)
				assert.Equal(t, tc.wantTarget, captured.target)
				assert.Equal(t, "preserved", captured.headers.Get("X-Test"))
				betas := strings.Join(captured.headers.Values("anthropic-beta"), ",")
				for _, beta := range tc.betas {
					assert.Contains(t, betas, beta)
				}
				if tc.wantHeaderBeta != "" {
					assert.Contains(t, betas, tc.wantHeaderBeta)
				}
				if len(tc.betas) == 0 && tc.wantHeaderBeta == "" {
					assert.Empty(t, betas)
				}
				var body map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(captured.body, &body))
				assert.JSONEq(t, `"claude-sonnet-4-6"`, string(body["model"]))
				assert.Equal(t, stream, string(body["stream"]) == "true")
				if tc.wantMetadata != "" {
					assert.JSONEq(t, tc.wantMetadata, string(body["metadata"]))
				}
			})
		}
	}
}
