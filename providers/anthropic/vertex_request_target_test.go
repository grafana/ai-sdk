package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
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

func TestModel_VertexRequestTarget(t *testing.T) {
	const baseURL = "https://us-east5-aiplatform.googleapis.com/v1/projects/project/locations/us-east5/publishers/anthropic/models/claude-sonnet-4-5@20250929:"
	original := vertexGoogleAuth
	t.Cleanup(func() { vertexGoogleAuth = original })
	creds := &google.Credentials{TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "vertex-token"})}
	for _, op := range []struct {
		name      string
		streaming bool
	}{
		{name: "rawPredict"},
		{name: "streamRawPredict", streaming: true},
	} {
		t.Run(op.name, func(t *testing.T) {
			for _, tc := range []struct {
				name    string
				query   string
				options []option.RequestOption
			}{
				{name: "default"},
				{
					name:  "unrelated query",
					query: "?feature=z&feature=a&view=full",
					options: []option.RequestOption{
						option.WithQuery("view", "full"),
						option.WithQueryAdd("feature", "z"),
						option.WithQueryAdd("feature", "a"),
					},
				},
				{
					name:  "explicit beta query",
					query: "?beta=false&beta=true",
					options: []option.RequestOption{
						option.WithQueryAdd("beta", "false"),
						option.WithQueryAdd("beta", "true"),
					},
				},
			} {
				t.Run(tc.name, func(t *testing.T) {
					var requests int
					var body map[string]json.RawMessage
					client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
						requests++
						assert.Equal(t, http.MethodPost, r.Method)
						assert.Equal(t, baseURL+op.name+tc.query, r.URL.String())
						assert.Equal(t, "Bearer vertex-token", r.Header.Get("Authorization"))
						assert.Empty(t, r.Header.Get("X-Api-Key"))
						assert.ElementsMatch(t, []string{"config-beta", "call-beta", "feature-beta"}, strings.Split(r.Header.Get("anthropic-beta"), ","))
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							return nil, err
						}
						return transportResponse(r, op.streaming), nil
					})}
					vertexGoogleAuth = func(ctx context.Context, region, project string, _ ...string) option.RequestOption {
						return option.Join(option.WithHTTPClient(client), vertexsdk.WithCredentials(ctx, region, project, creds))
					}
					requestOptions := append([]option.RequestOption{option.WithMaxRetries(0), option.WithHeader("anthropic-beta", "config-beta")}, tc.options...)
					m, err := NewVertex(t.Context(), "us-east5", "project", "claude-sonnet-4-5-20250929", WithRequestOptions(requestOptions...))
					require.NoError(t, err)
					maxTokens := 64
					call := provider.CallOptions{
						MaxOutputTokens: &maxTokens,
						Prompt:          []provider.Message{provider.UserText("hi")},
						Headers:         map[string]string{"anthropic-beta": "call-beta"},
						ProviderOptions: provider.BuildProviderOptions(AnthropicOptions{Betas: []string{"feature-beta"}}),
					}
					if op.streaming {
						result, err := m.DoStream(t.Context(), call)
						require.NoError(t, err)
						for range result.Stream {
						}
						assert.Equal(t, "true", string(body["stream"]))
					} else {
						_, err := m.DoGenerate(t.Context(), call)
						require.NoError(t, err)
						assert.NotContains(t, body, "stream")
					}
					assert.Equal(t, 1, requests)
					assert.NotContains(t, body, "model")
					assert.Equal(t, `"vertex-2023-10-16"`, string(body["anthropic_version"]))
					assert.Equal(t, "64", string(body["max_tokens"]))
					assert.JSONEq(t, `[{"role":"user","content":[{"type":"text","text":"hi"}]}]`, string(body["messages"]))
				})
			}
		})
	}
}
