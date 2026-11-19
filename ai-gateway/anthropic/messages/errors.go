package messages

import (
	"encoding/json"
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/internal/host"
)

type apiError struct {
	Type    apiErrorType `json:"type"`
	Message string       `json:"message"`
}

type errorEnvelope struct {
	Type  envelopeType `json:"type"`
	Error apiError     `json:"error"`
}

func errorType(status int) (typ apiErrorType, message string) {
	switch status {
	case 400:
		return apiErrorInvalidRequest, "invalid or unsupported request"
	case 401:
		return apiErrorAuthentication, "authentication failed"
	case 403:
		return apiErrorPermission, "permission denied"
	case 404:
		return apiErrorNotFound, "model or route not found"
	case 405:
		return apiErrorInvalidRequest, "method not allowed"
	case 413:
		return apiErrorRequestTooLarge, "request too large"
	case 415:
		return apiErrorInvalidRequest, "JSON content type required"
	case 424:
		return apiErrorAPI, "upstream dependency failed"
	case 429:
		return apiErrorRateLimit, "rate limit exceeded"
	case 499:
		return apiErrorAPI, "request canceled"
	case 502:
		return apiErrorAPI, "upstream failure"
	case 503:
		return apiErrorOverloaded, "service unavailable"
	case 504:
		return apiErrorTimeout, "request timed out"
	default:
		return apiErrorAPI, "internal error"
	}
}

func errorBody(status int) []byte {
	typ, message := errorType(status)
	body, _ := json.Marshal(errorEnvelope{Type: envelopeError, Error: apiError{Type: typ, Message: message}})
	return body
}

// WriteError writes the fixed Anthropic error document for status.
func WriteError(w http.ResponseWriter, status int) {
	switch status {
	case 400, 401, 403, 404, 405, 413, 415, 424, 429, 499, 500, 502, 503, 504:
	default:
		status = 500
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(errorBody(status))
}

func categoryStatus(category execution.Category) int {
	switch category {
	case execution.CategoryInvalidRequest, execution.CategoryBYOKCredentials, execution.CategoryBYOKSelector, execution.CategoryGatewayControl:
		return 400
	case execution.CategoryModelNotFound:
		return 404
	case execution.CategoryPermission:
		return 403
	case execution.CategoryRateLimit:
		return 429
	case execution.CategoryOverload, execution.CategoryUnavailable:
		return 503
	case execution.CategoryFailedDependency, execution.CategoryCatalogConfiguration:
		return 424
	case execution.CategoryUpstream:
		return 502
	case execution.CategoryTimeout:
		return 504
	case execution.CategoryCancellation:
		return 499
	default:
		return 500
	}
}

// HostErrorWriter writes fixed Anthropic documents for host failures.
type HostErrorWriter struct{}

// NewHostErrorWriter constructs a fixed-document host error writer.
func NewHostErrorWriter() *HostErrorWriter { return &HostErrorWriter{} }

// WriteHostError writes the fixed document for category.
func (*HostErrorWriter) WriteHostError(w http.ResponseWriter, category host.ErrorCategory) {
	status := 500
	switch category {
	case host.ErrorPermission:
		status = 403
	case host.ErrorBYOKDiscovery:
		status = 400
	}
	WriteError(w, status)
}
