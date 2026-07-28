package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMockAPIError(statusCode int) *sdk.Error {
	return &sdk.Error{
		StatusCode: statusCode,
		Request: &http.Request{
			Method: "POST",
			URL:    &url.URL{Scheme: "https", Host: "api.anthropic.com", Path: "/v1/messages"},
		},
		Response: &http.Response{
			StatusCode: statusCode,
			Header:     http.Header{"X-Request-Id": {"req-123"}},
		},
	}
}

// newMockAPIErrorWithBody builds an *sdk.Error whose RawJSON() returns body, by
// unmarshaling the body into the SDK error (which populates the internal raw
// JSON and error type) and then attaching a Request/Response so Error() works.
func newMockAPIErrorWithBody(statusCode int, body string) *sdk.Error {
	var apiErr sdk.Error
	if err := json.Unmarshal([]byte(body), &apiErr); err != nil {
		panic(err)
	}
	apiErr.StatusCode = statusCode
	apiErr.Request = &http.Request{
		Method: "POST",
		URL:    &url.URL{Scheme: "https", Host: "api.anthropic.com", Path: "/v1/messages"},
	}
	apiErr.Response = &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"X-Request-Id": {"req-123"}},
	}
	return &apiErr
}

func TestWrapAPIError_Retryable429(t *testing.T) {
	original := newMockAPIError(429)

	wrapped := wrapAPIError(original, "https://api.anthropic.com/v1/messages", map[string]string{"model": "claude"})

	var apiErr *provider.APICallError
	require.True(t, errors.As(wrapped, &apiErr))
	assert.Equal(t, 429, apiErr.StatusCode)
	assert.True(t, apiErr.IsRetryable)
	assert.Equal(t, "https://api.anthropic.com/v1/messages", apiErr.URL)
}

func TestWrapAPIError_NonRetryable400(t *testing.T) {
	original := newMockAPIError(400)

	wrapped := wrapAPIError(original, "https://api.anthropic.com/v1/messages", nil)

	var apiErr *provider.APICallError
	require.True(t, errors.As(wrapped, &apiErr))
	assert.Equal(t, 400, apiErr.StatusCode)
	assert.False(t, apiErr.IsRetryable)
}

func TestWrapAPIError_StructuredTypeUsesHTTPStatusInference(t *testing.T) {
	original := newMockAPIErrorWithBody(400, `{"type":"error","error":{"type":"api_error","message":"failed"}}`)

	wrapped := wrapAPIError(original, "https://api.anthropic.com/v1/messages", nil)

	var apiErr *provider.APICallError
	require.True(t, errors.As(wrapped, &apiErr))
	assert.Equal(t, 400, apiErr.StatusCode)
	assert.False(t, apiErr.IsRetryable)
}

func TestWrapAPIError_NonAPIError(t *testing.T) {
	original := errors.New("connection refused")

	wrapped := wrapAPIError(original, "https://api.anthropic.com/v1/messages", nil)

	assert.Equal(t, original, wrapped)

	var apiErr *provider.APICallError
	assert.False(t, errors.As(wrapped, &apiErr))
}

func TestWrapAPIError_UnwrapReturnsOriginal(t *testing.T) {
	original := newMockAPIError(503)

	wrapped := wrapAPIError(original, "", nil)

	var apiErr *provider.APICallError
	require.True(t, errors.As(wrapped, &apiErr))

	var sdkErr *sdk.Error
	require.True(t, errors.As(apiErr, &sdkErr))
	assert.Equal(t, 503, sdkErr.StatusCode)
}

func TestWrapAPIError_ResponseHeaders(t *testing.T) {
	original := newMockAPIError(500)

	wrapped := wrapAPIError(original, "", nil)

	var apiErr *provider.APICallError
	require.True(t, errors.As(wrapped, &apiErr))
	require.NotNil(t, apiErr.ResponseHeaders)
	assert.Equal(t, "req-123", apiErr.ResponseHeaders["X-Request-Id"][0])
}

func TestWrapAPIError_PopulatesData(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		wantType string
	}{
		{
			name:     "rate limit",
			status:   429,
			body:     `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`,
			wantType: "rate_limit_error",
		},
		{
			name:     "authentication",
			status:   401,
			body:     `{"type":"error","error":{"type":"authentication_error","message":"bad key"}}`,
			wantType: "authentication_error",
		},
		{
			name:     "invalid request",
			status:   400,
			body:     `{"type":"error","error":{"type":"invalid_request_error","message":"too long"}}`,
			wantType: "invalid_request_error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			original := newMockAPIErrorWithBody(tc.status, tc.body)

			wrapped := wrapAPIError(original, "https://api.anthropic.com/v1/messages", nil)

			var apiErr *provider.APICallError
			require.True(t, errors.As(wrapped, &apiErr))
			require.NotEmpty(t, apiErr.Data)

			// Layer 1 contract: the structured provider error type is preserved
			// in Data. Classification (layer 2) is the gateway provider's job and
			// is tested there.
			var env struct {
				Error struct {
					Type string `json:"type"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(apiErr.Data, &env))
			assert.Equal(t, tc.wantType, env.Error.Type)
		})
	}
}

func TestWrapAPIError_NoStructuredBody_LeavesDataEmpty(t *testing.T) {
	// newMockAPIError has no body, so RawJSON() is empty.
	original := newMockAPIError(500)

	wrapped := wrapAPIError(original, "", nil)

	var apiErr *provider.APICallError
	require.True(t, errors.As(wrapped, &apiErr))
	assert.Empty(t, apiErr.Data)
}

func TestDoStream_APIError_ReturnsAPICallErrorWithData(t *testing.T) {
	const errBody = `{"type":"error","error":{"type":"overloaded_error","message":"overloaded"}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(errBody))
	}))
	defer srv.Close()

	m := New("test-key", "claude-sonnet-4-5",
		WithRequestOptions(
			option.WithBaseURL(srv.URL),
			option.WithHTTPClient(srv.Client()),
			option.WithMaxRetries(0),
		),
	)

	result, err := m.DoStream(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hello")},
	})
	require.Error(t, err)
	assert.Nil(t, result)

	var apiErr *provider.APICallError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
	assert.True(t, apiErr.IsRetryable)

	require.NotEmpty(t, apiErr.Data)
	var env struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(apiErr.Data, &env))
	assert.Equal(t, "overloaded_error", env.Error.Type)
}
