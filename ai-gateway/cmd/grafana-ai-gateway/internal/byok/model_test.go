package byok

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
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

func TestParseRequest(t *testing.T) {
	const valid = `{"byok":{"openai":[{"apiKey":"first"},{"apiKey":"second"}],"anthropic":[{"apiKey":"unused"}]}}`
	for _, tc := range []struct {
		name, selector, controls string
		wantProvider             providerName
		wantModel                string
		wantKeys                 []string
	}{
		{"ordered", "openai/native/model", valid, openai, "native/model", []string{"first", "second"}},
		{"other selected", "anthropic/not-in-catalog", valid, anthropic, "not-in-catalog", []string{"unused"}},
		{"last member wins", "openai/model", `{"byok":{"openai":[{"apiKey":"discarded","apiKey":"last"}]}}`, openai, "model", []string{"last"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRequest(tc.selector, json.RawMessage(tc.controls))
			require.NoError(t, err)
			assert.Equal(t, tc.wantProvider, got.provider)
			assert.Equal(t, tc.wantModel, got.model)
			assert.Equal(t, tc.wantKeys, got.keys)
		})
	}
	for _, selector := range []string{"", "alias", "/model", "openai/", "OpenAI/model", "bedrock/model", "openai/a b", "openai/a\t", "openai/a\u00a0", string([]byte{'o', '/', 255})} {
		t.Run("invalid selector/"+selector, func(t *testing.T) {
			got, err := parseRequest(selector, json.RawMessage(valid))
			require.ErrorIs(t, err, ErrInvalidRequest)
			assert.Empty(t, got)
		})
	}
	for _, raw := range []string{
		`null`, `{}`, `[]`, `{"byok":null}`, `{"byok":{}}`, `{"byok":[]}`,
		`{"byok":{"openai":null}}`, `{"byok":{"openai":[]}}`, `{"byok":{"openai":[null]}}`,
		`{"byok":{"openai":[{}]}}`, `{"byok":{"openai":[{"apiKey":null}]}}`,
		`{"byok":{"openai":[{"apiKey":12}]}}`, `{"byok":{"openai":[{"apiKey":""}]}}`,
		`{"byok":{"openai":[{"apiKey":"dummy-secret marker"}]}}`,
		`{"byok":{"openai":[{"apiKey":"dummy-secret\nmarker"}]}}`,
		`{"byok":{"openai":[{"apiKey":"valid","dummy-secret-field":true}]}}`,
		`{"byok":{"openai":[{"APIKey":"dummy-secret"}]}}`,
		`{"byok":{"dummy-secret-provider":[{"apiKey":"valid"}]}}`,
		`{"byok":{"anthropic":[{"apiKey":"valid"}]}}`,
		`{"byok":{"openai":[{"apiKey":"valid"}],"anthropic":[{}]}}`,
		`{"byok":{"openai":[{"apiKey":"valid"}]},"order":["openai"]}`,
		`{"byok":{"openai":[{"apiKey":"valid"}]},"modelMappings":{}}`,
		`{"byok":{"openai":[{"apiKey":"valid"}]},"dummy-secret-control":true}`,
	} {
		t.Run("invalid controls/"+raw, func(t *testing.T) {
			got, err := parseRequest("openai/model", json.RawMessage(raw))
			require.ErrorIs(t, err, ErrInvalidRequest)
			assert.Empty(t, got)
			assert.NotContains(t, err.Error(), "dummy-secret")
		})
	}
}

func TestParseRequest_Boundaries(t *testing.T) {
	credential := `{"apiKey":"` + strings.Repeat("k", maxCredentialBytes) + `"}`
	for _, delta := range []int{0, 1} {
		t.Run("selector/"+strings.Repeat("+", delta), func(t *testing.T) {
			selector := "openai/" + strings.Repeat("m", MaxSelectorBytes-len("openai/")+delta)
			_, err := parseRequest(selector, json.RawMessage(`{"byok":{"openai":[`+credential+`]}}`))
			assert.Equal(t, delta != 0, err != nil)
		})
		t.Run("key/"+strings.Repeat("+", delta), func(t *testing.T) {
			raw := `{"byok":{"openai":[{"apiKey":"` + strings.Repeat("k", maxCredentialBytes+delta) + `"}]}}`
			_, err := parseRequest("openai/model", json.RawMessage(raw))
			assert.Equal(t, delta != 0, err != nil)
		})
		t.Run("count/"+strings.Repeat("+", delta), func(t *testing.T) {
			entries := strings.TrimSuffix(strings.Repeat(credential+",", maxCredentials+delta), ",")
			_, err := parseRequest("openai/model", json.RawMessage(`{"byok":{"openai":[`+entries+`]}}`))
			assert.Equal(t, delta != 0, err != nil)
		})
		t.Run("raw bytes/"+strings.Repeat("+", delta), func(t *testing.T) {
			mapBody := `{"openai":[{"apiKey":"valid"}]}`
			mapBody = mapBody[:len(mapBody)-1] + strings.Repeat(" ", maxBYOKBytes-len(mapBody)+delta) + "}"
			_, err := parseRequest("openai/model", json.RawMessage(`{"byok":`+mapBody+`}`))
			assert.Equal(t, delta != 0, err != nil)
		})
	}
}

func TestNew_OpenAIStreamPreflightAndCommitment(t *testing.T) {
	const created = "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"created_at\":1,\"model\":\"native-model\",\"output\":[]}}\n\n"
	const added = "event: response.output_item.added\ndata: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"type\":\"message\",\"id\":\"msg_1\",\"role\":\"assistant\",\"content\":[]}}\n\n"
	const completed = "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
	const rejection = "event: error\ndata: {\"type\":\"error\",\"code\":\"invalid_api_key\",\"message\":\"dummy-rejection\"}\n\n"
	const rateLimit = "event: error\ndata: {\"type\":\"error\",\"code\":\"rate_limit_exceeded\",\"message\":\"dummy-rejection\"}\n\n"
	for _, tc := range []struct {
		name                  string
		first                 string
		attempts              int
		setupError, partError bool
	}{
		{name: "classified credential rejection stops", first: rejection, attempts: 1, setupError: true},
		{name: "classified rate limit advances", first: rateLimit, attempts: 2},
		{name: "first native part commits before error", first: created + added + rejection, attempts: 1, partError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var keys []string
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				keys = append(keys, r.Header.Get("Authorization"))
				body := created + completed
				if len(keys) == 1 {
					body = tc.first
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}
			model, err := New("openai/native-model", json.RawMessage(`{"byok":{"openai":[{"apiKey":"dummy-first"},{"apiKey":"dummy-second"}]}}`), client)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result, err := model.DoStream(ctx, provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
			if tc.setupError {
				require.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				var kinds []provider.StreamPartType
				for part := range result.Stream {
					kinds = append(kinds, part.Type)
				}
				require.NotEmpty(t, kinds)
				assert.Equal(t, provider.PartStreamStart, kinds[0])
				if tc.partError {
					assert.Contains(t, kinds, provider.PartError)
				} else {
					assert.Contains(t, kinds, provider.PartFinish)
				}
			}
			require.Len(t, keys, tc.attempts)
			assert.Equal(t, "Bearer dummy-first", keys[0])
			if tc.attempts == 2 {
				assert.Equal(t, "Bearer dummy-second", keys[1])
			}
		})
	}
}

func TestNew_ConcurrentRequestIsolation(t *testing.T) {
	const count = 16
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var arrived atomic.Int32
	ready := make(chan struct{})
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		assert.Equal(t, "Bearer dummy-"+strings.TrimPrefix(body.Model, "native-"), r.Header.Get("Authorization"))
		if arrived.Add(1) == count {
			close(ready)
		}
		select {
		case <-ready:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"resp_1","object":"response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)), Request: r}, nil
	})}
	results := make(chan error, count)
	for i := range count {
		go func() {
			model, err := New(fmt.Sprintf("openai/native-%d", i), json.RawMessage(fmt.Sprintf(`{"byok":{"openai":[{"apiKey":"dummy-%d"}]}}`, i)), client)
			if err == nil {
				_, err = model.DoGenerate(ctx, provider.CallOptions{Prompt: []provider.Message{provider.UserText("ordinary input")}})
			}
			results <- err
		}()
	}
	for range count {
		require.NoError(t, <-results)
	}
	assert.Equal(t, int32(count), arrived.Load())
}

func TestNew_DoesNotFollowNativeRedirects(t *testing.T) {
	for _, name := range []providerName{anthropic, openai} {
		t.Run(string(name), func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				assert.Equal(t, "api."+string(name)+".com", r.URL.Host)
				return &http.Response{StatusCode: 307, Header: http.Header{"Content-Type": {"application/json"}, "Location": {"https://redirect.invalid"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"redirect"}}`)), Request: r}, nil
			})}
			model, err := New(string(name)+"/native-model", json.RawMessage(fmt.Sprintf(`{"byok":{"%s":[{"apiKey":"dummy-first"},{"apiKey":"dummy-second"}]}}`, name)), client)
			require.NoError(t, err)
			_, _ = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
			assert.Equal(t, 1, calls)
			assert.Nil(t, client.CheckRedirect)
		})
	}
}

func TestNew_AnthropicNativeOptionGuards(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, tc := range []struct {
			name, options string
			mcpHistory    bool
			allowed       bool
		}{
			{name: "MCP servers", options: `{"anthropic":{"mcpServers":[{"name":"external","url":"https://example.invalid"}]}}`},
			{name: "container skills", options: `{"anthropic":{"container":{"skills":[{"type":"custom","skillId":"external"}]}}}`},
			{name: "native fallback", options: `{"anthropic":{"fallbacks":"default"}}`},
			{name: "MCP history", options: `{}`, mcpHistory: true},
			{name: "ordinary fields and namespaces", options: `{"anthropic":{"model":"ignored","mcp_servers":[],"container":{"id":"supplied-container"}},"other":{"mcpServers":[{"name":"ordinary"}]}}`, allowed: true},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, streaming), func(t *testing.T) {
				calls := 0
				client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					assert.Equal(t, "api.anthropic.com", r.URL.Host)
					assert.Equal(t, "dummy-first", r.Header.Get("X-Api-Key"))
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.NotContains(t, string(body), "dummy-")
					if tc.allowed {
						assert.Contains(t, string(body), "supplied-container")
						assert.NotContains(t, string(body), "ignored")
						assert.Contains(t, string(body), "native-model")
					}
					return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"authentication_error","message":"rejected"}}`)), Request: r}, nil
				})}
				model, err := New("anthropic/native-model", json.RawMessage(`{"byok":{"anthropic":[{"apiKey":"dummy-first"},{"apiKey":"dummy-second"}]}}`), client)
				require.NoError(t, err)
				opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}}
				require.NoError(t, json.Unmarshal([]byte(tc.options), &opts.ProviderOptions))
				if tc.mcpHistory {
					part := provider.ToolCallPart("call", "lookup", json.RawMessage(`{}`))
					part.ProviderOptions = provider.ProviderOptions{"anthropic": provider.RawProviderOption{Raw: json.RawMessage(`{"type":"mcp-tool-use"}`)}}
					opts.Prompt = append(opts.Prompt, provider.NewAssistantMessage(part))
				}
				if streaming {
					_, err = model.DoStream(context.Background(), opts)
				} else {
					_, err = model.DoGenerate(context.Background(), opts)
				}
				if tc.allowed {
					require.Error(t, err)
					assert.NotErrorIs(t, err, catalog.ErrUnsupportedRequest)
					assert.Equal(t, 1, calls)
				} else {
					require.ErrorIs(t, err, catalog.ErrUnsupportedRequest)
					assert.Zero(t, calls)
				}
			})
		}
	}
}
