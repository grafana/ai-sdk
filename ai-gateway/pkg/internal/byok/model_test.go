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
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/nativemodel"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/outbound"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/routing"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func newTestModel(t *testing.T, selector string, controls json.RawMessage, policy outbound.EgressPolicy, client *http.Client) provider.LanguageModel {
	t.Helper()
	request, err := decodeGateway(selector, controls, policy)
	require.NoError(t, err)
	model, err := newAll(request, client)
	require.NoError(t, err)
	return model
}

func allowLoopback(t *testing.T) outbound.EgressPolicy {
	t.Helper()
	allow, err := outbound.ParseEgressList("127.0.0.1")
	require.NoError(t, err)
	return outbound.EgressPolicy{Allow: allow}
}

func invokeTestModel(ctx context.Context, model provider.LanguageModel, streaming bool, options provider.CallOptions) error {
	if streaming {
		_, err := model.DoStream(ctx, options)
		return err
	}
	_, err := model.DoGenerate(ctx, options)
	return err
}

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
				model := newTestModel(t, string(name)+"/native-model", json.RawMessage(controls), outbound.EgressPolicy{}, client)
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
			{name: "ignored additive fields", account: `{"apiKey":"dummy-key","headers":{"Authorization":"ignored-account-value"},"base_url":"https://ignored.invalid","maxRetries":100}`},
			{name: "standard field matching", account: `{"APIKEY":"dummy-key","BaseURL":"https://api.openai.com/v1","Organization":"org-customer","Project":"proj-customer"}`, organization: "org-customer", project: "proj-customer"},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, streaming), func(t *testing.T) {
				request, err := decodeGateway("openai/native-model", json.RawMessage(`{"byok":{"openai":[`+tc.account+`]}}`), outbound.EgressPolicy{})
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
					assert.NotContains(t, string(body), "ignored-account-value")
					assert.NotContains(t, string(body), "ignored.invalid")
					for _, field := range []string{"apiKey", "baseURL", "organization", "project", "dummy-key"} {
						assert.NotContains(t, string(body), field)
					}
					return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"rejected"}}`)), Request: r}, nil
				})}
				model, err := newAll(request, client)
				require.NoError(t, err)
				err = invokeTestModel(context.Background(), model, streaming, provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
				require.Error(t, err)
				assert.Equal(t, 1, calls)
			})
		}
	}
}

func TestNew_ClientChoice(t *testing.T) {
	for _, tc := range []struct {
		name, account string
		egress        bool
	}{
		{name: "no endpoint", account: `{"apiKey":"dummy-key"}`},
		{name: "explicit native endpoint", account: `{"apiKey":"dummy-key","baseURL":"https://api.openai.com/v1"}`},
		{name: "custom endpoint", account: `{"apiKey":"dummy-key","baseURL":"https://llm.example.com/v1"}`, egress: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var shared, egress int
			client := func(count *int) *http.Client {
				return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					*count++
					return nil, fmt.Errorf("wrapped: %w", execution.ErrEgressRefused)
				})}
			}
			deny, err := outbound.ParseEgressList("api.openai.com")
			require.NoError(t, err)
			request, err := decodeGateway("openai/model", json.RawMessage(`{"byok":{"openai":[`+tc.account+`]}}`), outbound.EgressPolicy{Deny: deny})
			require.NoError(t, err, "a deny entry for the native host doesn't refuse the native endpoint")
			model, err := New(request, []routing.Placement{{}}, outbound.ModelClients{Shared: client(&shared), Egress: client(&egress)})
			require.NoError(t, err)
			_, err = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
			require.ErrorIs(t, err, execution.ErrEgressRefused)
			if tc.egress {
				assert.Equal(t, []int{0, 1}, []int{shared, egress})
				assert.Same(t, execution.ErrEgressRefused, err, "a custom endpoint's egress error is bare")
				return
			}
			assert.Equal(t, []int{1, 0}, []int{shared, egress})
		})
	}
}

func TestNew_CustomAccountEndpoints(t *testing.T) {
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
				request, err := decodeGateway(string(name)+"/native-model", json.RawMessage(raw), outbound.EgressPolicy{})
				require.NoError(t, err)
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
				model, err := newAll(request, client)
				require.NoError(t, err)
				options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("ordinary input")}}
				err = invokeTestModel(context.Background(), model, streaming, options)
				require.Error(t, err)
				assert.Equal(t, 2, calls)
				assert.Nil(t, client.CheckRedirect)
			})
		}
	}
}

func TestDecodeRequest_AccountPolicy(t *testing.T) {
	deny, err := outbound.ParseEgressList("denied.example")
	require.NoError(t, err)
	policy := outbound.EgressPolicy{Deny: deny}
	for _, tc := range []struct {
		name, selector, raw string
		allowed             bool
		endpoint            string
		want                error
	}{
		{name: "public https endpoint", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"https://llm.example.com/v1"}]}}`, allowed: true, endpoint: "https://llm.example.com/v1"},
		{name: "endpoint is normalized", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"HTTPS://llm.example.com/v1"}]}}`, allowed: true, endpoint: "https://llm.example.com/v1"},
		{name: "native endpoint counts as unset", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"https://api.openai.com/v1"}]}}`, allowed: true},
		{name: "native endpoint with a trailing slash counts as unset", selector: "anthropic/model", raw: `{"byok":{"anthropic":[{"apiKey":"key","baseURL":"https://API.anthropic.com/"}]}}`, allowed: true},
		{name: "native endpoint with changed path is custom", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"https://api.openai.com/other"}]}}`, allowed: true, endpoint: "https://api.openai.com/other"},
		{name: "native endpoint with added port is custom", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"https://api.openai.com:443/v1"}]}}`, allowed: true, endpoint: "https://api.openai.com:443/v1"},
		{name: "case variant field follows struct decoding", raw: `{"byok":{"openai":[{"apiKey":"key","BaseURL":"https://llm.example.com/v1"}]}}`, allowed: true, endpoint: "https://llm.example.com/v1"},
		{name: "plain HTTP endpoint", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"http://llm.example.com/v1"}]}}`},
		{name: "endpoint with credentials", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"https://dummy-secret@llm.example.com/v1"}]}}`},
		{name: "endpoint with a query", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"https://llm.example.com/v1?dummy-secret=1"}]}}`},
		{name: "loopback literal", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"https://127.0.0.1/v1"}]}}`},
		{name: "metadata literal", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"https://169.254.169.254/v1"}]}}`},
		{name: "mapped private literal", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"https://[::ffff:10.0.0.1]/v1"}]}}`},
		{name: "numeric host form", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"https://2130706433/v1"}]}}`},
		{name: "denied host", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"https://denied.example/v1"}]}}`},
		{name: "unused provider with a refused endpoint", raw: `{"byok":{"openai":[{"apiKey":"key"}],"anthropic":[{"apiKey":"unused","baseURL":"https://127.0.0.1"}]}}`},
		{name: "approval is not a request control", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":"https://llm.example.com/v1"}]},"approvedBaseURLs":["https://llm.example.com/v1"]}`, want: execution.ErrUnsupportedGatewayControl},
		{name: "null endpoint", raw: `{"byok":{"openai":[{"apiKey":"key","baseURL":null}]}}`, allowed: true},
		{name: "null organization", raw: `{"byok":{"openai":[{"apiKey":"key","organization":null}]}}`, allowed: true},
		{name: "additive account field", raw: `{"byok":{"openai":[{"apiKey":"key","maxRetries":2}]}}`, allowed: true},
		{name: "inactive null mappings", raw: `{"byok":{"openai":[{"apiKey":"key","modelMappings":null}]}}`, allowed: true},
		{name: "inactive empty mappings", raw: `{"byok":{"openai":[{"apiKey":"key","modelMappings":[]}]}}`, allowed: true},
		{name: "meaningful unsupported mappings", raw: `{"byok":{"openai":[{"apiKey":"key","modelMappings":[{"gatewayModelSlug":"openai/model","customModelId":"mapped"}]}]}}`},
		{name: "malformed mappings", raw: `{"byok":{"openai":[{"apiKey":"key","modelMappings":42}]}}`},
		{name: "mapping casing follows struct decoding", raw: `{"byok":{"openai":[{"apiKey":"key","ModelMappings":[{}]}]}}`},
		{name: "last mapping member wins", raw: `{"byok":{"openai":[{"apiKey":"key","modelMappings":[{}],"modelMappings":[]}]}}`, allowed: true},
		{name: "malformed earlier mapping member", raw: `{"byok":{"openai":[{"apiKey":"key","modelMappings":42,"modelMappings":[]}]}}`},
		{name: "unused meaningful mapping", raw: `{"byok":{"openai":[{"apiKey":"key"}],"anthropic":[{"apiKey":"unused","modelMappings":[{}]}]}}`},
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
			got, err := decodeGateway(selector, json.RawMessage(tc.raw), policy)
			if tc.allowed {
				require.NoError(t, err)
				assert.Equal(t, tc.endpoint, got.Destinations[0].Accounts[0].BaseURL)
				return
			}
			if tc.want == nil {
				tc.want = ErrInvalidRequest
			}
			require.ErrorIs(t, err, tc.want)
			assert.Empty(t, got)
			for _, private := range []string{"llm.example.com", "denied.example", "127.0.0.1", "169.254", "2130706433", "dummy-secret"} {
				assert.NotContains(t, err.Error(), private)
			}
			if tc.name == "meaningful unsupported mappings" {
				assert.Contains(t, err.Error(), "modelMappings is not supported")
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
		{"standard account field matching", "openai/model", `{"byok":{"openai":[{"APIKEY":"matched","BaseURL":"https://api.openai.com/v1","Organization":"org","Project":"project"}]}}`, OpenAI, "model", []string{"matched"}},
		{"unrelated additive account fields", "openai/model", `{"byok":{"openai":[{"apiKey":"valid","dummy-secret-field":true,"headers":{"Authorization":"dummy-secret"},"future":{"nested":[null,false,42]}}]}}`, OpenAI, "model", []string{"valid"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeGateway(tc.selector, json.RawMessage(tc.controls), outbound.EgressPolicy{})
			require.NoError(t, err)
			require.Len(t, got.Destinations, 1)
			assert.Equal(t, tc.wantProvider, got.Destinations[0].Provider)
			assert.Equal(t, tc.wantModel, got.Destinations[0].Model)
			keys := make([]string, len(got.Destinations[0].Accounts))
			for i, credential := range got.Destinations[0].Accounts {
				keys[i] = credential.APIKey
			}
			assert.Equal(t, tc.wantKeys, keys)
		})
	}
	for _, selector := range []string{"", "alias", "/model", "openai/", "OpenAI/model", "bedrock/model", "openai/" + string([]byte{255})} {
		t.Run("invalid selector/"+selector, func(t *testing.T) {
			got, err := decodeGateway(selector, json.RawMessage(valid), outbound.EgressPolicy{})
			require.ErrorIs(t, err, ErrInvalidRequest)
			assert.Empty(t, got)
		})
	}
	for _, raw := range []string{`{"BYOK":{"openai":[{"apiKey":"k"}]}}`, `{"byok":{"openai":[{"apiKey":"other"}]},"Byok":{"openai":[{"apiKey":"k"}]}}`} {
		t.Run("case variant control/"+raw, func(t *testing.T) {
			got, err := decodeGateway("openai/model", json.RawMessage(raw), outbound.EgressPolicy{})
			require.NoError(t, err)
			require.Len(t, got.Destinations, 1)
			require.Len(t, got.Destinations[0].Accounts, 1)
			assert.Equal(t, "k", got.Destinations[0].Accounts[0].APIKey)
		})
	}
	for _, tc := range []struct {
		raw  string
		want error
	}{
		{`[]`, execution.ErrRoutingControls},
		{`{"byok":{"openai":[{"apiKey":"valid"}]}} {}`, execution.ErrRoutingControls},
		{`{"byok":{"openai":[{"apiKey":"valid"}]},"modelMappings":{}}`, execution.ErrUnsupportedGatewayControl},
		{`{"byok":{"openai":[{"apiKey":"valid"}]},"dummy-secret-control":true}`, execution.ErrUnsupportedGatewayControl},
		{`{"byok":{"openai":[{"apiKey":"valid"}]},"sort":"cost"}`, execution.ErrUnsupportedGatewayControl},
	} {
		t.Run("refused controls/"+tc.raw, func(t *testing.T) {
			got, err := decodeGateway("openai/model", json.RawMessage(tc.raw), outbound.EgressPolicy{})
			require.ErrorIs(t, err, tc.want)
			assert.Empty(t, got)
			assert.NotContains(t, err.Error(), "dummy-secret")
		})
	}
	for _, raw := range []string{
		`{"byok":{"openai":[{"apiKey":"valid"}]},"order":["openai"]}`,
		`{"byok":false,"byok":{"openai":[{"apiKey":"valid"}]}}`,
	} {
		t.Run("accepted controls/"+raw, func(t *testing.T) {
			_, err := decodeGateway("openai/model", json.RawMessage(raw), outbound.EgressPolicy{})
			require.NoError(t, err)
		})
	}
	for _, raw := range []string{
		`null`, `{}`, `{"byok":null}`, `{"byok":{}}`, `{"byok":[]}`,
		`{"byok":{"openai":[{"apiKey":"discarded"}]},"byok":{"anthropic":[{"apiKey":"unused"}]}}`,
		`{"byok":{"openai":null}}`, `{"byok":{"openai":[]}}`, `{"byok":{"openai":[null]}}`,
		`{"byok":{"openai":[{}]}}`, `{"byok":{"openai":[{"apiKey":null}]}}`,
		`{"byok":{"openai":[{"apiKey":12}]}}`, `{"byok":{"openai":[{"apiKey":""}]}}`,
		`{"byok":{"openai":[{"apiKey":42,"apiKey":"valid"}]}}`,
		`{"byok":{"openai":false,"openai":[{"apiKey":"valid"}]}}`,
		`{"byok":{"openai":[{"apiKey":"valid","baseURL":"http://dummy-secret.invalid"}]}}`,
		`{"byok":{"openai":[{"apiKey":"valid"}],"OpenAI":[{"apiKey":"dummy-secret"}]}}`,
		`{"byok":{"dummy-secret-provider":[{"apiKey":"valid"}]}}`,
		`{"byok":{"anthropic":[{"apiKey":"valid"}]}}`,
		`{"byok":{"openai":[{"apiKey":"valid"}],"anthropic":[{}]}}`,
	} {
		t.Run("invalid controls/"+raw, func(t *testing.T) {
			got, err := decodeGateway("openai/model", json.RawMessage(raw), outbound.EgressPolicy{})
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
		{name: "unsupported provider", provider: "bedrock", model: "model", accounts: []nativemodel.Config{{APIKey: "key"}}},
		{name: "missing model", provider: OpenAI, accounts: []nativemodel.Config{{APIKey: "key"}}},
		{name: "missing accounts", provider: OpenAI, model: "model"},
		{name: "too many accounts", provider: OpenAI, model: "model", accounts: make([]nativemodel.Config, config.MaxProviderAttempts+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model, err := newAll(Request{Destinations: []Destination{{Provider: tc.provider, Model: tc.model, Accounts: tc.accounts}}}, http.DefaultClient)
			require.ErrorIs(t, err, ErrInvalidRequest)
			assert.Nil(t, model)
		})
	}
	t.Run("missing transport", func(t *testing.T) {
		request, err := decodeGateway("openai/model", json.RawMessage(`{"byok":{"openai":[{"apiKey":"dummy-key"}]}}`), outbound.EgressPolicy{})
		require.NoError(t, err)
		model, err := newAll(request, nil)
		require.ErrorContains(t, err, "HTTP clients are required")
		assert.Nil(t, model)
	})
	t.Run("decoded request owns credentials", func(t *testing.T) {
		raw := json.RawMessage(`{"byok":{"openai":[{"apiKey":"dummy-key"}]}}`)
		request, err := decodeGateway("openai/model", raw, outbound.EgressPolicy{})
		require.NoError(t, err)
		clear(raw)
		calls := 0
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			assert.Equal(t, "Bearer dummy-key", r.Header.Get("Authorization"))
			assert.Equal(t, "api.openai.com", r.URL.Host)
			return nil, assert.AnError
		})}
		model, err := newAll(request, client)
		require.NoError(t, err)
		_, err = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
		require.Error(t, err)
		assert.Equal(t, 1, calls)
	})
}

func TestDecodeRequest_AccountCount(t *testing.T) {
	for _, name := range []Provider{OpenAI, Anthropic} {
		for _, count := range []int{0, config.MaxProviderAttempts, config.MaxProviderAttempts + 1} {
			t.Run(fmt.Sprintf("%s/%d", name, count), func(t *testing.T) {
				accounts := map[Provider][]nativemodel.Config{OpenAI: {{APIKey: "selected"}}}
				accounts[name] = make([]nativemodel.Config, count)
				for i := range accounts[name] {
					accounts[name][i].APIKey = "key"
				}
				raw, err := json.Marshal(map[string]any{"byok": accounts})
				require.NoError(t, err)
				_, err = decodeGateway("openai/model", raw, outbound.EgressPolicy{})
				selected := name == OpenAI
				if count == 0 || (selected && count > config.MaxProviderAttempts) {
					require.ErrorIs(t, err, ErrInvalidRequest)
				} else {
					require.NoError(t, err)
				}
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
	request, err := decodeGateway("openai/"+modelID, raw, outbound.EgressPolicy{})
	require.NoError(t, err)
	assert.Equal(t, modelID, request.Destinations[0].Model)
	assert.Equal(t, []nativemodel.Config{account}, request.Destinations[0].Accounts)
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
					model := newTestModel(t, string(name)+"/model", raw, allowLoopback(t), server.Client())
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}}
					err = invokeTestModel(ctx, model, streaming, options)
					require.ErrorContains(t, err, "invalid header field value")
					assert.Zero(t, calls.Load())
				})
			}
		}
	}
}

func TestNew_OpenAIStreamInitialEventsAndCommitment(t *testing.T) {
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
			model := newTestModel(t, "openai/native-model", json.RawMessage(`{"byok":{"openai":[{"apiKey":"dummy-first"},{"apiKey":"dummy-second"}]}}`), outbound.EgressPolicy{}, client)
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
			request, err := decodeGateway(fmt.Sprintf("openai/native-%d", i), json.RawMessage(fmt.Sprintf(`{"byok":{"openai":[{"apiKey":"dummy-%d","baseURL":"https://approved.example/v1","organization":"org-%d","project":"proj-%d"}]}}`, i, i, i)), outbound.EgressPolicy{})
			if err != nil {
				results <- err
				return
			}
			model, err := newAll(request, client)
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
				if custom {
					host = "approved.example"
					accountFields = `,"baseURL":"https://approved.example"`
				}
				calls := 0
				client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					assert.Equal(t, host, r.URL.Host)
					return &http.Response{StatusCode: 307, Header: http.Header{"Content-Type": {"application/json"}, "Location": {"https://redirect.invalid"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"redirect"}}`)), Request: r}, nil
				})}
				raw := fmt.Sprintf(`{"byok":{"%s":[{"apiKey":"dummy-first"%s},{"apiKey":"dummy-second"%s}]}}`, name, accountFields, accountFields)
				model := newTestModel(t, string(name)+"/native-model", json.RawMessage(raw), outbound.EgressPolicy{}, client)
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
			name, options, nativeField, beta string
			mcpHistory                       bool
			providerExecuted                 bool
			allowed                          bool
		}{
			{name: "MCP servers", options: mcpOptions, nativeField: "mcp_servers", allowed: true},
			{name: "invalid MCP destination", options: `{"anthropic":{"mcpServers":[{"name":"external","url":"http://example.invalid"}]}}`},
			{name: "malformed custom skill", options: `{"anthropic":{"container":{"skills":[{"type":"custom","skillId":"external"}]}}}`},
			{name: "too many skills", options: `{"anthropic":{"container":{"skills":[` + distinctSkills(21) + `]}}}`},
			{name: "over-long container ID", options: `{"anthropic":{"container":{"id":"` + strings.Repeat("c", 129) + `"}}}`},
			{name: "bounded custom skill with container ID", options: `{"anthropic":{"container":{"id":"container_01","skills":[{"type":"custom","providerReference":{"anthropic":"skill_01external"}}]}}}`, nativeField: "skill_01external", allowed: true},
			{name: "native fallback default", options: `{"anthropic":{"fallbacks":"default"}}`, nativeField: `"fallbacks":"default"`, beta: "server-side-fallback-2026-07-01", allowed: true},
			{name: "native fallback chain with a repeated model", options: `{"anthropic":{"fallbacks":[{"model":"claude-any"},{"model":"claude-any","thinking":{"type":"disabled"}}]}}`, nativeField: `"fallbacks":[{"model":"claude-any"},{"model":"claude-any","thinking":{"type":"disabled"}}]`, beta: "server-side-fallback-2026-06-01", allowed: true},
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
					assert.Contains(t, r.Header.Get("Anthropic-Beta"), tc.beta)
					if tc.allowed {
						assert.Contains(t, string(body), tc.nativeField)
						assert.NotContains(t, string(body), "ignored")
						assert.Contains(t, string(body), "native-model")
					}
					return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"authentication_error","message":"rejected"}}`)), Request: r}, nil
				})}
				model := newTestModel(t, "anthropic/native-model", json.RawMessage(`{"byok":{"anthropic":[{"apiKey":"dummy-first"},{"apiKey":"dummy-second"}]}}`), outbound.EgressPolicy{}, client)
				opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}}
				require.NoError(t, json.Unmarshal([]byte(tc.options), &opts.ProviderOptions))
				if tc.mcpHistory {
					part := provider.ToolCallPart("call", "lookup", json.RawMessage(`{}`))
					part.ProviderExecuted = tc.providerExecuted
					part.ProviderOptions = provider.ProviderOptions{"anthropic": provider.RawProviderOption{Raw: json.RawMessage(`{"type":"mcp-tool-use","serverName":"external"}`)}}
					opts.Prompt = append(opts.Prompt, provider.NewAssistantMessage(part))
				}
				err := invokeTestModel(context.Background(), model, streaming, opts)
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

func distinctSkills(n int) string {
	skills := make([]string, n)
	for i := range skills {
		skills[i] = fmt.Sprintf(`{"type":"anthropic","skillId":"skill-%d"}`, i)
	}
	return strings.Join(skills, ",")
}
