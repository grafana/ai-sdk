package chatcompletions

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
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

func errorBody(status int) []byte {
	code, typ, message := "internal_error", "server_error", "internal error"
	switch status {
	case 400:
		code, typ, message = "invalid_request", "invalid_request_error", "invalid or unsupported request"
	case 401:
		code, typ, message = "invalid_api_key", "authentication_error", "authentication failed"
	case 403:
		code, typ, message = "permission_denied", "permission_error", "permission denied"
	case 404:
		code, typ, message = "model_not_found", "invalid_request_error", "model or route not found"
	case 405:
		code, typ, message = "method_not_allowed", "invalid_request_error", "method not allowed"
	case 413:
		code, typ, message = "request_too_large", "invalid_request_error", "request too large"
	case 415:
		code, typ, message = "unsupported_media_type", "invalid_request_error", "JSON content type required"
	case 429:
		code, typ, message = "rate_limit_exceeded", "rate_limit_error", "rate limit exceeded"
	case 502:
		code, message = "upstream_error", "upstream failure"
	case 503:
		code, message = "unavailable", "service unavailable"
	case 504:
		code, message = "timeout", "request timed out"
	}
	b, _ := json.Marshal(errorEnvelope{apiError{message, typ, nil, code}})
	return b
}

// WriteError writes a fixed Chat Completions envelope for host-owned route/auth failures.
func WriteError(w http.ResponseWriter, status int) {
	switch status {
	case 400, 401, 403, 404, 405, 413, 415, 429, 500, 502, 503, 504:
	default:
		status = 500
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(errorBody(status))
}

func errorStatus(err error) int {
	if errors.Is(err, context.DeadlineExceeded) {
		return 504
	}
	if errors.Is(err, catalog.ErrUnknownModel) {
		return 404
	}
	if errors.Is(err, catalog.ErrUnsupportedRequest) {
		return 400
	}
	var api *provider.APICallError
	if errors.As(err, &api) && api != nil {
		switch api.StatusCode {
		case 429:
			return 429
		case 503, 529:
			return 503
		case 408, 504:
			return 504
		}
		return 502
	}
	return 500
}
