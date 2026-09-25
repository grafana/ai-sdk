package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	vertexsdk "github.com/anthropics/anthropic-sdk-go/vertex"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func TestModel_CallHeaders(t *testing.T) {
	for _, vertex := range []bool{false, true} {
		for _, streaming := range []bool{false, true} {
			name := "direct"
			if vertex {
				name = "vertex"
			}
			if streaming {
				name += "/stream"
			} else {
				name += "/generate"
			}
			t.Run(name, func(t *testing.T) {
				requests := make(chan http.Header, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests <- r.Header.Clone()
					if streaming {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-test\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n")
						_, _ = fmt.Fprint(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}\n\n")
						_, _ = fmt.Fprint(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, `{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"text","text":"Hello"}],"model":"claude-test","stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`)
				}))
				defer server.Close()

				m := newTransportTestModel(t, server, vertex,
					option.WithHeader("X-Shared", "configured"),
					option.WithHeader("X-Configured", "present"),
					option.WithHeader("anthropic-beta", "CONFIG-beta1,config-beta2"),
				)

				maxTokens := 64
				params := provider.CallOptions{
					Prompt:          []provider.Message{provider.UserText("hello")},
					MaxOutputTokens: &maxTokens,
					ProviderOptions: provider.BuildProviderOptions(AnthropicOptions{Betas: []string{"sdk-beta"}}),
					Headers: map[string]string{
						"x-shared": "per-call", "X-Agent-Marker": "agent-marker",
						"anthropic-beta": "REQUEST-beta1,config-beta2",
					},
				}
				if streaming {
					result, err := m.DoStream(context.Background(), params)
					require.NoError(t, err)
					for range result.Stream {
					}
				} else {
					_, err := m.DoGenerate(context.Background(), params)
					require.NoError(t, err)
				}
				select {
				case headers := <-requests:
					assert.Equal(t, "per-call", headers.Get("X-Shared"))
					assert.Equal(t, "agent-marker", headers.Get("X-Agent-Marker"))
					assert.Equal(t, "present", headers.Get("X-Configured"))
					assert.Equal(t, "config-beta1,config-beta2,request-beta1,sdk-beta", strings.ToLower(strings.Join(headers.Values("anthropic-beta"), ",")))
					if vertex {
						assert.Equal(t, "Bearer vertex-token", headers.Get("Authorization"))
					} else {
						assert.Equal(t, "direct-key", headers.Get("X-Api-Key"))
					}
				default:
					t.Fatal("request not received")
				}
			})
		}
	}
}

func newTransportTestModel(t *testing.T, server *httptest.Server, vertex bool, extras ...option.RequestOption) provider.LanguageModel {
	t.Helper()
	requestOpts := append([]option.RequestOption{
		option.WithBaseURL(server.URL),
		option.WithMaxRetries(0),
	}, extras...)
	if !vertex {
		requestOpts = append(requestOpts, option.WithHTTPClient(server.Client()))
		return New("direct-key", "claude-test", WithRequestOptions(requestOpts...))
	}
	original := vertexGoogleAuth
	t.Cleanup(func() { vertexGoogleAuth = original })
	vertexGoogleAuth = func(ctx context.Context, location, projectID string, _ ...string) option.RequestOption {
		creds := &google.Credentials{TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "vertex-token"})}
		return vertexsdk.WithCredentials(ctx, location, projectID, creds)
	}
	m, err := NewVertex(context.Background(), "us-east5", "test-project", "claude-test", WithRequestOptions(requestOpts...))
	require.NoError(t, err)
	return m
}
