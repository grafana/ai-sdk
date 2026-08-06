package providerwirev4

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type handlerResolverFunc func(context.Context, string) (catalog.ResolvedModel, error)

func (f handlerResolverFunc) ResolveModel(ctx context.Context, modelID string) (catalog.ResolvedModel, error) {
	return f(ctx, modelID)
}

type handlerModel struct {
	generate func(context.Context, provider.CallOptions) (*provider.GenerateResult, error)
	stream   func(context.Context, provider.CallOptions) (*provider.StreamResult, error)
}

func (*handlerModel) SpecificationVersion() string               { return "v4" }
func (*handlerModel) Provider() string                           { return "test" }
func (*handlerModel) ModelID() string                            { return "backend-model" }
func (*handlerModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *handlerModel) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	if m.generate != nil {
		return m.generate(ctx, options)
	}
	return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, Warnings: []provider.Warning{}}, nil
}
func (m *handlerModel) DoStream(ctx context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
	if m.stream != nil {
		return m.stream(ctx, options)
	}
	parts := make(chan provider.StreamPart)
	close(parts)
	return &provider.StreamResult{Stream: parts}, nil
}

func TestNewHandler_ValidationAndDefaults(t *testing.T) {
	resolver := fixedHandlerResolver(&handlerModel{})
	handler, err := NewHandler(resolver)
	require.NoError(t, err)
	assert.Equal(t, DefaultTotalTimeout, handler.totalTimeout)
	assert.Equal(t, DefaultIdleTimeout, handler.idleTimeout)
	assert.Equal(t, DefaultMaxRequestBodyBytes, handler.maxRequestBodyBytes)
	assert.Equal(t, DefaultMaxUnaryResponseBytes, handler.maxUnaryResponseBytes)
	assert.Equal(t, DefaultMaxSSEEventBytes, handler.maxSSEEventBytes)

	var nilResolver *testNilResolver
	cases := []struct {
		name     string
		resolver catalog.ModelResolver
		option   Option
	}{
		{name: "nil resolver"},
		{name: "typed nil resolver", resolver: nilResolver},
		{name: "nil option", resolver: resolver},
		{name: "zero total", resolver: resolver, option: WithTotalTimeout(0)},
		{name: "zero idle", resolver: resolver, option: WithIdleTimeout(0)},
		{name: "zero request", resolver: resolver, option: WithMaxRequestBodyBytes(0)},
		{name: "zero unary", resolver: resolver, option: WithMaxUnaryResponseBytes(0)},
		{name: "zero event", resolver: resolver, option: WithMaxSSEEventBytes(0)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var options []Option
			if tc.name == "nil option" {
				options = []Option{nil}
			} else if tc.option != nil {
				options = []Option{tc.option}
			}
			_, err := NewHandler(tc.resolver, options...)
			require.Error(t, err)
		})
	}
}

type testNilResolver struct{}

func (*testNilResolver) ResolveModel(context.Context, string) (catalog.ResolvedModel, error) {
	return catalog.ResolvedModel{}, nil
}

func TestHandler_ValidationGatewayAndRawRejectionBypassResolution(t *testing.T) {
	var calls atomic.Int32
	resolver := handlerResolverFunc(func(context.Context, string) (catalog.ResolvedModel, error) {
		calls.Add(1)
		return catalog.ResolvedModel{ID: "model", Model: &handlerModel{}}, nil
	})
	handler, err := NewHandler(resolver, WithMaxRequestBodyBytes(256))
	require.NoError(t, err)

	cases := []struct {
		name       string
		request    *http.Request
		wantStatus int
	}{
		{name: "method", request: requestFor(t, http.MethodGet, "model", false, `{}`), wantStatus: 405},
		{name: "content type", request: requestFor(t, http.MethodPost, "model", false, `{"prompt":[]}`), wantStatus: 415},
		{name: "accept", request: strictRequest(t, "model", false, `{"prompt":[]}`), wantStatus: 406},
		{name: "zero quality accept", request: strictRequest(t, "model", false, `{"prompt":[]}`), wantStatus: 406},
		{name: "malformed", request: strictRequest(t, "model", false, `{`), wantStatus: 400},
		{name: "gateway", request: strictRequest(t, "model", false, `{"prompt":[],"providerOptions":{"gateway":{"models":["x"]}}}`), wantStatus: 400},
		{name: "raw", request: strictRequest(t, "model", false, `{"prompt":[],"includeRawChunks":true}`), wantStatus: 400},
		{name: "too large", request: strictRequest(t, "model", false, strings.Repeat("x", 257)), wantStatus: 413},
	}
	cases[1].request.Header.Del("Content-Type")
	cases[2].request.Header.Set("Accept", "text/plain")
	cases[3].request.Header.Set("Accept", "application/json;q=0")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, tc.request)
			assert.Equal(t, tc.wantStatus, recorder.Code)
			assert.NotContains(t, recorder.Body.String(), "models")
		})
	}
	assert.Zero(t, calls.Load())
}

func TestHandler_ResolvesExactModelWithRequestContextAndRemovedEmptyGateway(t *testing.T) {
	type requestContextKey struct{}
	requestContextValue := requestContextKey{}
	model := &handlerModel{generate: func(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
		assert.Equal(t, "value", ctx.Value(requestContextValue))
		assert.NotContains(t, options.ProviderOptions, "gateway")
		assert.Contains(t, options.ProviderOptions, "provider")
		return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, Warnings: []provider.Warning{}}, nil
	}}
	resolver := handlerResolverFunc(func(ctx context.Context, modelID string) (catalog.ResolvedModel, error) {
		assert.Equal(t, " public-alias ", modelID)
		assert.Equal(t, "value", ctx.Value(requestContextValue))
		return catalog.ResolvedModel{ID: "canonical", Model: model}, nil
	})
	handler, err := NewHandler(resolver)
	require.NoError(t, err)
	request := strictRequest(t, " public-alias ", false, `{"prompt":[],"providerOptions":{"provider":{"keep":true},"gateway":{}}}`)
	request = request.WithContext(context.WithValue(request.Context(), requestContextValue, "value"))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestHandler_UnaryPrivacyLimitsAndInternalDefects(t *testing.T) {
	timestamp := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	result := &provider.GenerateResult{
		Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "public"}},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
		Warnings:     []provider.Warning{{Type: provider.WarnOther, Message: "public warning"}},
		Request:      &provider.RequestMetadata{Body: json.RawMessage(`{"secret":"request"}`)},
		Response: &provider.GenerateResponse{
			ResponseMetadata: provider.ResponseMetadata{ID: "response", ModelID: "private-model", Provider: "private-provider", Timestamp: timestamp},
			Headers:          map[string]string{"X-Secret": "secret"}, Body: json.RawMessage(`{"secret":"body"}`),
		},
	}
	model := &handlerModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return result, nil }}
	handler, err := NewHandler(fixedHandlerResolver(model))
	require.NoError(t, err)
	recorder := serveStrict(t, handler, "model", false)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "public warning")
	assert.Contains(t, recorder.Body.String(), "response")
	for _, private := range []string{"secret", "private-model", "private-provider"} {
		assert.NotContains(t, recorder.Body.String(), private)
	}

	encoded, err := encodeGenerateResultJSON(sanitizeGenerateResult(result))
	require.NoError(t, err)
	limited, err := NewHandler(fixedHandlerResolver(model), WithMaxUnaryResponseBytes(int64(len(encoded)-1)))
	require.NoError(t, err)
	recorder = serveStrict(t, limited, "model", false)
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "public")

	for name, broken := range map[string]catalog.ModelResolver{
		"nil resolved model": handlerResolverFunc(func(context.Context, string) (catalog.ResolvedModel, error) {
			return catalog.ResolvedModel{ID: "model"}, nil
		}),
		"nil generate result": fixedHandlerResolver(&handlerModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return nil, nil }}),
	} {
		t.Run(name, func(t *testing.T) {
			h, err := NewHandler(broken)
			require.NoError(t, err)
			r := serveStrict(t, h, "model", false)
			assert.Equal(t, http.StatusInternalServerError, r.Code)
			assert.Contains(t, r.Body.String(), `"isRetryable":false`)
		})
	}
}

func TestHandler_ExactRequestAndUnaryLimits(t *testing.T) {
	body := `{"prompt":[]}`
	result := &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, Warnings: []provider.Warning{}}
	model := &handlerModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return result, nil
	}}
	encoded, err := encodeGenerateResultJSON(sanitizeGenerateResult(result))
	require.NoError(t, err)

	cases := []struct {
		name       string
		option     Option
		wantStatus int
	}{
		{name: "request exact", option: WithMaxRequestBodyBytes(int64(len(body))), wantStatus: http.StatusOK},
		{name: "request limit plus one", option: WithMaxRequestBodyBytes(int64(len(body) - 1)), wantStatus: http.StatusRequestEntityTooLarge},
		{name: "unary exact", option: WithMaxUnaryResponseBytes(int64(len(encoded))), wantStatus: http.StatusOK},
		{name: "unary limit plus one", option: WithMaxUnaryResponseBytes(int64(len(encoded) - 1)), wantStatus: http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler, err := NewHandler(fixedHandlerResolver(model), tc.option)
			require.NoError(t, err)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, strictRequest(t, "model", false, body))
			assert.Equal(t, tc.wantStatus, recorder.Code)
		})
	}
}

func TestHandler_SafeErrorMappings(t *testing.T) {
	private := errors.New("private cause")
	permanentRetryable := false
	transientRetryable := true
	cases := []struct {
		name       string
		resolver   catalog.ModelResolver
		modelError error
		status     int
		typeValue  string
		retryable  bool
	}{
		{name: "unknown model", resolver: handlerResolverFunc(func(context.Context, string) (catalog.ResolvedModel, error) {
			return catalog.ResolvedModel{}, errors.Join(catalog.ErrUnknownModel, private)
		}), status: 404, typeValue: "model_not_found"},
		{name: "rate limit", modelError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "private rate", StatusCode: 429, Cause: private}), status: 429, typeValue: "rate_limit_exceeded", retryable: true},
		{name: "permanent dependency", modelError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "private permanent", StatusCode: 401, IsRetryable: &permanentRetryable, Cause: private}), status: 424, typeValue: "failed_dependency"},
		{name: "transient dependency", modelError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "private transient", StatusCode: 503, IsRetryable: &transientRetryable, Cause: private}), status: 502, typeValue: "failed_dependency", retryable: true},
		{name: "generic invocation dependency", modelError: private, status: 424, typeValue: "failed_dependency"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolver := tc.resolver
			if resolver == nil {
				model := &handlerModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					return nil, tc.modelError
				}}
				resolver = fixedHandlerResolver(model)
			}
			handler, err := NewHandler(resolver)
			require.NoError(t, err)
			recorder := serveStrict(t, handler, "public-model", false)
			assert.Equal(t, tc.status, recorder.Code)
			assert.Contains(t, recorder.Body.String(), `"type":"`+tc.typeValue+`"`)
			assert.Contains(t, recorder.Body.String(), `"isRetryable":`+map[bool]string{true: "true", false: "false"}[tc.retryable])
			assert.NotContains(t, recorder.Body.String(), "private")
			if tc.name == "unknown model" {
				assert.Contains(t, recorder.Body.String(), `"modelId":"public-model"`)
			}
		})
	}
}

func TestHandler_TotalTimeoutAndCancellation(t *testing.T) {
	model := &handlerModel{generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
		<-ctx.Done()
		return nil, context.Cause(ctx)
	}}
	handler, err := NewHandler(fixedHandlerResolver(model), WithTotalTimeout(10*time.Millisecond))
	require.NoError(t, err)
	recorder := serveStrict(t, handler, "model", false)
	assert.Equal(t, http.StatusGatewayTimeout, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"isRetryable":true`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := strictRequest(t, "model", false, `{"prompt":[]}`).WithContext(ctx)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	assert.Equal(t, 499, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"isRetryable":false`)
}

func TestHandler_ResolverSuccessAfterContextTerminationDoesNotInvokeModel(t *testing.T) {
	t.Run("total timeout", func(t *testing.T) {
		var modelCalls atomic.Int32
		model := &handlerModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			modelCalls.Add(1)
			return &provider.GenerateResult{}, nil
		}}
		resolver := handlerResolverFunc(func(ctx context.Context, _ string) (catalog.ResolvedModel, error) {
			<-ctx.Done()
			return catalog.ResolvedModel{ID: "model", Model: model}, nil
		})
		handler, err := NewHandler(resolver, WithTotalTimeout(10*time.Millisecond))
		require.NoError(t, err)
		recorder := serveStrict(t, handler, "model", false)
		assert.Equal(t, http.StatusGatewayTimeout, recorder.Code)
		assert.Zero(t, modelCalls.Load())
	})

	t.Run("custom cancellation", func(t *testing.T) {
		var modelCalls atomic.Int32
		model := &handlerModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			modelCalls.Add(1)
			return &provider.GenerateResult{}, nil
		}}
		requestCtx, cancel := context.WithCancelCause(context.Background())
		resolver := handlerResolverFunc(func(ctx context.Context, _ string) (catalog.ResolvedModel, error) {
			cancel(errors.New("private cancellation"))
			<-ctx.Done()
			return catalog.ResolvedModel{ID: "model", Model: model}, nil
		})
		handler, err := NewHandler(resolver)
		require.NoError(t, err)
		request := strictRequest(t, "model", false, `{"prompt":[]}`).WithContext(requestCtx)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		assert.Equal(t, 499, recorder.Code)
		assert.Zero(t, modelCalls.Load())
		assert.NotContains(t, recorder.Body.String(), "private cancellation")
	})
}

func TestHandler_StreamOrderingPrivacyErrorsAndCleanEOF(t *testing.T) {
	private := provider.NewAPICallError(provider.APICallErrorOptions{Message: "private provider detail", URL: "https://private", StatusCode: 503, Data: json.RawMessage(`{"secret":true}`)})
	parts := make(chan provider.StreamPart, 8)
	parts <- provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "response", ModelID: "private-model", Provider: "private-provider", ResponseHeaders: map[string]string{"X-Secret": "secret"}}
	parts <- provider.StreamPart{Type: provider.PartTextStart, ID: "text"}
	parts <- provider.StreamPart{Type: provider.PartError}
	parts <- provider.StreamPart{Type: provider.PartError, APICallError: private}
	parts <- provider.StreamPart{Type: provider.PartRaw, RawValue: json.RawMessage(`{"secret":"raw"}`)}
	parts <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "after error"}
	parts <- provider.StreamPart{Type: provider.PartTextEnd, ID: "text"}
	parts <- provider.StreamPart{Type: provider.PartFinish, Usage: &provider.Usage{}, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
	close(parts)
	model := &handlerModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: parts, Request: &provider.RequestMetadata{Body: json.RawMessage(`{"secret":true}`)}, Response: &provider.ResponseHeaders{Headers: map[string]string{"X-Secret": "secret"}}}, nil
	}}
	handler, err := NewHandler(fixedHandlerResolver(model))
	require.NoError(t, err)
	recorder := serveStrict(t, handler, "model", true)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, MIMESSE, recorder.Header().Get("Content-Type"))
	assert.Empty(t, recorder.Header().Get("Connection"))
	assert.NotContains(t, recorder.Body.String(), "[DONE]")
	assert.NotContains(t, recorder.Body.String(), "private")
	assert.NotContains(t, recorder.Body.String(), `"raw"`)
	assert.Contains(t, recorder.Body.String(), "internal server error")
	assert.Contains(t, recorder.Body.String(), "upstream dependency failed")
	assert.Contains(t, recorder.Body.String(), "after error")
	assert.Less(t, strings.Index(recorder.Body.String(), "upstream dependency failed"), strings.Index(recorder.Body.String(), "after error"))
	assert.Contains(t, recorder.Body.String(), `"type":"finish"`)
}

func TestHandler_StreamIdleTotalAndEventLimit(t *testing.T) {
	newBlockedModel := func(contexts chan<- context.Context) *handlerModel {
		return &handlerModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			contexts <- ctx
			return &provider.StreamResult{Stream: make(chan provider.StreamPart)}, nil
		}}
	}
	t.Run("idle", func(t *testing.T) {
		contexts := make(chan context.Context, 1)
		handler, err := NewHandler(fixedHandlerResolver(newBlockedModel(contexts)), WithIdleTimeout(10*time.Millisecond), WithTotalTimeout(time.Second))
		require.NoError(t, err)
		recorder := serveStrict(t, handler, "model", true)
		assert.Contains(t, recorder.Body.String(), "request timed out")
		assert.ErrorIs(t, context.Cause(<-contexts), ErrIdleTimeout)
	})
	t.Run("total", func(t *testing.T) {
		contexts := make(chan context.Context, 1)
		handler, err := NewHandler(fixedHandlerResolver(newBlockedModel(contexts)), WithIdleTimeout(time.Second), WithTotalTimeout(10*time.Millisecond))
		require.NoError(t, err)
		recorder := serveStrict(t, handler, "model", true)
		assert.Contains(t, recorder.Body.String(), "request timed out")
		assert.ErrorIs(t, context.Cause(<-contexts), ErrTotalTimeout)
	})
	t.Run("event limit", func(t *testing.T) {
		parts := make(chan provider.StreamPart, 1)
		parts <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: strings.Repeat("x", 1024)}
		close(parts)
		model := &handlerModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: parts}, nil
		}}
		errorPart := provider.StreamPart{Type: provider.PartError, APICallError: apiCallErrorForFailure(internalFailure(errors.New("private")))}
		errorEvent, err := encodeSSEEventWithinLimit(errorPart, 1024)
		require.NoError(t, err)
		handler, err := NewHandler(fixedHandlerResolver(model), WithMaxSSEEventBytes(int64(len(errorEvent))))
		require.NoError(t, err)
		recorder := serveStrict(t, handler, "model", true)
		assert.NotContains(t, recorder.Body.String(), strings.Repeat("x", 1024))
		assert.Contains(t, recorder.Body.String(), "internal server error")
	})
}

func TestHandler_StreamContextWinsOverReadyProviderChannel(t *testing.T) {
	handler := &Handler{idleTimeout: time.Hour}
	cases := []struct {
		name  string
		cause error
		parts func() <-chan provider.StreamPart
	}{
		{name: "closed on timeout", cause: ErrTotalTimeout, parts: func() <-chan provider.StreamPart {
			parts := make(chan provider.StreamPart)
			close(parts)
			return parts
		}},
		{name: "buffered after cancel", cause: errors.New("private cancellation"), parts: func() <-chan provider.StreamPart {
			parts := make(chan provider.StreamPart, 1)
			parts <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "stale"}
			return parts
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(context.Background())
			cancel(tc.cause)
			part, open, err := handler.nextPart(ctx, tc.parts())
			assert.ErrorIs(t, err, tc.cause)
			assert.False(t, open)
			assert.Equal(t, provider.StreamPart{}, part)
		})
	}
}

func TestHandler_StreamSetupAndResponseControllerFailures(t *testing.T) {
	setup := &handlerModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: "private", StatusCode: 503})
	}}
	handler, err := NewHandler(fixedHandlerResolver(setup))
	require.NoError(t, err)
	recorder := serveStrict(t, handler, "model", true)
	assert.Equal(t, http.StatusBadGateway, recorder.Code)
	assert.Equal(t, MIMEJSON, recorder.Header().Get("Content-Type"))

	streamModel := func(contexts chan<- context.Context) *handlerModel {
		return &handlerModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			contexts <- ctx
			parts := make(chan provider.StreamPart, 1)
			parts <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "value"}
			close(parts)
			return &provider.StreamResult{Stream: parts}, nil
		}}
	}

	t.Run("wrapped writer flushes initial headers and event", func(t *testing.T) {
		contexts := make(chan context.Context, 1)
		handler, err := NewHandler(fixedHandlerResolver(streamModel(contexts)))
		require.NoError(t, err)
		base := newStreamTestWriter()
		handler.ServeHTTP(&unwrapHandlerResponseWriter{ResponseWriter: base}, strictRequest(t, "model", true, `{"prompt":[]}`))
		assert.Equal(t, 2, base.flushes)
		assert.Equal(t, 1, base.writes)
		assert.Contains(t, base.body.String(), `"delta":"value"`)
	})

	t.Run("initial flush failure cancels without writing", func(t *testing.T) {
		contexts := make(chan context.Context, 1)
		handler, err := NewHandler(fixedHandlerResolver(streamModel(contexts)))
		require.NoError(t, err)
		writer := newStreamTestWriter()
		writer.failFlushAt = 1
		handler.ServeHTTP(writer, strictRequest(t, "model", true, `{"prompt":[]}`))
		assertContextDone(t, <-contexts)
		assert.Equal(t, 1, writer.flushes)
		assert.Zero(t, writer.writes)
	})

	for _, tc := range []struct {
		name        string
		failWrite   bool
		failFlushAt int
		wantFlushes int
	}{
		{name: "post-commit write failure", failWrite: true, wantFlushes: 1},
		{name: "post-commit flush failure", failFlushAt: 2, wantFlushes: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			contexts := make(chan context.Context, 1)
			handler, err := NewHandler(fixedHandlerResolver(streamModel(contexts)))
			require.NoError(t, err)
			writer := newStreamTestWriter()
			writer.failWrite = tc.failWrite
			writer.failFlushAt = tc.failFlushAt
			handler.ServeHTTP(writer, strictRequest(t, "model", true, `{"prompt":[]}`))
			assertContextDone(t, <-contexts)
			assert.Equal(t, 1, writer.writes)
			assert.Equal(t, tc.wantFlushes, writer.flushes)
		})
	}
}

type unwrapHandlerResponseWriter struct{ http.ResponseWriter }

func (writer *unwrapHandlerResponseWriter) Unwrap() http.ResponseWriter { return writer.ResponseWriter }

type streamTestWriter struct {
	header      http.Header
	body        strings.Builder
	writes      int
	flushes     int
	failWrite   bool
	failFlushAt int
}

func newStreamTestWriter() *streamTestWriter         { return &streamTestWriter{header: make(http.Header)} }
func (writer *streamTestWriter) Header() http.Header { return writer.header }
func (*streamTestWriter) WriteHeader(int)            {}
func (writer *streamTestWriter) Write(data []byte) (int, error) {
	writer.writes++
	if writer.failWrite {
		return 0, errors.New("write failed")
	}
	return writer.body.Write(data)
}
func (writer *streamTestWriter) FlushError() error {
	writer.flushes++
	if writer.flushes == writer.failFlushAt {
		return errors.New("flush failed")
	}
	return nil
}

func assertContextDone(t *testing.T, ctx context.Context) {
	t.Helper()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("model context was not canceled")
	}
}

func fixedHandlerResolver(model provider.LanguageModel) catalog.ModelResolver {
	return handlerResolverFunc(func(context.Context, string) (catalog.ResolvedModel, error) {
		return catalog.ResolvedModel{ID: "canonical", Model: model}, nil
	})
}

func requestFor(t *testing.T, method, modelID string, streaming bool, body string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, PathLanguageModel, strings.NewReader(body))
	request.Header.Set(HeaderModelID, modelID)
	request.Header.Set(HeaderSpecVersion, SpecVersionV4)
	request.Header.Set(HeaderStreaming, map[bool]string{true: "true", false: "false"}[streaming])
	return request
}

func strictRequest(t *testing.T, modelID string, streaming bool, body string) *http.Request {
	t.Helper()
	request := requestFor(t, http.MethodPost, modelID, streaming, body)
	request.Header.Set("Content-Type", MIMEJSON)
	request.Header.Set("Accept", map[bool]string{true: MIMESSE, false: MIMEJSON}[streaming])
	return request
}

func serveStrict(t *testing.T, handler *Handler, modelID string, streaming bool) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, strictRequest(t, modelID, streaming, `{"prompt":[]}`))
	return recorder
}
