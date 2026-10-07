package byok

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_ConcurrentRequestIsolation(t *testing.T) {
	const count = 16
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var arrived atomic.Int32
	ready := make(chan struct{})
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		assert.Equal(t, "Bearer dummy-"+strings.TrimPrefix(body.Model, "native-"), r.Header.Get("Authorization"))
		if arrived.Add(1) == count {
			close(ready)
		}
		select {
		case <-ready:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"resp_1","object":"response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)), Request: r}, nil
	})}
	results := make(chan error, count)
	for i := range count {
		go func() {
			model, err := New(fmt.Sprintf("openai/native-%d", i), json.RawMessage(fmt.Sprintf(`{"byok":{"openai":[{"apiKey":"dummy-%d"}]}}`, i)), client)
			if err == nil {
				_, err = model.DoGenerate(ctx, provider.CallOptions{Prompt: []provider.Message{provider.UserText("ordinary input")}})
			}
			results <- err
		}()
	}
	for range count {
		require.NoError(t, <-results)
	}
	assert.Equal(t, int32(count), arrived.Load())
}

func TestNew_DoesNotFollowNativeRedirects(t *testing.T) {
	for _, name := range []providerName{anthropic, openai} {
		t.Run(string(name), func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				assert.Equal(t, "api."+string(name)+".com", r.URL.Host)
				return &http.Response{StatusCode: 307, Header: http.Header{"Content-Type": {"application/json"}, "Location": {"https://redirect.invalid"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"redirect"}}`)), Request: r}, nil
			})}
			model, err := New(string(name)+"/native-model", json.RawMessage(fmt.Sprintf(`{"byok":{"%s":[{"apiKey":"dummy-first"},{"apiKey":"dummy-second"}]}}`, name)), client)
			require.NoError(t, err)
			_, _ = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
			assert.Equal(t, 1, calls)
			assert.Nil(t, client.CheckRedirect)
		})
	}
}
