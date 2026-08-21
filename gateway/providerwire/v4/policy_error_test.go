package v4

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/gateway/catalog"
	"github.com/grafana/ai-sdk/gateway/failure"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_PolicyAndResolutionOrdering(t *testing.T) {
	t.Run("policy rejection", func(t *testing.T) {
		model := &testModel{}
		resolver := resolverFor(model)
		policyCalls := 0
		denied := makeFailure(failure.CategoryPermission, messagePermission)
		handler, err := NewHandler(resolver, WithPolicy(PolicyFunc(func(ctx context.Context, request PolicyRequest) *failure.Failure {
			policyCalls++
			assert.Equal(t, "public/alias", request.ModelID)
			assert.Equal(t, CallModeUnary, request.Mode)
			assert.Equal(t, "x", request.Options.Prompt[0].Content[0].Text)
			return &denied
		})))
		require.NoError(t, err)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, validRequest(`{"prompt":[{"role":"user","content":[{"type":"text","text":"x"}]}]}`, false))
		assert.Equal(t, http.StatusForbidden, response.Code)
		assert.Equal(t, 1, policyCalls)
		assert.Zero(t, resolver.calls)
		assert.Zero(t, model.calls)
	})

	t.Run("alias canonical identity and at most once", func(t *testing.T) {
		type contextKey struct{}
		model := &testModel{}
		resolver := &testResolver{resolve: func(ctx context.Context, _ string) (catalog.ResolvedModel, error) {
			assert.Equal(t, "request-value", ctx.Value(contextKey{}))
			return catalog.ResolvedModel{ID: "public/canonical", Model: model}, nil
		}}
		policyCalls := 0
		handler, err := NewHandler(resolver, WithPolicy(PolicyFunc(func(ctx context.Context, _ PolicyRequest) *failure.Failure {
			policyCalls++
			assert.Equal(t, "request-value", ctx.Value(contextKey{}))
			return nil
		})))
		require.NoError(t, err)
		response := httptest.NewRecorder()
		requestContext := context.WithValue(context.Background(), contextKey{}, "request-value")
		handler.ServeHTTP(response, validRequest(`{"prompt":[]}`, false).WithContext(requestContext))
		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, 1, policyCalls)
		assert.Equal(t, 1, resolver.calls)
		assert.Equal(t, "public/alias", resolver.modelID)
		assert.Equal(t, 1, model.calls)
		assert.Contains(t, response.Body.String(), `"modelId":"public/canonical"`)
		assert.NotContains(t, response.Body.String(), "backend-model")
		assert.NotContains(t, response.Body.String(), "private-provider")
	})
}

func TestHandler_CancellationPrecedesPolicyAndResolverErrors(t *testing.T) {
	t.Run("policy", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		model := &testModel{}
		resolver := resolverFor(model)
		denied := makeFailure(failure.CategoryPermission, messagePermission)
		handler, err := NewHandler(resolver, WithPolicy(PolicyFunc(func(context.Context, PolicyRequest) *failure.Failure {
			cancel()
			return &denied
		})))
		require.NoError(t, err)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, validRequest(`{"prompt":[]}`, false).WithContext(ctx))
		assert.Equal(t, 499, response.Code)
		assert.Zero(t, resolver.calls)
	})

	t.Run("resolver", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		resolver := &testResolver{resolve: func(context.Context, string) (catalog.ResolvedModel, error) {
			cancel()
			return catalog.ResolvedModel{}, errors.New("private resolver detail")
		}}
		handler, err := NewHandler(resolver)
		require.NoError(t, err)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, validRequest(`{"prompt":[]}`, false).WithContext(ctx))
		assert.Equal(t, 499, response.Code)
		assert.NotContains(t, response.Body.String(), "private resolver detail")
	})
}

func TestHandler_ResolverFailures(t *testing.T) {
	invalidUTF8 := string([]byte{0xff})
	tests := []struct {
		name     string
		resolved catalog.ResolvedModel
		err      error
		status   int
	}{
		{"unknown", catalog.ResolvedModel{}, &catalog.UnknownModelError{ModelID: "private"}, http.StatusNotFound},
		{"arbitrary", catalog.ResolvedModel{}, errors.New("private resolver detail"), http.StatusInternalServerError},
		{"nil model", catalog.ResolvedModel{ID: "public/canonical"}, nil, http.StatusInternalServerError},
		{"empty canonical", catalog.ResolvedModel{Model: &testModel{}}, nil, http.StatusInternalServerError},
		{"invalid UTF-8 canonical", catalog.ResolvedModel{ID: invalidUTF8, Model: &testModel{}}, nil, http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolver := &testResolver{resolve: func(context.Context, string) (catalog.ResolvedModel, error) { return tc.resolved, tc.err }}
			handler, err := NewHandler(resolver)
			require.NoError(t, err)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, validRequest(`{"prompt":[]}`, false))
			assert.Equal(t, tc.status, response.Code)
			assert.NotContains(t, response.Body.String(), "private")
		})
	}
}

func TestHandler_InvalidUTF8CanonicalIDBypassesModel(t *testing.T) {
	model := &testModel{}
	resolver := &testResolver{resolve: func(context.Context, string) (catalog.ResolvedModel, error) {
		return catalog.ResolvedModel{ID: string([]byte{0xff}), Model: model}, nil
	}}
	handler, err := NewHandler(resolver)
	require.NoError(t, err)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, validRequest(`{"prompt":[]}`, false))
	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.Zero(t, model.calls)
}

func TestErrorEncoding_AllCategoriesAndFallback(t *testing.T) {
	tests := []struct {
		category  failure.Category
		status    int
		typeID    string
		code      string
		retryable bool
	}{
		{failure.CategoryInvalidRequest, 400, "invalid_request_error", "invalid_request", false},
		{failure.CategoryAuthentication, 401, "authentication_error", "authentication_error", false},
		{failure.CategoryPermission, 403, "forbidden", "forbidden", false},
		{failure.CategoryNotFound, 404, "model_not_found", "model_not_found", false},
		{failure.CategoryRateLimit, 429, "rate_limit_exceeded", "rate_limit_exceeded", true},
		{failure.CategoryOverload, 503, "internal_server_error", "overloaded", true},
		{failure.CategoryFailedDependency, 424, "failed_dependency", "failed_dependency", false},
		{failure.CategoryUpstreamFailure, 502, "internal_server_error", "upstream_error", true},
		{failure.CategoryTimeout, 504, "internal_server_error", "timeout", true},
		{failure.CategoryCancellation, 499, "internal_server_error", "canceled", false},
		{failure.CategoryInternalFailure, 500, "internal_server_error", "internal_error", true},
	}
	handler := newTestHandler(t, &testModel{})
	for _, tc := range tests {
		t.Run(string(tc.category), func(t *testing.T) {
			value, err := failure.New(tc.category, "safe message")
			require.NoError(t, err)
			body, status := handler.encodeError(value)
			assert.Equal(t, tc.status, status)
			require.NoError(t, handler.schemas.error.Validate(body))
			var decoded map[string]map[string]any
			require.NoError(t, json.Unmarshal(body, &decoded))
			assert.Equal(t, "safe message", decoded["error"]["message"])
			assert.Equal(t, tc.typeID, decoded["error"]["type"])
			assert.Equal(t, tc.code, decoded["error"]["code"])
			assert.Nil(t, decoded["error"]["param"])
			assert.Len(t, decoded["error"], 4)

			public, streamStatus := publicErrorFor(value, true)
			assert.Equal(t, tc.status, streamStatus)
			require.NotNil(t, public.Retryable)
			assert.Equal(t, tc.retryable, *public.Retryable)
		})
	}
	body, status := handler.encodeError(failure.Failure{})
	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Equal(t, string(canonicalErrorBytes), string(body))

	authentication, err := failure.New(failure.CategoryAuthentication, "safe authentication message")
	require.NoError(t, err)
	body, status = handler.encodeError(authentication)
	assert.Equal(t, http.StatusUnauthorized, status)
	assert.Equal(t, `{"error":{"message":"safe authentication message","type":"authentication_error","param":null,"code":"authentication_error"}}`, string(body))

	failedDependency, err := failure.New(failure.CategoryFailedDependency, "safe message")
	require.NoError(t, err)
	frame := handler.errorFrame(failedDependency)
	assert.Equal(t, "data: {\"type\":\"error\",\"error\":{\"message\":\"safe message\",\"type\":\"failed_dependency\",\"param\":null,\"code\":\"failed_dependency\",\"statusCode\":424,\"retryable\":false}}\n\n", string(frame))
	require.NoError(t, handler.schemas.stream.Validate(frame[6:len(frame)-2]))

	longValue, err := failure.New(failure.CategoryAuthentication, strings.Repeat("x", 1024))
	require.NoError(t, err)
	smallHandler := newTestHandler(t, &testModel{}, WithMaxErrorResponseBytes(int64(len(canonicalErrorBytes))))
	body, status = smallHandler.encodeError(longValue)
	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Equal(t, string(canonicalErrorBytes), string(body))
}

func TestErrorEncoding_InvalidUTF8UsesCanonicalFallback(t *testing.T) {
	value, err := failure.New(failure.CategoryAuthentication, string([]byte{0xff}))
	require.NoError(t, err)
	handler := newTestHandler(t, &testModel{})
	response := httptest.NewRecorder()
	handler.writeError(response, value)
	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.Equal(t, MIMEJSON, response.Header().Get("Content-Type"))
	assert.Equal(t, string(canonicalErrorBytes), response.Body.String())
	require.NoError(t, handler.schemas.error.Validate(response.Body.Bytes()))
	assert.Equal(t, string(canonicalErrorFrame), string(handler.errorFrame(value)))
}

func TestReduceProviderError(t *testing.T) {
	retryable := true
	nonRetryable := false
	tests := []struct {
		name     string
		err      error
		category failure.Category
	}{
		{"timeout 408", provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 408}), failure.CategoryTimeout},
		{"timeout 504", provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 504}), failure.CategoryTimeout},
		{"rate", provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 429}), failure.CategoryRateLimit},
		{"overload", provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503}), failure.CategoryOverload},
		{"retryable", provider.NewAPICallError(provider.APICallErrorOptions{Message: "private-message", URL: "https://private-url", RequestBodyValues: json.RawMessage(`{"privateRequest":true}`), ResponseHeaders: map[string][]string{"X-Private": {"private-header"}}, ResponseBody: "private-body", StatusCode: 500, IsRetryable: &retryable, Data: json.RawMessage(`{"privateData":true}`), Cause: errors.New("private-cause")}), failure.CategoryUpstreamFailure},
		{"permanent", provider.NewAPICallError(provider.APICallErrorOptions{Message: "private", StatusCode: 401, IsRetryable: &nonRetryable}), failure.CategoryFailedDependency},
		{"deadline", context.DeadlineExceeded, failure.CategoryTimeout},
		{"canceled", context.Canceled, failure.CategoryCancellation},
		{"other", errors.New("private"), failure.CategoryUpstreamFailure},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			value := reduceProviderError(context.Background(), tc.err)
			assert.Equal(t, tc.category, value.Category())
			assert.NotContains(t, value.Message(), "private")
		})
	}
}
