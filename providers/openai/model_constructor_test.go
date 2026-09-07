package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	openaisdk "github.com/openai/openai-go/v3"
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

func TestWithProviderName_EmptyPreservesDefault(t *testing.T) {
	m := NewResponses("test-key", "gpt-4o", WithProviderName(""))
	assert.Equal(t, "openai", m.Provider())
}

func TestNewResponsesWithClient_PreservesProviderClientConfigurationAndContinuation(t *testing.T) {
	var capturedRequests []*http.Request
	var capturedBodies [][]byte
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		capturedRequests = append(capturedRequests, req.Clone(req.Context()))
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		capturedBodies = append(capturedBodies, body)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"id":"resp_123",
				"created_at":1700000000,
				"model":"provider-model",
				"object":"response",
				"status":"completed",
				"output":[{
					"type":"message",
					"id":"msg_1",
					"role":"assistant",
					"status":"completed",
					"content":[{"type":"output_text","text":"first answer","annotations":[]}]
				}],
				"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}
			}`)),
			Request: req,
		}, nil
	})}
	client := openaisdk.NewClient(
		option.WithAPIKey("provider-key"),
		option.WithBaseURL("https://provider.example.test/v1"),
		option.WithHTTPClient(httpClient),
		option.WithHeader("X-Provider-Header", "provider"),
		option.WithMaxRetries(0),
	)

	m := NewResponsesWithClient(
		client,
		"provider-model",
		WithProviderName("example.responses"),
		WithRequestOptions(option.WithHeader("X-Model-Header", "model")),
	)
	assert.Equal(t, "example.responses", m.Provider())

	result, err := m.DoGenerate(t.Context(), provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("hi")},
		ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Instructions: "provider option"}),
	})
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	require.Contains(t, result.ProviderMetadata, "openai")
	assert.NotContains(t, result.ProviderMetadata, "example.responses")
	require.NotNil(t, result.Response)
	assert.Equal(t, "example.responses", result.Response.Provider)
	require.Contains(t, result.Content[0].ProviderMetadata, "openai")
	assert.NotContains(t, result.Content[0].ProviderMetadata, "example.responses")

	partOptions := make(provider.ProviderOptions, len(result.Content[0].ProviderMetadata))
	for name, raw := range result.Content[0].ProviderMetadata {
		partOptions[name] = provider.RawProviderOption{Key: name, Raw: raw}
	}
	_, err = m.DoGenerate(t.Context(), provider.CallOptions{
		Prompt: []provider.Message{
			provider.UserText("hi"),
			provider.NewAssistantMessage(provider.ContentPart{
				Type:            provider.ContentPartTypeText,
				Text:            result.Content[0].Text,
				ProviderOptions: partOptions,
			}),
			provider.UserText("continue"),
		},
	})
	require.NoError(t, err)
	require.Len(t, capturedRequests, 2)
	require.Len(t, capturedBodies, 2)

	assert.Equal(t, "https://provider.example.test/v1/responses", capturedRequests[0].URL.String())
	assert.Equal(t, "Bearer provider-key", capturedRequests[0].Header.Get("Authorization"))
	assert.Equal(t, "provider", capturedRequests[0].Header.Get("X-Provider-Header"))
	assert.Equal(t, "model", capturedRequests[0].Header.Get("X-Model-Header"))
	assert.Contains(t, string(capturedBodies[0]), `"instructions":"provider option"`)

	var continuationBody map[string]any
	require.NoError(t, json.Unmarshal(capturedBodies[1], &continuationBody))
	reference := findInput(continuationBody, "item_reference")
	require.NotNil(t, reference)
	assert.Equal(t, "msg_1", reference["id"])
	assert.NotContains(t, string(capturedBodies[1]), "first answer")
}

func TestNewResponsesWithClient_StreamMetadataUsesOpenAIOptionsNamespace(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader("event: response.created\n" +
				`data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_123","created_at":1700000000,"model":"provider-model","object":"response","status":"in_progress","output":[]}}` + "\n\n" +
				"event: response.output_item.added\n" +
				`data: {"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","content":[]}}` + "\n\n" +
				"event: response.output_text.delta\n" +
				`data: {"type":"response.output_text.delta","sequence_number":2,"output_index":0,"content_index":0,"item_id":"msg_1","delta":"answer","logprobs":[]}` + "\n\n" +
				"event: response.output_item.done\n" +
				`data: {"type":"response.output_item.done","sequence_number":3,"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"answer","annotations":[]}]}}` + "\n\n" +
				"event: response.completed\n" +
				`data: {"type":"response.completed","sequence_number":4,"response":{"id":"resp_123","created_at":1700000000,"model":"provider-model","object":"response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}` + "\n\n")),
			Request: req,
		}, nil
	})}
	client := openaisdk.NewClient(
		option.WithAPIKey("provider-key"),
		option.WithHTTPClient(httpClient),
		option.WithMaxRetries(0),
	)
	m := NewResponsesWithClient(client, "provider-model", WithProviderName("example.responses"))

	result, err := m.DoStream(t.Context(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
	require.NoError(t, err)

	var responseMetadata provider.StreamPart
	var textEnd provider.StreamPart
	var finish provider.StreamPart
	for part := range result.Stream {
		switch part.Type {
		case provider.PartResponseMeta:
			responseMetadata = part
		case provider.PartTextEnd:
			textEnd = part
		case provider.PartFinish:
			finish = part
		}
	}
	require.Equal(t, provider.PartResponseMeta, responseMetadata.Type)
	assert.Equal(t, "example.responses", responseMetadata.Provider)
	require.Equal(t, provider.PartTextEnd, textEnd.Type)
	require.Contains(t, textEnd.ProviderMetadata, "openai")
	assert.NotContains(t, textEnd.ProviderMetadata, "example.responses")
	require.Equal(t, provider.PartFinish, finish.Type)
	require.Contains(t, finish.ProviderMetadata, "openai")
	assert.NotContains(t, finish.ProviderMetadata, "example.responses")
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
