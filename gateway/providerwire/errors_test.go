package providerwire

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeAPICallError_FullRoundTrip(t *testing.T) {
	cause := errors.New("transport boom")
	apiErr := provider.NewAPICallError(provider.APICallErrorOptions{
		Message:           "rate limit exceeded",
		StatusCode:        429,
		URL:               "https://api.example/v1/messages",
		RequestBodyValues: json.RawMessage(`{"model":"x"}`),
		ResponseHeaders:   map[string][]string{"Retry-After": {"1"}},
		ResponseBody:      `{"error":"too many"}`,
		Data:              json.RawMessage(`{"code":42}`),
		Cause:             cause,
	})

	data, err := EncodeAPICallError(apiErr)
	require.NoError(t, err)

	got, err := DecodeAPICallError(data)
	require.NoError(t, err)

	assert.Equal(t, apiErr.Message, got.Message)
	assert.Equal(t, apiErr.StatusCode, got.StatusCode)
	assert.Equal(t, apiErr.URL, got.URL)
	assert.JSONEq(t, string(apiErr.RequestBodyValues), string(got.RequestBodyValues))
	assert.Equal(t, apiErr.ResponseHeaders, got.ResponseHeaders)
	assert.Equal(t, apiErr.ResponseBody, got.ResponseBody)
	assert.Equal(t, apiErr.IsRetryable, got.IsRetryable)
	assert.JSONEq(t, string(apiErr.Data), string(got.Data))

	// cause does not survive the wire.
	assert.Nil(t, got.Unwrap())
}

func TestEncodeDecodeAPICallError_RetryableAndNonRetryable(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   bool
	}{
		{"429 retryable", 429, true},
		{"503 retryable", 503, true},
		{"400 not retryable", 400, false},
		{"401 not retryable", 401, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			apiErr := provider.NewAPICallError(provider.APICallErrorOptions{
				Message:    "x",
				StatusCode: tc.status,
			})
			data, err := EncodeAPICallError(apiErr)
			require.NoError(t, err)
			got, err := DecodeAPICallError(data)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.IsRetryable)
		})
	}
}

func TestEncodeAPICallError_NilReturnsError(t *testing.T) {
	_, err := EncodeAPICallError(nil)
	assert.Error(t, err)
}

func TestWriteErrorResponse(t *testing.T) {
	apiErr := provider.NewAPICallError(provider.APICallErrorOptions{
		Message:      "rate limit exceeded",
		StatusCode:   http.StatusTooManyRequests,
		ResponseBody: `{"error":"too many"}`,
	})

	rec := httptest.NewRecorder()
	require.NoError(t, WriteErrorResponse(rec, apiErr))

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, MIMEJSON, rec.Header().Get("Content-Type"))

	got, err := DecodeAPICallError(rec.Body.Bytes())
	require.NoError(t, err)
	assert.Equal(t, apiErr.Message, got.Message)
	assert.Equal(t, apiErr.StatusCode, got.StatusCode)
	assert.Equal(t, apiErr.IsRetryable, got.IsRetryable)
}

func TestWriteErrorResponse_InvalidStatusDoesNotCommitResponse(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
	}{
		{name: "negative", status: -1},
		{name: "informational", status: http.StatusContinue},
		{name: "success", status: http.StatusOK},
		{name: "bodyless response", status: http.StatusNotModified},
		{name: "above HTTP range", status: 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			apiErr := provider.NewAPICallError(provider.APICallErrorOptions{
				Message:    "invalid status",
				StatusCode: tc.status,
			})

			rec := httptest.NewRecorder()
			err := WriteErrorResponse(rec, apiErr)
			require.Error(t, err)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Empty(t, rec.Header().Get("Content-Type"))
			assert.Empty(t, rec.Body.String())
		})
	}
}

func TestEncodeAPICallError_WrapsInErrorEnvelope(t *testing.T) {
	apiErr := provider.NewAPICallError(provider.APICallErrorOptions{
		Message:    "rate limited",
		StatusCode: http.StatusTooManyRequests,
	})
	data, err := EncodeAPICallError(apiErr)
	require.NoError(t, err)

	var envelope struct {
		Error struct {
			Message     string `json:"message"`
			StatusCode  int    `json:"statusCode"`
			IsRetryable bool   `json:"isRetryable"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(data, &envelope))
	assert.Equal(t, "rate limited", envelope.Error.Message)
	assert.Equal(t, http.StatusTooManyRequests, envelope.Error.StatusCode)
	assert.True(t, envelope.Error.IsRetryable)
}

func TestDecodeAPICallError_AcceptsWrappedAndFlat(t *testing.T) {
	t.Run("wrapped envelope", func(t *testing.T) {
		got, err := DecodeAPICallError([]byte(`{"error":{"message":"boom","statusCode":500,"isRetryable":true}}`))
		require.NoError(t, err)
		assert.Equal(t, "boom", got.Message)
		assert.Equal(t, 500, got.StatusCode)
		assert.True(t, got.IsRetryable)
	})

	t.Run("legacy flat form", func(t *testing.T) {
		got, err := DecodeAPICallError([]byte(`{"message":"boom","statusCode":500,"isRetryable":true}`))
		require.NoError(t, err)
		assert.Equal(t, "boom", got.Message)
		assert.Equal(t, 500, got.StatusCode)
		assert.True(t, got.IsRetryable)
	})
}

func TestWriteErrorResponse_EncodeErrorDoesNotCommitResponse(t *testing.T) {
	apiErr := provider.NewAPICallError(provider.APICallErrorOptions{
		Message:           "bad raw json",
		StatusCode:        http.StatusBadRequest,
		RequestBodyValues: json.RawMessage(`{"broken"`),
	})

	rec := httptest.NewRecorder()
	err := WriteErrorResponse(rec, apiErr)
	require.Error(t, err)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Header().Get("Content-Type"))
	assert.Empty(t, rec.Body.String())
}

func TestDecodeErrorResponse_PopulatesHTTPMetadata(t *testing.T) {
	respBody := `{"message":"bad request","isRetryable":false}`
	req := httptest.NewRequest(http.MethodPost, "https://api.example/language-model", nil)
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"X-Trace-ID": {"trace-1"}},
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Request:    req,
	}

	got, err := DecodeErrorResponse(resp)
	require.NoError(t, err)
	assert.Equal(t, "bad request", got.Message)
	assert.Equal(t, http.StatusBadRequest, got.StatusCode)
	assert.False(t, got.IsRetryable)
	assert.Equal(t, "https://api.example/language-model", got.URL)
	assert.Equal(t, map[string][]string{"X-Trace-ID": {"trace-1"}}, got.ResponseHeaders)
	assert.Equal(t, respBody, got.ResponseBody)
}
