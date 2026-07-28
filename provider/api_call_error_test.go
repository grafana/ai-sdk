package provider

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(b bool) *bool { return &b }

func TestNewAPICallError_DefaultRetryability(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       bool
	}{
		{"408 is retryable", 408, true},
		{"409 is retryable", 409, true},
		{"429 is retryable", 429, true},
		{"500 is retryable", 500, true},
		{"503 is retryable", 503, true},
		{"502 is retryable", 502, true},
		{"400 is not retryable", 400, false},
		{"401 is not retryable", 401, false},
		{"403 is not retryable", 403, false},
		{"404 is not retryable", 404, false},
		{"422 is not retryable", 422, false},
		{"200 is not retryable", 200, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := NewAPICallError(APICallErrorOptions{
				Message:    "test",
				StatusCode: tc.statusCode,
			})
			assert.Equal(t, tc.want, err.IsRetryable)
		})
	}
}

func TestNewAPICallError_ExplicitIsRetryable(t *testing.T) {
	t.Run("true overrides non-retryable status", func(t *testing.T) {
		err := NewAPICallError(APICallErrorOptions{
			Message:     "bad request",
			StatusCode:  400,
			IsRetryable: boolPtr(true),
		})
		assert.True(t, err.IsRetryable)
	})

	t.Run("false overrides retryable status", func(t *testing.T) {
		err := NewAPICallError(APICallErrorOptions{
			Message:     "server error",
			StatusCode:  500,
			IsRetryable: boolPtr(false),
		})
		assert.False(t, err.IsRetryable)
	})
}

func TestAPICallError_Error(t *testing.T) {
	err := NewAPICallError(APICallErrorOptions{
		Message:    "rate limit exceeded",
		StatusCode: 429,
	})

	msg := err.Error()
	assert.Contains(t, msg, "429")
	assert.Contains(t, msg, "rate limit exceeded")
	assert.Equal(t, "aisdk: API call error (status 429): rate limit exceeded", msg)
}

func TestAPICallError_JSONRoundTrip(t *testing.T) {
	cause := errors.New("transport boom")
	original := NewAPICallError(APICallErrorOptions{
		Message:           "rate limit exceeded",
		StatusCode:        429,
		URL:               "https://api.example/v1/messages",
		RequestBodyValues: json.RawMessage(`{"model":"x"}`),
		ResponseHeaders:   map[string][]string{"Retry-After": {"1"}},
		ResponseBody:      `{"error":"too many"}`,
		Data:              json.RawMessage(`{"error_code":42}`),
		Cause:             cause,
	})

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded APICallError
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, original.Message, decoded.Message)
	assert.Equal(t, original.StatusCode, decoded.StatusCode)
	assert.Equal(t, original.URL, decoded.URL)
	assert.JSONEq(t, string(original.RequestBodyValues), string(decoded.RequestBodyValues))
	assert.Equal(t, original.ResponseHeaders, decoded.ResponseHeaders)
	assert.Equal(t, original.ResponseBody, decoded.ResponseBody)
	assert.True(t, decoded.IsRetryable, "IsRetryable preserved")
	assert.JSONEq(t, string(original.Data), string(decoded.Data))

	// cause does not survive the wire.
	assert.Nil(t, decoded.Unwrap())
	// cause is preserved in-process on the original.
	assert.Equal(t, cause, original.Unwrap())
}

func TestAPICallError_JSONShape(t *testing.T) {
	err := NewAPICallError(APICallErrorOptions{
		Message:    "boom",
		StatusCode: 500,
	})
	data, jerr := json.Marshal(err)
	require.NoError(t, jerr)
	assert.JSONEq(t, `{"message":"boom","statusCode":500,"isRetryable":true}`, string(data))
}

func TestAPICallError_Unwrap(t *testing.T) {
	cause := errors.New("original error")
	apiErr := NewAPICallError(APICallErrorOptions{
		Message:    "wrapped",
		StatusCode: 500,
		Cause:      cause,
	})

	t.Run("Unwrap returns cause", func(t *testing.T) {
		unwrapped := apiErr.Unwrap()
		require.NotNil(t, unwrapped)
		assert.Equal(t, cause, unwrapped)
	})

	t.Run("errors.Is traverses chain", func(t *testing.T) {
		assert.True(t, errors.Is(apiErr, cause))
	})

	t.Run("errors.As extracts APICallError", func(t *testing.T) {
		var target *APICallError
		require.True(t, errors.As(apiErr, &target))
		assert.Equal(t, 500, target.StatusCode)
	})

	t.Run("nil cause returns nil", func(t *testing.T) {
		noCause := NewAPICallError(APICallErrorOptions{
			Message:    "no cause",
			StatusCode: 400,
		})
		assert.Nil(t, noCause.Unwrap())
	})
}
