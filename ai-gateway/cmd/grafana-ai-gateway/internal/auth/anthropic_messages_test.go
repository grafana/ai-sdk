package auth

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnthropicMessagesAuthenticator(t *testing.T) {
	tests := []struct {
		name    string
		headers http.Header
		want    string
		bearer  string
		valid   bool
	}{
		{name: "SDK API key", headers: http.Header{"X-Api-Key": {"gateway-token"}}, want: "gateway-token", valid: true},
		{name: "SDK bearer", headers: http.Header{"Authorization": {"Bearer gateway-token"}}, bearer: "Bearer gateway-token", valid: true},
		{name: "Gateway access token", headers: http.Header{"X-Access-Token": {"gateway-token"}}, want: "gateway-token", valid: true},
		{name: "conflicting sources", headers: http.Header{"X-Api-Key": {"one"}, "X-Access-Token": {"two"}}},
		{name: "duplicate API key", headers: http.Header{"X-Api-Key": {"one", "two"}}},
		{name: "authorization delegated to shared verifier", headers: http.Header{"Authorization": {"Basic token"}}, bearer: "Basic token", valid: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var received http.Header
			next := requestAuthenticatorFunc(func(_ context.Context, headers http.Header) (Caller, error) {
				received = headers
				return Caller{}, nil
			})
			_, err := NewAnthropicMessagesAuthenticator(next).Authenticate(context.Background(), tc.headers)
			if tc.valid {
				require.NoError(t, err)
				assert.Equal(t, tc.want, received.Get("X-Access-Token"))
				assert.Empty(t, received.Get("X-Api-Key"))
				assert.Equal(t, tc.bearer, received.Get("Authorization"))
			} else {
				assert.Error(t, err)
				assert.Nil(t, received)
			}
		})
	}
}

type requestAuthenticatorFunc func(context.Context, http.Header) (Caller, error)

func (authenticate requestAuthenticatorFunc) Authenticate(ctx context.Context, headers http.Header) (Caller, error) {
	return authenticate(ctx, headers)
}
