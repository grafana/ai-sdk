package grafana

import (
	"context"
	"encoding/json"
	"errors"
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

const unaryFixture = `{"content":[{"type":"text","text":"hello"},{"type":"text","text":""}],"finishReason":{"unified":"stop","raw":"end_turn"},"usage":{"inputTokens":{"total":2,"noCache":2,"cacheRead":0,"cacheWrite":0},"outputTokens":{"total":1,"text":1,"reasoning":0}}}`

func TestDecodeGenerate_FunctionCalls(t *testing.T) {
	for _, input := range []string{"", "{}", "{\"city\":\"Rio\"}"} {
		encoded, err := json.Marshal(input)
		require.NoError(t, err)
		body := `{"content":[{"type":"text","text":""},{"type":"tool-call","toolCallId":"call","toolName":"weather","input":` + string(encoded) + `}],"finishReason":{"unified":"tool-calls"},"usage":{"inputTokens":{},"outputTokens":{}}}`
		result, err := decodeGenerate([]byte(body))
		require.NoError(t, err)
		require.Len(t, result.Content, 2)
		assert.Equal(t, input, string(result.Content[1].Input))
		assert.Equal(t, "call", result.Content[1].ToolCallID)
		for _, marker := range []string{"providerExecuted", "dynamic"} {
			marked := strings.Replace(body, `"toolName":"weather"`, `"toolName":"weather","`+marker+`":true`, 1)
			_, err := decodeGenerate([]byte(marked))
			require.Error(t, err)
		}
	}
}

func TestDecodeGenerate_FunctionCallRequiredFields(t *testing.T) {
	for _, content := range []string{
		`{"type":"tool-call","toolName":"f","input":""}`,
		`{"type":"tool-call","toolCallId":"a","input":""}`,
		`{"type":"tool-call","toolCallId":"a","toolName":"f"}`,
		`{"type":"tool-call","toolCallId":"a","toolName":"f","input":null}`,
		`{"type":"tool-call","toolCallId":"a","toolName":"f","input":{}}`,
	} {
		body := `{"content":[` + content + `],"finishReason":{"unified":"tool-calls"},"usage":{"inputTokens":{},"outputTokens":{}}}`
		_, err := decodeGenerate([]byte(body))
		require.Error(t, err)
	}
}

func TestModel_GenerateRequestAndNormalization(t *testing.T) {
	var calls atomic.Int32
	p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/aisdk/language-model", r.URL.Path)
		for key, want := range map[string]string{"Content-Type": "application/json", "Accept": "application/json", "X-Access-Token": "access-token", "X-Grafana-Id": "user-token", "Ai-Language-Model-Id": "assistant", "Ai-Language-Model-Specification-Version": "4", "Ai-Language-Model-Streaming": "false", "X-Custom": "call"} {
			assert.Equal(t, []string{want}, r.Header.Values(key), key)
		}
		var body map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.JSONEq(t, `[]`, string(body["prompt"]))
		assert.JSONEq(t, `0`, string(body["maxOutputTokens"]))
		var headers map[string]string
		require.NoError(t, json.Unmarshal(body["headers"], &headers))
		assert.Equal(t, "call", headers["x-custom"])
		assert.Equal(t, "evil-token", headers["X-Access-Token"])
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Server", "actual")
		w.Header().Add("X-Multi", "one")
		w.Header().Add("X-Multi", "two")
		_, _ = io.WriteString(w, strings.TrimSuffix(unaryFixture, "}")+`,"warnings":[{"type":"other","message":"private-warning"}],"request":{"body":"private-request"},"response":{"modelId":"private-backend","id":"private-id","headers":{"X-Server":"fake"}},"providerMetadata":{"private":"secret"}}`)
	}, nil)
	p.headers.Set("X-Custom", "configured")
	p.headers.Set("Ai-Language-Model-Id", "configured-model")
	m, err := p.LanguageModel("assistant")
	require.NoError(t, err)
	zero := 0
	result, err := m.DoGenerate(WithUserIDToken(context.Background(), "user-token"), provider.CallOptions{Prompt: []provider.Message{}, MaxOutputTokens: &zero, Headers: map[string]string{"x-custom": "call", "X-Access-Token": "evil-token", "X-Grafana-Id": "evil-user", "ai-language-model-id": "evil-model", "accept": "evil-accept", "content-type": "evil-type", "ai-language-model-streaming": "true", "ai-language-model-specification-version": "3"}})
	require.NoError(t, err)
	require.Len(t, result.Content, 2)
	assert.Equal(t, "hello", result.Content[0].Text)
	assert.Empty(t, result.Content[1].Text)
	assert.Equal(t, provider.FinishReasonStop, result.FinishReason.Unified)
	assert.Equal(t, "end_turn", result.FinishReason.Raw)
	assert.Equal(t, 2, *result.Usage.InputTokens.Total)
	require.NotNil(t, result.Warnings)
	assert.Equal(t, []provider.Warning{{Type: provider.WarnOther, Message: "private-warning"}}, result.Warnings)
	assert.Nil(t, result.ProviderMetadata)
	require.NotNil(t, result.Request)
	assert.Contains(t, string(result.Request.Body), "maxOutputTokens")
	assert.NotContains(t, string(result.Request.Body), "private-request")
	require.NotNil(t, result.Response)
	assert.Equal(t, "actual", result.Response.Headers["X-Server"])
	assert.Equal(t, "one, two", result.Response.Headers["X-Multi"])
	assert.Empty(t, result.Response.ModelID)
	assert.Empty(t, result.Response.ID)
	assert.Equal(t, int32(1), calls.Load())
}

func TestModel_UnaryFailures(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"missing content", `{"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}}`},
		{"null content", strings.Replace(unaryFixture, `[{"type":"text","text":"hello"},{"type":"text","text":""}]`, `null`, 1)},
		{"unknown family", strings.Replace(unaryFixture, `"type":"text"`, `"type":"reasoning"`, 1)},
		{"missing text", strings.Replace(unaryFixture, `,"text":"hello"`, "", 1)},
		{"null text", strings.Replace(unaryFixture, `"text":"hello"`, `"text":null`, 1)},
		{"unknown finish", strings.Replace(unaryFixture, `"unified":"stop"`, `"unified":"unknown"`, 1)},
		{"unknown warning", strings.TrimSuffix(unaryFixture, "}") + `,"warnings":[{"type":"unknown","message":"invalid"}]}`},
		{"missing warning message", strings.TrimSuffix(unaryFixture, "}") + `,"warnings":[{"type":"other"}]}`},
		{"missing warning feature", strings.TrimSuffix(unaryFixture, "}") + `,"warnings":[{"type":"unsupported"}]}`},
		{"missing deprecated setting", strings.TrimSuffix(unaryFixture, "}") + `,"warnings":[{"type":"deprecated","message":"invalid"}]}`},
		{"null finish", strings.Replace(unaryFixture, `{"unified":"stop","raw":"end_turn"}`, `null`, 1)},
		{"negative usage", strings.Replace(unaryFixture, `"total":2`, `"total":-1`, 1)},
		{"null usage", strings.Replace(unaryFixture, `"total":2`, `"total":null`, 1)},
		{"unsafe usage", strings.Replace(unaryFixture, `"total":2`, `"total":9007199254740992`, 1)},
		{"fractional usage", strings.Replace(unaryFixture, `"total":2`, `"total":0.5`, 1)},
		{"missing usage side", strings.Replace(unaryFixture, `"outputTokens"`, `"missing"`, 1)},
		{"trailing", unaryFixture + `{}`},
		{"malformed", `{"content":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.body)
			}, nil)
			m, err := p.LanguageModel("assistant")
			require.NoError(t, err)
			result, err := m.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			require.Error(t, err)
			assert.Nil(t, result)
			var api *provider.APICallError
			require.ErrorAs(t, err, &api)
			assert.False(t, api.IsRetryable)
		})
	}
}

type observedResponseBody struct {
	io.ReadCloser
	closed bool
}

func (b *observedResponseBody) Close() error {
	b.closed = true
	return b.ReadCloser.Close()
}

func TestModel_PostHeaderTransportFailure(t *testing.T) {
	var calls atomic.Int32
	const privateBody = `{"private":"sensitive-value"`
	p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "4096")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, privateBody)
		w.(http.Flusher).Flush()
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijacking response: %v", err)
			return
		}
		_ = conn.Close()
	}, nil)
	var body *observedResponseBody
	p.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp, err := http.DefaultTransport.RoundTrip(req)
		if err == nil {
			body = &observedResponseBody{ReadCloser: resp.Body}
			resp.Body = body
		}
		return resp, err
	})
	m, err := p.LanguageModel("assistant")
	require.NoError(t, err)
	result, err := m.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
	require.Error(t, err)
	assert.Nil(t, result)
	var api *provider.APICallError
	require.ErrorAs(t, err, &api)
	assert.Equal(t, http.StatusOK, api.StatusCode)
	assert.True(t, api.IsRetryable)
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
	assert.NotContains(t, err.Error(), privateBody)
	assert.NotContains(t, err.Error(), "sensitive-value")
	assert.Less(t, len(api.Message), 100)
	require.NotNil(t, body)
	assert.True(t, body.closed)
	assert.Equal(t, int32(1), calls.Load())

	t.Run("non-EOF body read failure", func(t *testing.T) {
		cause := errors.New("private socket reset")
		body := &trackedBody{Reader: &partialReadError{data: []byte(`{"partial":`), err: cause}}
		var calls atomic.Int32
		p, err := NewWithAccessToken(AccessTokenConfig{
			AccessToken: "token", BaseURL: "https://example.test",
			HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls.Add(1)
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: body, Request: req}, nil
			})},
		})
		require.NoError(t, err)
		m, err := p.LanguageModel("assistant")
		require.NoError(t, err)
		result, err := m.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
		require.Error(t, err)
		assert.Nil(t, result)
		var api *provider.APICallError
		require.ErrorAs(t, err, &api)
		assert.Equal(t, http.StatusOK, api.StatusCode)
		assert.True(t, api.IsRetryable)
		assert.ErrorIs(t, err, cause)
		assert.NotContains(t, err.Error(), "private socket reset")
		assert.Less(t, len(api.Message), 100)
		assert.True(t, body.closed)
		assert.Equal(t, int32(1), calls.Load())
	})
}

type partialReadError struct {
	data   []byte
	err    error
	onRead func()
}

func (r *partialReadError) Read(p []byte) (int, error) {
	n := copy(p, r.data)
	r.data = nil
	if r.onRead != nil {
		r.onRead()
	}
	return n, r.err
}

func TestModel_UnaryProtocolFailures(t *testing.T) {
	for _, tc := range []struct {
		name, media, body string
		limit             int64
	}{
		{name: "wrong media", media: "text/html", body: `{"private":"sensitive-value"}`},
		{name: "malformed JSON", media: "application/json", body: `{"private":"sensitive-value"`},
		{name: "invalid schema", media: "application/json", body: `{"private":"sensitive-value"}`},
		{name: "byte limit", media: "application/json", body: `{"private":"sensitive-value"}`, limit: 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			limits := DefaultLimits()
			if tc.limit != 0 {
				limits.UnaryBytes = tc.limit
			}
			p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", tc.media)
				_, _ = io.WriteString(w, tc.body)
			}, &limits)
			m, err := p.LanguageModel("assistant")
			require.NoError(t, err)
			result, err := m.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			require.Error(t, err)
			assert.Nil(t, result)
			var api *provider.APICallError
			require.ErrorAs(t, err, &api)
			assert.False(t, api.IsRetryable)
			assert.Equal(t, http.StatusOK, api.StatusCode)
			assert.Empty(t, api.ResponseBody)
			assert.NotContains(t, err.Error(), "sensitive-value")
			assert.Less(t, len(api.Message), 100)
			assert.Equal(t, int32(1), calls.Load())
		})
	}
}

func TestModel_UnaryReadPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name             string
		cancelDuringRead bool
	}{
		{name: "limit before transport"},
		{name: "cancellation before limit", cancelDuringRead: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reader := &partialReadError{data: []byte("private"), err: io.ErrUnexpectedEOF}
			if tc.cancelDuringRead {
				reader.onRead = cancel
			}
			body := &trackedBody{Reader: reader}
			limits := DefaultLimits()
			limits.UnaryBytes = 4
			var calls atomic.Int32
			p, err := NewWithAccessToken(AccessTokenConfig{
				AccessToken: "token", BaseURL: "https://example.test", Limits: &limits,
				HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					calls.Add(1)
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: body, Request: req}, nil
				})},
			})
			require.NoError(t, err)
			m, err := p.LanguageModel("assistant")
			require.NoError(t, err)
			result, err := m.DoGenerate(ctx, provider.CallOptions{Prompt: []provider.Message{}})
			require.Error(t, err)
			assert.Nil(t, result)
			var api *provider.APICallError
			require.ErrorAs(t, err, &api)
			assert.Equal(t, http.StatusOK, api.StatusCode)
			assert.False(t, api.IsRetryable)
			if tc.cancelDuringRead {
				assert.ErrorIs(t, err, context.Canceled)
			} else {
				assert.ErrorContains(t, errors.Unwrap(api), "response byte limit exceeded")
			}
			assert.NotErrorIs(t, err, io.ErrUnexpectedEOF)
			assert.NotContains(t, err.Error(), "private")
			assert.True(t, body.closed)
			assert.Equal(t, int32(1), calls.Load())
		})
	}
}

func TestModel_UnaryReadCancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		name := "cancel"
		if deadline {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			started := make(chan struct{})
			p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				close(started)
				<-r.Context().Done()
			}, nil)
			m, err := p.LanguageModel("assistant")
			require.NoError(t, err)
			var ctx context.Context
			var cancel context.CancelFunc
			expected := context.Canceled
			if deadline {
				ctx, cancel = context.WithTimeout(context.Background(), time.Second)
				expected = context.DeadlineExceeded
			} else {
				ctx, cancel = context.WithCancel(context.Background())
			}
			defer cancel()
			done := make(chan error, 1)
			go func() {
				result, err := m.DoGenerate(ctx, provider.CallOptions{Prompt: []provider.Message{}})
				if result != nil {
					done <- errors.New("grafana: canceled unary call returned a result")
					return
				}
				done <- err
			}()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("unary response headers were not sent")
			}
			if !deadline {
				cancel()
			}
			select {
			case err := <-done:
				require.ErrorIs(t, err, expected)
				var api *provider.APICallError
				if errors.As(err, &api) {
					assert.False(t, api.IsRetryable)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("canceled unary read did not finish")
			}
			assert.Equal(t, int32(1), calls.Load())
		})
	}
}

func TestModel_UnaryBoundsAndTransport(t *testing.T) {
	for _, delta := range []int64{0, -1} {
		t.Run(string(rune('a'-delta)), func(t *testing.T) {
			limits := DefaultLimits()
			limits.UnaryBytes = int64(len(unaryFixture)) + delta
			p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, unaryFixture)
			}, &limits)
			m, err := p.LanguageModel("assistant")
			require.NoError(t, err)
			result, err := m.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			if delta == 0 {
				require.NoError(t, err)
				assert.NotNil(t, result)
			} else {
				require.Error(t, err)
				assert.Nil(t, result)
			}
		})
	}
	t.Run("transport error", func(t *testing.T) {
		cause := errors.New("private transport detail")
		p, err := NewWithAccessToken(AccessTokenConfig{AccessToken: "token", BaseURL: "https://example.test", HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, cause })}})
		require.NoError(t, err)
		m, err := p.LanguageModel("assistant")
		require.NoError(t, err)
		_, err = m.DoGenerate(context.Background(), provider.CallOptions{})
		require.Error(t, err)
		assert.ErrorIs(t, err, cause)
		assert.NotContains(t, err.Error(), "private transport detail")
		var api *provider.APICallError
		require.ErrorAs(t, err, &api)
		assert.True(t, api.IsRetryable)
	})
}
