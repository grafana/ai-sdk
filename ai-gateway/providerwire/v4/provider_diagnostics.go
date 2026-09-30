package v4

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

type providerErrorFields struct {
	Error   json.RawMessage
	Message json.RawMessage
	Type    json.RawMessage
	Code    json.RawMessage
	Param   json.RawMessage
}

type providerErrorData struct {
	message string
	detail  providerErrorDetail
}

type providerDiagnosticType string

const (
	diagnosticInvalidRequest   providerDiagnosticType = "invalid_request_error"
	diagnosticRateLimit        providerDiagnosticType = "rate_limit_exceeded"
	diagnosticFailedDependency providerDiagnosticType = "failed_dependency"
	diagnosticInternal         providerDiagnosticType = "internal_server_error"
)

type providerErrorBody struct {
	Message string                 `json:"message"`
	Type    providerDiagnosticType `json:"type"`
	Param   json.RawMessage        `json:"param"`
	Code    string                 `json:"code"`
}

func trustedProviderAPIError(err error, schema catalog.ProviderErrorSchema) (api *provider.APICallError) {
	defer func() {
		if recover() != nil {
			api = nil
		}
	}()
	if schema != catalog.AnthropicErrorSchema && schema != catalog.OpenAIErrorSchema || isNilInterface(err) {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, catalog.ErrUnsupportedRequest) {
		return nil
	}
	var aggregate interface{ Unwrap() []error }
	if errors.As(err, &aggregate) || !errors.As(err, &api) || api == nil {
		return nil
	}
	if api.StatusCode != http.StatusOK && (api.StatusCode < 400 || api.StatusCode > 599) {
		return nil
	}
	return api
}

func decodeProviderErrorFields(raw json.RawMessage) (providerErrorFields, bool) {
	var fields providerErrorFields
	if !utf8.Valid(raw) || !json.Valid(raw) {
		return fields, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return fields, false
	}
	for decoder.More() {
		name, err := decoder.Token()
		if err != nil {
			return fields, false
		}
		var ignored json.RawMessage
		target := &ignored
		switch name {
		case "error":
			target = &fields.Error
		case "message":
			target = &fields.Message
		case "type":
			target = &fields.Type
		case "code":
			target = &fields.Code
		case "param":
			target = &fields.Param
		}
		if decoder.Decode(target) != nil {
			return fields, false
		}
	}
	return fields, true
}

func decodeProviderError(raw json.RawMessage, schema catalog.ProviderErrorSchema) (providerErrorData, bool) {
	fields, ok := decodeProviderErrorFields(raw)
	if ok && len(fields.Error) > 0 {
		fields, ok = decodeProviderErrorFields(fields.Error)
	}
	if !ok {
		return providerErrorData{}, false
	}
	message, ok := providerErrorString(fields.Message, maxProviderMessageBytes)
	if !ok || message == "" {
		return providerErrorData{}, false
	}
	detail := providerErrorDetail{}
	if len(fields.Type) > 0 && string(fields.Type) != "null" {
		detail.Type, ok = providerErrorString(fields.Type, maxProviderCodeBytes)
		if !ok {
			return providerErrorData{}, false
		}
	}
	switch schema {
	case catalog.AnthropicErrorSchema:
		if detail.Type == "" {
			return providerErrorData{}, false
		}
	case catalog.OpenAIErrorSchema:
		if len(fields.Code) > 0 && !validProviderCode(fields.Code) {
			return providerErrorData{}, false
		}
		if len(fields.Param) > 0 && string(fields.Param) != "null" {
			if _, ok := providerErrorString(fields.Param, maxProviderDetailBytes); !ok {
				return providerErrorData{}, false
			}
		}
		detail.Code, detail.Param = fields.Code, fields.Param
	default:
		return providerErrorData{}, false
	}
	return providerErrorData{message: message, detail: detail}, true
}

func providerErrorString(raw json.RawMessage, limit int) (string, bool) {
	var value string
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	return value, len(value) <= limit && utf8.ValidString(value)
}

func validProviderCode(raw json.RawMessage) bool {
	if string(raw) == "null" {
		return true
	}
	if len(raw) > 0 && raw[0] == '"' {
		_, ok := providerErrorString(raw, maxProviderCodeBytes)
		return ok
	}
	var number json.Number
	if json.Unmarshal(raw, &number) != nil {
		return false
	}
	_, err := strconv.ParseFloat(string(number), 64)
	return err == nil
}

func mapProviderError(api *provider.APICallError, schema catalog.ProviderErrorSchema) (providerErrorBody, bool) {
	if len(api.Data) > maxProviderErrorBytes || len(api.ResponseBody) > maxProviderErrorBytes {
		return providerErrorBody{}, false
	}
	raw := api.Data
	if len(raw) == 0 {
		raw = json.RawMessage(api.ResponseBody)
	}
	data, ok := decodeProviderError(raw, schema)
	if !ok {
		return providerErrorBody{}, false
	}
	if api.StatusCode == http.StatusUnauthorized || api.StatusCode == http.StatusForbidden || data.detail.Type == "authentication_error" || data.detail.Type == "permission_error" {
		data = providerErrorData{message: "provider account authorization failed", detail: providerErrorDetail{Type: "provider_authorization_error"}}
	}
	param, err := json.Marshal(data.detail)
	if err != nil || len(param) > maxProviderDetailBytes {
		return providerErrorBody{}, false
	}
	category, code := providerErrorCategory(api.StatusCode)
	return providerErrorBody{Message: data.message, Type: category, Code: code, Param: param}, true
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

func (h *handler) writeProviderError(w http.ResponseWriter, err error, schema catalog.ProviderErrorSchema) {
	api := trustedProviderAPIError(err, schema)
	if api != nil && api.StatusCode >= 400 {
		if value, ok := mapProviderError(api, schema); ok {
			body, marshalErr := json.Marshal(struct {
				Error providerErrorBody `json:"error"`
			}{value})
			if marshalErr == nil && int64(len(body)) <= min(int64(maxProviderErrorBytes), h.limits.UnaryResponseBytes) {
				writeSafeErrorDocument(w, safeErrorDocument{status: api.StatusCode, body: body})
				return
			}
		}
	}
	h.writeSafeError(w, safeErrorFromProvider(err))
}

func (h *handler) emitProviderStreamError(w http.ResponseWriter, err error, schema catalog.ProviderErrorSchema) streamWriteResult {
	api := trustedProviderAPIError(err, schema)
	if api != nil {
		if value, ok := mapProviderError(api, schema); ok {
			result := h.emitStreamEvent(w, streamEvent{typeName: provider.PartError, providerError: &streamProviderError{providerErrorBody: value, StatusCode: api.StatusCode, Retryable: api.IsRetryable}})
			if result != streamWriteEncodingFailure {
				return result
			}
		}
	}
	return h.emitSafeStreamError(w, safeErrorFromStreamProvider(err))
}
