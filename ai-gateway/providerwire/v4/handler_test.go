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
	"github.com/grafana/ai-sdk/provider"
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

type policyStub struct {
	calls int
}

func (p *policyStub) Apply(_ context.Context, options provider.CallOptions) (provider.CallOptions, error) {
	p.calls++
	return options, nil
}

func testLimits() Limits {
	return Limits{
		RequestBytes:       1 << 20,
		JSONDepth:          64,
		JSONTokens:         100_000,
		NumberBytes:        128,
		UnaryResponseBytes: 1 << 20,
		ErrorResponseBytes: 1 << 20,
		ModelDuration:      time.Second,
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
		resolver := &resolverStub{}
		created, err := New(Config{Resolver: resolver, Limits: limits})
		require.NoError(t, err)
		h := created.(*handler)
		require.NotNil(t, h.requestSchema)
		require.NotNil(t, h.unarySuccessSchema)
		require.NotNil(t, h.errorSchema)
		assert.IsType(t, noOpPolicy{}, h.policy)

		limits.RequestBytes = 1
		assert.Equal(t, int64(1<<20), h.limits.RequestBytes)
	})

	t.Run("small positive unary response and exact error fallback limits", func(t *testing.T) {
		limits := testLimits()
		limits.UnaryResponseBytes = 1
		limits.ErrorResponseBytes = int64(len(canonicalInternalError))
		_, err := New(Config{Resolver: &resolverStub{}, Limits: limits})
		require.NoError(t, err)
	})

	t.Run("nil dependencies", func(t *testing.T) {
		_, err := New(Config{Limits: testLimits()})
		require.Error(t, err)

		var resolver *resolverStub
		_, err = New(Config{Resolver: resolver, Limits: testLimits()})
		require.Error(t, err)

		var policy *policyStub
		created, err := New(Config{Resolver: &resolverStub{}, Policy: policy, Limits: testLimits()})
		require.NoError(t, err)
		assert.IsType(t, noOpPolicy{}, created.(*handler).policy)
	})

	t.Run("invalid limits", func(t *testing.T) {
		tests := []struct {
			name   string
			mutate func(*Limits)
		}{
			{name: "request zero", mutate: func(l *Limits) { l.RequestBytes = 0 }},
			{name: "request negative", mutate: func(l *Limits) { l.RequestBytes = -1 }},
			{name: "request overflow", mutate: func(l *Limits) { l.RequestBytes = math.MaxInt64 }},
			{name: "depth zero", mutate: func(l *Limits) { l.JSONDepth = 0 }},
			{name: "depth negative", mutate: func(l *Limits) { l.JSONDepth = -1 }},
			{name: "tokens zero", mutate: func(l *Limits) { l.JSONTokens = 0 }},
			{name: "tokens negative", mutate: func(l *Limits) { l.JSONTokens = -1 }},
			{name: "number zero", mutate: func(l *Limits) { l.NumberBytes = 0 }},
			{name: "number negative", mutate: func(l *Limits) { l.NumberBytes = -1 }},
			{name: "number overflow", mutate: func(l *Limits) { l.NumberBytes = int(^uint(0) >> 1) }},
			{name: "unary zero", mutate: func(l *Limits) { l.UnaryResponseBytes = 0 }},
			{name: "unary overflow", mutate: func(l *Limits) { l.UnaryResponseBytes = math.MaxInt64 }},
			{name: "error zero", mutate: func(l *Limits) { l.ErrorResponseBytes = 0 }},
			{name: "error overflow", mutate: func(l *Limits) { l.ErrorResponseBytes = math.MaxInt64 }},
			{name: "duration zero", mutate: func(l *Limits) { l.ModelDuration = 0 }},
			{name: "duration negative", mutate: func(l *Limits) { l.ModelDuration = -time.Second }},
			{name: "error fallback too small", mutate: func(l *Limits) { l.ErrorResponseBytes = int64(len(canonicalInternalError) - 1) }},
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
		{name: "encoded path alias", mutate: func(r *http.Request) {
			r.URL.Path = LanguageModelPath
			r.URL.RawPath = "/%6canguage-model"
		}},
		{name: "content type missing", mutate: func(r *http.Request) { r.Header.Del("Content-Type") }},
		{name: "content type invalid", mutate: func(r *http.Request) { r.Header.Set("Content-Type", "text/json") }},
		{name: "content type repeated", mutate: func(r *http.Request) { r.Header["Content-Type"] = []string{"application/json", "application/json"} }},
		{name: "spec missing", mutate: func(r *http.Request) { r.Header.Del(HeaderSpecificationVersion) }},
		{name: "spec empty", mutate: func(r *http.Request) { r.Header.Set(HeaderSpecificationVersion, "") }},
		{name: "spec invalid", mutate: func(r *http.Request) { r.Header.Set(HeaderSpecificationVersion, "v4") }},
		{name: "spec repeated", mutate: func(r *http.Request) {
			r.Header[http.CanonicalHeaderKey(HeaderSpecificationVersion)] = []string{"4", "4"}
		}},
		{name: "spec collision normalized", mutate: func(r *http.Request) { r.Header[HeaderSpecificationVersion] = []string{"4"} }},
		{name: "model missing", mutate: func(r *http.Request) { r.Header.Del(HeaderModelID) }},
		{name: "model empty", mutate: func(r *http.Request) { r.Header.Set(HeaderModelID, "") }},
		{name: "model repeated", mutate: func(r *http.Request) { r.Header[http.CanonicalHeaderKey(HeaderModelID)] = []string{"a", "b"} }},
		{name: "model collision normalized", mutate: func(r *http.Request) { r.Header[HeaderModelID] = []string{"other"} }},
		{name: "stream missing", mutate: func(r *http.Request) { r.Header.Del(HeaderStreaming) }},
		{name: "stream empty", mutate: func(r *http.Request) { r.Header.Set(HeaderStreaming, "") }},
		{name: "stream repeated", mutate: func(r *http.Request) {
			r.Header[http.CanonicalHeaderKey(HeaderStreaming)] = []string{"false", "false"}
		}},
		{name: "stream collision normalized", mutate: func(r *http.Request) { r.Header[HeaderStreaming] = []string{"false"} }},
		{name: "stream true", mutate: func(r *http.Request) { r.Header.Set(HeaderStreaming, "true") }},
		{name: "stream invalid", mutate: func(r *http.Request) { r.Header.Set(HeaderStreaming, "False") }},
	}

	resolver := &resolverStub{}
	policy := &policyStub{}
	created, err := New(Config{Resolver: resolver, Policy: policy, Limits: testLimits()})
	require.NoError(t, err)
	h := created.(*handler)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := validRequest(`{"prompt":[]}`)
			tc.mutate(req)
			_, failure := h.validateRequest(req)
			require.NotNil(t, failure)
			assert.Equal(t, stageEnvelope, failure.stage)

			rawRequest := validRequest(`{"prompt":[]}`)
			tc.mutate(rawRequest)
			recorder := httptest.NewRecorder()
			h.ServeHTTP(recorder, rawRequest)
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.Equal(t, string(canonicalInvalidRequestError), recorder.Body.String())
			assert.Zero(t, policy.calls)
			assert.Zero(t, resolver.calls)
		})
	}

	t.Run("valid preserves model and ignores unrelated headers", func(t *testing.T) {
		req := validRequest(`{"prompt":[]}`)
		req.Header.Set("X-Host-Only", "private")
		validated, failure := h.validateRequest(req)
		require.Nil(t, failure)
		assert.Equal(t, " public/model ", validated.modelID)
	})
}

func TestHandlerBodyBoundaryAndClose(t *testing.T) {
	base := `{"prompt":[]}`
	limits := testLimits()
	limits.RequestBytes = int64(len(base) + 1)
	h := newTestHandler(t, limits)

	tests := []struct {
		name      string
		body      string
		wantStage requestStage
	}{
		{name: "below", body: base},
		{name: "at", body: base + " "},
		{name: "above", body: base + "  ", wantStage: stageBody},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := validRequest(tc.body)
			body := &trackingReadCloser{Reader: bytes.NewBufferString(tc.body)}
			req.Body = body
			_, failure := h.validateRequest(req)
			assert.True(t, body.closed)
			if tc.wantStage == "" {
				require.Nil(t, failure)
				return
			}
			require.NotNil(t, failure)
			assert.Equal(t, tc.wantStage, failure.stage)
			assert.LessOrEqual(t, body.read, int(limits.RequestBytes+1))
		})
	}
}

func TestHandlerBodyFailures(t *testing.T) {
	resolver := &resolverStub{}
	policy := &policyStub{}
	created, err := New(Config{Resolver: resolver, Policy: policy, Limits: testLimits()})
	require.NoError(t, err)
	h := created.(*handler)

	tests := []struct {
		name     string
		body     io.ReadCloser
		wantSafe safeErrorCategory
	}{
		{name: "nil body"},
		{name: "read error", body: &failingReadCloser{readErr: errors.New("read secret")}, wantSafe: safeInternal},
		{name: "close error", body: &trackingReadCloser{Reader: strings.NewReader(`{"prompt":[]}`), closeErr: errors.New("close secret")}, wantSafe: safeInternal},
		{name: "read and close error", body: &failingReadCloser{readErr: errors.New("read secret"), closeErr: errors.New("close secret")}, wantSafe: safeInternal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := validRequest(`{"prompt":[]}`)
			req.Body = tc.body
			_, failure := h.validateRequest(req)
			require.NotNil(t, failure)
			assert.Equal(t, stageBody, failure.stage)
			assert.Equal(t, tc.wantSafe, failure.safe.category)
			if body, ok := tc.body.(interface{ wasClosed() bool }); ok {
				assert.True(t, body.wasClosed())
			}

			recorder := httptest.NewRecorder()
			req = validRequest(`{"prompt":[]}`)
			if tc.body == nil {
				req.Body = nil
			} else {
				req.Body = &failingReadCloser{readErr: errors.New("private"), closeErr: errors.New("private")}
			}
			h.ServeHTTP(recorder, req)
			if tc.wantSafe == safeInternal {
				assert.Equal(t, http.StatusInternalServerError, recorder.Code)
				assert.Equal(t, string(canonicalInternalError), recorder.Body.String())
			} else {
				assert.Equal(t, http.StatusBadRequest, recorder.Code)
				assert.Equal(t, string(canonicalInvalidRequestError), recorder.Body.String())
			}
			assert.Zero(t, policy.calls)
			assert.Zero(t, resolver.calls)
		})
	}
}

func TestHandlerSchemaStage(t *testing.T) {
	resolver := &resolverStub{}
	policy := &policyStub{}
	created, err := New(Config{Resolver: resolver, Policy: policy, Limits: testLimits()})
	require.NoError(t, err)
	h := created.(*handler)

	tests := []struct {
		name      string
		body      string
		wantStage requestStage
	}{
		{name: "valid complete registered branch", body: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"text","text":"data"},"mediaType":"text/plain"}]}]}`},
		{name: "duplicate member stops before schema", body: `{"prompt":[],"prompt":[]}`, wantStage: stageLexical},
		{name: "unknown member", body: `{"prompt":[],"unknown":true}`, wantStage: stageSchema},
		{name: "typed null", body: `{"prompt":[],"maxOutputTokens":null}`, wantStage: stageSchema},
		{name: "fractional integer", body: `{"prompt":[],"maxOutputTokens":1.5}`, wantStage: stageSchema},
		{name: "unknown discriminator", body: `{"prompt":[{"role":"future","content":[]}]}`, wantStage: stageSchema},
		{name: "inactive union arm", body: `{"prompt":[],"responseFormat":{"type":"text","schema":{}}}`, wantStage: stageSchema},
		{name: "role incompatible content", body: `{"prompt":[{"role":"system","content":[]}]}`, wantStage: stageSchema},
		{name: "malformed provider options", body: `{"prompt":[],"providerOptions":{"provider":1}}`, wantStage: stageSchema},
		{name: "huge exponent", body: `{"prompt":[],"temperature":1e309}`, wantStage: stageSchema},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, failure := h.validateRequest(validRequest(tc.body))
			if tc.wantStage == "" {
				require.Nil(t, failure)
			} else {
				require.NotNil(t, failure)
				assert.Equal(t, tc.wantStage, failure.stage)
			}
			assert.Zero(t, policy.calls)
			assert.Zero(t, resolver.calls)
		})
	}
}

func TestHandlerFailureWireShape(t *testing.T) {
	t.Run("ordinary invalid request", func(t *testing.T) {
		h := newTestHandler(t, testLimits())
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, validRequest(`{"prompt":[],"unknown":true}`))
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
		assert.Equal(t, string(canonicalInvalidRequestError), recorder.Body.String())
	})

	t.Run("invalid request falls back within error limit", func(t *testing.T) {
		require.Greater(t, len(canonicalInvalidRequestError), len(canonicalInternalError))
		limits := testLimits()
		limits.ErrorResponseBytes = int64(len(canonicalInternalError))
		h := newTestHandler(t, limits)
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, validRequest(`{"prompt":[],"unknown":true}`))
		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.Equal(t, string(canonicalInternalError), recorder.Body.String())
		assert.LessOrEqual(t, recorder.Body.Len(), int(limits.ErrorResponseBytes))
	})
}

type trackingReadCloser struct {
	io.Reader
	read     int
	closed   bool
	closeErr error
}

func (r *trackingReadCloser) Read(data []byte) (int, error) {
	n, err := r.Reader.Read(data)
	r.read += n
	return n, err
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return r.closeErr
}

func (r *trackingReadCloser) wasClosed() bool { return r.closed }

type failingReadCloser struct {
	readErr  error
	closeErr error
	closed   bool
}

func (r *failingReadCloser) Read([]byte) (int, error) { return 0, r.readErr }

func (r *failingReadCloser) Close() error {
	r.closed = true
	return r.closeErr
}

func (r *failingReadCloser) wasClosed() bool { return r.closed }
