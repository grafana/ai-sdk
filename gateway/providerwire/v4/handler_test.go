package v4

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/gateway/catalog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type resolverStub struct {
	calls int
}

func (r *resolverStub) ResolveModel(context.Context, string) (catalog.ResolvedModel, error) {
	r.calls++
	return catalog.ResolvedModel{}, nil
}

func testLimits() Limits {
	return Limits{
		RequestBytes:        1 << 20,
		UnaryResponseBytes:  1 << 20,
		StreamParts:         1_000,
		StreamFrameBytes:    1 << 20,
		ModelDuration:       time.Second,
		StreamIdleDuration:  time.Second,
		StreamDrainDuration: time.Second,
	}
}

func newTestHandler(t *testing.T, limits Limits) *handler {
	t.Helper()
	created, err := New(Config{Resolver: &resolverStub{}, Limits: limits})
	require.NoError(t, err)
	h, ok := created.(*handler)
	require.True(t, ok)
	return h
}

func validRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, LanguageModelPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderSpecificationVersion, SpecificationVersion)
	req.Header.Set(HeaderModelID, " public/model ")
	req.Header.Set(HeaderStreaming, "false")
	return req
}

func TestNew(t *testing.T) {
	t.Run("valid immutable configuration", func(t *testing.T) {
		limits := testLimits()
		created, err := New(Config{Resolver: &resolverStub{}, Limits: limits})
		require.NoError(t, err)
		h := created.(*handler)
		require.NotNil(t, h.requestSchema)
		limits.RequestBytes = 1
		assert.Equal(t, int64(1<<20), h.limits.RequestBytes)
	})

	t.Run("maximum fixed stream frame boundary", func(t *testing.T) {
		largest := 0
		for _, frame := range [][]byte{
			canonicalEmptyStartFrame,
			canonicalRateLimitStreamErrorFrame,
			canonicalOverloadStreamErrorFrame,
			canonicalDependencyStreamErrorFrame,
			canonicalUpstreamStreamErrorFrame,
			canonicalTimeoutStreamErrorFrame,
			canonicalCancellationStreamErrorFrame,
			canonicalInternalStreamErrorFrame,
		} {
			largest = max(largest, len(frame))
		}
		limits := testLimits()
		limits.StreamFrameBytes = int64(largest)
		_, err := New(Config{Resolver: &resolverStub{}, Limits: limits})
		require.NoError(t, err)
		limits.StreamFrameBytes--
		_, err = New(Config{Resolver: &resolverStub{}, Limits: limits})
		require.Error(t, err)
	})

	t.Run("nil resolver", func(t *testing.T) {
		_, err := New(Config{Limits: testLimits()})
		require.Error(t, err)
		var resolver *resolverStub
		_, err = New(Config{Resolver: resolver, Limits: testLimits()})
		require.Error(t, err)
	})

	t.Run("invalid limits", func(t *testing.T) {
		tests := []struct {
			name   string
			mutate func(*Limits)
		}{
			{name: "request zero", mutate: func(l *Limits) { l.RequestBytes = 0 }},
			{name: "request negative", mutate: func(l *Limits) { l.RequestBytes = -1 }},
			{name: "request overflow", mutate: func(l *Limits) { l.RequestBytes = math.MaxInt64 }},
			{name: "response zero", mutate: func(l *Limits) { l.UnaryResponseBytes = 0 }},
			{name: "response overflow", mutate: func(l *Limits) { l.UnaryResponseBytes = math.MaxInt64 }},
			{name: "stream parts zero", mutate: func(l *Limits) { l.StreamParts = 0 }},
			{name: "stream parts overflow", mutate: func(l *Limits) { l.StreamParts = int(^uint(0) >> 1) }},
			{name: "stream frame zero", mutate: func(l *Limits) { l.StreamFrameBytes = 0 }},
			{name: "stream frame overflow", mutate: func(l *Limits) { l.StreamFrameBytes = math.MaxInt64 }},
			{name: "stream frame fallback", mutate: func(l *Limits) { l.StreamFrameBytes = int64(len(canonicalTimeoutStreamErrorFrame) - 1) }},
			{name: "duration zero", mutate: func(l *Limits) { l.ModelDuration = 0 }},
			{name: "duration negative", mutate: func(l *Limits) { l.ModelDuration = -time.Second }},
			{name: "idle zero", mutate: func(l *Limits) { l.StreamIdleDuration = 0 }},
			{name: "drain zero", mutate: func(l *Limits) { l.StreamDrainDuration = 0 }},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				limits := testLimits()
				tc.mutate(&limits)
				_, err := New(Config{Resolver: &resolverStub{}, Limits: limits})
				require.Error(t, err)
			})
		}
	})
}

func TestHandlerEnvelope(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*http.Request)
	}{
		{name: "method", mutate: func(r *http.Request) { r.Method = http.MethodGet }},
		{name: "path", mutate: func(r *http.Request) { r.URL.Path = "/prefix/language-model" }},
		{name: "encoded path alias", mutate: func(r *http.Request) { r.URL.RawPath = "/%6canguage-model" }},
		{name: "content type missing", mutate: func(r *http.Request) { r.Header.Del("Content-Type") }},
		{name: "content type invalid", mutate: func(r *http.Request) { r.Header.Set("Content-Type", "text/json") }},
		{name: "content type repeated", mutate: func(r *http.Request) { r.Header["Content-Type"] = []string{"application/json", "application/json"} }},
		{name: "spec missing", mutate: func(r *http.Request) { r.Header.Del(HeaderSpecificationVersion) }},
		{name: "spec invalid", mutate: func(r *http.Request) { r.Header.Set(HeaderSpecificationVersion, "v4") }},
		{name: "spec repeated", mutate: func(r *http.Request) {
			r.Header[http.CanonicalHeaderKey(HeaderSpecificationVersion)] = []string{"4", "4"}
		}},
		{name: "model missing", mutate: func(r *http.Request) { r.Header.Del(HeaderModelID) }},
		{name: "model empty", mutate: func(r *http.Request) { r.Header.Set(HeaderModelID, "") }},
		{name: "model repeated", mutate: func(r *http.Request) { r.Header[http.CanonicalHeaderKey(HeaderModelID)] = []string{"a", "b"} }},
		{name: "stream missing", mutate: func(r *http.Request) { r.Header.Del(HeaderStreaming) }},
		{name: "stream invalid", mutate: func(r *http.Request) { r.Header.Set(HeaderStreaming, "TRUE") }},
		{name: "stream repeated", mutate: func(r *http.Request) { r.Header[http.CanonicalHeaderKey(HeaderStreaming)] = []string{"false", "false"} }},
	}

	resolver := &resolverStub{}
	created, err := New(Config{Resolver: resolver, Limits: testLimits()})
	require.NoError(t, err)
	h := created.(*handler)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := validRequest(`{"prompt":[]}`)
			tc.mutate(req)
			recorder := httptest.NewRecorder()
			h.ServeHTTP(recorder, req)
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.Equal(t, string(canonicalInvalidRequestError), recorder.Body.String())
			assert.Zero(t, resolver.calls)
		})
	}

	t.Run("valid preserves exact model ID and selects mode", func(t *testing.T) {
		for _, tc := range []struct {
			value string
			mode  executionMode
		}{
			{value: "false", mode: executionUnary},
			{value: "true", mode: executionStreaming},
		} {
			req := validRequest(`{"prompt":[]}`)
			req.Header.Set(HeaderStreaming, tc.value)
			validated, failure := h.validateRequest(req)
			require.Nil(t, failure)
			assert.Equal(t, " public/model ", validated.modelID)
			assert.Equal(t, tc.mode, validated.mode)
		}
	})
}

func TestHandlerBodyProcessing(t *testing.T) {
	t.Run("byte boundary and close", func(t *testing.T) {
		base := `{"prompt":[]}`
		limits := testLimits()
		limits.RequestBytes = int64(len(base) + 1)
		h := newTestHandler(t, limits)
		for _, tc := range []struct {
			name    string
			body    string
			invalid bool
		}{
			{name: "below", body: base},
			{name: "at", body: base + " "},
			{name: "above", body: base + "  ", invalid: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				req := validRequest(tc.body)
				body := &trackingReadCloser{Reader: bytes.NewBufferString(tc.body)}
				req.Body = body
				_, failure := h.validateRequest(req)
				assert.True(t, body.closed)
				assert.Equal(t, tc.invalid, failure != nil)
				assert.LessOrEqual(t, body.read, int(limits.RequestBytes+1))
			})
		}
	})

	t.Run("invalid UTF-8 and JSON", func(t *testing.T) {
		h := newTestHandler(t, testLimits())
		bodies := [][]byte{
			append([]byte(`{"prompt":[],"providerOptions":{"example":"`), append([]byte{0xff}, []byte(`"}}`)...)...),
			[]byte(`{"prompt":[}`),
			[]byte(`{"prompt":[]} {}`),
			[]byte(`{"prompt":[],"unknown":true}`),
		}
		for _, body := range bodies {
			req := validRequest("")
			req.Body = io.NopCloser(bytes.NewReader(body))
			_, failure := h.validateRequest(req)
			require.NotNil(t, failure)
		}
	})

	t.Run("body failures", func(t *testing.T) {
		h := newTestHandler(t, testLimits())
		for _, body := range []io.ReadCloser{
			nil,
			&failingReadCloser{readErr: errors.New("read secret")},
			&trackingReadCloser{Reader: strings.NewReader(`{"prompt":[]}`), closeErr: errors.New("close secret")},
		} {
			req := validRequest(`{"prompt":[]}`)
			req.Body = body
			recorder := httptest.NewRecorder()
			h.ServeHTTP(recorder, req)
			if body == nil {
				assert.Equal(t, http.StatusBadRequest, recorder.Code)
			} else {
				assert.Equal(t, http.StatusInternalServerError, recorder.Code)
			}
		}
	})
}

type trackingReadCloser struct {
	io.Reader
	closed   bool
	read     int
	closeErr error
}

func (r *trackingReadCloser) Read(data []byte) (int, error) {
	count, err := r.Reader.Read(data)
	r.read += count
	return count, err
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return r.closeErr
}

type failingReadCloser struct {
	readErr error
	closed  bool
}

func (r *failingReadCloser) Read([]byte) (int, error) { return 0, r.readErr }
func (r *failingReadCloser) Close() error {
	r.closed = true
	return nil
}
