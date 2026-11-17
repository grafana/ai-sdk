package messages

import (
	"encoding/json"
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/internal/host"
)

type apiError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Type  string   `json:"type"`
	Error apiError `json:"error"`
}

func errorType(status int) (typ, message string) {
	switch status {
	case 400:
		return "invalid_request_error", "invalid or unsupported request"
	case 401:
		return "authentication_error", "authentication failed"
	case 403:
		return "permission_error", "permission denied"
	case 404:
		return "not_found_error", "model or route not found"
	case 405:
		return "invalid_request_error", "method not allowed"
	case 413:
		return "request_too_large", "request too large"
	case 415:
		return "invalid_request_error", "JSON content type required"
	case 424:
		return "api_error", "upstream dependency failed"
	case 429:
		return "rate_limit_error", "rate limit exceeded"
	case 499:
		return "api_error", "request canceled"
	case 502:
		return "api_error", "upstream failure"
	case 503:
		return "overloaded_error", "service unavailable"
	case 504:
		return "timeout_error", "request timed out"
	default:
		return "api_error", "internal error"
	}
}

func errorBody(status int) []byte {
	typ, message := errorType(status)
	body, _ := json.Marshal(errorEnvelope{Type: "error", Error: apiError{Type: typ, Message: message}})
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
