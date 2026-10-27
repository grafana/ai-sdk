package openaicompatible

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/internal/streamrecover"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type panickingBody struct {
	prefix      []byte
	closePanics bool
	closed      atomic.Int32
}

func (b *panickingBody) Read(p []byte) (int, error) {
	if len(b.prefix) > 0 {
		n := copy(p, b.prefix)
		b.prefix = b.prefix[n:]
		return n, nil
	}
	panic("secret-panic-value")
}

func (b *panickingBody) Close() error {
	b.closed.Add(1)
	if b.closePanics {
		panic("secret-panic-value")
	}
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

const textDeltaEvent = "data: {\"id\":\"c\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"}}]}\n\n"

func panicStreamModel(body io.ReadCloser) provider.LanguageModel {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body, Request: req}, nil
	})}
	return New("test-model", WithBaseURL("http://provider.invalid/v1"), WithHTTPClient(client))
}

func TestDoStream_WorkerPanic(t *testing.T) {
	tests := []struct {
		name        string
		prefix      string
		closePanics bool
		before      []provider.StreamPartType
	}{
		{name: "before the first event", before: []provider.StreamPartType{provider.PartStreamStart}},
		{
			name:   "after a text delta",
			prefix: textDeltaEvent,
			before: []provider.StreamPartType{provider.PartStreamStart, provider.PartResponseMeta, provider.PartTextStart, provider.PartTextDelta},
		},
		{
			name:        "after a text delta, with a panicking body close",
			prefix:      textDeltaEvent,
			closePanics: true,
			before:      []provider.StreamPartType{provider.PartStreamStart, provider.PartResponseMeta, provider.PartTextStart, provider.PartTextDelta},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := &panickingBody{prefix: []byte(tc.prefix), closePanics: tc.closePanics}
			result, err := panicStreamModel(body).DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
			require.NoError(t, err)

			var parts []provider.StreamPart
			for part := range result.Stream {
				parts = append(parts, part)
			}
			require.NotEmpty(t, parts)
			last := parts[len(parts)-1]
			assert.Equal(t, provider.PartError, last.Type)
			require.NotNil(t, last.APICallError)
			assert.True(t, errors.Is(last.APICallError, streamrecover.ErrPanic))
			assert.False(t, last.APICallError.IsRetryable)
			assert.NotContains(t, last.APICallError.Error(), "secret-panic-value")
			var types []provider.StreamPartType
			for _, part := range parts[:len(parts)-1] {
				assert.NotEqual(t, provider.PartError, part.Type)
				types = append(types, part.Type)
			}
			if tc.before != nil {
				assert.Equal(t, tc.before, types)
			}
			assert.Equal(t, int32(1), body.closed.Load())
		})
	}
}

func TestDoStream_BodyClosePanicsAfterFinish(t *testing.T) {
	finish := "data: {\"id\":\"c\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
	body := &panickingBody{prefix: []byte(textDeltaEvent + finish), closePanics: true}
	result, err := panicStreamModel(body).DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
	require.NoError(t, err)
	var types []provider.StreamPartType
	for part := range result.Stream {
		types = append(types, part.Type)
	}
	require.NotEmpty(t, types)
	assert.Equal(t, provider.PartFinish, types[len(types)-1])
	assert.NotContains(t, types, provider.PartError)
	assert.Equal(t, int32(1), body.closed.Load())
}

var _ io.ReadCloser = (*panickingBody)(nil)
