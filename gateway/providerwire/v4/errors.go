package v4

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/gateway/catalog"
	"github.com/grafana/ai-sdk/gateway/failure"
	"github.com/grafana/ai-sdk/provider"
)

const (
	messageInvalidRequest   = "invalid request"
	messageAuthentication   = "authentication failed"
	messagePermission       = "permission denied"
	messageNotFound         = "model not found"
	messageRateLimit        = "rate limit exceeded"
	messageOverload         = "provider overloaded"
	messageFailedDependency = "provider request failed"
	messageUpstream         = "upstream provider failed"
	messageTimeout          = "request timed out"
	messageCancellation     = "request canceled"
	messageInternal         = "internal server error"
)

type publicErrorType string

type publicErrorCode string

const (
	publicErrorTypeInvalidRequest   publicErrorType = "invalid_request_error"
	publicErrorTypeAuthentication   publicErrorType = "authentication_error"
	publicErrorTypeForbidden        publicErrorType = "forbidden"
	publicErrorTypeModelNotFound    publicErrorType = "model_not_found"
	publicErrorTypeRateLimit        publicErrorType = "rate_limit_exceeded"
	publicErrorTypeInternal         publicErrorType = "internal_server_error"
	publicErrorTypeFailedDependency publicErrorType = "failed_dependency"

	publicErrorCodeInvalidRequest   publicErrorCode = "invalid_request"
	publicErrorCodeAuthentication   publicErrorCode = "authentication_error"
	publicErrorCodeForbidden        publicErrorCode = "forbidden"
	publicErrorCodeModelNotFound    publicErrorCode = "model_not_found"
	publicErrorCodeRateLimit        publicErrorCode = "rate_limit_exceeded"
	publicErrorCodeOverloaded       publicErrorCode = "overloaded"
	publicErrorCodeFailedDependency publicErrorCode = "failed_dependency"
	publicErrorCodeUpstream         publicErrorCode = "upstream_error"
	publicErrorCodeTimeout          publicErrorCode = "timeout"
	publicErrorCodeCanceled         publicErrorCode = "canceled"
	publicErrorCodeInternal         publicErrorCode = "internal_error"
)

type errorDescriptor struct {
	status int
	typeID publicErrorType
	code   publicErrorCode
}

type publicError struct {
	Message    string          `json:"message"`
	Type       publicErrorType `json:"type"`
	Param      any             `json:"param"`
	Code       publicErrorCode `json:"code"`
	StatusCode int             `json:"statusCode,omitempty"`
	Retryable  *bool           `json:"retryable,omitempty"`
}

type errorResponse struct {
	Error publicError `json:"error"`
}

type streamErrorPart struct {
	Type  streamPartType `json:"type"`
	Error publicError    `json:"error"`
}

var (
	canonicalInternal   = makeFailure(failure.CategoryInternalFailure, messageInternal)
	canonicalErrorBytes = []byte(`{"error":{"message":"internal server error","type":"internal_server_error","param":null,"code":"internal_error"}}`)
	canonicalErrorFrame = appendFrame(canonicalStreamErrorBytes())
)

func makeFailure(category failure.Category, message string) failure.Failure {
	value, _ := failure.New(category, message)
	return value
}

func failureDescriptor(category failure.Category) (errorDescriptor, bool) {
	switch category {
	case failure.CategoryInvalidRequest:
		return errorDescriptor{http.StatusBadRequest, publicErrorTypeInvalidRequest, publicErrorCodeInvalidRequest}, true
	case failure.CategoryAuthentication:
		return errorDescriptor{http.StatusUnauthorized, publicErrorTypeAuthentication, publicErrorCodeAuthentication}, true
	case failure.CategoryPermission:
		return errorDescriptor{http.StatusForbidden, publicErrorTypeForbidden, publicErrorCodeForbidden}, true
	case failure.CategoryNotFound:
		return errorDescriptor{http.StatusNotFound, publicErrorTypeModelNotFound, publicErrorCodeModelNotFound}, true
	case failure.CategoryRateLimit:
		return errorDescriptor{http.StatusTooManyRequests, publicErrorTypeRateLimit, publicErrorCodeRateLimit}, true
	case failure.CategoryOverload:
		return errorDescriptor{http.StatusServiceUnavailable, publicErrorTypeInternal, publicErrorCodeOverloaded}, true
	case failure.CategoryFailedDependency:
		return errorDescriptor{http.StatusFailedDependency, publicErrorTypeFailedDependency, publicErrorCodeFailedDependency}, true
	case failure.CategoryUpstreamFailure:
		return errorDescriptor{http.StatusBadGateway, publicErrorTypeInternal, publicErrorCodeUpstream}, true
	case failure.CategoryTimeout:
		return errorDescriptor{http.StatusGatewayTimeout, publicErrorTypeInternal, publicErrorCodeTimeout}, true
	case failure.CategoryCancellation:
		return errorDescriptor{499, publicErrorTypeInternal, publicErrorCodeCanceled}, true
	case failure.CategoryInternalFailure:
		return errorDescriptor{http.StatusInternalServerError, publicErrorTypeInternal, publicErrorCodeInternal}, true
	default:
		return errorDescriptor{}, false
	}
}

func normalizeFailure(value failure.Failure) failure.Failure {
	if !value.Valid() || !utf8.ValidString(value.Message()) {
		return canonicalInternal
	}
	return value
}

func publicErrorFor(value failure.Failure, stream bool) (publicError, int) {
	value = normalizeFailure(value)
	descriptor, ok := failureDescriptor(value.Category())
	if !ok {
		value = canonicalInternal
		descriptor, _ = failureDescriptor(value.Category())
	}
	result := publicError{
		Message: value.Message(),
		Type:    descriptor.typeID,
		Param:   nil,
		Code:    descriptor.code,
	}
	if stream {
		retryable := value.Retryable()
		result.StatusCode = descriptor.status
		result.Retryable = &retryable
	}
	return result, descriptor.status
}

func (h *Handler) encodeError(value failure.Failure) ([]byte, int) {
	body, status := encodeErrorUnchecked(value)
	if int64(len(body)) > h.maxErrorResponseBytes || h.schemas.error.Validate(body) != nil {
		return canonicalErrorBytes, http.StatusInternalServerError
	}
	return body, status
}

func encodeErrorUnchecked(value failure.Failure) ([]byte, int) {
	public, status := publicErrorFor(value, false)
	body, err := json.Marshal(errorResponse{Error: public})
	if err != nil {
		return canonicalErrorBytes, http.StatusInternalServerError
	}
	return body, status
}

func (h *Handler) writeError(w http.ResponseWriter, value failure.Failure) {
	body, status := h.encodeError(value)
	w.Header().Set("Content-Type", MIMEJSON)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func canonicalStreamErrorBytes() []byte {
	public, _ := publicErrorFor(canonicalInternal, true)
	body, _ := json.Marshal(streamErrorPart{Type: streamPartError, Error: public})
	return body
}

func (h *Handler) errorFrame(value failure.Failure) []byte {
	public, _ := publicErrorFor(value, true)
	body, err := json.Marshal(streamErrorPart{Type: streamPartError, Error: public})
	if err != nil || h.schemas.stream.Validate(body) != nil {
		return canonicalErrorFrame
	}
	frame := appendFrame(body)
	if int64(len(frame)) > h.maxEventBytes {
		return canonicalErrorFrame
	}
	return frame
}

func invalidRequest(message string) failure.Failure {
	if message == "" {
		message = messageInvalidRequest
	}
	return makeFailure(failure.CategoryInvalidRequest, message)
}

func contextFailure(ctx context.Context) (failure.Failure, bool) {
	if ctx == nil || ctx.Err() == nil {
		return failure.Failure{}, false
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(context.Cause(ctx), context.DeadlineExceeded) {
		return makeFailure(failure.CategoryTimeout, messageTimeout), true
	}
	return makeFailure(failure.CategoryCancellation, messageCancellation), true
}

func reduceResolverError(ctx context.Context, err error) failure.Failure {
	if value, ok := contextFailure(ctx); ok {
		return value
	}
	if errors.Is(err, catalog.ErrUnknownModel) {
		return makeFailure(failure.CategoryNotFound, messageNotFound)
	}
	return canonicalInternal
}

func reduceProviderError(ctx context.Context, err error) failure.Failure {
	if value, ok := contextFailure(ctx); ok {
		return value
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return makeFailure(failure.CategoryTimeout, messageTimeout)
	case errors.Is(err, context.Canceled):
		return makeFailure(failure.CategoryCancellation, messageCancellation)
	}
	var apiError *provider.APICallError
	if errors.As(err, &apiError) && apiError != nil {
		return reduceAPICallError(apiError)
	}
	return makeFailure(failure.CategoryUpstreamFailure, messageUpstream)
}

func reduceAPICallError(apiError *provider.APICallError) failure.Failure {
	if apiError == nil {
		return makeFailure(failure.CategoryUpstreamFailure, messageUpstream)
	}
	switch apiError.StatusCode {
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return makeFailure(failure.CategoryTimeout, messageTimeout)
	case http.StatusTooManyRequests:
		return makeFailure(failure.CategoryRateLimit, messageRateLimit)
	case http.StatusServiceUnavailable:
		return makeFailure(failure.CategoryOverload, messageOverload)
	}
	if apiError.IsRetryable {
		return makeFailure(failure.CategoryUpstreamFailure, messageUpstream)
	}
	return makeFailure(failure.CategoryFailedDependency, messageFailedDependency)
}
