package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_ProviderToolsMCPOptionsReachNativeTransport(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var body map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.JSONEq(t, `[{"type":"code_execution_20260120","name":"code_execution"}]`, string(body["tools"]))
		assert.JSONEq(t, `[{"type":"url","name":"echo","url":"https://mcp.example.test/tools","authorization_token":"mcp-private-token","tool_configuration":{"enabled":false,"allowed_tools":[]}}]`, string(body["mcp_servers"]))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"claude-sonnet-4-6","stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()
	model := New("key", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))
	maxOutputTokens := 64
	_, err := model.DoGenerate(context.Background(), provider.CallOptions{
		MaxOutputTokens: &maxOutputTokens,
		Prompt:          []provider.Message{provider.UserText("Use tools")},
		Tools:           []provider.Tool{{Type: provider.ToolTypeProvider, ID: "anthropic.code_execution_20260120", Name: "python", Args: map[string]json.RawMessage{}}},
		ProviderOptions: provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"mcpServers":[{"type":"url","name":"echo","url":"https://mcp.example.test/tools","authorizationToken":"mcp-private-token","toolConfiguration":{"enabled":false,"allowedTools":[]}}]}`)}},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, requests)
}

func TestDoGenerate_LargeDefaultOutputUsesContextDeadline(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var body map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.JSONEq(t, `128000`, string(body["max_tokens"]))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"claude-sonnet-4-6","stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()
	model := New("key", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))
	options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := model.DoGenerate(ctx, options)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int32(1), requests.Load())

	_, err = model.DoGenerate(context.Background(), options)
	require.ErrorContains(t, err, "streaming is required")
	assert.Equal(t, int32(1), requests.Load())
}

func TestDoGenerate_ExplicitRequestTimeoutOverridesContextDefault(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		select {
		case <-r.Context().Done():
		case <-time.After(500 * time.Millisecond):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`))
		}
	}))
	defer server.Close()
	model := New("key", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0), option.WithRequestTimeout(50*time.Millisecond)))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := model.DoGenerate(ctx, provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
	require.Error(t, err)
	assert.NoError(t, ctx.Err())
	assert.Equal(t, int32(1), requests.Load())
}

func TestDoGenerate_ExpiredDeadlineDoesNotInvokeProvider(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	model := New("key", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err := model.DoGenerate(ctx, provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
	require.True(t, errors.Is(err, context.DeadlineExceeded), "expected deadline cause, got %v", err)
	assert.Zero(t, requests.Load())
}

func TestNew_MessagesRequestTarget(t *testing.T) {
	for _, tc := range []struct {
		name           string
		requestOptions []option.RequestOption
		wantTarget     string
		betas          []string
		wantHeaderBeta string
		wantMetadata   string
	}{
		{name: "default", wantTarget: "/v1/messages"},
		{name: "provider beta headers", wantTarget: "/v1/messages", betas: []string{"test-beta"}},
		{
			name: "caller repeated query and body options",
			requestOptions: []option.RequestOption{
				option.WithQuery("api-version", "v1"),
				option.WithQueryAdd("feature", "a"),
				option.WithQueryAdd("feature", "b"),
				option.WithJSONSet("metadata", map[string]string{"user_id": "test-user"}),
			},
			wantTarget:   "/v1/messages?api-version=v1&feature=a&feature=b",
			betas:        []string{"test-beta"},
			wantMetadata: `{"user_id":"test-user"}`,
		},
		{
			name:           "caller beta query override",
			requestOptions: []option.RequestOption{option.WithQuery("beta", "false")},
			wantTarget:     "/v1/messages?beta=false",
			betas:          []string{"test-beta"},
		},
		{
			name:           "caller beta header",
			requestOptions: []option.RequestOption{option.WithHeader("anthropic-beta", "caller-beta")},
			wantTarget:     "/v1/messages",
			betas:          []string{"test-beta"},
			wantHeaderBeta: "caller-beta",
		},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				type capturedRequest struct {
					method, target string
					headers        http.Header
					body           []byte
					err            error
				}
				requests := make(chan capturedRequest, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					requests <- capturedRequest{r.Method, r.RequestURI, r.Header.Clone(), body, err}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"captured"}}`)
				}))
				defer server.Close()

				requestOptions := []option.RequestOption{
					option.WithBaseURL(server.URL),
					option.WithHTTPClient(server.Client()),
					option.WithMaxRetries(0),
					option.WithHeader("X-Test", "preserved"),
				}
				requestOptions = append(requestOptions, tc.requestOptions...)
				model := New("test-key", "claude-sonnet-4-6", WithRequestOptions(requestOptions...))
				maxTokens := 64
				opts := provider.CallOptions{
					Prompt:          []provider.Message{provider.UserText("hello")},
					MaxOutputTokens: &maxTokens,
					ProviderOptions: provider.BuildProviderOptions(AnthropicOptions{Betas: tc.betas}),
				}
				var err error
				if stream {
					_, err = model.DoStream(t.Context(), opts)
				} else {
					_, err = model.DoGenerate(t.Context(), opts)
				}
				require.ErrorContains(t, err, "captured")
				captured := <-requests
				require.NoError(t, captured.err)
				assert.Equal(t, http.MethodPost, captured.method)
				assert.Equal(t, tc.wantTarget, captured.target)
				assert.Equal(t, "preserved", captured.headers.Get("X-Test"))
				betas := strings.Join(captured.headers.Values("anthropic-beta"), ",")
				for _, beta := range tc.betas {
					assert.Contains(t, betas, beta)
				}
				if tc.wantHeaderBeta != "" {
					assert.Contains(t, betas, tc.wantHeaderBeta)
				}
				if len(tc.betas) == 0 && tc.wantHeaderBeta == "" {
					assert.Empty(t, betas)
				}
				var body map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(captured.body, &body))
				assert.JSONEq(t, `"claude-sonnet-4-6"`, string(body["model"]))
				assert.Equal(t, stream, string(body["stream"]) == "true")
				if tc.wantMetadata != "" {
					assert.JSONEq(t, tc.wantMetadata, string(body["metadata"]))
				}
			})
		}
	}
}
