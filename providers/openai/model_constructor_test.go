package openai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewResponses_MissingOutput(t *testing.T) {
	tests := []struct {
		name        string
		response    string
		wantMessage string
		wantStatus  int
		wantError   bool
	}{
		{
			name:        "content filter",
			response:    `{"id":"resp_1","status":"incomplete","incomplete_details":{"reason":"content_filter"},"usage":{"input_tokens":1,"output_tokens":0,"total_tokens":1}}`,
			wantMessage: "Responses API returned no output (content_filter)",
			wantStatus:  http.StatusInternalServerError,
			wantError:   true,
		},
		{
			name:        "no incomplete reason",
			response:    `{"id":"resp_1","status":"completed","usage":{"input_tokens":1,"output_tokens":0,"total_tokens":1}}`,
			wantMessage: "Responses API returned no output",
			wantStatus:  http.StatusInternalServerError,
			wantError:   true,
		},
		{
			name:      "explicit empty output",
			response:  `{"id":"resp_1","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":0,"total_tokens":1}}`,
			wantError: false,
		},
		{
			name:        "response error without output",
			response:    `{"id":"resp_1","status":"failed","error":{"code":"server_error","message":"generation failed"},"usage":{"input_tokens":1,"output_tokens":0,"total_tokens":1}}`,
			wantMessage: "generation failed",
			wantStatus:  http.StatusBadRequest,
			wantError:   true,
		},
		{
			name:        "response error with empty output",
			response:    `{"id":"resp_1","status":"failed","error":{"code":"server_error","message":"generation failed"},"output":[],"usage":{"input_tokens":1,"output_tokens":0,"total_tokens":1}}`,
			wantMessage: "generation failed",
			wantStatus:  http.StatusBadRequest,
			wantError:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-ID": []string{"req_1"}},
					Body:       io.NopCloser(strings.NewReader(tc.response)),
					Request:    req,
				}, nil
			})}
			m := NewResponses("test-key", "gpt-4o", WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))

			result, err := m.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
			if !tc.wantError {
				require.NoError(t, err)
				require.NotNil(t, result)
				return
			}

			require.Error(t, err)
			var apiErr *provider.APICallError
			require.True(t, errors.As(err, &apiErr))
			assert.Equal(t, tc.wantMessage, apiErr.Message)
			assert.Equal(t, tc.wantStatus, apiErr.StatusCode)
			assert.False(t, apiErr.IsRetryable)
			assert.Equal(t, "https://api.openai.com/v1/responses", apiErr.URL)
			assert.Equal(t, []string{"req_1"}, apiErr.ResponseHeaders["X-Request-ID"])
			assert.JSONEq(t, tc.response, apiErr.ResponseBody)
			assert.Contains(t, string(apiErr.RequestBodyValues), `"model":"gpt-4o"`)
		})
	}
}

func TestNewResponses_UsesProductionBaseURLByDefault(t *testing.T) {
	unsetEnv(t, "OPENAI_BASE_URL")

	var capturedURL string
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		capturedURL = req.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"id": "resp_123",
				"created_at": 1700000000,
				"model": "gpt-4o",
				"object": "response",
				"status": "completed",
				"output": [
					{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hello!","annotations":[]}]}
				],
				"usage": {"input_tokens": 5, "output_tokens": 2, "total_tokens": 7}
			}`)),
			Request: req,
		}, nil
	})}

	m := NewResponses("test-key", "gpt-4o",
		WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)),
	)

	res, err := m.DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "https://api.openai.com/v1/responses", capturedURL)
}

func TestModel_PerCallHeaders(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		call        func(t *testing.T, model provider.LanguageModel)
	}{
		{
			name:        "generate",
			contentType: "application/json",
			body: `{
				"id":"resp_123",
				"created_at":1700000000,
				"model":"gpt-4o",
				"object":"response",
				"status":"completed",
				"output":[],
				"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}
			}`,
			call: func(t *testing.T, model provider.LanguageModel) {
				result, err := model.DoGenerate(t.Context(), provider.CallOptions{
					Prompt:  []provider.Message{provider.UserText("hi")},
					Headers: map[string]string{"X-Call-Only": "call", "X-Shared": "call"},
				})
				require.NoError(t, err)
				require.NotNil(t, result)
			},
		},
		{
			name:        "stream",
			contentType: "text/event-stream",
			body: "event: response.completed\n" +
				`data: {"type":"response.completed","sequence_number":0,"response":{"id":"resp_123","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}` + "\n\n",
			call: func(t *testing.T, model provider.LanguageModel) {
				result, err := model.DoStream(t.Context(), provider.CallOptions{
					Prompt:  []provider.Message{provider.UserText("hi")},
					Headers: map[string]string{"X-Call-Only": "call", "X-Shared": "call"},
				})
				require.NoError(t, err)
				require.NotNil(t, result)
				for range result.Stream {
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedHeaders http.Header
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				capturedHeaders = req.Header.Clone()
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{tt.contentType}},
					Body:       io.NopCloser(strings.NewReader(tt.body)),
					Request:    req,
				}, nil
			})}

			model := NewResponses("test-key", "gpt-4o", WithRequestOptions(
				option.WithHTTPClient(client),
				option.WithMaxRetries(0),
				option.WithHeader("X-Configured-Only", "configured"),
				option.WithHeader("X-Shared", "configured"),
			))
			tt.call(t, model)

			assert.Equal(t, "configured", capturedHeaders.Get("X-Configured-Only"))
			assert.Equal(t, "call", capturedHeaders.Get("X-Call-Only"))
			assert.Equal(t, "call", capturedHeaders.Get("X-Shared"))
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	value, ok := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(key, value)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}
