package foundry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Validate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config Config
		valid  bool
	}{
		{name: "api key", config: Config{APIKey: "key"}, valid: true},
		{name: "bearer token", config: Config{AuthToken: "token"}, valid: true},
		{name: "credential callback", config: Config{Credential: func(context.Context) (Credential, error) { return Credential{APIKey: "key"}, nil }}, valid: true},
		{name: "no credential"},
		{name: "key and token", config: Config{APIKey: "key", AuthToken: "token"}},
		{name: "key and callback", config: Config{APIKey: "key", Credential: func(context.Context) (Credential, error) { return Credential{APIKey: "key"}, nil }}},
		{name: "token and callback", config: Config{AuthToken: "token", Credential: func(context.Context) (Credential, error) { return Credential{APIKey: "key"}, nil }}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.config.BaseURL = "https://example.services.ai.azure.com"
			err := tc.config.Validate()
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"account", "https://example.services.ai.azure.com", "https://example.services.ai.azure.com/anthropic/"},
		{"anthropic prefix", "https://example.services.ai.azure.com/anthropic", "https://example.services.ai.azure.com/anthropic/"},
		{"trailing slash", "https://example.services.ai.azure.com/anthropic/", "https://example.services.ai.azure.com/anthropic/"},
		{"loopback", "http://127.0.0.1:9090", "http://127.0.0.1:9090/anthropic/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeBaseURL(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
	for _, raw := range []string{"", "example.services.ai.azure.com", "http://example.services.ai.azure.com", "https://user:password@example.services.ai.azure.com", "https://example.services.ai.azure.com?key=secret", "https://example.services.ai.azure.com#fragment", "ftp://example.services.ai.azure.com"} {
		t.Run(raw, func(t *testing.T) {
			_, err := NormalizeBaseURL(raw)
			require.Error(t, err)
		})
	}
}
