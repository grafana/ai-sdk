package openai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoGenerate_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad request","type":"invalid_request_error","code":"invalid"}}`))
	}))
	defer srv.Close()

	m := NewResponses("test-key", "gpt-4o",
		WithRequestOptions(option.WithBaseURL(srv.URL), option.WithHTTPClient(srv.Client()), option.WithMaxRetries(0)),
	)

	_, err := m.DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
	require.Error(t, err)

	var apiErr *provider.APICallError
	require.True(t, errors.As(err, &apiErr), "error should be *provider.APICallError, got %T", err)
	assert.Equal(t, 400, apiErr.StatusCode)
	assert.Contains(t, apiErr.Message, "bad request")
	assert.False(t, apiErr.IsRetryable)
	assert.NotEmpty(t, apiErr.Data, "structured error data should be present")
}

func TestDoStream_APIError_EmitsPartError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit_error"}}`))
	}))
	defer srv.Close()

	m := NewResponses("test-key", "gpt-4o",
		WithRequestOptions(option.WithBaseURL(srv.URL), option.WithHTTPClient(srv.Client()), option.WithMaxRetries(0)),
	)

	res, err := m.DoStream(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
	require.NoError(t, err)

	var sawError bool
	for part := range res.Stream {
		if part.Type == provider.PartError {
			sawError = true
			require.NotNil(t, part.APICallError)
			assert.Equal(t, 429, part.APICallError.StatusCode)
			assert.True(t, part.APICallError.IsRetryable)
		}
	}
	assert.True(t, sawError, "stream should emit a PartError carrying an APICallError")
}

func TestNewResponses_DoesNotPanic(t *testing.T) {
	require.NotPanics(t, func() {
		_ = NewResponses("k", "gpt-4o")
	})
}
