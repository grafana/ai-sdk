package grafana

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ provider.ProviderOption = GatewayOptions{}

func gatewayOptionsJSON(t *testing.T, option provider.ProviderOption) string {
	t.Helper()
	body, err := encodeRequest(provider.CallOptions{Prompt: []provider.Message{}, ProviderOptions: provider.ProviderOptions{option.ProviderKey(): option}})
	require.NoError(t, err)
	var request struct {
		ProviderOptions map[string]json.RawMessage `json:"providerOptions"`
	}
	require.NoError(t, json.Unmarshal(body, &request))
	return string(request.ProviderOptions["gateway"])
}

func TestGatewayOptions_Encoding(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options GatewayOptions
		want    string
	}{
		{name: "unset", want: `{}`},
		{name: "nil only is absent", options: GatewayOptions{Models: []string{"b"}}, want: `{"models":["b"]}`},
		{name: "empty only is kept", options: GatewayOptions{Only: []string{}}, want: `{"only":[]}`},
		{name: "routing controls keep order", options: GatewayOptions{Models: []string{"b", "a"}, Order: []string{"openai", "anthropic"}, Only: []string{"anthropic", "openai"}}, want: `{"models":["b","a"],"order":["openai","anthropic"],"only":["anthropic","openai"]}`},
		{
			name: "request accounts",
			options: GatewayOptions{
				Models: []string{"openai/y"},
				BYOK: map[string][]BYOKAccount{
					"anthropic": {{APIKey: "dummy-a1"}, {APIKey: "dummy-a2", BaseURL: "https://approved.example/anthropic"}},
					"openai":    {{APIKey: "dummy-o1", Organization: "org", Project: "proj"}},
				},
			},
			want: `{"models":["openai/y"],"byok":{"anthropic":[{"apiKey":"dummy-a1"},{"apiKey":"dummy-a2","baseURL":"https://approved.example/anthropic"}],"openai":[{"apiKey":"dummy-o1","organization":"org","project":"proj"}]}}`,
		},
		{name: "values are not validated", options: GatewayOptions{Order: []string{""}, BYOK: map[string][]BYOKAccount{"vertex": {}}}, want: `{"order":[""],"byok":{"vertex":[]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			typed := gatewayOptionsJSON(t, tc.options)
			assert.JSONEq(t, tc.want, typed)
			raw := gatewayOptionsJSON(t, provider.RawProviderOption{Key: "gateway", Raw: json.RawMessage(tc.want)})
			assert.JSONEq(t, raw, typed)
		})
	}
}

func TestGatewayOptions_OneGatewayRequest(t *testing.T) {
	calls := 0
	p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), `"gateway":{"models":["b","c","d"]}`)
		assert.Equal(t, "a", r.Header.Get("Ai-Language-Model-Id"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"message":"a providerOptions.gateway.models entry is not an available model","type":"model_not_found","param":null,"code":"model_not_found"}}`)
	})
	model, err := p.LanguageModel("a")
	require.NoError(t, err)
	_, err = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}, ProviderOptions: provider.ProviderOptions{"gateway": GatewayOptions{Models: []string{"b", "c", "d"}}}})
	var gateway *GatewayError
	require.ErrorAs(t, err, &gateway)
	assert.Equal(t, GatewayModelNotFound, gateway.Category)
	assert.False(t, gateway.IsRetryable)
	assert.Equal(t, 1, calls)
}
