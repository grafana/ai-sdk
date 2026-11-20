package responses

import (
	"encoding/json"
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
)

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   any    `json:"param"`
	Code    string `json:"code"`
}

type errorEnvelope struct {
	Error apiError `json:"error"`
}

func errorFor(status int) apiError {
	code, typ, message := "internal_error", "server_error", "internal error"
	switch status {
	case 400:
		code, typ, message = "invalid_request", "invalid_request_error", "invalid or unsupported request"
	case 403:
		code, typ, message = "permission_denied", "permission_error", "permission denied"
	case 404:
		code, typ, message = "model_not_found", "invalid_request_error", "model or route not found"
	case 405:
		code, typ, message = "method_not_allowed", "invalid_request_error", "method not allowed"
	case 415:
		code, typ, message = "unsupported_media_type", "invalid_request_error", "JSON content type required"
	case 424:
		code, message = "failed_dependency", "upstream dependency failed"
	case 429:
		code, typ, message = "rate_limit_exceeded", "rate_limit_error", "rate limit exceeded"
	case 499:
		code, message = "canceled", "request canceled"
	case 502:
		code, message = "upstream_error", "upstream failure"
	case 503:
		code, message = "unavailable", "service unavailable"
	case 504:
		code, message = "timeout", "request timed out"
	}
	return apiError{message, typ, nil, code}
}

func writeError(w http.ResponseWriter, status int) {
	switch status {
	case 400, 403, 404, 405, 415, 424, 429, 499, 500, 502, 503, 504:
	default:
		status = 500
	}
	b, _ := json.Marshal(errorEnvelope{errorFor(status)})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

func categoryStatus(category execution.Category) int {
	switch category {
	case execution.CategoryInvalidRequest, execution.CategoryBYOKCredentials, execution.CategoryBYOKSelector, execution.CategoryGatewayControl:
		return 400
	case execution.CategoryPermission:
		return 403
	case execution.CategoryModelNotFound:
		return 404
	case execution.CategoryFailedDependency, execution.CategoryCatalogConfiguration:
		return 424
	case execution.CategoryRateLimit:
		return 429
	case execution.CategoryCancellation:
		return 499
	case execution.CategoryUpstream, execution.CategoryProviderStream:
		return 502
	case execution.CategoryOverload, execution.CategoryUnavailable:
		return 503
	case execution.CategoryTimeout:
		return 504
	default:
		return 500
	}
}
