package providerwire

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type handlerTestModel struct {
	generate func(context.Context, provider.CallOptions) (*provider.GenerateResult, error)
	stream   func(context.Context, provider.CallOptions) (*provider.StreamResult, error)
}

func (m *handlerTestModel) SpecificationVersion() string               { return "v4" }
func (m *handlerTestModel) Provider() string                           { return "test" }
func (m *handlerTestModel) ModelID() string                            { return "model" }
func (m *handlerTestModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *handlerTestModel) DoGenerate(ctx context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
	if m.generate == nil {
		return nil, errors.New("unexpected DoGenerate")
	}
	return m.generate(ctx, opts)
}
func (m *handlerTestModel) DoStream(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	if m.stream == nil {
		return nil, errors.New("unexpected DoStream")
	}
	return m.stream(ctx, opts)
}

var (
	_ http.Handler           = (*Handler)(nil)
	_ ModelResolver          = ModelResolverFunc(nil)
	_ ModelResolver          = (*handlerTestResolver)(nil)
	_ provider.LanguageModel = (*handlerTestModel)(nil)
)

type handlerContextKey struct{}

type handlerTestResolver struct {
	model provider.LanguageModel
	err   error
	calls int
	req   *http.Request
	id    string
}

func (r *handlerTestResolver) ResolveLanguageModel(req *http.Request, modelID string) (provider.LanguageModel, error) {
	r.calls++
	r.req = req
	r.id = modelID
	return r.model, r.err
}

func validHandlerRequest(t *testing.T, streaming bool, body []byte) *http.Request {
	t.Helper()
	if body == nil {
		body = []byte(`{}`)
	}
	req := httptest.NewRequest(http.MethodPost, PathLanguageModel, bytes.NewReader(body))
	req.Header.Set(HeaderModelID, " model ")
	req.Header.Set(HeaderSpecVersion, SpecVersionV4)
	req.Header.Set(HeaderStreaming, fmt.Sprintf("%t", streaming))
	req.Header.Set("Content-Type", MIMEJSON)
	if streaming {
		req.Header.Set("Accept", MIMESSE)
	} else {
		req.Header.Set("Accept", MIMEJSON)
	}
	return req
}

func executeHandler(t *testing.T, h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeHandlerError(t *testing.T, rec *httptest.ResponseRecorder) *provider.APICallError {
	t.Helper()
	apiErr, err := DecodeAPICallError(rec.Body.Bytes())
	require.NoError(t, err)
	return apiErr
}

func TestNewHandler_Construction(t *testing.T) {
	t.Run("nil resolver", func(t *testing.T) {
		h, err := NewHandler(nil)
		require.Error(t, err)
		assert.Nil(t, h)
	})
	t.Run("typed nil resolver", func(t *testing.T) {
		var resolver *handlerTestResolver
		h, err := NewHandler(resolver)
		require.Error(t, err)
		assert.Nil(t, h)
	})
	t.Run("nil function resolver", func(t *testing.T) {
		var resolver ModelResolverFunc
		h, err := NewHandler(resolver)
		require.Error(t, err)
		assert.Nil(t, h)
	})
	t.Run("defaults", func(t *testing.T) {
		h, err := NewHandler(&handlerTestResolver{})
		require.NoError(t, err)
		assert.Equal(t, DefaultTotalTimeout, h.totalTimeout)
		assert.Equal(t, DefaultIdleTimeout, h.idleTimeout)
		assert.Equal(t, DefaultMaxRequestBodyBytes, h.maxRequestBodyBytes)
	})
	t.Run("overrides", func(t *testing.T) {
		h, err := NewHandler(&handlerTestResolver{}, WithTotalTimeout(time.Second), WithIdleTimeout(2*time.Second), WithMaxRequestBodyBytes(3))
		require.NoError(t, err)
		assert.Equal(t, time.Second, h.totalTimeout)
		assert.Equal(t, 2*time.Second, h.idleTimeout)
		assert.Equal(t, int64(3), h.maxRequestBodyBytes)
	})
	t.Run("nil option", func(t *testing.T) {
		_, err := NewHandler(&handlerTestResolver{}, nil)
		require.Error(t, err)
	})
	for _, tc := range []struct {
		name string
		opt  Option
	}{
		{name: "zero total", opt: WithTotalTimeout(0)},
		{name: "negative total", opt: WithTotalTimeout(-1)},
		{name: "zero idle", opt: WithIdleTimeout(0)},
		{name: "negative idle", opt: WithIdleTimeout(-1)},
		{name: "zero body", opt: WithMaxRequestBodyBytes(0)},
		{name: "negative body", opt: WithMaxRequestBodyBytes(-1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewHandler(&handlerTestResolver{}, tc.opt)
			require.Error(t, err)
		})
	}
}

func TestModelResolverFunc_Adapter(t *testing.T) {
	model := &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return &provider.GenerateResult{}, nil
	}}
	var gotReq *http.Request
	var gotID string
	resolver := ModelResolverFunc(func(req *http.Request, modelID string) (provider.LanguageModel, error) {
		gotReq, gotID = req, modelID
		return model, nil
	})
	h, err := NewHandler(resolver)
	require.NoError(t, err)
	req := validHandlerRequest(t, false, nil)
	rec := executeHandler(t, h, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Same(t, req, gotReq)
	assert.Equal(t, "model", gotID)
}

func TestHandler_RequestValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*http.Request)
		status int
	}{
		{name: "method", mutate: func(r *http.Request) { r.Method = http.MethodGet }, status: http.StatusMethodNotAllowed},
		{name: "missing model", mutate: func(r *http.Request) { r.Header.Del(HeaderModelID) }, status: http.StatusBadRequest},
		{name: "blank model", mutate: func(r *http.Request) { r.Header.Set(HeaderModelID, "  ") }, status: http.StatusBadRequest},
		{name: "missing spec", mutate: func(r *http.Request) { r.Header.Del(HeaderSpecVersion) }, status: http.StatusBadRequest},
		{name: "blank spec", mutate: func(r *http.Request) { r.Header.Set(HeaderSpecVersion, " ") }, status: http.StatusBadRequest},
		{name: "unsupported spec", mutate: func(r *http.Request) { r.Header.Set(HeaderSpecVersion, "3") }, status: http.StatusBadRequest},
		{name: "missing streaming", mutate: func(r *http.Request) { r.Header.Del(HeaderStreaming) }, status: http.StatusBadRequest},
		{name: "blank streaming", mutate: func(r *http.Request) { r.Header.Set(HeaderStreaming, " ") }, status: http.StatusBadRequest},
		{name: "invalid streaming", mutate: func(r *http.Request) { r.Header.Set(HeaderStreaming, "TRUE") }, status: http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolver := &handlerTestResolver{}
			h, err := NewHandler(resolver)
			require.NoError(t, err)
			req := validHandlerRequest(t, false, nil)
			tc.mutate(req)
			rec := executeHandler(t, h, req)
			assert.Equal(t, tc.status, rec.Code)
			assert.False(t, decodeHandlerError(t, rec).IsRetryable)
			assert.Zero(t, resolver.calls)
		})
	}
}

func TestHandler_ContentNegotiation(t *testing.T) {
	cases := []struct {
		name        string
		streaming   bool
		contentType *string
		accept      *string
		status      int
	}{
		{name: "omitted content type", status: http.StatusOK},
		{name: "parameterized content type", contentType: stringPointer("application/json; charset=utf-8"), status: http.StatusOK},
		{name: "malformed content type", contentType: stringPointer("application/json; ="), status: http.StatusUnsupportedMediaType},
		{name: "wrong content type", contentType: stringPointer("text/plain"), status: http.StatusUnsupportedMediaType},
		{name: "omitted unary accept", status: http.StatusOK},
		{name: "exact unary accept q zero", accept: stringPointer("application/json;q=0"), status: http.StatusOK},
		{name: "wildcard unary accept q zero", accept: stringPointer("application/*;q=0"), status: http.StatusOK},
		{name: "all wildcard unary", accept: stringPointer("*/*"), status: http.StatusOK},
		{name: "incompatible unary", accept: stringPointer("text/event-stream"), status: http.StatusNotAcceptable},
		{name: "exact stream accept", streaming: true, accept: stringPointer("text/event-stream"), status: http.StatusOK},
		{name: "wildcard stream accept q zero", streaming: true, accept: stringPointer("text/*;q=0"), status: http.StatusOK},
		{name: "incompatible stream", streaming: true, accept: stringPointer("application/json"), status: http.StatusNotAcceptable},
		{name: "leading empty entry", accept: stringPointer(",application/xml"), status: http.StatusOK},
		{name: "empty parameter entry", accept: stringPointer(";q=0"), status: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := &handlerTestModel{
				generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					return &provider.GenerateResult{}, nil
				},
				stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					ch := make(chan provider.StreamPart)
					close(ch)
					return &provider.StreamResult{Stream: ch}, nil
				},
			}
			resolver := &handlerTestResolver{model: model}
			h, err := NewHandler(resolver)
			require.NoError(t, err)
			req := validHandlerRequest(t, tc.streaming, nil)
			req.Header.Del("Content-Type")
			req.Header.Del("Accept")
			if tc.contentType != nil {
				req.Header.Set("Content-Type", *tc.contentType)
			}
			if tc.accept != nil {
				req.Header.Set("Accept", *tc.accept)
			}
			rec := executeHandler(t, h, req)
			assert.Equal(t, tc.status, rec.Code)
			if tc.status >= 400 {
				assert.False(t, decodeHandlerError(t, rec).IsRetryable)
				assert.Zero(t, resolver.calls)
			} else {
				assert.Equal(t, 1, resolver.calls)
			}
		})
	}
}

func stringPointer(value string) *string { return &value }

type failingReadCloser struct{ err error }

func (r failingReadCloser) Read([]byte) (int, error) { return 0, r.err }
func (r failingReadCloser) Close() error             { return nil }

func TestHandler_BodyValidationAndResolverOrder(t *testing.T) {
	maxTokens := 12
	encoded, err := EncodeCallOptions(provider.CallOptions{MaxOutputTokens: &maxTokens})
	require.NoError(t, err)
	ctxKey := handlerContextKey{}
	cases := []struct {
		name         string
		body         []byte
		reader       io.ReadCloser
		limit        int64
		status       int
		resolve      bool
		legacySystem bool
	}{
		{name: "canonical", body: encoded, limit: int64(len(encoded)), status: http.StatusOK, resolve: true},
		{name: "legacy system content array", body: []byte(`{"prompt":[{"role":"system","content":[{"type":"text","text":"legacy"}]}]}`), limit: 100, status: http.StatusOK, resolve: true, legacySystem: true},
		{name: "malformed", body: []byte(`{`), limit: 10, status: http.StatusBadRequest},
		{name: "exact boundary", body: []byte(`{} `), limit: 3, status: http.StatusOK, resolve: true},
		{name: "maximum limit", body: []byte(`{}`), limit: math.MaxInt64, status: http.StatusOK, resolve: true},
		{name: "oversized", body: []byte(`{}  `), limit: 3, status: http.StatusRequestEntityTooLarge},
		{name: "read failure", reader: failingReadCloser{err: io.ErrUnexpectedEOF}, limit: 10, status: http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotOpts provider.CallOptions
			model := &handlerTestModel{generate: func(_ context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
				gotOpts = opts
				return &provider.GenerateResult{}, nil
			}}
			resolver := &handlerTestResolver{model: model}
			h, err := NewHandler(resolver, WithMaxRequestBodyBytes(tc.limit))
			require.NoError(t, err)
			req := validHandlerRequest(t, false, tc.body)
			if tc.reader != nil {
				req.Body = tc.reader
			}
			req = req.WithContext(context.WithValue(req.Context(), ctxKey, "tenant"))
			rec := executeHandler(t, h, req)
			assert.Equal(t, tc.status, rec.Code)
			if tc.resolve {
				assert.Equal(t, 1, resolver.calls)
				assert.Same(t, req, resolver.req)
				assert.Equal(t, "tenant", resolver.req.Context().Value(ctxKey))
				assert.Equal(t, "model", resolver.id)
			} else {
				assert.Zero(t, resolver.calls)
			}
			if tc.name == "canonical" {
				assert.Equal(t, provider.CallOptions{MaxOutputTokens: &maxTokens}, gotOpts)
			}
			if tc.legacySystem {
				assert.Equal(t, []provider.Message{provider.NewSystemMessage("legacy")}, gotOpts.Prompt)
			}
		})
	}
}

func TestHandler_UnaryDispatch(t *testing.T) {
	maxTokens := 42
	opts := provider.CallOptions{MaxOutputTokens: &maxTokens, Prompt: []provider.Message{provider.UserText("hello")}}
	body, err := EncodeCallOptions(opts)
	require.NoError(t, err)
	expected := &provider.GenerateResult{
		Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "yes"}},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
	}
	var gotOpts provider.CallOptions
	model := &handlerTestModel{generate: func(ctx context.Context, actual provider.CallOptions) (*provider.GenerateResult, error) {
		gotOpts = actual
		deadline, ok := ctx.Deadline()
		assert.True(t, ok)
		assert.WithinDuration(t, time.Now().Add(time.Second), deadline, 100*time.Millisecond)
		return expected, nil
	}}
	h, err := NewHandler(&handlerTestResolver{model: model}, WithTotalTimeout(time.Second))
	require.NoError(t, err)
	rec := executeHandler(t, h, validHandlerRequest(t, false, body))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, MIMEJSON, rec.Header().Get("Content-Type"))
	assert.Equal(t, opts, gotOpts)
	got, err := DecodeGenerateResult(rec.Body.Bytes())
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestHandler_PreCommitErrors(t *testing.T) {
	retryable := false
	preserved := provider.NewAPICallError(provider.APICallErrorOptions{
		Message:           "preserved",
		StatusCode:        http.StatusTeapot,
		URL:               "https://example.test",
		RequestBodyValues: json.RawMessage(`{"request":true}`),
		ResponseHeaders:   map[string][]string{"X-Test": {"value"}},
		ResponseBody:      "body",
		Data:              json.RawMessage(`{"code":"x"}`),
		IsRetryable:       &retryable,
	})
	unencodableAPIErr := provider.NewAPICallError(provider.APICallErrorOptions{
		Message:     "unencodable API error",
		StatusCode:  http.StatusTeapot,
		Data:        json.RawMessage(`{`),
		IsRetryable: &retryable,
	})
	apiErrorWithStatus := func(status int) *provider.APICallError {
		return provider.NewAPICallError(provider.APICallErrorOptions{
			Message:     "invalid API error status",
			StatusCode:  status,
			IsRetryable: &retryable,
		})
	}
	var nilModel *handlerTestModel
	var nilAPIErr *provider.APICallError
	cases := []struct {
		name      string
		resolver  *handlerTestResolver
		streaming bool
		status    int
		retryable bool
		message   string
		check     func(*testing.T, *provider.APICallError)
	}{
		{name: "wrapped resolver API error", resolver: &handlerTestResolver{err: fmt.Errorf("resolve: %w", preserved)}, status: http.StatusTeapot, check: func(t *testing.T, got *provider.APICallError) { assert.Equal(t, preserved, got) }},
		{name: "resolver error", resolver: &handlerTestResolver{err: errors.New("catalog unavailable")}, status: http.StatusBadGateway, retryable: true, message: "catalog unavailable"},
		{name: "unencodable resolver API error", resolver: &handlerTestResolver{err: unencodableAPIErr}, status: http.StatusInternalServerError, retryable: true, message: "encoding API call error response"},
		{name: "negative resolver API error status", resolver: &handlerTestResolver{err: apiErrorWithStatus(-1)}, status: http.StatusInternalServerError, retryable: true, message: "encoding API call error response"},
		{name: "successful resolver API error status", resolver: &handlerTestResolver{err: apiErrorWithStatus(http.StatusOK)}, status: http.StatusInternalServerError, retryable: true, message: "encoding API call error response"},
		{name: "bodyless resolver API error status", resolver: &handlerTestResolver{err: apiErrorWithStatus(http.StatusNotModified)}, status: http.StatusInternalServerError, retryable: true, message: "encoding API call error response"},
		{name: "out-of-range resolver API error status", resolver: &handlerTestResolver{err: apiErrorWithStatus(1000)}, status: http.StatusInternalServerError, retryable: true, message: "encoding API call error response"},
		{name: "nil model", resolver: &handlerTestResolver{}, status: http.StatusInternalServerError, retryable: true, message: "model resolver returned nil model"},
		{name: "typed nil model", resolver: &handlerTestResolver{model: nilModel}, status: http.StatusInternalServerError, retryable: true, message: "model resolver returned nil model"},
		{name: "generate error", resolver: &handlerTestResolver{model: &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return nil, errors.New("backend failed")
		}}}, status: http.StatusBadGateway, retryable: true, message: "backend failed"},
		{name: "typed nil API error", resolver: &handlerTestResolver{model: &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return nil, nilAPIErr
		}}}, status: http.StatusInternalServerError, retryable: true, message: "nil API call error"},
		{name: "unencodable model API error", resolver: &handlerTestResolver{model: &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return nil, unencodableAPIErr
		}}}, status: http.StatusInternalServerError, retryable: true, message: "encoding API call error response"},
		{name: "nil generate result", resolver: &handlerTestResolver{model: &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return nil, nil }}}, status: http.StatusInternalServerError, retryable: true, message: "model returned nil generate result"},
		{name: "unencodable generate result", resolver: &handlerTestResolver{model: &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return &provider.GenerateResult{ProviderMetadata: provider.ProviderMetadata{"bad": json.RawMessage(`{`)}}, nil
		}}}, status: http.StatusInternalServerError, retryable: true, message: "encoding generate result"},
		{name: "stream error", streaming: true, resolver: &handlerTestResolver{model: &handlerTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return nil, errors.New("stream setup failed")
		}}}, status: http.StatusBadGateway, retryable: true, message: "stream setup failed"},
		{name: "nil stream result", streaming: true, resolver: &handlerTestResolver{model: &handlerTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { return nil, nil }}}, status: http.StatusInternalServerError, retryable: true, message: "model returned nil stream"},
		{name: "nil stream channel", streaming: true, resolver: &handlerTestResolver{model: &handlerTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{}, nil
		}}}, status: http.StatusInternalServerError, retryable: true, message: "model returned nil stream"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, err := NewHandler(tc.resolver)
			require.NoError(t, err)
			rec := executeHandler(t, h, validHandlerRequest(t, tc.streaming, nil))
			assert.Equal(t, tc.status, rec.Code)
			assert.Equal(t, MIMEJSON, rec.Header().Get("Content-Type"))
			got := decodeHandlerError(t, rec)
			assert.Equal(t, tc.retryable, got.IsRetryable)
			assert.Contains(t, got.Message, tc.message)
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

func TestHandler_ModelAPICallErrorPreservedWhenContextDone(t *testing.T) {
	retryable := false
	preserved := provider.NewAPICallError(provider.APICallErrorOptions{
		Message:           "model error after timeout",
		StatusCode:        http.StatusTeapot,
		URL:               "https://example.test/model",
		RequestBodyValues: json.RawMessage(`{"request":true}`),
		ResponseHeaders:   map[string][]string{"X-Test": {"value"}},
		ResponseBody:      "response body",
		Data:              json.RawMessage(`{"code":"preserved"}`),
		IsRetryable:       &retryable,
	})
	for _, streaming := range []bool{false, true} {
		name := "unary"
		if streaming {
			name = "pre-stream"
		}
		t.Run(name, func(t *testing.T) {
			model := &handlerTestModel{
				generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
					<-ctx.Done()
					return nil, fmt.Errorf("generate: %w", preserved)
				},
				stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
					<-ctx.Done()
					return nil, fmt.Errorf("stream: %w", preserved)
				},
			}
			h, err := NewHandler(&handlerTestResolver{model: model}, WithTotalTimeout(5*time.Millisecond))
			require.NoError(t, err)
			rec := executeHandler(t, h, validHandlerRequest(t, streaming, nil))
			got := decodeHandlerError(t, rec)
			assert.Equal(t, http.StatusTeapot, rec.Code)
			assert.Equal(t, preserved, got)
		})
	}
}

func TestHandler_UnaryCancellationAndTimeout(t *testing.T) {
	cases := []struct {
		name      string
		timeout   time.Duration
		status    int
		retryable bool
		message   string
	}{
		{name: "request cancellation", timeout: time.Second, status: 499, message: "consumer disconnected"},
		{name: "total timeout", timeout: 5 * time.Millisecond, status: http.StatusGatewayTimeout, retryable: true, message: "total timeout exceeded"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := &handlerTestModel{generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				if tc.name == "request cancellation" {
					return nil, ctx.Err()
				}
				<-ctx.Done()
				assert.ErrorIs(t, context.Cause(ctx), ErrTotalTimeout)
				return nil, ctx.Err()
			}}
			h, err := NewHandler(&handlerTestResolver{model: model}, WithTotalTimeout(tc.timeout))
			require.NoError(t, err)
			req := validHandlerRequest(t, false, nil)
			if tc.name == "request cancellation" {
				ctx, cancel := context.WithCancel(req.Context())
				cancel()
				req = req.WithContext(ctx)
			}
			rec := executeHandler(t, h, req)
			got := decodeHandlerError(t, rec)
			assert.Equal(t, tc.status, rec.Code)
			assert.Equal(t, tc.retryable, got.IsRetryable)
			assert.Equal(t, tc.message, got.Message)
		})
	}
}

type flushRecorder struct {
	header     http.Header
	status     int
	body       bytes.Buffer
	flushes    int
	firstFlush chan struct{}
	once       sync.Once
}

func newFlushRecorder() *flushRecorder {
	return &flushRecorder{header: make(http.Header), firstFlush: make(chan struct{})}
}
func (w *flushRecorder) Header() http.Header { return w.header }
func (w *flushRecorder) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *flushRecorder) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(data)
}
func (w *flushRecorder) Flush() {
	w.flushes++
	w.once.Do(func() { close(w.firstFlush) })
}

func TestHandler_StreamSetupAndForwarding(t *testing.T) {
	parts := []provider.StreamPart{
		{Type: provider.PartTextStart, ID: "text"},
		{Type: provider.PartTextDelta, ID: "text", Delta: "hello"},
		{Type: provider.PartTextEnd, ID: "text"},
	}
	stream := make(chan provider.StreamPart)
	model := &handlerTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: stream}, nil
	}}
	h, err := NewHandler(&handlerTestResolver{model: model})
	require.NoError(t, err)
	writer := newFlushRecorder()
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(writer, validHandlerRequest(t, true, nil))
		close(done)
	}()
	select {
	case <-writer.firstFlush:
	case <-time.After(time.Second):
		t.Fatal("initial headers were not flushed")
	}
	assert.Equal(t, http.StatusOK, writer.status)
	assert.Equal(t, MIMESSE, writer.header.Get("Content-Type"))
	assert.Equal(t, "no-cache, no-transform", writer.header.Get("Cache-Control"))
	assert.Equal(t, "keep-alive", writer.header.Get("Connection"))
	assert.Equal(t, "no", writer.header.Get("X-Accel-Buffering"))
	for _, part := range parts {
		stream <- part
	}
	close(stream)
	<-done
	assert.Equal(t, len(parts)+1, writer.flushes)
	assert.NotContains(t, writer.body.String(), "[DONE]")
	reader := NewSSEReader(strings.NewReader(writer.body.String()))
	for _, expected := range parts {
		actual, err := reader.Next()
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	}
	_, err = reader.Next()
	assert.ErrorIs(t, err, io.EOF)
}

func TestHandler_UpstreamPartErrorDoesNotTerminate(t *testing.T) {
	retryable := true
	parts := make(chan provider.StreamPart, 3)
	parts <- provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "boom", StatusCode: 502, IsRetryable: &retryable})}
	parts <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "after"}
	parts <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonError}, Usage: &provider.Usage{}}
	close(parts)
	model := &handlerTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: parts}, nil
	}}
	h, err := NewHandler(&handlerTestResolver{model: model})
	require.NoError(t, err)
	rec := executeHandler(t, h, validHandlerRequest(t, true, nil))
	got := readAllHandlerParts(t, rec.Body.String())
	require.Len(t, got, 3)
	require.NotNil(t, got[0].APICallError)
	assert.True(t, got[0].APICallError.IsRetryable)
	assert.Equal(t, provider.PartTextDelta, got[1].Type)
	assert.Equal(t, "after", got[1].Delta)
	assert.Equal(t, provider.PartFinish, got[2].Type)
	require.NotNil(t, got[2].FinishReason)
	assert.Equal(t, provider.FinishReasonError, got[2].FinishReason.Unified)
}

func readAllHandlerParts(t *testing.T, body string) []provider.StreamPart {
	t.Helper()
	reader := NewSSEReader(strings.NewReader(body))
	var parts []provider.StreamPart
	for {
		part, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return parts
		}
		require.NoError(t, err)
		parts = append(parts, part)
	}
}

type failWriter struct {
	header http.Header
	writes int
	err    error
}

func (w *failWriter) Header() http.Header       { return w.header }
func (w *failWriter) WriteHeader(int)           {}
func (w *failWriter) Flush()                    {}
func (w *failWriter) Write([]byte) (int, error) { w.writes++; return 0, w.err }

func TestHandler_StreamOutputFailureCancelsModel(t *testing.T) {
	cases := []struct {
		name   string
		part   provider.StreamPart
		writer http.ResponseWriter
	}{
		{name: "encoding", part: provider.StreamPart{Type: provider.PartRaw, RawValue: json.RawMessage(`{`)}, writer: newFlushRecorder()},
		{name: "writing", part: provider.StreamPart{Type: provider.PartTextDelta, Delta: "x"}, writer: &failWriter{header: make(http.Header), err: io.ErrClosedPipe}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cancelled := make(chan error, 1)
			stream := make(chan provider.StreamPart, 2)
			stream <- tc.part
			stream <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "second"}
			model := &handlerTestModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				go func() {
					<-ctx.Done()
					cancelled <- context.Cause(ctx)
				}()
				return &provider.StreamResult{Stream: stream}, nil
			}}
			h, err := NewHandler(&handlerTestResolver{model: model}, WithIdleTimeout(time.Second))
			require.NoError(t, err)
			h.ServeHTTP(tc.writer, validHandlerRequest(t, true, nil))
			select {
			case cause := <-cancelled:
				assert.Contains(t, cause.Error(), "writing stream part")
			case <-time.After(time.Second):
				t.Fatal("model context was not canceled")
			}
			if writer, ok := tc.writer.(*failWriter); ok {
				assert.Equal(t, 1, writer.writes)
			}
		})
	}
}

func TestHandler_StreamTimeouts(t *testing.T) {
	t.Run("total before commitment", func(t *testing.T) {
		model := &handlerTestModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			<-ctx.Done()
			assert.ErrorIs(t, context.Cause(ctx), ErrTotalTimeout)
			return nil, ctx.Err()
		}}
		h, err := NewHandler(&handlerTestResolver{model: model}, WithTotalTimeout(5*time.Millisecond))
		require.NoError(t, err)
		rec := executeHandler(t, h, validHandlerRequest(t, true, nil))
		got := decodeHandlerError(t, rec)
		assert.Equal(t, http.StatusGatewayTimeout, rec.Code)
		assert.True(t, got.IsRetryable)
		assert.Equal(t, "total timeout exceeded", got.Message)
	})

	for _, tc := range []struct {
		name  string
		total time.Duration
		idle  time.Duration
		want  error
	}{
		{name: "total during stream", total: 15 * time.Millisecond, idle: time.Second, want: ErrTotalTimeout},
		{name: "idle before first part", total: time.Second, idle: 15 * time.Millisecond, want: ErrIdleTimeout},
		{name: "idle between parts", total: time.Second, idle: 15 * time.Millisecond, want: ErrIdleTimeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := make(chan provider.StreamPart, 1)
			cause := make(chan error, 1)
			model := &handlerTestModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				go func() {
					<-ctx.Done()
					cause <- context.Cause(ctx)
				}()
				if tc.name == "idle between parts" {
					stream <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "first"}
				}
				return &provider.StreamResult{Stream: stream}, nil
			}}
			h, err := NewHandler(&handlerTestResolver{model: model}, WithTotalTimeout(tc.total), WithIdleTimeout(tc.idle))
			require.NoError(t, err)
			rec := executeHandler(t, h, validHandlerRequest(t, true, nil))
			gotCause := <-cause
			assert.ErrorIs(t, gotCause, tc.want)
			parts := readAllHandlerParts(t, rec.Body.String())
			require.NotEmpty(t, parts)
			last := parts[len(parts)-1]
			assert.Equal(t, provider.PartError, last.Type)
			assert.Equal(t, http.StatusGatewayTimeout, last.APICallError.StatusCode)
			assert.True(t, last.APICallError.IsRetryable)
		})
	}
}

func TestHandler_IdleTimeoutResetsAfterPartError(t *testing.T) {
	retryable := false
	stream := make(chan provider.StreamPart)
	model := &handlerTestModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		go func() {
			defer close(stream)
			parts := []provider.StreamPart{
				{Type: provider.PartTextDelta, ID: "text", Delta: "before"},
				{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "provider error", IsRetryable: &retryable})},
				{Type: provider.PartTextDelta, ID: "text", Delta: "after"},
			}
			for i, part := range parts {
				select {
				case stream <- part:
				case <-ctx.Done():
					return
				}
				if i < len(parts)-1 {
					time.Sleep(60 * time.Millisecond)
				}
			}
		}()
		return &provider.StreamResult{Stream: stream}, nil
	}}
	h, err := NewHandler(&handlerTestResolver{model: model}, WithTotalTimeout(time.Second), WithIdleTimeout(100*time.Millisecond))
	require.NoError(t, err)
	rec := executeHandler(t, h, validHandlerRequest(t, true, nil))
	parts := readAllHandlerParts(t, rec.Body.String())
	require.Len(t, parts, 3)
	assert.Equal(t, provider.PartTextDelta, parts[0].Type)
	assert.Equal(t, "before", parts[0].Delta)
	assert.Equal(t, provider.PartError, parts[1].Type)
	assert.Equal(t, provider.PartTextDelta, parts[2].Type)
	assert.Equal(t, "after", parts[2].Delta)
}

func TestHandler_CanceledRequestAfterCommitEmits499(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stream := make(chan provider.StreamPart)
	model := &handlerTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: stream}, nil
	}}
	h, err := NewHandler(&handlerTestResolver{model: model})
	require.NoError(t, err)
	writer := newFlushRecorder()
	done := make(chan struct{})
	req := validHandlerRequest(t, true, nil).WithContext(ctx)
	go func() { h.ServeHTTP(writer, req); close(done) }()
	<-writer.firstFlush
	cancel()
	<-done
	parts := readAllHandlerParts(t, writer.body.String())
	require.Len(t, parts, 1)
	assert.Equal(t, 499, parts[0].APICallError.StatusCode)
	assert.False(t, parts[0].APICallError.IsRetryable)
	assert.Equal(t, "consumer disconnected", parts[0].APICallError.Message)
}

func TestHandler_ConsumerDisconnectCancelsProducer(t *testing.T) {
	cancelled := make(chan struct{})
	stream := make(chan provider.StreamPart)
	model := &handlerTestModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		go func() {
			<-ctx.Done()
			close(cancelled)
		}()
		return &provider.StreamResult{Stream: stream}, nil
	}}
	h, err := NewHandler(&handlerTestResolver{model: model}, WithTotalTimeout(time.Second), WithIdleTimeout(time.Second))
	require.NoError(t, err)
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+PathLanguageModel, strings.NewReader(`{}`))
	require.NoError(t, err)
	for name, values := range validHandlerRequest(t, true, nil).Header {
		req.Header[name] = values
	}
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	cancel()
	_ = resp.Body.Close()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("producer was not canceled after client disconnect")
	}
}
