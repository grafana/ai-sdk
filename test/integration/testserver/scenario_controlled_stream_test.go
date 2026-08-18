package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const abortableHandlerTimeout = time.Second

type writeSignalResponseWriter struct {
	header http.Header
	body   []byte
	wrote  chan struct{}
	once   sync.Once
}

func newWriteSignalResponseWriter() *writeSignalResponseWriter {
	return &writeSignalResponseWriter{
		header: make(http.Header),
		wrote:  make(chan struct{}),
	}
}

func (w *writeSignalResponseWriter) Header() http.Header { return w.header }
func (*writeSignalResponseWriter) WriteHeader(int)       {}
func (w *writeSignalResponseWriter) Write(p []byte) (int, error) {
	w.body = append(w.body, p...)
	if bytes.Contains(w.body, []byte(controlledPartialText)) {
		w.once.Do(func() { close(w.wrote) })
	}
	return len(p), nil
}
func (*writeSignalResponseWriter) Flush() {}

func TestAbortableStreamHandlers_ReturnAfterCancellation(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{name: "UI stream", handler: handleAbortableUIStream},
		{name: "text stream", handler: handleAbortableTextStream},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			request := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
			writer := newWriteSignalResponseWriter()
			done := make(chan struct{})

			go func() {
				tc.handler(writer, request)
				close(done)
			}()

			requireChannelSignal(t, writer.wrote)
			cancel()
			requireChannelSignal(t, done)
		})
	}
}

func requireChannelSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(abortableHandlerTimeout):
		require.FailNow(t, "timed out waiting for stream signal")
	}
}
