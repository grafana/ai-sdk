package providerwirev4

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/gateway/catalog"
	"github.com/grafana/ai-sdk/gateway/failure"
	gatewayruntime "github.com/grafana/ai-sdk/gateway/runtime"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type handlerModel struct {
	generate func(context.Context, provider.CallOptions) (*provider.GenerateResult, error)
	stream   func(context.Context, provider.CallOptions) (*provider.StreamResult, error)
}

func (*handlerModel) SpecificationVersion() string               { return "v4" }
func (*handlerModel) Provider() string                           { return "provider" }
func (*handlerModel) ModelID() string                            { return "backend-model" }
func (*handlerModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (model *handlerModel) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	if model.generate == nil {
		return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, Warnings: []provider.Warning{}}, nil
	}
	return model.generate(ctx, options)
}
func (model *handlerModel) DoStream(ctx context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
	if model.stream == nil {
		parts := make(chan provider.StreamPart)
		close(parts)
		return &provider.StreamResult{Stream: parts}, nil
	}
	return model.stream(ctx, options)
}

func TestNewHandler_ValidationAndDefaults(t *testing.T) {
	runtime := newHandlerRuntime(t, &handlerModel{}, nil)
	handler, err := NewHandler(runtime)
	require.NoError(t, err)
	assert.Equal(t, DefaultMaxRequestBodyBytes, handler.maxRequestBodyBytes)
	assert.Equal(t, DefaultMaxUnaryResponseBytes, handler.maxUnaryResponseBytes)
	assert.Equal(t, DefaultMaxSSEEventBytes, handler.maxSSEEventBytes)
	assert.Equal(t, DefaultIdleTimeout, handler.idleTimeout)

	cases := []struct {
		name    string
		runtime *gatewayruntime.Runtime
		option  Option
	}{
		{name: "nil runtime"},
		{name: "nil option", runtime: runtime},
		{name: "zero request limit", runtime: runtime, option: WithMaxRequestBodyBytes(0)},
		{name: "zero unary limit", runtime: runtime, option: WithMaxUnaryResponseBytes(0)},
		{name: "zero event limit", runtime: runtime, option: WithMaxSSEEventBytes(0)},
		{name: "zero idle", runtime: runtime, option: WithIdleTimeout(0)},
		{name: "nil extractor", runtime: runtime, option: WithMetadataExtractor(nil)},
		{name: "nil ID generator", runtime: runtime, option: WithRequestIDGenerator(nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			options := []Option{}
			if tc.name == "nil option" {
				options = append(options, nil)
			} else if tc.option != nil {
				options = append(options, tc.option)
			}
			got, err := NewHandler(tc.runtime, options...)
			require.Error(t, err)
			assert.Nil(t, got)
		})
	}
}

func TestHandler_AdapterLocalValidationBypassesRuntime(t *testing.T) {
	var resolverCalls atomic.Int32
	runtime, err := gatewayruntime.New(gatewayruntime.ModelResolverFunc(func(context.Context, gatewayruntime.GatewayCall) (catalog.ResolvedModel, error) {
		resolverCalls.Add(1)
		return catalog.ResolvedModel{ID: "canonical", Model: &handlerModel{}}, nil
	}))
	require.NoError(t, err)
	handler, err := NewHandler(runtime, WithMaxRequestBodyBytes(2))
	require.NoError(t, err)

	cases := []struct {
		name    string
		mutate  func(*http.Request)
		body    []byte
		status  int
		message string
	}{
		{name: "method", mutate: func(request *http.Request) { request.Method = http.MethodGet }, status: 405, message: "method not allowed"},
		{name: "missing model", mutate: func(request *http.Request) { request.Header.Del(HeaderModelID) }, status: 400, message: "model ID is required"},
		{name: "wrong version", mutate: func(request *http.Request) { request.Header.Set(HeaderSpecVersion, "3") }, status: 400, message: "unsupported specification version"},
		{name: "invalid streaming", mutate: func(request *http.Request) { request.Header.Set(HeaderStreaming, "TRUE") }, status: 400, message: "streaming header must be true or false"},
		{name: "missing content type", mutate: func(request *http.Request) { request.Header.Del("Content-Type") }, status: 415, message: "Content-Type must be application/json"},
		{name: "wrong content type", mutate: func(request *http.Request) { request.Header.Set("Content-Type", "text/plain") }, status: 415, message: "Content-Type must be application/json"},
		{name: "unary unacceptable", mutate: func(request *http.Request) { request.Header.Set("Accept", "application/json;q=0") }, status: 406, message: "requested response media type is not acceptable"},
		{name: "stream unacceptable", mutate: func(request *http.Request) {
			request.Header.Set(HeaderStreaming, "true")
			request.Header.Set("Accept", "text/event-stream;q=0")
		}, status: 406, message: "requested response media type is not acceptable"},
		{name: "oversized", body: []byte("{} "), status: 413, message: "request body too large"},
		{name: "malformed DTO", body: []byte("{}"), status: 400, message: "invalid LanguageModelV4 request"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := validStrictRequest(t, false, tc.body)
			if tc.mutate != nil {
				tc.mutate(request)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			assert.Equal(t, tc.status, recorder.Code)
			apiErr, err := DecodeErrorResponse(recorder.Body.Bytes(), recorder.Code)
			require.NoError(t, err)
			assert.False(t, apiErr.IsRetryable)
			assert.Equal(t, tc.message, apiErr.Message)
			var envelope gatewayErrorEnvelopeDTO
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
			assert.Equal(t, "invalid_request_error", envelope.Error.Type)
			assert.JSONEq(t, "null", string(envelope.Error.Param))
		})
	}
	assert.Zero(t, resolverCalls.Load())
}

func TestHandler_RejectsMalformedNestedGatewayControlsBeforeResolution(t *testing.T) {
	var policyCalls atomic.Int32
	var resolverCalls atomic.Int32
	runtime, err := gatewayruntime.New(gatewayruntime.ModelResolverFunc(func(context.Context, gatewayruntime.GatewayCall) (catalog.ResolvedModel, error) {
		resolverCalls.Add(1)
		return catalog.ResolvedModel{ID: "canonical", Model: &handlerModel{}}, nil
	}), gatewayruntime.WithCallPolicies(gatewayruntime.CallPolicyFunc(func(_ context.Context, call gatewayruntime.GatewayCall) (gatewayruntime.GatewayCall, error) {
		policyCalls.Add(1)
		return call, nil
	})))
	require.NoError(t, err)
	handler, err := NewHandler(runtime)
	require.NoError(t, err)

	gateways := []string{
		`{"byok":{"openai":null}}`,
		`{"byok":{"openai":[null]}}`,
		`{"providerTimeouts":{"byok":null}}`,
		`{"providerTimeouts":{"future":{"provider":100}}}`,
		`{"models":[null]}`,
		`{"only":[1]}`,
		`{"order":{}}`,
		`{"tags":"prod"}`,
		`{"has":[{}]}`,
	}
	for i, gateway := range gateways {
		t.Run(fmt.Sprintf("case-%d", i), func(t *testing.T) {
			body := []byte(`{"prompt":[],"providerOptions":{"gateway":` + gateway + `}}`)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, validStrictRequest(t, false, body))
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			apiErr, err := DecodeErrorResponse(recorder.Body.Bytes(), recorder.Code)
			require.NoError(t, err)
			assert.False(t, apiErr.IsRetryable)
		})
	}
	assert.Zero(t, policyCalls.Load())
	assert.Zero(t, resolverCalls.Load())
}

func TestHandler_StandardsConsistentNegotiation(t *testing.T) {
	handler, err := NewHandler(newHandlerRuntime(t, &handlerModel{}, nil))
	require.NoError(t, err)
	cases := []struct {
		name      string
		streaming bool
		accept    string
		status    int
	}{
		{name: "missing", status: 200},
		{name: "unary wildcard", accept: "application/*;q=0.5", status: 200},
		{name: "all wildcard", accept: "*/*", status: 200},
		{name: "stream wildcard", streaming: true, accept: "text/*;q=0.5", status: 200},
		{name: "exact unary exclusion overrides wildcard", accept: "application/json;q=0, */*;q=1", status: 406},
		{name: "exact stream exclusion overrides wildcard", streaming: true, accept: "text/event-stream;q=0, */*;q=1", status: 406},
		{name: "type exclusion overrides all wildcard", accept: "application/*;q=0, */*;q=1", status: 406},
		{name: "empty entries", accept: ", ;q=0", status: 406},
		{name: "malformed", accept: "application/json;q=nope", status: 406},
		{name: "unsupported unary parameter", accept: "application/json;profile=unsupported", status: 406},
		{name: "unsupported stream parameter", streaming: true, accept: "text/event-stream;charset=utf-8", status: 406},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := validStrictRequest(t, tc.streaming, nil)
			if tc.accept != "" {
				request.Header.Set("Accept", tc.accept)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			assert.Equal(t, tc.status, recorder.Code)
		})
	}
}

func TestHandler_StreamSetupErrorsRetainRuntimeClassification(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		retryable bool
		want      int
	}{
		{name: "rate limit", status: http.StatusTooManyRequests, retryable: true, want: http.StatusTooManyRequests},
		{name: "permanent dependency", status: http.StatusBadRequest, want: http.StatusFailedDependency},
		{name: "transient dependency", status: http.StatusServiceUnavailable, retryable: true, want: http.StatusBadGateway},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := &handlerModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return nil, provider.NewAPICallError(provider.APICallErrorOptions{
					Message: "private setup failure", StatusCode: tc.status, IsRetryable: &tc.retryable,
				})
			}}
			handler, err := NewHandler(newHandlerRuntime(t, model, nil))
			require.NoError(t, err)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, validStrictRequest(t, true, nil))
			assert.Equal(t, tc.want, recorder.Code)
			apiErr, err := DecodeErrorResponse(recorder.Body.Bytes(), recorder.Code)
			require.NoError(t, err)
			assert.Equal(t, tc.retryable, apiErr.IsRetryable)
			assert.NotContains(t, recorder.Body.String(), "private")
		})
	}
}

func TestHandler_GatewayCallMetadataPolicyResolutionAndMiddlewareOrder(t *testing.T) {
	var sequence []string
	var sourceAttributes = map[string]string{"tenant": "trusted"}
	policy := gatewayruntime.CallPolicyFunc(func(_ context.Context, call gatewayruntime.GatewayCall) (gatewayruntime.GatewayCall, error) {
		sequence = append(sequence, "policy")
		assert.Equal(t, "request-fixed", call.CallMetadata.RequestID)
		assert.Equal(t, " public-alias ", call.RequestedModelID)
		assert.Equal(t, "trusted", call.CallMetadata.AuthenticatedAttributes["tenant"])
		assert.NotEqual(t, "caller", call.CallMetadata.AuthenticatedAttributes["tenant"])
		assert.NotContains(t, call.CallOptions.ProviderOptions, "gateway")
		assert.Equal(t, []string{"fallback"}, call.GatewayOptions.Models)
		return call, nil
	})
	model := &handlerModel{generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
		sequence = append(sequence, "model")
		requestID, ok := gatewayruntime.RequestIDFromContext(ctx)
		require.True(t, ok)
		assert.Equal(t, "request-fixed", requestID)
		return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, Warnings: []provider.Warning{}}, nil
	}}
	resolver := gatewayruntime.ModelResolverFunc(func(_ context.Context, call gatewayruntime.GatewayCall) (catalog.ResolvedModel, error) {
		sequence = append(sequence, "resolver")
		assert.Equal(t, " public-alias ", call.RequestedModelID)
		assert.Equal(t, []string{"fallback"}, call.GatewayOptions.Models)
		return catalog.ResolvedModel{ID: "canonical", Model: model}, nil
	})
	mw := middleware.Middleware{TransformParams: func(_ context.Context, input middleware.TransformParamsInput) (provider.CallOptions, error) {
		sequence = append(sequence, "middleware")
		return input.Params, nil
	}}
	runtime, err := gatewayruntime.New(resolver, gatewayruntime.WithCallPolicies(policy), gatewayruntime.WithMiddleware(mw))
	require.NoError(t, err)
	handler, err := NewHandler(runtime,
		WithMetadataExtractor(func(*http.Request) (gatewayruntime.CallMetadata, error) {
			return gatewayruntime.CallMetadata{AuthenticatedAttributes: sourceAttributes}, nil
		}),
		WithRequestIDGenerator(func() (string, error) { return "request-fixed", nil }),
	)
	require.NoError(t, err)

	body := []byte(`{"prompt":[],"headers":{"X-Tenant":"caller"},"providerOptions":{"gateway":{"models":["fallback"]}}}`)
	request := validStrictRequest(t, false, body)
	request.Header.Set(HeaderModelID, " public-alias ")
	request.Header.Set("X-Tenant", "caller")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, []string{"policy", "resolver", "middleware", "model"}, sequence)
	assert.Equal(t, map[string]string{"tenant": "trusted"}, sourceAttributes)
}

func TestHandler_UnaryPrivacyAndLimits(t *testing.T) {
	timestamp := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	result := &provider.GenerateResult{
		Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "hello"}},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
		Warnings:     []provider.Warning{{Type: provider.WarnOther, Message: "public"}},
		Request:      &provider.RequestMetadata{Body: json.RawMessage(`{"secret":"request"}`)},
		Response:     &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: "response", ModelID: "backend-model", Provider: "backend", Timestamp: timestamp}, Headers: map[string]string{"X-Secret": "header"}, Body: json.RawMessage(`{"secret":"body"}`)},
	}
	model := &handlerModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return result, nil }}
	runtime := newHandlerRuntime(t, model, nil)
	handler, err := NewHandler(runtime)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, validStrictRequest(t, false, nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	decoded, err := DecodeGenerateResult(recorder.Body.Bytes())
	require.NoError(t, err)
	assert.Nil(t, decoded.Request)
	require.NotNil(t, decoded.Response)
	assert.Equal(t, "response", decoded.Response.ID)
	assert.Equal(t, timestamp, decoded.Response.Timestamp)
	assert.Empty(t, decoded.Response.ModelID)
	assert.Empty(t, decoded.Response.Provider)
	assert.Empty(t, decoded.Response.Headers)
	assert.Empty(t, decoded.Response.Body)
	assert.Equal(t, result.Warnings, decoded.Warnings)
	assert.NotContains(t, recorder.Body.String(), "secret")
	assert.NotContains(t, recorder.Body.String(), "backend-model")

	sanitized, err := EncodeGenerateResult(sanitizeGenerateResult(result))
	require.NoError(t, err)
	limited, err := NewHandler(runtime, WithMaxUnaryResponseBytes(int64(len(sanitized)-1)))
	require.NoError(t, err)
	recorder = httptest.NewRecorder()
	limited.ServeHTTP(recorder, validStrictRequest(t, false, nil))
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "hello")
}

func TestHandler_ExactTransportLimits(t *testing.T) {
	body, err := EncodeCallOptions(provider.CallOptions{Prompt: []provider.Message{}})
	require.NoError(t, err)
	model := &handlerModel{}
	runtime := newHandlerRuntime(t, model, nil)

	exactRequest, err := NewHandler(runtime, WithMaxRequestBodyBytes(int64(len(body))))
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	exactRequest.ServeHTTP(recorder, validStrictRequest(t, false, body))
	assert.Equal(t, http.StatusOK, recorder.Code)

	overRequest, err := NewHandler(runtime, WithMaxRequestBodyBytes(int64(len(body)-1)))
	require.NoError(t, err)
	recorder = httptest.NewRecorder()
	overRequest.ServeHTTP(recorder, validStrictRequest(t, false, body))
	assert.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)

	result := &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, Warnings: []provider.Warning{}}
	encodedResult, err := EncodeGenerateResult(result)
	require.NoError(t, err)
	exactUnary, err := NewHandler(runtime, WithMaxUnaryResponseBytes(int64(len(encodedResult))))
	require.NoError(t, err)
	recorder = httptest.NewRecorder()
	exactUnary.ServeHTTP(recorder, validStrictRequest(t, false, body))
	assert.Equal(t, http.StatusOK, recorder.Code)

	part := provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "x"}
	event, err := EncodeSSEEventWithinLimit(part, 1024)
	require.NoError(t, err)
	streamModel := &handlerModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		parts := make(chan provider.StreamPart, 1)
		parts <- part
		close(parts)
		return &provider.StreamResult{Stream: parts}, nil
	}}
	exactEvent, err := NewHandler(newHandlerRuntime(t, streamModel, nil), WithMaxSSEEventBytes(int64(len(event))))
	require.NoError(t, err)
	recorder = httptest.NewRecorder()
	exactEvent.ServeHTTP(recorder, validStrictRequest(t, true, body))
	assert.Equal(t, event, recorder.Body.Bytes())
}

func TestHandler_RuntimeFailuresAreRedacted(t *testing.T) {
	private := provider.NewAPICallError(provider.APICallErrorOptions{
		Message: "private message", URL: "https://backend", StatusCode: 401,
		RequestBodyValues: json.RawMessage(`{"secret":true}`), ResponseBody: "private body",
	})
	model := &handlerModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return nil, private }}
	handler, err := NewHandler(newHandlerRuntime(t, model, nil))
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, validStrictRequest(t, false, nil))
	assert.Equal(t, http.StatusFailedDependency, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "private")
	assert.NotContains(t, recorder.Body.String(), "backend")
}

func TestHandler_NilStreamPartErrorFailsClosedAndContinues(t *testing.T) {
	model := &handlerModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		parts := make(chan provider.StreamPart, 2)
		parts <- provider.StreamPart{Type: provider.PartError}
		parts <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "after"}
		close(parts)
		return &provider.StreamResult{Stream: parts}, nil
	}}
	handler, err := NewHandler(newHandlerRuntime(t, model, nil))
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, validStrictRequest(t, true, nil))
	require.Equal(t, http.StatusOK, recorder.Code)

	reader, err := NewSSEReader(bytes.NewReader(recorder.Body.Bytes()), DefaultMaxSSEEventBytes)
	require.NoError(t, err)
	first, err := reader.Next()
	require.NoError(t, err)
	require.NotNil(t, first.APICallError)
	assert.Equal(t, provider.PartError, first.Type)
	assert.Equal(t, http.StatusInternalServerError, first.APICallError.StatusCode)
	assert.False(t, first.APICallError.IsRetryable)
	second, err := reader.Next()
	require.NoError(t, err)
	assert.Equal(t, provider.PartTextDelta, second.Type)
	assert.Equal(t, "after", second.Delta)
}

func TestHandler_StreamOrderingPrivacyRawFilteringAndCleanEOF(t *testing.T) {
	usage := provider.Usage{}
	finish := provider.FinishReason{Unified: provider.FinishReasonStop}
	parts := []provider.StreamPart{
		{Type: provider.PartResponseMeta, ResponseID: "response", ModelID: "backend-model", Provider: "backend", ResponseHeaders: map[string]string{"X-Secret": "secret"}},
		{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "private", URL: "https://backend", StatusCode: 429})},
		{Type: provider.PartRaw, RawValue: json.RawMessage(`{"raw":true}`)},
		{Type: provider.PartTextStart, ID: "text"},
		{Type: provider.PartTextDelta, ID: "text", Delta: "after"},
		{Type: provider.PartTextEnd, ID: "text"},
		{Type: provider.PartFinish, Usage: &usage, FinishReason: &finish},
	}
	model := &handlerModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		stream := make(chan provider.StreamPart, len(parts))
		for _, part := range parts {
			stream <- part
		}
		close(stream)
		return &provider.StreamResult{Stream: stream, Request: &provider.RequestMetadata{Body: json.RawMessage(`{"secret":true}`)}, Response: &provider.ResponseHeaders{Headers: map[string]string{"X-Secret": "secret"}}}, nil
	}}
	handler, err := NewHandler(newHandlerRuntime(t, model, nil))
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, validStrictRequest(t, true, nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, MIMESSE, recorder.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache, no-transform", recorder.Header().Get("Cache-Control"))
	assert.Equal(t, "no", recorder.Header().Get("X-Accel-Buffering"))
	assert.Empty(t, recorder.Header().Get("Connection"))

	reader, err := NewSSEReader(bytes.NewReader(recorder.Body.Bytes()), DefaultMaxSSEEventBytes)
	require.NoError(t, err)
	var got []provider.StreamPart
	for {
		part, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		got = append(got, part)
	}
	require.Len(t, got, 6)
	assert.Equal(t, []provider.StreamPartType{provider.PartResponseMeta, provider.PartError, provider.PartTextStart, provider.PartTextDelta, provider.PartTextEnd, provider.PartFinish}, streamTypes(got))
	assert.Empty(t, got[0].ModelID)
	assert.Empty(t, got[0].Provider)
	assert.Empty(t, got[0].ResponseHeaders)
	assert.Equal(t, "rate limit exceeded", got[1].APICallError.Message)
	assert.NotContains(t, recorder.Body.String(), "private")
	assert.NotContains(t, recorder.Body.String(), "backend")
	assert.NotContains(t, recorder.Body.String(), "[DONE]")
}

func TestHandler_StreamRawChunkRequiresRequestAndPolicyAcceptance(t *testing.T) {
	stream := make(chan provider.StreamPart, 1)
	stream <- provider.StreamPart{Type: provider.PartRaw, RawValue: json.RawMessage(`{"raw":true}`)}
	close(stream)
	model := &handlerModel{stream: func(_ context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
		assert.True(t, options.IncludeRawChunks)
		return &provider.StreamResult{Stream: stream}, nil
	}}
	policy := gatewayruntime.CallPolicyFunc(func(_ context.Context, call gatewayruntime.GatewayCall) (gatewayruntime.GatewayCall, error) {
		if !call.CallOptions.IncludeRawChunks {
			return gatewayruntime.GatewayCall{}, failure.ErrForbidden
		}
		return call, nil
	})
	runtime := newHandlerRuntime(t, model, []gatewayruntime.Option{gatewayruntime.WithCallPolicies(policy)})
	handler, err := NewHandler(runtime)
	require.NoError(t, err)
	body, err := EncodeCallOptions(provider.CallOptions{Prompt: []provider.Message{}, IncludeRawChunks: true})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, validStrictRequest(t, true, body))
	reader, err := NewSSEReader(bytes.NewReader(recorder.Body.Bytes()), DefaultMaxSSEEventBytes)
	require.NoError(t, err)
	part, err := reader.Next()
	require.NoError(t, err)
	assert.Equal(t, provider.PartRaw, part.Type)
	assert.JSONEq(t, `{"raw":true}`, string(part.RawValue))
}

func TestHandler_StreamIdleAndEventLimit(t *testing.T) {
	t.Run("idle timeout", func(t *testing.T) {
		stream := make(chan provider.StreamPart)
		model := &handlerModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: stream}, nil
		}}
		handler, err := NewHandler(newHandlerRuntime(t, model, nil), WithIdleTimeout(10*time.Millisecond))
		require.NoError(t, err)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, validStrictRequest(t, true, nil))
		reader, err := NewSSEReader(bytes.NewReader(recorder.Body.Bytes()), DefaultMaxSSEEventBytes)
		require.NoError(t, err)
		part, err := reader.Next()
		require.NoError(t, err)
		assert.Equal(t, provider.PartError, part.Type)
		assert.Equal(t, http.StatusGatewayTimeout, part.APICallError.StatusCode)
	})

	t.Run("runtime total timeout after commitment", func(t *testing.T) {
		stream := make(chan provider.StreamPart)
		model := &handlerModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: stream}, nil
		}}
		runtime := newHandlerRuntime(t, model, []gatewayruntime.Option{gatewayruntime.WithTotalTimeout(10 * time.Millisecond)})
		handler, err := NewHandler(runtime, WithIdleTimeout(time.Second))
		require.NoError(t, err)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, validStrictRequest(t, true, nil))
		reader, err := NewSSEReader(bytes.NewReader(recorder.Body.Bytes()), DefaultMaxSSEEventBytes)
		require.NoError(t, err)
		part, err := reader.Next()
		require.NoError(t, err)
		assert.Equal(t, provider.PartError, part.Type)
		assert.Equal(t, http.StatusGatewayTimeout, part.APICallError.StatusCode)
	})

	t.Run("oversized event is not written", func(t *testing.T) {
		stream := make(chan provider.StreamPart, 1)
		stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: strings.Repeat("oversized", 100)}
		close(stream)
		model := &handlerModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: stream}, nil
		}}
		errorPart := provider.StreamPart{Type: provider.PartError, APICallError: apiCallErrorForClassification(failure.Classify(failure.ErrInternal))}
		errorEvent, err := EncodeSSEEventWithinLimit(errorPart, 1024)
		require.NoError(t, err)
		handler, err := NewHandler(newHandlerRuntime(t, model, nil), WithMaxSSEEventBytes(int64(len(errorEvent))))
		require.NoError(t, err)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, validStrictRequest(t, true, nil))
		assert.NotContains(t, recorder.Body.String(), "oversized")
		assert.Contains(t, recorder.Body.String(), "internal server error")
	})
}

func TestHandler_CommittedStreamRequestContextTermination(t *testing.T) {
	cases := []struct {
		name       string
		newContext func() (context.Context, context.CancelFunc)
		terminate  func(context.CancelFunc)
		wantStatus int
	}{
		{
			name: "custom cancellation cause",
			newContext: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancelCause(context.Background())
				return ctx, func() {
					cancel(failure.Wrap(failure.ErrInternal, errors.New("private custom cancellation")))
				}
			},
			terminate:  func(cancel context.CancelFunc) { cancel() },
			wantStatus: 499,
		},
		{
			name: "request deadline",
			newContext: func() (context.Context, context.CancelFunc) {
				return context.WithDeadlineCause(context.Background(), time.Now().Add(50*time.Millisecond), errors.New("private deadline cause"))
			},
			terminate:  func(context.CancelFunc) {},
			wantStatus: http.StatusGatewayTimeout,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stream := make(chan provider.StreamPart)
			model := &handlerModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: stream}, nil
			}}
			handler, err := NewHandler(newHandlerRuntime(t, model, nil), WithIdleTimeout(time.Second))
			require.NoError(t, err)
			ctx, cancel := tc.newContext()
			defer cancel()
			request := validStrictRequest(t, true, nil).WithContext(ctx)
			writer := newCommitSignalResponseWriter()
			done := make(chan struct{})
			go func() {
				handler.ServeHTTP(writer, request)
				close(done)
			}()

			select {
			case <-writer.committed:
			case <-time.After(time.Second):
				t.Fatal("stream was not committed")
			}
			tc.terminate(cancel)
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("handler did not finish after request context termination")
			}

			assert.Equal(t, http.StatusOK, writer.status)
			reader, err := NewSSEReader(bytes.NewReader(writer.body.Bytes()), DefaultMaxSSEEventBytes)
			require.NoError(t, err)
			part, err := reader.Next()
			require.NoError(t, err)
			require.NotNil(t, part.APICallError)
			assert.Equal(t, provider.PartError, part.Type)
			assert.Equal(t, tc.wantStatus, part.APICallError.StatusCode)
			assert.NotEqual(t, "internal server error", part.APICallError.Message)
		})
	}
}

func TestRequestContextError_PreservesPrivateCause(t *testing.T) {
	privateCancellation := errors.New("private cancellation")
	cancellationCause := failure.Wrap(failure.ErrInternal, privateCancellation)
	canceledCtx, cancel := context.WithCancelCause(context.Background())
	cancel(cancellationCause)
	canceledErr := requestContextError(canceledCtx)
	assert.ErrorIs(t, canceledErr, failure.ErrCanceled)
	assert.ErrorIs(t, canceledErr, privateCancellation)
	canceledClassification := classifyLifecycleError(canceledCtx, errors.New("runtime lifecycle failure"))
	assert.Equal(t, failure.KindCanceled, canceledClassification.Kind)
	assert.ErrorIs(t, canceledClassification.Cause, privateCancellation)

	deadlineCause := errors.New("private deadline")
	deadlineCtx, deadlineCancel := context.WithDeadlineCause(context.Background(), time.Now().Add(-time.Second), deadlineCause)
	defer deadlineCancel()
	deadlineErr := requestContextError(deadlineCtx)
	assert.ErrorIs(t, deadlineErr, failure.ErrTimeout)
	assert.ErrorIs(t, deadlineErr, deadlineCause)
	deadlineClassification := classifyLifecycleError(deadlineCtx, errors.New("runtime lifecycle failure"))
	assert.Equal(t, failure.KindTimeout, deadlineClassification.Kind)
	assert.ErrorIs(t, deadlineClassification.Cause, deadlineCause)
}

func TestHandler_ResponseControllerAndWriteFailures(t *testing.T) {
	partStream := func(context.Context) (*provider.StreamResult, error) {
		stream := make(chan provider.StreamPart, 1)
		stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "x"}
		close(stream)
		return &provider.StreamResult{Stream: stream}, nil
	}
	model := &handlerModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		return partStream(ctx)
	}}
	handler, err := NewHandler(newHandlerRuntime(t, model, nil))
	require.NoError(t, err)

	t.Run("wrapped writer flushes", func(t *testing.T) {
		base := httptest.NewRecorder()
		writer := &unwrapResponseWriter{ResponseWriter: base}
		handler.ServeHTTP(writer, validStrictRequest(t, true, nil))
		assert.True(t, base.Flushed)
		assert.Contains(t, base.Body.String(), `"delta":"x"`)
	})

	t.Run("unsupported flush cancels", func(t *testing.T) {
		contexts := make(chan context.Context, 1)
		localModel := &handlerModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			contexts <- ctx
			return &provider.StreamResult{Stream: make(chan provider.StreamPart)}, nil
		}}
		localHandler, err := NewHandler(newHandlerRuntime(t, localModel, nil))
		require.NoError(t, err)
		writer := newBasicResponseWriter()
		localHandler.ServeHTTP(writer, validStrictRequest(t, true, nil))
		assert.Empty(t, writer.body.String())
		invocationContext := <-contexts
		select {
		case <-invocationContext.Done():
		case <-time.After(time.Second):
			t.Fatal("runtime context was not canceled after flush failure")
		}
	})

	t.Run("failed flush stops without error event", func(t *testing.T) {
		contexts := make(chan context.Context, 1)
		localModel := &handlerModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			contexts <- ctx
			stream := make(chan provider.StreamPart, 1)
			stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "x"}
			return &provider.StreamResult{Stream: stream}, nil
		}}
		localHandler, err := NewHandler(newHandlerRuntime(t, localModel, nil))
		require.NoError(t, err)
		writer := &flushErrorWriter{basicResponseWriter: *newBasicResponseWriter(), err: errors.New("flush failed")}
		localHandler.ServeHTTP(writer, validStrictRequest(t, true, nil))
		assert.Empty(t, writer.body.String())
		assert.Equal(t, 1, writer.flushes)
		assertContextCanceled(t, <-contexts)
	})

	t.Run("write failure stops without second event", func(t *testing.T) {
		contexts := make(chan context.Context, 1)
		localModel := &handlerModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			contexts <- ctx
			stream := make(chan provider.StreamPart, 1)
			stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "x"}
			return &provider.StreamResult{Stream: stream}, nil
		}}
		localHandler, err := NewHandler(newHandlerRuntime(t, localModel, nil))
		require.NoError(t, err)
		writer := &writeErrorWriter{basicResponseWriter: *newBasicResponseWriter()}
		localHandler.ServeHTTP(writer, validStrictRequest(t, true, nil))
		assert.Equal(t, 1, writer.writes)
		assertContextCanceled(t, <-contexts)
	})

	t.Run("synchronous write time is excluded from idle timeout", func(t *testing.T) {
		localHandler, err := NewHandler(newHandlerRuntime(t, model, nil), WithIdleTimeout(10*time.Millisecond))
		require.NoError(t, err)
		writer := newBlockingResponseWriter()
		done := make(chan struct{})
		go func() {
			localHandler.ServeHTTP(writer, validStrictRequest(t, true, nil))
			close(done)
		}()
		<-writer.entered
		time.Sleep(30 * time.Millisecond)
		close(writer.release)
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("handler did not finish after write release")
		}
		assert.Contains(t, writer.body.String(), `"delta":"x"`)
		assert.NotContains(t, writer.body.String(), "timed out")
	})
}

func assertContextCanceled(t *testing.T, ctx context.Context) {
	t.Helper()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("runtime context was not canceled")
	}
}

func validStrictRequest(t *testing.T, streaming bool, body []byte) *http.Request {
	t.Helper()
	if body == nil {
		var err error
		body, err = EncodeCallOptions(provider.CallOptions{Prompt: []provider.Message{}})
		require.NoError(t, err)
	}
	request := httptest.NewRequest(http.MethodPost, PathLanguageModel, bytes.NewReader(body))
	request.Header.Set(HeaderModelID, "model")
	request.Header.Set(HeaderSpecVersion, SpecVersionV4)
	request.Header.Set(HeaderStreaming, map[bool]string{true: "true", false: "false"}[streaming])
	request.Header.Set("Content-Type", MIMEJSON)
	return request
}

func newHandlerRuntime(t *testing.T, model provider.LanguageModel, options []gatewayruntime.Option) *gatewayruntime.Runtime {
	t.Helper()
	resolver := gatewayruntime.ModelResolverFunc(func(context.Context, gatewayruntime.GatewayCall) (catalog.ResolvedModel, error) {
		return catalog.ResolvedModel{ID: "canonical", Model: model}, nil
	})
	runtime, err := gatewayruntime.New(resolver, options...)
	require.NoError(t, err)
	return runtime
}

func streamTypes(parts []provider.StreamPart) []provider.StreamPartType {
	types := make([]provider.StreamPartType, len(parts))
	for i, part := range parts {
		types[i] = part.Type
	}
	return types
}

type unwrapResponseWriter struct{ http.ResponseWriter }

func (writer *unwrapResponseWriter) Unwrap() http.ResponseWriter { return writer.ResponseWriter }

type basicResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newBasicResponseWriter() *basicResponseWriter {
	return &basicResponseWriter{header: make(http.Header)}
}
func (writer *basicResponseWriter) Header() http.Header            { return writer.header }
func (writer *basicResponseWriter) WriteHeader(status int)         { writer.status = status }
func (writer *basicResponseWriter) Write(data []byte) (int, error) { return writer.body.Write(data) }

type commitSignalResponseWriter struct {
	basicResponseWriter
	committed chan struct{}
	once      sync.Once
}

func newCommitSignalResponseWriter() *commitSignalResponseWriter {
	return &commitSignalResponseWriter{
		basicResponseWriter: *newBasicResponseWriter(),
		committed:           make(chan struct{}),
	}
}

func (writer *commitSignalResponseWriter) FlushError() error {
	writer.once.Do(func() { close(writer.committed) })
	return nil
}

type flushErrorWriter struct {
	basicResponseWriter
	err     error
	flushes int
}

func (writer *flushErrorWriter) FlushError() error { writer.flushes++; return writer.err }

type writeErrorWriter struct {
	basicResponseWriter
	writes int
}

func (*writeErrorWriter) FlushError() error { return nil }
func (writer *writeErrorWriter) Write([]byte) (int, error) {
	writer.writes++
	return 0, errors.New("write failed")
}

type blockingResponseWriter struct {
	basicResponseWriter
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingResponseWriter() *blockingResponseWriter {
	return &blockingResponseWriter{
		basicResponseWriter: *newBasicResponseWriter(),
		entered:             make(chan struct{}),
		release:             make(chan struct{}),
	}
}

func (*blockingResponseWriter) FlushError() error { return nil }
func (writer *blockingResponseWriter) Write(data []byte) (int, error) {
	writer.once.Do(func() { close(writer.entered) })
	<-writer.release
	return writer.body.Write(data)
}
