package openaicompatible

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenLimit_NativeHTTP(t *testing.T) {
	for _, field := range []MaxTokensField{"", MaxTokensFieldLegacy, MaxTokensFieldCompletion} {
		for _, streaming := range []bool{false, true} {
			for _, raw := range []bool{false, true} {
				limit := 27
				options := provider.CallOptions{MaxOutputTokens: &limit, Prompt: []provider.Message{provider.UserText("test")}, ProviderOptions: provider.BuildProviderOptions(OpenAIOptions{MaxTokensField: field})}
				if raw {
					encoded, err := json.Marshal(OpenAIOptions{MaxTokensField: field})
					require.NoError(t, err)
					options.ProviderOptions = provider.ProviderOptions{"vendor": provider.RawProviderOption{Key: "vendor", Raw: encoded}}
				}
				calls := 0
				client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					calls++
					var body map[string]json.RawMessage
					require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
					expected := field
					if expected == "" {
						expected = MaxTokensFieldLegacy
					}
					assert.JSONEq(t, "27", string(body[string(expected)]))
					other := MaxTokensFieldLegacy
					if expected == other {
						other = MaxTokensFieldCompletion
					}
					assert.NotContains(t, body, string(other))
					assert.NotContains(t, body, "maxTokensField")
					return &http.Response{StatusCode: 418, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"synthetic capture"}}`)), Request: req}, nil
				})}
				model := New("native", WithProviderName("vendor.chat"), WithHTTPClient(client))
				var err error
				if streaming {
					_, err = model.DoStream(context.Background(), options)
				} else {
					_, err = model.DoGenerate(context.Background(), options)
				}
				require.Error(t, err)
				assert.Equal(t, 1, calls)
				assert.Equal(t, 27, *options.MaxOutputTokens)
			}
		}
	}
}

func TestTokenLimit_InvalidSelectorWithoutBudget(t *testing.T) {
	_, _, err := (&model{modelID: "native", providerName: "vendor"}).buildRequest(provider.CallOptions{ProviderOptions: provider.BuildProviderOptions(OpenAIOptions{MaxTokensField: "messages"})}, false)
	require.Error(t, err)
}

func TestTokenLimit_CaseInsensitiveLocalOption(t *testing.T) {
	n := 7
	body, _, err := (&model{modelID: "native", providerName: "vendor"}).buildRequest(provider.CallOptions{MaxOutputTokens: &n, ProviderOptions: provider.ProviderOptions{"vendor": provider.RawProviderOption{Key: "vendor", Raw: json.RawMessage(`{"MaxTokensField":"max_completion_tokens"}`)}}}, false)
	require.NoError(t, err)
	assert.Equal(t, 7, body["max_completion_tokens"])
	assert.NotContains(t, body, "MaxTokensField")
}
