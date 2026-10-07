package grafana

import (
	"context"
	"net/http"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvider_ReservedAuthenticationHeaders(t *testing.T) {
	for _, flow := range []struct {
		name string
		new  func(http.Header) (*Provider, error)
	}{
		{"access", func(headers http.Header) (*Provider, error) {
			return NewWithAccessToken(AccessTokenConfig{AccessToken: "dummy-access", BaseURL: "https://gateway.invalid", Headers: headers})
		}},
		{"exchange", func(headers http.Header) (*Provider, error) {
			return NewWithTokenExchange(TokenExchangeConfig{CAPToken: "dummy-cap", Namespace: "stacks-1", TokenExchangeURL: "https://exchange.invalid", BaseURL: "https://gateway.invalid", Headers: headers})
		}},
		{"cloud", func(headers http.Header) (*Provider, error) {
			return NewWithCloudCredentials(CloudCredentialsConfig{StackID: 1, CAPToken: "dummy-cap", BaseURL: "https://gateway.invalid", Headers: headers})
		}},
	} {
		t.Run(flow.name, func(t *testing.T) {
			p, err := flow.new(nil)
			require.NoError(t, err)
			calls := 0
			p.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				t.Error("reserved authentication header reached network")
				return nil, context.Canceled
			})
			model, err := p.LanguageModel("openai/model")
			require.NoError(t, err)
			for _, name := range []string{"Authorization", "aUtHoRiZaTiOn", "X-Access-Token", "x-grafana-id", "X-Scope-OrgID", "X-Cloud-Org-ID", "X-Access-Policy-ID"} {
				for _, value := range []string{"", "dummy-private-marker"} {
					t.Run(name+"/"+value, func(t *testing.T) {
						configured, err := flow.new(http.Header{name: {value}})
						require.Error(t, err)
						assert.Nil(t, configured)
						assert.NotContains(t, err.Error(), "dummy-private-marker")
						opts := provider.CallOptions{Headers: map[string]string{name: value}}
						generated, err := model.DoGenerate(context.Background(), opts)
						require.Error(t, err)
						assert.Nil(t, generated)
						assert.NotContains(t, err.Error(), "dummy-private-marker")
						streamed, err := model.DoStream(context.Background(), opts)
						require.Error(t, err)
						assert.Nil(t, streamed)
						assert.NotContains(t, err.Error(), "dummy-private-marker")
					})
				}
			}
			assert.Zero(t, calls)
		})
	}
}
