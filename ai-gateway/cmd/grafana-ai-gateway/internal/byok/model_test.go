package byok

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/nativemodel"
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
	for _, name := range []Provider{Anthropic, OpenAI} {
		for _, tc := range []struct{ status, attempts int }{{200, 1}, {401, 1}, {403, 1}, {429, 2}, {503, 2}, {502, 2}, {-1, 2}} {
			t.Run(fmt.Sprintf("%s/%d", name, tc.status), func(t *testing.T) {
				var keys []string
				client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					assert.Equal(t, "https", r.URL.Scheme)
					assert.Equal(t, "api."+string(name)+".com", r.URL.Host)
					var key string
					if name == Anthropic {
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
					if name == Anthropic {
						assert.JSONEq(t, `{"type":"disabled"}`, string(request["thinking"]))
					} else {
						assert.JSONEq(t, `false`, string(request["store"]))
					}
					if len(keys) == 1 && tc.status == -1 {
						return nil, errors.New("dummy-transport-failure")
					}
					status := http.StatusOK
					response := `{"id":"msg_1","type":"message","role":"assistant","model":"native-model","content":[{"type":"text","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
					if name == OpenAI {
						response = `{"id":"resp_1","object":"response","status":"completed","model":"native-model","output":[{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"answer","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1}}`
					}
					if (len(keys) == 1 && tc.status > 200) || tc.status == 502 {
						status = tc.status
						response = `{"type":"error","error":{"type":"api_error","message":"dummy-rejection"}}`
					}
					return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(response)), Request: r}, nil
				})}
				other := Anthropic
				if name == Anthropic {
					other = OpenAI
				}
				controls := fmt.Sprintf(`{"byok":{"%s":[{"apiKey":"dummy-first"},{"apiKey":"dummy-second"}],"%s":[{"apiKey":"unused-key"}]}}`, name, other)
				request, err := DecodeRequest(string(name)+"/native-model", json.RawMessage(controls), nil)
				require.NoError(t, err)
				model, err := New(request.Provider, request.Model, request.Accounts, client)
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

func TestNew_AccountConfigDefaults(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, tc := range []struct {
			name, account, organization, project string
		}{
			{name: "explicit native endpoint", account: `{"apiKey":"dummy-key","baseURL":"https://api.openai.com/v1"}`},
			{name: "OpenAI account fields", account: `{"apiKey":"dummy-key","organization":"org-customer","project":"proj-customer"}`, organization: "org-customer", project: "proj-customer"},
			{name: "empty optional fields", account: `{"apiKey":"dummy-key","baseURL":"","organization":"","project":""}`},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, streaming), func(t *testing.T) {
				request, err := DecodeRequest("openai/native-model", json.RawMessage(`{"byok":{"openai":[`+tc.account+`]}}`), nil)
				require.NoError(t, err)
				calls := 0
				client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					assert.Equal(t, "https://api.openai.com/v1/responses", r.URL.String())
					assert.Equal(t, "Bearer dummy-key", r.Header.Get("Authorization"))
					assert.Equal(t, tc.organization, r.Header.Get("OpenAI-Organization"))
					assert.Equal(t, tc.project, r.Header.Get("OpenAI-Project"))
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.Contains(t, string(body), `"model":"native-model"`)
					for _, field := range []string{"apiKey", "baseURL", "organization", "project", "dummy-key"} {
						assert.NotContains(t, string(body), field)
					}
					return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"rejected"}}`)), Request: r}, nil
				})}
				model, err := New(request.Provider, request.Model, request.Accounts, client)
				require.NoError(t, err)
				if streaming {
					_, err = model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
				} else {
					_, err = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
				}
				require.Error(t, err)
				assert.Equal(t, 1, calls)
			})
		}
	}
}

func TestNew_ApprovedAccountOverrides(t *testing.T) {
	for _, name := range []Provider{Anthropic, OpenAI} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", name, streaming), func(t *testing.T) {
				first := "https://approved.example/first"
				second := "https://approved.example/second"
				extraFirst, extraSecond := "", ""
				if name == OpenAI {
					extraFirst = `,"organization":"org-first","project":"proj-first"`
					extraSecond = `,"organization":"org-second","project":"proj-second"`
				}
				raw := fmt.Sprintf(`{"byok":{"%s":[{"apiKey":"dummy-first","baseURL":%q%s},{"apiKey":"dummy-second","baseURL":%q%s}]}}`, name, first, extraFirst, second, extraSecond)
				approval := map[Provider][]string{name: {first, second}}
				request, err := DecodeRequest(string(name)+"/native-model", json.RawMessage(raw), approval)
				require.NoError(t, err)
				delete(approval, name)
				calls := 0
				client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					account := "first"
					status := http.StatusServiceUnavailable
					if calls > 1 {
						account = "second"
						status = http.StatusUnauthorized
					}
					assert.Equal(t, "https", r.URL.Scheme)
					assert.Equal(t, "approved.example", r.URL.Host)
					path := "/" + account + "/v1/messages"
					if name == OpenAI {
						path = "/" + account + "/responses"
						assert.Equal(t, "Bearer dummy-"+account, r.Header.Get("Authorization"))
						assert.Equal(t, "org-"+account, r.Header.Get("OpenAI-Organization"))
						assert.Equal(t, "proj-"+account, r.Header.Get("OpenAI-Project"))
					} else {
						assert.Equal(t, "dummy-"+account, r.Header.Get("X-Api-Key"))
						assert.Empty(t, r.Header.Get("OpenAI-Organization"))
						assert.Empty(t, r.Header.Get("OpenAI-Project"))
					}
					assert.Equal(t, path, r.URL.Path)
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.Contains(t, string(body), `"model":"native-model"`)
					assert.Contains(t, string(body), "ordinary input")
					for _, field := range []string{"dummy-", "org-", "proj-", "baseURL", "approved.example"} {
						assert.NotContains(t, string(body), field)
					}
					return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"api_error","message":"rejected"}}`)), Request: r}, nil
				})}
				model, err := New(request.Provider, request.Model, request.Accounts, client)
				require.NoError(t, err)
				options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("ordinary input")}}
				if streaming {
					_, err = model.DoStream(context.Background(), options)
				} else {
					_, err = model.DoGenerate(context.Background(), options)
				}
				require.Error(t, err)
				assert.Equal(t, 2, calls)
				assert.Nil(t, client.CheckRedirect)
			})
		}
	}
}

func TestDecodeRequest_AccountPolicy(t *testing.T) {
	const approved = "https://approved.example/v1"
	for _, tc := range []struct {
		name, selector, raw string
		approvals           map[Provider][]string
		allowed             bool
	}{
		{name: "approved endpoint", selector: "openai/model", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"https://approved.example/v1"}]}}`, approvals: map[Provider][]string{OpenAI: {approved}}, allowed: true},
		{name: "unapproved endpoint", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"https://approved.example/v1"}]}}`},
		{name: "unapproved endpoint with secret", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"https://dummy-secret@approved.example/v1"}]}}`},
		{name: "wrong provider approval", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"https://approved.example/v1"}]}}`, approvals: map[Provider][]string{Anthropic: {approved}}},
		{name: "approval is not a request control", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"https://approved.example/v1"}]},"approvedBaseURLs":["https://approved.example/v1"]}`},
		{name: "approval does not supply defaults", raw: `{"byok":{"openai":[{"apiKey":"key"}]}}`, approvals: map[Provider][]string{OpenAI: {approved}}, allowed: true},
		{name: "unused unapproved endpoint", raw: `{"byok":{"openai":[{"apiKey":"key"}],"anthropic":[{"apiKey":"unused","baseURL":"https://approved.example/v1"}]}}`},
		{name: "native endpoint with changed path", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"https://api.openai.com/other"}]}}`},
		{name: "native endpoint with added port", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"https://api.openai.com:443/v1"}]}}`},
		{name: "approved prefix is not approved URL", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"https://approved.example/v1/other"}]}}`, approvals: map[Provider][]string{OpenAI: {approved}}},
		{name: "null endpoint", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":null}]}}`, allowed: true},
		{name: "null organization", raw: `{"byok":{"openai":[{"apiKey":"key","organization":null}]}}`, allowed: true},
		{name: "case variant endpoint still requires approval", raw: `{"byok":{"openai":[{"apiKey":"key","BaseURL":"https://approved.example/v1"}]}}`},
		{name: "host owns endpoint syntax", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"http://approved.example/v1"}]}}`, approvals: map[Provider][]string{OpenAI: {"http://approved.example/v1"}}, allowed: true},
		{name: "unknown field", raw: `{"byok":{"openai":[{"apiKey":"key","maxRetries":2}]}}`},
		{name: "Anthropic account does not accept OpenAI fields", selector: "anthropic/model", raw: `{"byok":{"anthropic":[{"apiKey":"key","project":"proj"}]}}`},
		{name: "organization whitespace", raw: `{"byok":{"openai":[{"apiKey":"key","organization":"org value"}]}}`, allowed: true},
		{name: "header validity belongs to transport", raw: `{"byok":{"openai":[{"apiKey":"key","project":"proj\r\nX-Test: value"}]}}`, allowed: true},
		{name: "replaced endpoint type", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":42,"baseURL":"https://api.openai.com/v1"}]}}`},
		{name: "replaced organization type", raw: `{"byok":{"openai":[{"apiKey":"key","organization":false,"organization":"org"}]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selector := tc.selector
			if selector == "" {
				selector = "openai/model"
			}
			got, err := DecodeRequest(selector, json.RawMessage(tc.raw), tc.approvals)
			if tc.allowed {
				require.NoError(t, err)
				if tc.name == "approval does not supply defaults" {
					assert.Empty(t, got.Accounts[0].BaseURL)
				}
			} else {
				require.ErrorIs(t, err, ErrInvalidRequest)
				assert.Empty(t, got)
				assert.NotContains(t, err.Error(), "approved.example")
				assert.NotContains(t, err.Error(), "dummy-secret")
			}
		})
	}
}

func TestDecodeRequest(t *testing.T) {
	const valid = `{"byok":{"openai":[{"apiKey":"first"},{"apiKey":"second"}],"anthropic":[{"apiKey":"unused"}]}}`
	for _, tc := range []struct {
		name, selector, controls string
		wantProvider             Provider
		wantModel                string
		wantKeys                 []string
	}{
		{"ordered", "openai/native/model", valid, OpenAI, "native/model", []string{"first", "second"}},
		{"other selected", "anthropic/not-in-catalog", valid, Anthropic, "not-in-catalog", []string{"unused"}},
		{"last member wins", "openai/model", `{"byok":{"openai":[{"apiKey":"discarded","apiKey":"last"}]}}`, OpenAI, "model", []string{"last"}},
		{"null retains string", "openai/model", `{"byok":{"openai":[{"apiKey":"last","apiKey":null}]}}`, OpenAI, "model", []string{"last"}},
		{"duplicate control replaces map", "anthropic/model", `{"byok":{"openai":[{"apiKey":"discarded"}]},"byok":{"anthropic":[{"apiKey":"last"}]}}`, Anthropic, "model", []string{"last"}},
		{"key whitespace", "openai/model", `{"byok":{"openai":[{"apiKey":"key with spaces"}]}}`, OpenAI, "model", []string{"key with spaces"}},
		{"key header validation is deferred", "openai/model", `{"byok":{"openai":[{"apiKey":"key\r\nX-Test: value"}]}}`, OpenAI, "model", []string{"key\r\nX-Test: value"}},
		{"model is native data", "openai/a b", valid, OpenAI, "a b", []string{"first", "second"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodeRequest(tc.selector, json.RawMessage(tc.controls), nil)
			require.NoError(t, err)
			assert.Equal(t, tc.wantProvider, got.Provider)
			assert.Equal(t, tc.wantModel, got.Model)
			keys := make([]string, len(got.Accounts))
			for i, credential := range got.Accounts {
				keys[i] = credential.APIKey
			}
			assert.Equal(t, tc.wantKeys, keys)
		})
	}
	for _, selector := range []string{"", "alias", "/model", "openai/", "OpenAI/model", "bedrock/model", "openai/" + string([]byte{255})} {
		t.Run("invalid selector/"+selector, func(t *testing.T) {
			got, err := DecodeRequest(selector, json.RawMessage(valid), nil)
			require.ErrorIs(t, err, ErrInvalidRequest)
			assert.Empty(t, got)
		})
	}
	for _, raw := range []string{
		`null`, `{}`, `[]`, `{"byok":null}`, `{"byok":{}}`, `{"byok":[]}`,
		`{"BYOK":{"openai":[{"apiKey":"dummy-secret"}]}}`,
		`{"byok":{"openai":[{"apiKey":"valid"}]},"BYOK":{"openai":[{"apiKey":"dummy-secret"}]}}`,
		`{"byok":{"openai":[{"apiKey":"discarded"}]},"byok":{"anthropic":[{"apiKey":"unused"}]}}`,
		`{"byok":{"openai":null}}`, `{"byok":{"openai":[]}}`, `{"byok":{"openai":[null]}}`,
		`{"byok":{"openai":[{}]}}`, `{"byok":{"openai":[{"apiKey":null}]}}`,
		`{"byok":{"openai":[{"apiKey":12}]}}`, `{"byok":{"openai":[{"apiKey":""}]}}`,
		`{"byok":{"openai":[{"apiKey":42,"apiKey":"valid"}]}}`,
		`{"byok":{"openai":false,"openai":[{"apiKey":"valid"}]}}`,
		`{"byok":false,"byok":{"openai":[{"apiKey":"valid"}]}}`,
		`{"byok":{"openai":[{"apiKey":"valid"}]}} {}`,
		`{"byok":{"openai":[{"apiKey":"valid","dummy-secret-field":true}]}}`,
		`{"byok":{"openai":[{"apiKey":"valid","baseURL":"https://dummy-secret.invalid"}]}}`,
		`{"byok":{"openai":[{"apiKey":"valid","headers":{"Authorization":"dummy-secret"}}]}}`,
		`{"byok":{"openai":[{"apiKey":"valid"}],"OpenAI":[{"apiKey":"dummy-secret"}]}}`,
		`{"byok":{"dummy-secret-provider":[{"apiKey":"valid"}]}}`,
		`{"byok":{"anthropic":[{"apiKey":"valid"}]}}`,
		`{"byok":{"openai":[{"apiKey":"valid"}],"anthropic":[{}]}}`,
		`{"byok":{"openai":[{"apiKey":"valid"}]},"order":["openai"]}`,
		`{"byok":{"openai":[{"apiKey":"valid"}]},"modelMappings":{}}`,
		`{"byok":{"openai":[{"apiKey":"valid"}]},"dummy-secret-control":true}`,
	} {
		t.Run("invalid controls/"+raw, func(t *testing.T) {
			got, err := DecodeRequest("openai/model", json.RawMessage(raw), nil)
			require.ErrorIs(t, err, ErrInvalidRequest)
			assert.Empty(t, got)
			assert.NotContains(t, err.Error(), "dummy-secret")
		})
	}
}

func TestNew_PlainAccounts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider Provider
		model    string
		accounts []nativemodel.Config
	}{
		{name: "unsupported provider"},
		{name: "missing model", provider: OpenAI, accounts: []nativemodel.Config{{APIKey: "key"}}},
		{name: "missing accounts", provider: OpenAI, model: "model"},
		{name: "too many accounts", provider: OpenAI, model: "model", accounts: make([]nativemodel.Config, maxCredentials+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model, err := New(tc.provider, tc.model, tc.accounts, http.DefaultClient)
			require.ErrorIs(t, err, ErrInvalidRequest)
			assert.Nil(t, model)
		})
	}
	t.Run("missing transport", func(t *testing.T) {
		request, err := DecodeRequest("openai/model", json.RawMessage(`{"byok":{"openai":[{"apiKey":"dummy-key"}]}}`), nil)
		require.NoError(t, err)
		model, err := New(request.Provider, request.Model, request.Accounts, nil)
		require.ErrorContains(t, err, "HTTP client is required")
		assert.Nil(t, model)
	})
	t.Run("decoded request owns credentials", func(t *testing.T) {
		raw := json.RawMessage(`{"byok":{"openai":[{"apiKey":"dummy-key"}]}}`)
		request, err := DecodeRequest("openai/model", raw, nil)
		require.NoError(t, err)
		clear(raw)
		calls := 0
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			assert.Equal(t, "Bearer dummy-key", r.Header.Get("Authorization"))
			assert.Equal(t, "api.openai.com", r.URL.Host)
			return nil, assert.AnError
		})}
		model, err := New(request.Provider, request.Model, request.Accounts, client)
		require.NoError(t, err)
		_, err = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
		require.Error(t, err)
		assert.Equal(t, 1, calls)
	})
}

func TestDecodeRequest_AccountCount(t *testing.T) {
	for _, name := range []Provider{OpenAI, Anthropic} {
		for _, count := range []int{0, maxCredentials, maxCredentials + 1} {
			t.Run(fmt.Sprintf("%s/%d", name, count), func(t *testing.T) {
				accounts := map[Provider][]nativemodel.Config{OpenAI: {{APIKey: "selected"}}}
				accounts[name] = make([]nativemodel.Config, count)
				for i := range accounts[name] {
					accounts[name][i].APIKey = "key"
				}
				raw, err := json.Marshal(map[string]any{"byok": accounts})
				require.NoError(t, err)
				_, err = DecodeRequest("openai/model", raw, nil)
				assert.Equal(t, count == 0 || count > maxCredentials, err != nil)
			})
		}
	}
}

func TestDecodeRequest_NoSeparateByteLimits(t *testing.T) {
	account := nativemodel.Config{
		APIKey: strings.Repeat("k", 8192), BaseURL: "https://approved.example/" + strings.Repeat("p", 4096),
		Organization: strings.Repeat("o", 1024), Project: strings.Repeat("p", 1024),
	}
	raw, err := json.Marshal(map[string]any{"byok": map[Provider][]nativemodel.Config{OpenAI: {account}}})
	require.NoError(t, err)
	raw = append(raw[:len(raw)-1], []byte(strings.Repeat(" ", 70_000)+"}")...)
	modelID := strings.Repeat("m", 4096)
	request, err := DecodeRequest("openai/"+modelID, raw, map[Provider][]string{OpenAI: {account.BaseURL}})
	require.NoError(t, err)
	assert.Equal(t, modelID, request.Model)
	assert.Equal(t, []nativemodel.Config{account}, request.Accounts)
}

func TestNew_HTTPTransportRejectsInvalidHeaders(t *testing.T) {
	for _, name := range []Provider{Anthropic, OpenAI} {
		fields := []string{"apiKey"}
		if name == OpenAI {
			fields = append(fields, "organization", "project")
		}
		for _, field := range fields {
			for _, streaming := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/streaming=%t", name, field, streaming), func(t *testing.T) {
					var calls atomic.Int32
					server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						calls.Add(1)
						w.WriteHeader(http.StatusNoContent)
					}))
					defer server.Close()
					account := map[string]string{"apiKey": "key", "baseURL": server.URL}
					account[field] = "dummy-secret\r\nX-Test: value"
					raw, err := json.Marshal(map[string]any{"byok": map[Provider][]map[string]string{name: {account}}})
					require.NoError(t, err)
					request, err := DecodeRequest(string(name)+"/model", raw, map[Provider][]string{name: {server.URL}})
					require.NoError(t, err)
					model, err := New(request.Provider, request.Model, request.Accounts, server.Client())
					require.NoError(t, err)
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}}
					if streaming {
						_, err = model.DoStream(ctx, options)
					} else {
						_, err = model.DoGenerate(ctx, options)
					}
					require.ErrorContains(t, err, "invalid header field value")
					assert.Zero(t, calls.Load())
				})
			}
		}
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
			request, err := DecodeRequest("openai/native-model", json.RawMessage(`{"byok":{"openai":[{"apiKey":"dummy-first"},{"apiKey":"dummy-second"}]}}`), nil)
			require.NoError(t, err)
			model, err := New(request.Provider, request.Model, request.Accounts, client)
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
		index := strings.TrimPrefix(body.Model, "native-")
		assert.Equal(t, "Bearer dummy-"+index, r.Header.Get("Authorization"))
		assert.Equal(t, "org-"+index, r.Header.Get("OpenAI-Organization"))
		assert.Equal(t, "proj-"+index, r.Header.Get("OpenAI-Project"))
		assert.Equal(t, "https://approved.example/v1/responses", r.URL.String())
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
			request, err := DecodeRequest(fmt.Sprintf("openai/native-%d", i), json.RawMessage(fmt.Sprintf(`{"byok":{"openai":[{"apiKey":"dummy-%d","baseURL":"https://approved.example/v1","organization":"org-%d","project":"proj-%d"}]}}`, i, i, i)), map[Provider][]string{OpenAI: {"https://approved.example/v1"}})
			if err != nil {
				results <- err
				return
			}
			model, err := New(request.Provider, request.Model, request.Accounts, client)
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
	for _, name := range []Provider{Anthropic, OpenAI} {
		for _, custom := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/custom=%t", name, custom), func(t *testing.T) {
				host := "api." + string(name) + ".com"
				accountFields := ""
				var approvals map[Provider][]string
				if custom {
					host = "approved.example"
					accountFields = `,"baseURL":"https://approved.example"`
					approvals = map[Provider][]string{name: {"https://approved.example"}}
				}
				calls := 0
				client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					assert.Equal(t, host, r.URL.Host)
					return &http.Response{StatusCode: 307, Header: http.Header{"Content-Type": {"application/json"}, "Location": {"https://redirect.invalid"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"redirect"}}`)), Request: r}, nil
				})}
				raw := fmt.Sprintf(`{"byok":{"%s":[{"apiKey":"dummy-first"%s},{"apiKey":"dummy-second"%s}]}}`, name, accountFields, accountFields)
				request, err := DecodeRequest(string(name)+"/native-model", json.RawMessage(raw), approvals)
				require.NoError(t, err)
				model, err := New(request.Provider, request.Model, request.Accounts, client)
				require.NoError(t, err)
				_, _ = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
				assert.Equal(t, 1, calls)
				assert.Nil(t, client.CheckRedirect)
			})
		}
	}
}

func TestNew_AnthropicNativeOptionGuards(t *testing.T) {
	const mcpOptions = `{"anthropic":{"mcpServers":[{"name":"external","url":"https://example.invalid"}]}}`
	for _, streaming := range []bool{false, true} {
		for _, tc := range []struct {
			name, options, nativeField string
			mcpHistory                 bool
			providerExecuted           bool
			allowed                    bool
		}{
			{name: "MCP servers", options: mcpOptions, nativeField: "mcp_servers", allowed: true},
			{name: "invalid MCP destination", options: `{"anthropic":{"mcpServers":[{"name":"external","url":"http://example.invalid"}]}}`},
			{name: "container skills", options: `{"anthropic":{"container":{"skills":[{"type":"custom","skillId":"external"}]}}}`},
			{name: "native fallback", options: `{"anthropic":{"fallbacks":"default"}}`},
			{name: "unconfigured MCP history", options: `{}`, mcpHistory: true, providerExecuted: true},
			{name: "configured MCP history", options: mcpOptions, mcpHistory: true, providerExecuted: true, nativeField: "mcp_tool_use", allowed: true},
			{name: "local MCP marker", options: `{}`, mcpHistory: true, nativeField: "tool_use", allowed: true},
			{name: "ordinary fields and namespaces", options: `{"anthropic":{"model":"ignored","mcp_servers":[],"container":{"id":"supplied-container"}},"other":{"mcpServers":[{"name":"ordinary"}]}}`, nativeField: "supplied-container", allowed: true},
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
						assert.Contains(t, string(body), tc.nativeField)
						assert.NotContains(t, string(body), "ignored")
						assert.Contains(t, string(body), "native-model")
					}
					return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"authentication_error","message":"rejected"}}`)), Request: r}, nil
				})}
				request, err := DecodeRequest("anthropic/native-model", json.RawMessage(`{"byok":{"anthropic":[{"apiKey":"dummy-first"},{"apiKey":"dummy-second"}]}}`), nil)
				require.NoError(t, err)
				model, err := New(request.Provider, request.Model, request.Accounts, client)
				require.NoError(t, err)
				opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}}
				require.NoError(t, json.Unmarshal([]byte(tc.options), &opts.ProviderOptions))
				if tc.mcpHistory {
					part := provider.ToolCallPart("call", "lookup", json.RawMessage(`{}`))
					part.ProviderExecuted = tc.providerExecuted
					part.ProviderOptions = provider.ProviderOptions{"anthropic": provider.RawProviderOption{Raw: json.RawMessage(`{"type":"mcp-tool-use","serverName":"external"}`)}}
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
