package bedrock

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStubBedrockProvider(t *testing.T, handler http.Handler) provider.LanguageModel {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return New("anthropic.claude-sonnet-4-5-20250929-v1:0",
		WithRegion("us-east-1"),
		WithBaseURL(server.URL),
		WithCredentials(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")),
	)
}

func TestEncodeModelIDPathSegment(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		want    string
	}{
		{
			name:    "standard model id",
			modelID: "anthropic.claude-sonnet-4-5-20250929-v1:0",
			want:    "anthropic.claude-sonnet-4-5-20250929-v1%3A0",
		},
		{
			name:    "application inference profile ARN",
			modelID: "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/abc123xyz",
			want:    "arn%3Aaws%3Abedrock%3Aus-east-1%3A123456789012%3Aapplication-inference-profile%2Fabc123xyz",
		},
		{
			name:    "inference profile ARN",
			modelID: "arn:aws:bedrock:eu-west-1:474668406012:inference-profile/eu.amazon.nova-lite-v1:0",
			want:    "arn%3Aaws%3Abedrock%3Aeu-west-1%3A474668406012%3Ainference-profile%2Feu.amazon.nova-lite-v1%3A0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, encodeModelIDPathSegment(tt.modelID))
		})
	}
}

func TestInferenceProfileARNRequestPath(t *testing.T) {
	const modelID = "arn:aws:bedrock:eu-west-1:474668406012:inference-profile/eu.amazon.nova-lite-v1:0"
	const encodedModelID = "arn%3Aaws%3Abedrock%3Aeu-west-1%3A474668406012%3Ainference-profile%2Feu.amazon.nova-lite-v1%3A0"

	t.Run("generate", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/model/"+encodedModelID+"/converse", r.URL.EscapedPath())
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"output":{"message":{"role":"assistant","content":[{"text":"ok"}]}},"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":1}}`))
		}))
		defer server.Close()
		model := New(modelID, WithRegion("us-east-1"), WithBaseURL(server.URL), WithCredentials(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")))
		_, err := model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
		require.NoError(t, err)
	})

	t.Run("stream", func(t *testing.T) {
		frames := encodeFixtures(t,
			`{"messageStart":{"role":"assistant"}}`,
			`{"messageStop":{"stopReason":"end_turn"}}`,
			`{"metadata":{"usage":{"inputTokens":1,"outputTokens":1}}}`,
		)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/model/"+encodedModelID+"/converse-stream", r.URL.EscapedPath())
			w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
			_, _ = w.Write(frames)
		}))
		defer server.Close()
		model := New(modelID, WithRegion("us-east-1"), WithBaseURL(server.URL), WithCredentials(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")))
		result, err := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
		require.NoError(t, err)
		for range result.Stream {
		}
	})
}

func TestDoGenerate_Success(t *testing.T) {
	respBody := `{
		"output": {"message": {"role": "assistant", "content": [{"text": "hello"}]}},
		"stopReason": "end_turn",
		"usage": {"inputTokens": 10, "outputTokens": 5}
	}`
	lm := newStubBedrockProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.HasSuffix(r.URL.Path, "/converse"))
		body, _ := io.ReadAll(r.Body)
		var req converseInput
		require.NoError(t, json.Unmarshal(body, &req))
		require.NotEmpty(t, req.Messages)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Amzn-RequestId", "req-abc")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(respBody))
		require.NoError(t, err)
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := lm.DoGenerate(ctx, provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	assert.Equal(t, "hello", result.Content[0].Text)
	assert.Equal(t, "req-abc", result.Response.ID)
}

func TestDoGenerate_NonRetryableError(t *testing.T) {
	lm := newStubBedrockProvider(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, err := w.Write([]byte(`{"message":"validation failed","type":"ValidationException"}`))
		require.NoError(t, err)
	}))

	ctx := context.Background()
	_, err := lm.DoGenerate(ctx, provider.CallOptions{Prompt: []provider.Message{provider.UserText("x")}})
	require.Error(t, err)
	apiErr, ok := err.(*provider.APICallError)
	require.True(t, ok, "expected *APICallError, got %T", err)
	assert.Equal(t, 400, apiErr.StatusCode)
	assert.False(t, apiErr.IsRetryable)
	assert.Contains(t, apiErr.Message, "validation failed")
}

func TestDoGenerate_RetryableError(t *testing.T) {
	lm := newStubBedrockProvider(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, err := w.Write([]byte(`{"message":"rate","type":"ThrottlingException"}`))
		require.NoError(t, err)
	}))

	_, err := lm.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("x")}})
	require.Error(t, err)
	apiErr := err.(*provider.APICallError)
	assert.Equal(t, 429, apiErr.StatusCode)
	assert.True(t, apiErr.IsRetryable)
}

func TestDoGenerate_5xxRetryable(t *testing.T) {
	lm := newStubBedrockProvider(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, err := w.Write([]byte(`{"message":"unavailable"}`))
		require.NoError(t, err)
	}))
	_, err := lm.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("x")}})
	require.Error(t, err)
	apiErr := err.(*provider.APICallError)
	assert.Equal(t, 503, apiErr.StatusCode)
	assert.True(t, apiErr.IsRetryable)
}

func TestDoStream_Success(t *testing.T) {
	frames := encodeFixtures(t,
		`{"messageStart":{"role":"assistant"}}`,
		`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":"hi"}}}`,
		`{"contentBlockStop":{"contentBlockIndex":0}}`,
		`{"messageStop":{"stopReason":"end_turn"}}`,
		`{"metadata":{"usage":{"inputTokens":1,"outputTokens":1}}}`,
	)
	lm := newStubBedrockProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.HasSuffix(r.URL.Path, "/converse-stream"))
		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(frames)
		require.NoError(t, err)
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := lm.DoStream(ctx, provider.CallOptions{Prompt: []provider.Message{provider.UserText("x")}})
	require.NoError(t, err)

	var parts []provider.StreamPart
	for p := range result.Stream {
		parts = append(parts, p)
	}
	require.NotEmpty(t, parts)
	assert.Equal(t, provider.PartStreamStart, parts[0].Type)
	last := parts[len(parts)-1]
	assert.Equal(t, provider.PartFinish, last.Type)
	require.NotNil(t, last.FinishReason)
	assert.Equal(t, provider.FinishReasonStop, last.FinishReason.Unified)
}

func TestDoStream_HTTPErrorBeforeStream(t *testing.T) {
	lm := newStubBedrockProvider(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, err := w.Write([]byte(`{"message":"bad","type":"ValidationException"}`))
		require.NoError(t, err)
	}))

	_, err := lm.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("x")}})
	require.Error(t, err)
	apiErr := err.(*provider.APICallError)
	assert.Equal(t, 400, apiErr.StatusCode)
	assert.False(t, apiErr.IsRetryable)
}

func TestDoStream_TransportFailureMidStream(t *testing.T) {
	// Server starts writing valid bytes but cuts off mid-frame.
	frames := encodeFixtures(t, `{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":"hi"}}}`)
	truncated := frames[:len(frames)/2]
	lm := newStubBedrockProvider(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(truncated)
		require.NoError(t, err)
		// Hijack and close so the client sees a truncated body.
		if hj, ok := w.(http.Hijacker); ok {
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
		}
	}))

	result, err := lm.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("x")}})
	require.NoError(t, err)

	var sawError bool
	for p := range result.Stream {
		if p.Type == provider.PartError {
			sawError = true
			require.NotNil(t, p.APICallError)
			assert.True(t, p.APICallError.IsRetryable)
		}
	}
	assert.True(t, sawError, "expected a PartError for truncated stream")
}

func TestDoStream_ContextCancellation(t *testing.T) {
	// Slow handler so cancellation kicks in.
	lm := newStubBedrockProvider(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
		w.WriteHeader(http.StatusOK)
		// Sleep then write -- we expect the client to abort before this.
		time.Sleep(500 * time.Millisecond)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	result, err := lm.DoStream(ctx, provider.CallOptions{Prompt: []provider.Message{provider.UserText("x")}})
	require.NoError(t, err)
	cancel()
	// Drain channel — no panic, no infinite block.
	for range result.Stream {
	}
}

func TestDoGenerate_RequestSignedAndBodyRoundtrip(t *testing.T) {
	var seenAuth string
	var seenBody string
	lm := newStubBedrockProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		seenBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"output":{"message":{"role":"assistant","content":[{"text":"ok"}]}},"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":1}}`))
		require.NoError(t, err)
	}))

	_, err := lm.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("ping")}})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(seenAuth, "AWS4-HMAC-SHA256"), "expected SigV4, got %q", seenAuth)
	assert.Contains(t, seenBody, `"messages":[{"role":"user"`)
}
