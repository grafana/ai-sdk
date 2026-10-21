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
		require.NoError(t, err)
		if bytes.Contains(prompt, []byte("panic")) {
			return panicking.DoStream(ctx, options)
		}
		close(normalEntered)
		parts := make(chan provider.StreamPart)
		go func() {
			defer close(parts)
			<-panicDone
			parts <- finishPart()
		}()
		return &provider.StreamResult{Stream: parts}, nil
	}
	server := httptest.NewServer(harness.handler)
	defer server.Close()

	stream := func(text string) string {
		req := streamRequest(`{"prompt":[{"role":"user","content":[{"type":"text","text":"` + text + `"}]}]}`).WithContext(t.Context())
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(server.URL, "http://")
		req.RequestURI = ""
		response, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { require.NoError(t, response.Body.Close()) }()
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		return string(body)
	}

	var wg sync.WaitGroup
	var concurrent string
	wg.Go(func() { concurrent = stream("hello") })
	<-normalEntered
	panicked := stream("panic")
	close(panicDone)
	wg.Wait()

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
