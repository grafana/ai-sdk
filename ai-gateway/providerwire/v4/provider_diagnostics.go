package v4

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
)

const (
	maxProviderErrorBytes   = 16384
	maxProviderMessageBytes = 4096
	maxProviderDetailBytes  = 4096
	maxProviderCodeBytes    = 256
)

type providerErrorDetail struct {
	Type  string          `json:"type,omitempty"`
	Code  json.RawMessage `json:"code,omitempty"`
	Param json.RawMessage `json:"param,omitempty"`
}

type providerDiagnosticType string

const (
	diagnosticInvalidRequest   providerDiagnosticType = "invalid_request_error"
	diagnosticRateLimit        providerDiagnosticType = "rate_limit_exceeded"
	diagnosticFailedDependency providerDiagnosticType = "failed_dependency"
	diagnosticInternal         providerDiagnosticType = "internal_server_error"
)

type providerErrorProjection struct {
	status     int
	Message    string                 `json:"message"`
	Type       providerDiagnosticType `json:"type"`
	Param      json.RawMessage        `json:"param"`
	Code       string                 `json:"code"`
	StatusCode int                    `json:"statusCode,omitempty"`
	Retryable  *bool                  `json:"retryable,omitempty"`
}

func projectProviderError(err error, format catalog.ProviderErrorFormat, streaming bool) (projection providerErrorProjection, valid bool) {
	defer func() {
		if recover() != nil {
			projection, valid = providerErrorProjection{}, false
		}
	}()
	if format != catalog.ProviderErrorAnthropic && format != catalog.ProviderErrorOpenAI || isNilInterface(err) {
		return providerErrorProjection{}, false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, catalog.ErrUnsupportedRequest) {
		return providerErrorProjection{}, false
	}
	var aggregate interface{ Unwrap() []error }
	if errors.As(err, &aggregate) {
		return providerErrorProjection{}, false
	}
	var api *provider.APICallError
	if !errors.As(err, &api) || api == nil || api.StatusCode < 400 && (!streaming || api.StatusCode != http.StatusOK) || api.StatusCode > 599 {
		return providerErrorProjection{}, false
	}
	if len(api.Data) > maxProviderErrorBytes || len(api.ResponseBody) > maxProviderErrorBytes {
		return providerErrorProjection{}, false
	}
	raw := api.Data
	if len(raw) == 0 {
		raw = json.RawMessage(api.ResponseBody)
	}
	if !utf8.Valid(raw) || !json.Valid(raw) {
		return providerErrorProjection{}, false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return providerErrorProjection{}, false
	}
	if inner, exists := fields["error"]; exists {
		if json.Unmarshal(inner, &fields) != nil || fields == nil {
			return providerErrorProjection{}, false
		}
	}
	var message string
	if !diagnosticString(fields["message"], maxProviderMessageBytes, &message) || message == "" {
		return providerErrorProjection{}, false
	}
	detail := providerErrorDetail{}
	if raw, exists := fields["type"]; exists && string(raw) != "null" {
		if !diagnosticString(raw, maxProviderCodeBytes, &detail.Type) {
			return providerErrorProjection{}, false
		}
	}
	if format == catalog.ProviderErrorAnthropic && detail.Type == "" {
		return providerErrorProjection{}, false
	}
	if format == catalog.ProviderErrorOpenAI {
		if code, exists := fields["code"]; exists {
			if !validProviderCode(code) {
				return providerErrorProjection{}, false
			}
			detail.Code = code
		}
		if param, exists := fields["param"]; exists {
			var text string
			if string(param) != "null" && !diagnosticString(param, maxProviderDetailBytes, &text) {
				return providerErrorProjection{}, false
			}
			detail.Param = param
		}
	}
	category, code := providerErrorCategory(api.StatusCode)
	if api.StatusCode == http.StatusUnauthorized || api.StatusCode == http.StatusForbidden || detail.Type == "authentication_error" || detail.Type == "permission_error" {
		message = "provider account authorization failed"
		detail = providerErrorDetail{Type: "provider_authorization_error"}
	}
	param, marshalErr := json.Marshal(detail)
	if marshalErr != nil || len(param) > maxProviderDetailBytes {
		return providerErrorProjection{}, false
	}
	result := providerErrorProjection{status: api.StatusCode, Message: message, Type: category, Code: code, Param: param}
	if streaming {
		retryable := api.IsRetryable
		result.StatusCode, result.Retryable = api.StatusCode, &retryable
	}
	return result, true
}

func diagnosticString(raw json.RawMessage, limit int, value *string) bool {
	return len(raw) > 0 && raw[0] == '"' && json.Unmarshal(raw, value) == nil && len(*value) <= limit && utf8.ValidString(*value)
}

func validProviderCode(raw json.RawMessage) bool {
	if string(raw) == "null" {
		return true
	}
	var text string
	if len(raw) > 0 && raw[0] == '"' {
		return diagnosticString(raw, maxProviderCodeBytes, &text)
	}
	var number json.Number
	if json.Unmarshal(raw, &number) != nil {
		return false
	}
	value, err := strconv.ParseFloat(string(number), 64)
	return err == nil && !math.IsInf(value, 0) && !math.IsNaN(value)
}

func providerErrorCategory(status int) (providerDiagnosticType, string) {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return diagnosticInvalidRequest, "invalid_request"
	case http.StatusTooManyRequests:
		return diagnosticRateLimit, "rate_limit_exceeded"
	}
	if status >= 400 && status < 500 {
		return diagnosticFailedDependency, "failed_dependency"
	}
	return diagnosticInternal, "upstream_error"
}

func (h *handler) writeProviderError(w http.ResponseWriter, err error, format catalog.ProviderErrorFormat) {
	projection, ok := projectProviderError(err, format, false)
	if ok {
		body, marshalErr := json.Marshal(struct {
			Error providerErrorProjection `json:"error"`
		}{projection})
		if marshalErr == nil && int64(len(body)) <= min(int64(maxProviderErrorBytes), h.limits.UnaryResponseBytes) {
			writeSafeErrorDocument(w, safeErrorDocument{status: projection.status, body: body})
			return
		}
	}
	h.writeSafeError(w, safeErrorFromProvider(err))
}

func (h *handler) emitProviderStreamError(w http.ResponseWriter, err error, format catalog.ProviderErrorFormat) streamWriteResult {
	projection, ok := projectProviderError(err, format, true)
	if ok {
		payload, marshalErr := json.Marshal(struct {
			Type  provider.StreamPartType `json:"type"`
			Error providerErrorProjection `json:"error"`
		}{provider.PartError, projection})
		if marshalErr == nil && int64(len(payload)+len("data: \n\n")) <= min(int64(maxProviderErrorBytes), h.limits.StreamFrameBytes) {
			frame := append(append([]byte("data: "), payload...), '\n', '\n')
			if !writeCompleteStreamFrame(w, frame) {
				return streamWriteWriterFailure
			}
			return streamWriteSuccess
		}
	}
	return h.emitSafeStreamError(w, safeErrorFromStreamProvider(err))
}
