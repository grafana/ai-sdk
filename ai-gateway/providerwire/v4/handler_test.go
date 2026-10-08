package v4

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
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
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
	created, err := New(Config{Selector: CatalogSelector(&resolverStub{}), Limits: limits})
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
		created, err := New(Config{Selector: CatalogSelector(&resolverStub{}), Limits: limits})
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
		_, err := New(Config{Selector: CatalogSelector(&resolverStub{}), Limits: limits})
		require.NoError(t, err)
		limits.StreamFrameBytes--
		_, err = New(Config{Selector: CatalogSelector(&resolverStub{}), Limits: limits})
		require.Error(t, err)
		limits.StreamFrameBytes = math.MaxInt64
		_, err = New(Config{Selector: CatalogSelector(&resolverStub{}), Limits: limits})
		require.NoError(t, err)
	})

	t.Run("nil resolver", func(t *testing.T) {
		_, err := New(Config{Limits: testLimits()})
		require.Error(t, err)
		var resolver *resolverStub
		_, err = New(Config{Selector: CatalogSelector(resolver), Limits: testLimits()})
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
				_, err := New(Config{Selector: CatalogSelector(&resolverStub{}), Limits: limits})
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
	created, err := New(Config{Selector: CatalogSelector(resolver), Limits: testLimits()})
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

func TestRequestSelection_CatalogIndependent(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(strconv.FormatBool(streaming), func(t *testing.T) {
			model := &recordingModel{}
			var selections atomic.Int32
			h, err := New(Config{Limits: testLimits(), Selector: func(_ context.Context, id string, options provider.CallOptions, gateway json.RawMessage) (Selection, error) {
				selections.Add(1)
				assert.Equal(t, "openai/native-model", id)
				assert.JSONEq(t, `{"byok":{"openai":[{"apiKey":"dummy-key"}]}}`, string(gateway))
				assert.NotContains(t, options.ProviderOptions, "gateway")
				assert.Contains(t, options.ProviderOptions, "openai")
				return Selection{ID: id, Model: model}, nil
			}})
			require.NoError(t, err)
			r := validRequest(`{"prompt":[],"providerOptions":{"gateway":{"byok":{"openai":[{"apiKey":"dummy-key"}]}},"openai":{"store":false}}}`)
			r.Header.Set(HeaderModelID, "openai/native-model")
			r.Header.Set(HeaderStreaming, strconv.FormatBool(streaming))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, int32(1), selections.Load())
			generate, stream := model.invocationCounts()
			if streaming {
				assert.Equal(t, 1, stream)
				assert.Zero(t, generate)
			} else {
				assert.Equal(t, 1, generate)
				assert.Zero(t, stream)
			}
			assert.NotContains(t, model.receivedOptions().ProviderOptions, "gateway")
			assert.Contains(t, model.receivedOptions().ProviderOptions, "openai")
			assert.NotContains(t, w.Body.String(), "dummy-key")
		})
	}
}

func TestRequestSelection_RequestByteLimit(t *testing.T) {
	key := strings.Repeat("k", 70_000)
	body := `{"prompt":[],"providerOptions":{"gateway":{"byok":{"openai":[{"apiKey":"` + key + `"}]}}}}`
	for _, streaming := range []bool{false, true} {
		for _, delta := range []int{0, 1} {
			t.Run(fmt.Sprintf("streaming=%t/bytes+%d", streaming, delta), func(t *testing.T) {
				limits := testLimits()
				limits.RequestBytes = int64(len(body))
				model := &recordingModel{}
				var selections atomic.Int32
				h, err := New(Config{Limits: limits, Selector: func(_ context.Context, id string, _ provider.CallOptions, gateway json.RawMessage) (Selection, error) {
					selections.Add(1)
					assert.Contains(t, string(gateway), key)
					return Selection{ID: id, Model: model}, nil
				}})
				require.NoError(t, err)
				r := validRequest(body + strings.Repeat(" ", delta))
				r.Header.Set(HeaderModelID, "openai/native-model")
				r.Header.Set(HeaderStreaming, strconv.FormatBool(streaming))
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if delta == 0 {
					assert.Equal(t, http.StatusOK, w.Code)
					assert.Equal(t, int32(1), selections.Load())
					assert.Equal(t, 1, model.callCount())
				} else {
					assert.Equal(t, http.StatusBadRequest, w.Code)
					assert.Zero(t, selections.Load())
					assert.Zero(t, model.callCount())
				}
				assert.NotContains(t, w.Body.String(), key)
			})
		}
	}
}

func TestRequestSelection_Deadline(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run("late/"+strconv.FormatBool(streaming), func(t *testing.T) {
			limits := testLimits()
			limits.ModelDuration = 30 * time.Millisecond
			model := &recordingModel{}
			release, selected := make(chan struct{}), make(chan struct{})
			h, err := New(Config{Limits: limits, Selector: func(_ context.Context, id string, options provider.CallOptions, _ json.RawMessage) (Selection, error) {
				defer close(selected)
				<-release
				return Selection{ID: id, Model: model}, nil
			}})
			require.NoError(t, err)
			r := validRequest(`{"prompt":[]}`)
			r.Header.Set(HeaderStreaming, strconv.FormatBool(streaming))
			w := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { defer close(done); h.ServeHTTP(w, r) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				close(release)
				require.FailNow(t, "selection exceeded handler latency bound")
			}
			assert.Equal(t, http.StatusGatewayTimeout, w.Code)
			assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
			assert.Zero(t, model.callCount())
			close(release)
			<-selected
			assert.Zero(t, model.callCount())
		})
		t.Run("shared/"+strconv.FormatBool(streaming), func(t *testing.T) {
			limits := testLimits()
			limits.ModelDuration = 150 * time.Millisecond
			var selectionDeadline time.Time
			model := &recordingModel{
				generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
					deadline, ok := ctx.Deadline()
					assert.True(t, ok)
					assert.Equal(t, selectionDeadline, deadline)
					return validGenerateResult(), nil
				},
				stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
					deadline, ok := ctx.Deadline()
					assert.True(t, ok)
					assert.Equal(t, selectionDeadline, deadline)
					parts := make(chan provider.StreamPart, 1)
					parts <- finishPart()
					close(parts)
					return &provider.StreamResult{Stream: parts}, nil
				},
			}
			h, err := New(Config{Limits: limits, Selector: func(ctx context.Context, id string, options provider.CallOptions, _ json.RawMessage) (Selection, error) {
				var ok bool
				selectionDeadline, ok = ctx.Deadline()
				assert.True(t, ok)
				select {
				case <-time.After(30 * time.Millisecond):
				case <-ctx.Done():
					return Selection{}, ctx.Err()
				}
				assert.LessOrEqual(t, time.Until(selectionDeadline), limits.ModelDuration-20*time.Millisecond)
				return Selection{ID: id, Model: model}, nil
			}})
			require.NoError(t, err)
			r := validRequest(`{"prompt":[]}`)
			r.Header.Set(HeaderStreaming, strconv.FormatBool(streaming))
			ctx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
			defer cancel()
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r.WithContext(ctx))
			assert.Equal(t, http.StatusOK, w.Code)
			deadline, _ := ctx.Deadline()
			assert.True(t, selectionDeadline.Before(deadline))
		})
	}
}
