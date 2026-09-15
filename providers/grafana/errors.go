package grafana

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/grafana/ai-sdk/provider"
)

// GatewayErrorCategory identifies a registered public Gateway failure category.
type GatewayErrorCategory string

const (
	// GatewayAuthentication means the Gateway rejected authentication.
	GatewayAuthentication GatewayErrorCategory = "authentication_error"
	// GatewayForbidden means the caller lacks permission.
	GatewayForbidden GatewayErrorCategory = "forbidden"
	// GatewayInvalidRequest means the request is invalid or unsupported.
	GatewayInvalidRequest GatewayErrorCategory = "invalid_request_error"
	// GatewayModelNotFound means the public model is unavailable.
	GatewayModelNotFound GatewayErrorCategory = "model_not_found"
	// GatewayRateLimit means the service's request limit was reached.
	GatewayRateLimit GatewayErrorCategory = "rate_limit_exceeded"
	// GatewayFailedDependency means a prerequisite failed.
	GatewayFailedDependency GatewayErrorCategory = "failed_dependency"
	// GatewayInternalServer means execution failed inside the Gateway.
	GatewayInternalServer GatewayErrorCategory = "internal_server_error"
)

// GatewayError exposes the public Gateway error and wraps the SDK API error.
type GatewayError struct {
	Category    GatewayErrorCategory
	Code        string
	Message     string
	StatusCode  int
	IsRetryable bool
	cause       *provider.APICallError
}

func (e *GatewayError) Error() string { return "grafana: " + e.Message }

// Unwrap exposes the bounded provider error for SDK retry/error handling.
func (e *GatewayError) Unwrap() error { return e.cause }

type wireError struct {
	Message *string              `json:"message"`
	Type    GatewayErrorCategory `json:"type"`
	Code    string               `json:"code"`
	Param   json.RawMessage      `json:"param"`
}

func readGatewayError(ctx context.Context, resp *http.Response, limit int64) error {
	body, err := readJSON(ctx, resp, limit)
	if err != nil {
		return protocolError("grafana: invalid Gateway error response", resp.StatusCode, err)
	}
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	var value wireError
	if decodeFields(body, &envelope, "error") != nil || decodeFields(envelope.Error, &value, "message", "type", "code", "param") != nil {
		return protocolError("grafana: invalid Gateway error envelope", resp.StatusCode, nil)
	}
	return mapGatewayError(&value, resp.StatusCode)
}

func mapGatewayError(value *wireError, status int) error {
	if value.Message == nil || !validPublicText(*value.Message) || string(value.Param) != "null" || !registeredError(value.Type, value.Code, status) {
		return protocolError("grafana: invalid Gateway error fields", status, nil)
	}
	publicBody, _ := json.Marshal(struct {
		Error *wireError `json:"error"`
	}{value})
	cause := provider.NewAPICallError(provider.APICallErrorOptions{Message: *value.Message, StatusCode: status, ResponseBody: string(publicBody)})
	return &GatewayError{Category: value.Type, Code: value.Code, Message: *value.Message, StatusCode: status, IsRetryable: cause.IsRetryable, cause: cause}
}

func registeredError(category GatewayErrorCategory, code string, status int) bool {
	switch category {
	case GatewayInvalidRequest:
		return status == 400 && code == "invalid_request"
	case GatewayAuthentication:
		return status == 401 && code == "authentication_error"
	case GatewayForbidden:
		return status == 403 && code == "forbidden"
	case GatewayModelNotFound:
		return status == 404 && code == "model_not_found"
	case GatewayRateLimit:
		return status == 429 && code == "rate_limit_exceeded"
	case GatewayFailedDependency:
		return status == 424 && code == "failed_dependency"
	case GatewayInternalServer:
		return status == 500 && code == "internal_error" || status == 502 && code == "upstream_error" || status == 503 && code == "overloaded" || status == 504 && code == "timeout" || status == 499 && code == "canceled"
	}
	return false
}
