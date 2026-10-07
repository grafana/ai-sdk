package byok

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func TestNew_RequestOnlyAccountsAndDefaultFallback(t *testing.T) {
	for _, name := range []string{"OPENAI_API_KEY", "OPENAI_ADMIN_KEY", "OPENAI_ORG_ID", "OPENAI_PROJECT_ID", "OPENAI_WEBHOOK_SECRET", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"} {
		t.Setenv(name, "dummy-environment-secret")
	}
	t.Setenv("OPENAI_BASE_URL", "https://environment.invalid")
	t.Setenv("ANTHROPIC_BASE_URL", "https://environment.invalid")
	t.Setenv("OPENAI_CUSTOM_HEADERS", "X-Environment: dummy-environment-secret")
	for _, name := range []providerName{anthropic, openai} {
		for _, tc := range []struct{ status, attempts int }{{200, 1}, {401, 1}, {403, 1}, {429, 2}, {503, 2}, {502, 2}, {-1, 2}} {
			t.Run(fmt.Sprintf("%s/%d", name, tc.status), func(t *testing.T) {
				var keys []string
				client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					assert.Equal(t, "https", r.URL.Scheme)
					assert.Equal(t, "api."+string(name)+".com", r.URL.Host)
					var key string
					if name == anthropic {
						assert.Equal(t, "/v1/messages", r.URL.Path)
						key = r.Header.Get("X-Api-Key")
						assert.Empty(t, r.Header.Get("Authorization"))
					} else {
						assert.Equal(t, "/v1/responses", r.URL.Path)
						key = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
						assert.Empty(t, r.Header.Get("Openai-Organization"))
						assert.Empty(t, r.Header.Get("Openai-Project"))
					}
					keys = append(keys, key)
					assert.NotContains(t, fmt.Sprint(r.Header), "dummy-environment-secret")
					assert.NotContains(t, fmt.Sprint(r.Header), "unused-key")
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.Contains(t, string(body), "hello")
					assert.Contains(t, string(body), "previous reply")
					assert.Contains(t, string(body), "continue")
					assert.Contains(t, string(body), "lookup")
					assert.Contains(t, string(body), "https://assets.invalid/example.png")
					assert.NotContains(t, string(body), "apiKey")
					assert.NotContains(t, string(body), "dummy-")
					var request map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(body, &request))
					assert.JSONEq(t, `"native-model"`, string(request["model"]))
					if name == anthropic {
						assert.JSONEq(t, `{"type":"disabled"}`, string(request["thinking"]))
					} else {
						assert.JSONEq(t, `false`, string(request["store"]))
					}
					if len(keys) == 1 && tc.status == -1 {
						return nil, errors.New("dummy-transport-failure")
					}
					status := http.StatusOK
					response := `{"id":"msg_1","type":"message","role":"assistant","model":"native-model","content":[{"type":"text","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
					if name == openai {
						response = `{"id":"resp_1","object":"response","status":"completed","model":"native-model","output":[{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"answer","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1}}`
					}
					if (len(keys) == 1 && tc.status > 200) || tc.status == 502 {
						status = tc.status
						response = `{"type":"error","error":{"type":"api_error","message":"dummy-rejection"}}`
					}
					return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(response)), Request: r}, nil
				})}
				other := anthropic
				if name == anthropic {
					other = openai
				}
				controls := fmt.Sprintf(`{"byok":{"%s":[{"apiKey":"dummy-first"},{"apiKey":"dummy-second"}],"%s":[{"apiKey":"unused-key"}]}}`, name, other)
				model, err := New(string(name)+"/native-model", json.RawMessage(controls), client)
				require.NoError(t, err)
				result, err := model.DoGenerate(context.Background(), provider.CallOptions{
					Prompt: []provider.Message{provider.UserText("hello"), provider.AssistantText("previous reply"), provider.NewUserMessage(provider.TextPart("continue"), provider.FilePart("image/png", provider.DataContent{URL: "https://assets.invalid/example.png"}))},
					Tools:  []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)}},
					ProviderOptions: provider.ProviderOptions{
						"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"disabled"}}`)},
						"openai":    provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"store":false}`)},
					},
				})
				if tc.status == 401 || tc.status == 403 || tc.status == 502 {
					require.Error(t, err)
					assert.Nil(t, result)
				} else {
					require.NoError(t, err)
					require.NotNil(t, result)
				}
				require.Len(t, keys, tc.attempts)
				assert.Equal(t, "dummy-first", keys[0])
				if tc.attempts == 2 {
					assert.Equal(t, "dummy-second", keys[1])
				}
				assert.Nil(t, client.CheckRedirect)
			})
		}
	}
}
