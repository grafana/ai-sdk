package byok

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/nativemodel"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	startTimeout           = 50 * time.Millisecond
	slowResponse           = 2 * time.Second
	slowBodyDelay          = 150 * time.Millisecond
	timeoutUnaryBody       = `{"id":"resp_1","object":"response","status":"completed","model":"model","output":[{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"answer","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1}}`
	timeoutAnthropicBody   = `{"id":"msg_1","type":"message","role":"assistant","model":"model","content":[{"type":"text","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
	timeoutAnthropicStream = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"model\",\"content\":[],\"stop_reason\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	timeoutStream = "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"created_at\":1,\"model\":\"model\",\"output\":[]}}\n\n" +
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
)

// timeoutServer answers OpenAI Responses requests by account key: dummy-slow
// sends nothing for slowResponse, dummy-slow-body sends headers at once and its
// body after slowBodyDelay, and any other key answers at once.
type timeoutServer struct {
	*httptest.Server
	mu   sync.Mutex
	keys []string
}

func newTimeoutServer(t *testing.T) *timeoutServer {
	t.Helper()
	server := &timeoutServer{}
	server.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Api-Key")
		if key == "" {
			key = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		}
		raw, _ := io.ReadAll(r.Body)
		server.mu.Lock()
		server.keys = append(server.keys, key)
		server.mu.Unlock()
		wait := func(delay time.Duration) bool {
			select {
			case <-time.After(delay):
				return true
			case <-r.Context().Done():
				return false
			}
		}
		if key == "dummy-slow" && !wait(slowResponse) {
			return
		}
		anthropic := strings.HasSuffix(r.URL.Path, "/messages")
		body := map[bool]string{false: timeoutUnaryBody, true: timeoutAnthropicBody}[anthropic]
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(raw), `"stream":true`) {
			body = map[bool]string{false: timeoutStream, true: timeoutAnthropicStream}[anthropic]
			w.Header().Set("Content-Type", "text/event-stream")
		}
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		if key == "dummy-slow-body" && !wait(slowBodyDelay) {
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

func (s *timeoutServer) requestKeys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.keys...)
}

func (s *timeoutServer) model(t *testing.T, name Provider, timeout time.Duration, keys ...string) provider.LanguageModel {
	t.Helper()
	accounts := make([]nativemodel.Config, len(keys))
	for i, key := range keys {
		accounts[i] = nativemodel.Config{APIKey: key, BaseURL: s.URL}
	}
	request := Request{Destinations: []Destination{{Provider: name, Model: "model", Accounts: accounts}}}
	if timeout > 0 {
		request.Timeouts = map[Provider]time.Duration{name: timeout}
	}
	model, err := newAll(request, s.Client())
	require.NoError(t, err)
	return model
}

func invokeTimeoutModel(ctx context.Context, model provider.LanguageModel, streaming bool) error {
	options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}}
	if !streaming {
		_, err := model.DoGenerate(ctx, options)
		return err
	}
	result, err := model.DoStream(ctx, options)
	if err != nil {
		return err
	}
	for part := range result.Stream {
		if part.Type == provider.PartError {
			return part.APICallError
		}
	}
	return nil
}

func TestStartTimeout(t *testing.T) {
	for _, run := range []struct {
		name      Provider
		streaming bool
	}{{OpenAI, false}, {OpenAI, true}, {Anthropic, false}, {Anthropic, true}} {
		streaming := run.streaming
		mode := string(run.name) + "/" + map[bool]string{false: "unary", true: "stream"}[streaming]
		t.Run(mode+"/slow account falls through", func(t *testing.T) {
			server := newTimeoutServer(t)
			var attempts []fallback.Attempt
			ctx := fallback.WithAttemptObserver(t.Context(), func(_ context.Context, attempt fallback.Attempt) { attempts = append(attempts, attempt) })
			started := time.Now()
			require.NoError(t, invokeTimeoutModel(ctx, server.model(t, run.name, startTimeout, "dummy-slow", "dummy-fast"), streaming))
			assert.Less(t, time.Since(started), slowResponse)
			assert.Equal(t, []string{"dummy-slow", "dummy-fast"}, server.requestKeys())
			require.Len(t, attempts, 2)
			assert.Equal(t, fallback.AttemptFailed, attempts[0].Outcome)
			assert.True(t, attempts[0].WillFallback)
			require.ErrorIs(t, attempts[0].SourceErr, ErrProviderStartTimeout)
			assert.Equal(t, fallback.AttemptSelected, attempts[1].Outcome)
		})
		t.Run(mode+"/no effect after the first byte", func(t *testing.T) {
			server := newTimeoutServer(t)
			require.NoError(t, invokeTimeoutModel(t.Context(), server.model(t, run.name, startTimeout, "dummy-slow-body", "dummy-fast"), streaming))
			assert.Equal(t, []string{"dummy-slow-body"}, server.requestKeys())
		})
		t.Run(mode+"/last attempt times out", func(t *testing.T) {
			server := newTimeoutServer(t)
			err := invokeTimeoutModel(t.Context(), server.model(t, run.name, startTimeout, "dummy-slow"), streaming)
			require.ErrorIs(t, err, ErrProviderStartTimeout)
			assert.Equal(t, execution.CategoryTimeout, execution.ClassifyProvider(err))
			assert.Equal(t, []string{"dummy-slow"}, server.requestKeys())
		})
		t.Run(mode+"/request cancellation is not a provider timeout", func(t *testing.T) {
			server := newTimeoutServer(t)
			ctx, cancel := context.WithCancel(t.Context())
			time.AfterFunc(startTimeout, cancel)
			err := invokeTimeoutModel(ctx, server.model(t, run.name, time.Minute, "dummy-slow", "dummy-fast"), streaming)
			require.ErrorIs(t, err, context.Canceled)
			assert.NotErrorIs(t, err, ErrProviderStartTimeout)
			assert.Equal(t, []string{"dummy-slow"}, server.requestKeys())
		})
		t.Run(mode+"/unlisted provider has no timeout", func(t *testing.T) {
			server := newTimeoutServer(t)
			require.NoError(t, invokeTimeoutModel(t.Context(), server.model(t, run.name, 0, "dummy-slow-body"), streaming))
		})
	}
}
