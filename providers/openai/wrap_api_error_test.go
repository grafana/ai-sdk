package openai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/fallback"
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

func TestDoStream_APIError_ReturnsErrorBeforeStream(t *testing.T) {
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
	require.Error(t, err)
	assert.Nil(t, res)
	var apiErr *provider.APICallError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 429, apiErr.StatusCode)
	assert.True(t, apiErr.IsRetryable)
}

func TestDoStream_NestedSSEErrorReturnsStructuredError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Test-Response", "present")
		_, _ = w.Write([]byte("data: {\"type\":\"error\",\"error\":{\"message\":\"quota exhausted\",\"type\":\"insufficient_quota\",\"code\":\"insufficient_quota\"}}\n\n"))
	}))
	defer srv.Close()

	m := NewResponses("test-key", "gpt-4o",
		WithRequestOptions(option.WithBaseURL(srv.URL), option.WithHTTPClient(srv.Client()), option.WithMaxRetries(0)),
	)
	result, err := m.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
	require.Error(t, err)
	assert.Nil(t, result)
	var apiErr *provider.APICallError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 429, apiErr.StatusCode)
	assert.True(t, apiErr.IsRetryable)
	assert.Contains(t, apiErr.URL, srv.URL)
	assert.Equal(t, []string{"present"}, apiErr.ResponseHeaders["X-Test-Response"])
	assert.JSONEq(t, `{"type":"error","error":{"message":"quota exhausted","type":"insufficient_quota","code":"insufficient_quota"}}`, string(apiErr.Data))
}

func TestDoStream_LateNestedSSEErrorEmitsErrorAndFinish(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"type\":\"response.in_progress\",\"sequence_number\":1,\"response\":{\"id\":\"resp_1\",\"status\":\"in_progress\"}}\n\n"))
		flusher.Flush()
		time.Sleep(acceptedStreamErrorGrace + 25*time.Millisecond)
		_, _ = w.Write([]byte("data: {\"type\":\"error\",\"error\":{\"message\":\"late quota error\",\"type\":\"insufficient_quota\",\"code\":\"insufficient_quota\"}}\n\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	m := NewResponses("test-key", "gpt-4o",
		WithRequestOptions(option.WithBaseURL(srv.URL), option.WithHTTPClient(srv.Client()), option.WithMaxRetries(0)),
	)
	result, err := m.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
	require.NoError(t, err)

	var errorsSeen, finishesSeen int
	for part := range result.Stream {
		switch part.Type {
		case provider.PartError:
			errorsSeen++
			require.NotNil(t, part.APICallError)
			assert.Equal(t, 429, part.APICallError.StatusCode)
		case provider.PartFinish:
			finishesSeen++
			require.NotNil(t, part.FinishReason)
			assert.Equal(t, provider.FinishReasonError, part.FinishReason.Unified)
			assert.JSONEq(t, `{"responseId":null}`, string(part.ProviderMetadata["openai"]))
		}
	}
	assert.Equal(t, 1, errorsSeen)
	assert.Equal(t, 1, finishesSeen)
}

func TestDoStream_CancelWithoutConsumingReleasesResponse(t *testing.T) {
	requestDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(requestDone)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"type\":\"response.created\",\"sequence_number\":0,\"response\":{\"id\":\"resp_1\",\"created_at\":1,\"model\":\"gpt-4o\",\"object\":\"response\",\"status\":\"in_progress\",\"output\":[]}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_item.added\",\"sequence_number\":1,\"output_index\":0,\"item\":{\"type\":\"message\",\"id\":\"msg_1\",\"role\":\"assistant\",\"status\":\"in_progress\",\"content\":[]}}\n\n"))
		flusher.Flush()
		for i := 0; i < 300; i++ {
			_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"sequence_number\":2,\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"x\"}\n\n"))
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	m := NewResponses("test-key", "gpt-4o",
		WithRequestOptions(option.WithBaseURL(srv.URL), option.WithHTTPClient(srv.Client()), option.WithMaxRetries(0)),
	)
	result, err := m.DoStream(ctx, provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
	require.NoError(t, err)
	require.NotNil(t, result)
	cancel()

	select {
	case <-requestDone:
	case <-time.After(2 * time.Second):
		t.Fatal("stream response was not released after cancellation")
	}
}

func TestDoStream_TransportErrorIsRetryableAndAllowsFallback(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial failed")
	})}
	primary := NewResponses("test-key", "gpt-4o",
		WithRequestOptions(option.WithBaseURL("https://api.test/v1"), option.WithHTTPClient(client), option.WithMaxRetries(0)),
	)

	result, err := primary.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
	require.Error(t, err)
	assert.Nil(t, result)
	var apiErr *provider.APICallError
	require.True(t, errors.As(err, &apiErr))
	assert.True(t, apiErr.IsRetryable)
	assert.Contains(t, apiErr.URL, "https://api.test/v1/responses")

	model, err := fallback.New(primary, &fallbackSuccessModel{})
	require.NoError(t, err)
	result, err = model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
	require.NoError(t, err)
	var text string
	for part := range result.Stream {
		if part.Type == provider.PartTextDelta {
			text += part.Delta
		}
	}
	assert.Equal(t, "fallback", text)
}

type fallbackSuccessModel struct{}

func (*fallbackSuccessModel) SpecificationVersion() string               { return "v4" }
func (*fallbackSuccessModel) Provider() string                           { return "fallback" }
func (*fallbackSuccessModel) ModelID() string                            { return "fallback-model" }
func (*fallbackSuccessModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*fallbackSuccessModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (*fallbackSuccessModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	stream := make(chan provider.StreamPart, 2)
	stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: "fallback"}
	stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func TestDoStream_NestedSSEErrorAllowsFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"error\",\"error\":{\"message\":\"quota exhausted\",\"type\":\"insufficient_quota\",\"code\":\"insufficient_quota\"}}\n\n"))
	}))
	defer srv.Close()

	primary := NewResponses("test-key", "gpt-4o",
		WithRequestOptions(option.WithBaseURL(srv.URL), option.WithHTTPClient(srv.Client()), option.WithMaxRetries(0)),
	)
	model, err := fallback.New(primary, &fallbackSuccessModel{})
	require.NoError(t, err)

	result, err := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
	require.NoError(t, err)
	var text string
	for part := range result.Stream {
		if part.Type == provider.PartTextDelta {
			text += part.Delta
		}
	}
	assert.Equal(t, "fallback", text)
}

func TestNewResponses_DoesNotPanic(t *testing.T) {
	require.NotPanics(t, func() {
		_ = NewResponses("k", "gpt-4o")
	})
}
