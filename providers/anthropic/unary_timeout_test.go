package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
