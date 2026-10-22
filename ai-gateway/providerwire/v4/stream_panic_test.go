package v4

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	openaicompatible "github.com/grafana/ai-sdk/providers/openai-compatible"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type panicOnReadBody struct{}

func (panicOnReadBody) Read([]byte) (int, error) { panic("private-panic-value") }
func (panicOnReadBody) Close() error             { return nil }

type panicTransport struct{}

func (panicTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: panicOnReadBody{}, Request: req}, nil
}

func TestRuntimeStreamWorkerPanic(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	panicking := openaicompatible.New("model", openaicompatible.WithBaseURL("http://provider.invalid/v1"), openaicompatible.WithHTTPClient(&http.Client{Transport: panicTransport{}}))
	panicDone := make(chan struct{})
	normalEntered := make(chan struct{})
	harness.model.stream = func(ctx context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
		prompt, err := json.Marshal(options.Prompt)
		if err != nil {
			return nil, err
		}
		if bytes.Contains(prompt, []byte("panic")) {
			return panicking.DoStream(ctx, options)
		}
		close(normalEntered)
		parts := make(chan provider.StreamPart, 1)
		go func() {
			defer close(parts)
			<-panicDone
			parts <- finishPart()
		}()
		return &provider.StreamResult{Stream: parts}, nil
	}
	server := httptest.NewServer(harness.handler)
	defer server.Close()

	stream := func(text string) (string, error) {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		req := streamRequest(`{"prompt":[{"role":"user","content":[{"type":"text","text":"` + text + `"}]}]}`).WithContext(ctx)
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(server.URL, "http://")
		req.RequestURI = ""
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		defer func() { _ = response.Body.Close() }()
		body, err := io.ReadAll(response.Body)
		return string(body), err
	}

	var wg sync.WaitGroup
	var concurrent string
	var concurrentErr error
	wg.Go(func() { concurrent, concurrentErr = stream("hello") })
	select {
	case <-normalEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("normal request never reached the model")
	}
	panicked, err := stream("panic")
	close(panicDone)
	wg.Wait()
	require.NoError(t, err)
	require.NoError(t, concurrentErr)

	assert.Equal(t, 1, strings.Count(panicked, `"type":"error"`))
	assert.True(t, strings.HasSuffix(panicked, string(canonicalInternalStreamErrorFrame)), panicked)
	assert.NotContains(t, panicked, "private-panic-value")
	assert.Contains(t, concurrent, `"type":"finish"`)
	assert.NotContains(t, concurrent, `"type":"error"`)
}

func TestRuntimeStreamMalformedErrorChain(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	malformed := provider.NewAPICallError(provider.APICallErrorOptions{Cause: (*provider.APICallError)(nil)})
	harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartError, APICallError: malformed}, finishPart())}, nil
	}
	body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
	requireStreamBodyMatchesSchema(t, body)
	assert.Equal(t, 1, strings.Count(body, `"type":"error"`))
	assert.Contains(t, body, `"type":"finish"`)
}
