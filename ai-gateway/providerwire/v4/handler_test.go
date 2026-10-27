package v4

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
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

func testLimits() execution.Limits {
	return execution.Limits{
		StreamIdleDuration:  time.Second,
		StreamDrainDuration: time.Second,
	}
}

func newTestHandler(t *testing.T, limits execution.Limits) *handler {
	t.Helper()
	created, err := New(Config{Selector: execution.CatalogSelector(&resolverStub{}), Limits: limits})
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
		created, err := New(Config{Selector: execution.CatalogSelector(&resolverStub{}), Limits: limits})
		require.NoError(t, err)
		h := created.(*handler)
		require.NotNil(t, h.requestSchema)
		limits.StreamIdleDuration = time.Hour
		assert.Equal(t, time.Second, h.limits.StreamIdleDuration)
	})

	t.Run("nil resolver", func(t *testing.T) {
		_, err := New(Config{Limits: testLimits()})
		require.Error(t, err)
		var resolver *resolverStub
		_, err = New(Config{Selector: execution.CatalogSelector(resolver), Limits: testLimits()})
		require.Error(t, err)
	})

	t.Run("invalid limits", func(t *testing.T) {
		tests := []struct {
			name   string
			mutate func(*execution.Limits)
		}{
			{name: "idle negative", mutate: func(l *execution.Limits) { l.StreamIdleDuration = -time.Second }},
			{name: "drain negative", mutate: func(l *execution.Limits) { l.StreamDrainDuration = -time.Second }},
			{name: "idle zero", mutate: func(l *execution.Limits) { l.StreamIdleDuration = 0 }},
			{name: "drain zero", mutate: func(l *execution.Limits) { l.StreamDrainDuration = 0 }},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				limits := testLimits()
				tc.mutate(&limits)
				_, err := New(Config{Selector: execution.CatalogSelector(&resolverStub{}), Limits: limits})
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
	created, err := New(Config{Selector: execution.CatalogSelector(resolver), Limits: testLimits()})
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
	t.Run("host byte boundary and close", func(t *testing.T) {
		base := `{"prompt":[]}`
		limit := len(base) + 1
		h := newTestHandler(t, testLimits())
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
				_, failure := h.validateRequest(withBodyLimit(req, limit))
				assert.True(t, body.closed)
				if tc.invalid {
					require.NotNil(t, failure)
					assert.Equal(t, safeErrorCategory(0), failure.safe.category)
				} else {
					assert.Nil(t, failure)
				}
				assert.LessOrEqual(t, body.read, limit+1)
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

func withBodyLimit(r *http.Request, limit int) *http.Request {
	r.Body = http.MaxBytesReader(nil, r.Body, int64(limit))
	return r
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
			h, err := New(Config{Limits: testLimits(), Selector: func(_ context.Context, id string, options provider.CallOptions, gateway json.RawMessage) (execution.Selection, error) {
				selections.Add(1)
				assert.Equal(t, "openai/native-model", id)
				assert.JSONEq(t, `{"byok":{"openai":[{"apiKey":"dummy-key"}]}}`, string(gateway))
				assert.NotContains(t, options.ProviderOptions, "gateway")
				assert.Contains(t, options.ProviderOptions, "openai")
				return execution.Selection{ID: id, Model: model}, nil
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

func TestRequestSelection_ValidationErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		message string
	}{
		{name: "accounts", err: execution.ErrInvalidBYOK, message: "providerOptions.gateway.byok requires supported provider arrays with valid accounts"},
		{name: "selector", err: execution.ErrInvalidBYOKSelector, message: "BYOK requires a supported provider/model selector"},
		{name: "controls", err: execution.ErrUnsupportedGatewayControl, message: "unsupported gateway control; only gateway.byok is supported"},
	} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/streaming=%t", tc.name, streaming), func(t *testing.T) {
				model := &recordingModel{}
				h, err := New(Config{Limits: testLimits(), Selector: func(_ context.Context, id string, _ provider.CallOptions, _ json.RawMessage) (execution.Selection, error) {
					return execution.Selection{ID: id, Model: model}, fmt.Errorf("private-key private-organization https://private.invalid: %w", tc.err)
				}})
				require.NoError(t, err)
				r := validRequest(`{"prompt":[],"providerOptions":{"gateway":{"byok":{"openai":[{"apiKey":"private-key"}]}}}}`)
				r.Header.Set(HeaderModelID, "openai/native-model")
				r.Header.Set(HeaderStreaming, strconv.FormatBool(streaming))
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				assert.Equal(t, http.StatusBadRequest, w.Code)
				assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
				assert.JSONEq(t, fmt.Sprintf(`{"error":{"message":%q,"type":"invalid_request_error","param":null,"code":"invalid_request"}}`, tc.message), w.Body.String())
				assert.Zero(t, model.callCount())
			})
		}
	}
}

func TestRequestSelection_RequestByteLimit(t *testing.T) {
	key := strings.Repeat("k", 70_000)
	body := `{"prompt":[],"providerOptions":{"gateway":{"byok":{"openai":[{"apiKey":"` + key + `"}]}}}}`
	for _, streaming := range []bool{false, true} {
		for _, delta := range []int{0, 1} {
			t.Run(fmt.Sprintf("streaming=%t/bytes+%d", streaming, delta), func(t *testing.T) {
				model := &recordingModel{}
				var selections atomic.Int32
				h, err := New(Config{Limits: testLimits(), Selector: func(_ context.Context, id string, _ provider.CallOptions, gateway json.RawMessage) (execution.Selection, error) {
					selections.Add(1)
					assert.Contains(t, string(gateway), key)
					return execution.Selection{ID: id, Model: model}, nil
				}})
				require.NoError(t, err)
				r := withBodyLimit(validRequest(body+strings.Repeat(" ", delta)), len(body))
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

func withRequestDeadline(t *testing.T, r *http.Request, duration time.Duration) *http.Request {
	t.Helper()
	ctx, cancel := context.WithTimeout(r.Context(), duration)
	t.Cleanup(cancel)
	return r.WithContext(ctx)
}

func TestRequestSelection_Deadline(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, tc := range []struct {
			name   string
			end    func(*testing.T, *http.Request) *http.Request
			status int
		}{
			{name: "late/deadline", end: func(t *testing.T, r *http.Request) *http.Request {
				return withRequestDeadline(t, r, 30*time.Millisecond)
			}, status: http.StatusGatewayTimeout},
			{name: "late/canceled", end: func(t *testing.T, r *http.Request) *http.Request {
				ctx, cancel := context.WithCancel(r.Context())
				time.AfterFunc(30*time.Millisecond, cancel)
				t.Cleanup(cancel)
				return r.WithContext(ctx)
			}, status: statusClientClosedRequest},
		} {
			t.Run(tc.name+"/"+strconv.FormatBool(streaming), func(t *testing.T) {
				model := &recordingModel{}
				release, selected := make(chan struct{}), make(chan struct{})
				h, err := New(Config{Limits: testLimits(), Selector: func(_ context.Context, id string, options provider.CallOptions, _ json.RawMessage) (execution.Selection, error) {
					defer close(selected)
					<-release
					return execution.Selection{ID: id, Model: model}, nil
				}})
				require.NoError(t, err)
				r := validRequest(`{"prompt":[]}`)
				r.Header.Set(HeaderStreaming, strconv.FormatBool(streaming))
				r = tc.end(t, r)
				w := httptest.NewRecorder()
				done := make(chan struct{})
				go func() { defer close(done); h.ServeHTTP(w, r) }()
				select {
				case <-done:
				case <-time.After(time.Second):
					close(release)
					require.FailNow(t, "selection exceeded handler latency bound")
				}
				assert.Equal(t, tc.status, w.Code)
				assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
				assert.Zero(t, model.callCount())
				close(release)
				<-selected
				assert.Zero(t, model.callCount())
			})
		}
		t.Run("shared/"+strconv.FormatBool(streaming), func(t *testing.T) {
			const requestDeadline = 500 * time.Millisecond
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
			h, err := New(Config{Limits: testLimits(), Selector: func(ctx context.Context, id string, options provider.CallOptions, _ json.RawMessage) (execution.Selection, error) {
				var ok bool
				selectionDeadline, ok = ctx.Deadline()
				assert.True(t, ok)
				select {
				case <-time.After(30 * time.Millisecond):
				case <-ctx.Done():
					return execution.Selection{}, ctx.Err()
				}
				assert.LessOrEqual(t, time.Until(selectionDeadline), requestDeadline-20*time.Millisecond)
				return execution.CatalogSelector(&recordingResolver{resolved: catalog.ResolvedModel{ID: id, Model: model, Candidates: []catalog.ConfiguredCandidate{{ProviderInstance: "configured", Provider: model.Provider(), ModelID: model.ModelID()}}}})(ctx, id, options, nil)
			}})
			require.NoError(t, err)
			r := validRequest(`{"prompt":[]}`)
			r.Header.Set(HeaderStreaming, strconv.FormatBool(streaming))
			r = withRequestDeadline(t, r, requestDeadline)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Contains(t, w.Body.String(), `"providerInstance":"configured"`)
			assert.Contains(t, w.Body.String(), `"outcome":"selected"`)
			deadline, _ := r.Context().Deadline()
			assert.Equal(t, deadline, selectionDeadline)
		})
		t.Run("no deadline/"+strconv.FormatBool(streaming), func(t *testing.T) {
			deadlines := make(chan bool, 2)
			model := &recordingModel{
				generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
					_, ok := ctx.Deadline()
					deadlines <- ok
					return validGenerateResult(), nil
				},
				stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
					_, ok := ctx.Deadline()
					deadlines <- ok
					parts := make(chan provider.StreamPart, 1)
					parts <- finishPart()
					close(parts)
					return &provider.StreamResult{Stream: parts}, nil
				},
			}
			h, err := New(Config{Limits: testLimits(), Selector: func(ctx context.Context, id string, _ provider.CallOptions, _ json.RawMessage) (execution.Selection, error) {
				_, ok := ctx.Deadline()
				deadlines <- ok
				return execution.Selection{ID: id, Model: model}, nil
			}})
			require.NoError(t, err)
			r := validRequest(`{"prompt":[]}`)
			r.Header.Set(HeaderStreaming, strconv.FormatBool(streaming))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			assert.Equal(t, http.StatusOK, w.Code)
			assert.False(t, <-deadlines, "selection deadline")
			assert.False(t, <-deadlines, "model deadline")
		})
	}
}
