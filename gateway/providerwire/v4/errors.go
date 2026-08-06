package providerwirev4

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/grafana/ai-sdk/gateway/failure"
	"github.com/grafana/ai-sdk/provider"
)

type gatewayErrorEnvelopeDTO struct {
	Error gatewayErrorDTO `json:"error"`
}

type gatewayErrorDTO struct {
	Message     string          `json:"message"`
	Type        string          `json:"type"`
	StatusCode  int             `json:"statusCode"`
	IsRetryable bool            `json:"isRetryable"`
	Param       json.RawMessage `json:"param"`
}

// EncodeFailure maps a runtime classification to the registered safe V4
// gateway envelope and HTTP status.
func EncodeFailure(classification failure.Classification) (int, []byte, error) {
	dto := projectFailure(classification)
	data, err := json.Marshal(gatewayErrorEnvelopeDTO{Error: dto})
	if err != nil {
		return 0, nil, fmt.Errorf("providerwirev4: encoding failure: %w", err)
	}
	return dto.StatusCode, data, nil
}

// EncodeProtocolError encodes an adapter-owned transport or DTO error without
// exposing its private cause.
func EncodeProtocolError(status int, message string) ([]byte, error) {
	if status < 400 || status > 599 {
		return nil, fmt.Errorf("providerwirev4: invalid protocol error status %d", status)
	}
	dto := gatewayErrorDTO{Message: message, Type: "invalid_request_error", StatusCode: status, Param: json.RawMessage("null")}
	data, err := json.Marshal(gatewayErrorEnvelopeDTO{Error: dto})
	if err != nil {
		return nil, fmt.Errorf("providerwirev4: encoding protocol error: %w", err)
	}
	return data, nil
}

// DecodeErrorResponse decodes a strict V4 safe error envelope into an API call
// error while preserving explicit status and retryability.
func DecodeErrorResponse(data []byte, httpStatus int) (*provider.APICallError, error) {
	object, err := decodeObject(data, "error response")
	if err != nil {
		return nil, err
	}
	innerRaw, err := requireField(object, "error", "error response")
	if err != nil {
		return nil, err
	}
	innerObject, err := decodeObject(innerRaw, "error response error")
	if err != nil {
		return nil, err
	}
	for _, field := range []string{"message", "type", "statusCode", "isRetryable"} {
		if _, err := requireField(innerObject, field, "error response error"); err != nil {
			return nil, err
		}
	}
	var envelope gatewayErrorEnvelopeDTO
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("providerwirev4: decoding error response: %w", err)
	}
	if envelope.Error.Message == "" || envelope.Error.Type == "" || envelope.Error.StatusCode == 0 {
		return nil, errors.New("providerwirev4: incomplete error response")
	}
	if envelope.Error.StatusCode < 400 || envelope.Error.StatusCode > 599 {
		return nil, fmt.Errorf("providerwirev4: invalid error status %d", envelope.Error.StatusCode)
	}
	switch envelope.Error.Type {
	case "authentication_error", "invalid_request_error", "model_not_found", "forbidden",
		"rate_limit_exceeded", "failed_dependency", "internal_server_error":
	default:
		return nil, fmt.Errorf("providerwirev4: unsupported error type %q", envelope.Error.Type)
	}
	if httpStatus != 0 && envelope.Error.StatusCode != httpStatus {
		return nil, fmt.Errorf("providerwirev4: error status mismatch: HTTP %d, envelope %d", httpStatus, envelope.Error.StatusCode)
	}
	inner, err := json.Marshal(envelope.Error)
	if err != nil {
		return nil, err
	}
	retryable := envelope.Error.IsRetryable
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message: envelope.Error.Message, StatusCode: envelope.Error.StatusCode,
		IsRetryable: &retryable, Data: inner,
	}), nil
}

func projectFailure(classification failure.Classification) gatewayErrorDTO {
	dto := gatewayErrorDTO{IsRetryable: classification.Retryable, Param: json.RawMessage("null")}
	switch classification.Kind {
	case failure.KindUnauthenticated:
		dto.Message, dto.Type, dto.StatusCode = "authentication required", "authentication_error", http.StatusUnauthorized
	case failure.KindInvalidCall:
		dto.Message, dto.Type, dto.StatusCode = "invalid request", "invalid_request_error", http.StatusBadRequest
	case failure.KindUnknownModel:
		dto.Message, dto.Type, dto.StatusCode = "model not found", "model_not_found", http.StatusNotFound
		if classification.SafeParameters.RequestedModelID != "" {
			dto.Param, _ = json.Marshal(struct {
				ModelID string `json:"modelId"`
			}{ModelID: classification.SafeParameters.RequestedModelID})
		}
	case failure.KindForbidden:
		dto.Message, dto.Type, dto.StatusCode = "request forbidden", "forbidden", http.StatusForbidden
	case failure.KindRateLimited:
		dto.Message, dto.Type, dto.StatusCode = "rate limit exceeded", "rate_limit_exceeded", http.StatusTooManyRequests
	case failure.KindTimeout:
		dto.Message, dto.Type, dto.StatusCode = "request timed out", "internal_server_error", http.StatusGatewayTimeout
	case failure.KindCanceled:
		dto.Message, dto.Type, dto.StatusCode = "request canceled", "internal_server_error", 499
	case failure.KindFailedDependency:
		dto.Message, dto.Type = "upstream dependency failed", "failed_dependency"
		if classification.Retryable {
			dto.StatusCode = http.StatusBadGateway
		} else {
			dto.StatusCode = http.StatusFailedDependency
		}
	default:
		dto.Message, dto.Type, dto.StatusCode = "internal server error", "internal_server_error", http.StatusInternalServerError
	}
	return dto
}

func sanitizePartError(apiErr *provider.APICallError) *provider.APICallError {
	return apiCallErrorForClassification(failure.Classify(apiErr))
}

func apiCallErrorForClassification(classification failure.Classification) *provider.APICallError {
	dto := projectFailure(classification)
	data, _ := json.Marshal(dto)
	retryable := dto.IsRetryable
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message: dto.Message, StatusCode: dto.StatusCode, IsRetryable: &retryable, Data: data,
	})
}
