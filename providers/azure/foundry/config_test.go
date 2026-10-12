package foundry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_CredentialFailuresDoNotRetry(t *testing.T) {
	callbackErr := errors.New("credential source unavailable")
	for _, tc := range []struct {
		name       string
		credential Credential
		err        error
	}{
		{name: "callback error", err: callbackErr},
		{name: "empty credential"},
		{name: "conflicting credentials", credential: Credential{APIKey: "key", AuthToken: "token"}},
	} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/streaming=%t", tc.name, streaming), func(t *testing.T) {
				var calls, requests atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					requests.Add(1)
					w.WriteHeader(http.StatusOK)
				}))
				t.Cleanup(server.Close)
				opts, err := (Config{BaseURL: server.URL, Credential: func(context.Context) (Credential, error) {
					calls.Add(1)
					return tc.credential, tc.err
				}}).RequestOptions()
				require.NoError(t, err)
				client := anthropic.NewClient(opts...)
				params := anthropic.MessageNewParams{
					Model: "test-deployment", MaxTokens: 1,
					Messages: []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hello"))},
				}
				if streaming {
					stream := client.Messages.NewStreaming(context.Background(), params)
					assert.False(t, stream.Next())
					err = stream.Err()
					require.NoError(t, stream.Close())
				} else {
					_, err = client.Messages.New(context.Background(), params)
				}
				require.Error(t, err)
				if tc.err != nil {
					assert.ErrorIs(t, err, tc.err)
				}
				assert.Equal(t, int32(1), calls.Load())
				assert.Zero(t, requests.Load())
			})
		}
	}
}

func TestConfig_RefreshesCredentialOnHTTPRetry(t *testing.T) {
	var calls atomic.Int32
	var keys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("x-api-key"))
		w.Header().Set("Content-Type", "application/json")
		if len(keys) == 1 {
			w.Header().Set("retry-after-ms", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"retry"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"msg_retry","type":"message","role":"assistant","model":"test-deployment","content":[],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":0}}`))
	}))
	t.Cleanup(server.Close)
	opts, err := (Config{BaseURL: server.URL, Credential: func(context.Context) (Credential, error) {
		return Credential{APIKey: fmt.Sprintf("key-%d", calls.Add(1))}, nil
	}}).RequestOptions()
	require.NoError(t, err)
	client := anthropic.NewClient(opts...)
	_, err = client.Messages.New(context.Background(), anthropic.MessageNewParams{
		Model: "test-deployment", MaxTokens: 1,
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hello"))},
	})
	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load())
	assert.Equal(t, []string{"key-1", "key-2"}, keys)
}

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
