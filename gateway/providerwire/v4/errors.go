package providerwirev4

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/grafana/ai-sdk/gateway/catalog"
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

type failureKind uint8

const (
	failureInternal failureKind = iota
	failureUnknownModel
	failureRateLimited
	failureTimeout
	failureCanceled
	failureDependency
)

type safeFailure struct {
	kind             failureKind
	retryable        bool
	cause            error
	requestedModelID string
}

func classifyResolverError(ctx context.Context, err error, modelID string) safeFailure {
	if contextFailure, ok := classifyContextError(ctx, err); ok {
		return contextFailure
	}
	if errors.Is(err, catalog.ErrUnknownModel) {
		return safeFailure{kind: failureUnknownModel, cause: err, requestedModelID: modelID}
	}
	return safeFailure{kind: failureInternal, cause: err}
}

func classifyInvocationError(ctx context.Context, err error) safeFailure {
	if contextFailure, ok := classifyContextError(ctx, err); ok {
		return contextFailure
	}
	return classifyProviderError(err)
}

func classifyProviderError(err error) safeFailure {
	if err == nil {
		return internalFailure(errors.New("providerwirev4: provider error part is nil"))
	}
	var apiErr *provider.APICallError
	if errors.As(err, &apiErr) {
		if apiErr == nil {
			return internalFailure(errors.Join(errors.New("providerwirev4: provider returned a nil API call error"), err))
		}
		if apiErr.StatusCode == http.StatusTooManyRequests {
			return safeFailure{kind: failureRateLimited, retryable: true, cause: err}
		}
		retryable := apiErr.StatusCode == http.StatusRequestTimeout || apiErr.StatusCode >= http.StatusInternalServerError || (apiErr.StatusCode == 0 && apiErr.IsRetryable)
		return safeFailure{kind: failureDependency, retryable: retryable, cause: err}
	}
	return safeFailure{kind: failureDependency, cause: err}
}

func classifyContextError(ctx context.Context, err error) (safeFailure, bool) {
	cause := errors.Join(err, context.Cause(ctx))
	switch {
	case errors.Is(context.Cause(ctx), ErrTotalTimeout), errors.Is(ctx.Err(), context.DeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
		return safeFailure{kind: failureTimeout, retryable: true, cause: cause}, true
	case errors.Is(ctx.Err(), context.Canceled), errors.Is(err, context.Canceled):
		return safeFailure{kind: failureCanceled, cause: cause}, true
	default:
		return safeFailure{}, false
	}
}

func internalFailure(err error) safeFailure {
	return safeFailure{kind: failureInternal, cause: err}
}

func timeoutFailure(err error) safeFailure {
	return safeFailure{kind: failureTimeout, retryable: true, cause: err}
}

func projectFailure(failure safeFailure) gatewayErrorDTO {
	dto := gatewayErrorDTO{IsRetryable: failure.retryable, Param: json.RawMessage("null")}
	switch failure.kind {
	case failureUnknownModel:
		dto.Message, dto.Type, dto.StatusCode = "model not found", "model_not_found", http.StatusNotFound
		if failure.requestedModelID != "" {
			dto.Param, _ = json.Marshal(struct {
				ModelID string `json:"modelId"`
			}{ModelID: failure.requestedModelID})
		}
	case failureRateLimited:
		dto.Message, dto.Type, dto.StatusCode = "rate limit exceeded", "rate_limit_exceeded", http.StatusTooManyRequests
	case failureTimeout:
		dto.Message, dto.Type, dto.StatusCode = "request timed out", "internal_server_error", http.StatusGatewayTimeout
	case failureCanceled:
		dto.Message, dto.Type, dto.StatusCode = "request canceled", "internal_server_error", 499
	case failureDependency:
		dto.Message, dto.Type = "upstream dependency failed", "failed_dependency"
		if failure.retryable {
			dto.StatusCode = http.StatusBadGateway
		} else {
			dto.StatusCode = http.StatusFailedDependency
		}
	default:
		dto.Message, dto.Type, dto.StatusCode = "internal server error", "internal_server_error", http.StatusInternalServerError
	}
	return dto
}

func encodeFailure(failure safeFailure) (int, []byte, error) {
	dto := projectFailure(failure)
	data, err := json.Marshal(gatewayErrorEnvelopeDTO{Error: dto})
	if err != nil {
		return 0, nil, fmt.Errorf("providerwirev4: encoding failure: %w", err)
	}
	return dto.StatusCode, data, nil
}

func encodeProtocolError(status int, message string) ([]byte, error) {
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

func sanitizePartError(apiErr *provider.APICallError) *provider.APICallError {
	if apiErr == nil {
		return apiCallErrorForFailure(internalFailure(errors.New("providerwirev4: provider error part is nil")))
	}
	return apiCallErrorForFailure(classifyProviderError(apiErr))
}

func apiCallErrorForFailure(failure safeFailure) *provider.APICallError {
	dto := projectFailure(failure)
	data, _ := json.Marshal(dto)
	retryable := dto.IsRetryable
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message: dto.Message, StatusCode: dto.StatusCode, IsRetryable: &retryable, Data: data, Cause: failure.cause,
	})
}
